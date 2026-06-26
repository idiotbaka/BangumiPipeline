package bangumi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const stageStatusCompleted = "completed"

func isProcessed(ctx context.Context, db *sql.DB, bangumiID int64) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM anime_metadata WHERE bangumi_id = ?)", bangumiID,
	).Scan(&exists)
	return exists, err
}

func activeSubjectExists(ctx context.Context, db *sql.DB, bangumiID int64) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM anime_metadata WHERE bangumi_id = ? AND deleted_at IS NULL)", bangumiID,
	).Scan(&exists)
	return exists, err
}

func insertBaseMetadata(ctx context.Context, db *sql.DB, item calendarItem, cover imageDownload, downloadErr error, now time.Time) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(
    bangumi_id, url, name, name_cn, air_date, air_weekday,
    image_large_url, image_local_path, image_status, image_error, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.URL, item.Name, item.NameCN, item.AirDate, item.AirWeekday,
		item.Images.Large, cover.Path, cover.Status, truncateError(downloadErr), now.UTC().Unix(),
	)
	return err
}

func upsertBaseMetadataFromDetail(ctx context.Context, db *sql.DB, bangumiID int64, detail subjectDetail, cover imageDownload, downloadErr error, now time.Time) error {
	timestamp := now.UTC().Unix()
	_, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(
    bangumi_id, url, name, name_cn, air_date, air_weekday,
    image_large_url, image_local_path, image_status, image_error, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(bangumi_id) DO UPDATE SET
    url = excluded.url,
    name = CASE WHEN excluded.name != '' THEN excluded.name ELSE anime_metadata.name END,
    name_cn = CASE WHEN excluded.name_cn != '' THEN excluded.name_cn ELSE anime_metadata.name_cn END,
    air_date = CASE WHEN excluded.air_date != '' THEN excluded.air_date ELSE anime_metadata.air_date END,
    air_weekday = CASE WHEN excluded.air_weekday != 0 THEN excluded.air_weekday ELSE anime_metadata.air_weekday END,
    image_large_url = CASE WHEN excluded.image_large_url != '' THEN excluded.image_large_url ELSE anime_metadata.image_large_url END,
    image_local_path = excluded.image_local_path,
    image_status = excluded.image_status,
    image_error = excluded.image_error,
    detail_status = 'pending',
    detail_error = '',
    characters_status = 'pending',
    characters_error = '',
    episodes_status = 'pending',
    episodes_error = '',
    deleted_at = NULL,
    created_at = CASE WHEN anime_metadata.deleted_at IS NULL THEN anime_metadata.created_at ELSE excluded.created_at END`,
		bangumiID, fmt.Sprintf("https://bgm.tv/subject/%d", bangumiID), detail.Name, detail.NameCN,
		detail.Date, subjectAirWeekday(detail.Date), detail.Images.Large, cover.Path, cover.Status,
		truncateError(downloadErr), timestamp,
	)
	return err
}

func subjectAirWeekday(date string) int {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0
	}
	weekday := int(parsed.Weekday())
	if weekday == 0 {
		return 7
	}
	return weekday
}

func listRetryableAnimeImages(ctx context.Context, db *sql.DB) ([]pendingAnimeImage, error) {
	rows, err := db.QueryContext(ctx, `
SELECT bangumi_id, image_large_url
FROM anime_metadata
WHERE deleted_at IS NULL AND image_large_url != '' AND image_status IN ('pending', 'failed')
ORDER BY bangumi_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]pendingAnimeImage, 0)
	for rows.Next() {
		var image pendingAnimeImage
		if err := rows.Scan(&image.BangumiID, &image.SourceURL); err != nil {
			return nil, err
		}
		result = append(result, image)
	}
	return result, rows.Err()
}

func updateAnimeImage(ctx context.Context, db *sql.DB, bangumiID int64, download imageDownload, downloadErr error) error {
	errorMessage := truncateError(downloadErr)
	_, err := db.ExecContext(ctx, `
UPDATE anime_metadata
SET image_local_path = ?, image_status = ?, image_error = ?
WHERE bangumi_id = ?`, download.Path, download.Status, errorMessage, bangumiID)
	return err
}

func listIncompleteSubjects(ctx context.Context, db *sql.DB) ([]incompleteSubject, error) {
	rows, err := db.QueryContext(ctx, `
SELECT bangumi_id, detail_status, characters_status, episodes_status,
       (
           (CASE WHEN total_episodes > 0 THEN total_episodes ELSE eps END) > 0
           AND (
               SELECT COUNT(DISTINCT ep_number)
               FROM anime_episodes
               WHERE anime_episodes.bangumi_id = anime_metadata.bangumi_id
                 AND anime_episodes.type = 0
                 AND anime_episodes.ep_number > 0
           ) < (CASE WHEN total_episodes > 0 THEN total_episodes ELSE eps END)
       ) AS episodes_missing
FROM anime_metadata
WHERE deleted_at IS NULL AND (
    detail_status != ?
    OR characters_status != ?
    OR episodes_status != ?
    OR (
        (CASE WHEN total_episodes > 0 THEN total_episodes ELSE eps END) > 0
        AND (
            SELECT COUNT(DISTINCT ep_number)
            FROM anime_episodes
            WHERE anime_episodes.bangumi_id = anime_metadata.bangumi_id
              AND anime_episodes.type = 0
              AND anime_episodes.ep_number > 0
        ) < (CASE WHEN total_episodes > 0 THEN total_episodes ELSE eps END)
    )
)
ORDER BY bangumi_id`, stageStatusCompleted, stageStatusCompleted, stageStatusCompleted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]incompleteSubject, 0)
	for rows.Next() {
		var subject incompleteSubject
		if err := rows.Scan(&subject.BangumiID, &subject.DetailStatus, &subject.CharacterStatus, &subject.EpisodesStatus, &subject.EpisodesMissing); err != nil {
			return nil, err
		}
		result = append(result, subject)
	}
	return result, rows.Err()
}

func saveSubjectDetail(ctx context.Context, db *sql.DB, bangumiID int64, detail subjectDetail, now time.Time) error {
	infoboxJSON, err := json.Marshal(detail.Infobox)
	if err != nil {
		return err
	}
	metaTagsJSON, err := json.Marshal(detail.MetaTags)
	if err != nil {
		return err
	}
	ratingJSON := normalizedRawJSON(detail.Rating, "{}")
	collectionJSON := normalizedRawJSON(detail.Collection, "{}")
	aliases := extractAliases(detail.Infobox)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fetchedAt := now.UTC().Unix()
	result, err := tx.ExecContext(ctx, `
UPDATE anime_metadata
SET detail_date = ?,
    air_date = CASE WHEN ? != '' THEN ? ELSE air_date END,
    platform = ?, summary = ?,
    name = CASE WHEN ? != '' THEN ? ELSE name END,
    name_cn = CASE WHEN ? != '' THEN ? ELSE name_cn END,
    eps = ?, total_episodes = ?, volumes = ?, series = ?, locked = ?, nsfw = ?,
    infobox_json = ?, rating_json = ?, collection_json = ?, meta_tags_json = ?,
    image_large_url = CASE WHEN ? != '' THEN ? ELSE image_large_url END,
    detail_status = 'completed', detail_error = '', detail_fetched_at = ?
WHERE bangumi_id = ?`,
		detail.Date, detail.Date, detail.Date, detail.Platform, detail.Summary,
		detail.Name, detail.Name, detail.NameCN, detail.NameCN,
		detail.Eps, detail.TotalEpisodes, detail.Volumes, detail.Series, detail.Locked, detail.NSFW,
		string(infoboxJSON), ratingJSON, collectionJSON, string(metaTagsJSON),
		detail.Images.Large, detail.Images.Large, fetchedAt, bangumiID,
	)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		if err != nil {
			return err
		}
		return errors.New("anime metadata record not found")
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM anime_tags WHERE bangumi_id = ?", bangumiID); err != nil {
		return err
	}
	for _, tag := range detail.Tags {
		if strings.TrimSpace(tag.Name) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO anime_tags(bangumi_id, name, count, total_count)
VALUES (?, ?, ?, ?)`, bangumiID, tag.Name, tag.Count, tag.TotalCount); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM anime_aliases WHERE bangumi_id = ?", bangumiID); err != nil {
		return err
	}
	for index, alias := range aliases {
		if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO anime_aliases(bangumi_id, alias, sort_order)
VALUES (?, ?, ?)`, bangumiID, alias, index); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func saveCharacters(ctx context.Context, db *sql.DB, bangumiID int64, characters []storedCharacter, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM anime_characters WHERE bangumi_id = ?", bangumiID); err != nil {
		return err
	}
	timestamp := now.UTC().Unix()
	for _, character := range characters {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO anime_characters(
    bangumi_id, character_id, name, summary, relation, type,
    image_large_url, image_local_path, image_status, image_error,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			bangumiID, character.CharacterID, character.Name, character.Summary, character.Relation,
			character.Type, character.ImageLargeURL, character.ImagePath, character.ImageStatus,
			character.ImageError, timestamp, timestamp,
		); err != nil {
			return err
		}
		for index, actorID := range character.ActorIDs {
			if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO character_actors(bangumi_id, character_id, actor_id, sort_order)
VALUES (?, ?, ?, ?)`, bangumiID, character.CharacterID, actorID, index); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE anime_metadata
SET characters_status = 'completed', characters_error = '', characters_fetched_at = ?
WHERE bangumi_id = ?`, timestamp, bangumiID); err != nil {
		return err
	}
	return tx.Commit()
}

func saveEpisodes(ctx context.Context, db *sql.DB, bangumiID int64, episodes []episodeDetail, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM anime_episodes WHERE bangumi_id = ?", bangumiID); err != nil {
		return err
	}
	timestamp := now.UTC().Unix()
	for _, episode := range episodes {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO anime_episodes(
    bangumi_id, episode_id, ep_number, sort_number, type, disc,
    airdate, name, name_cn, duration, duration_seconds, description,
    comment_count, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			bangumiID, episode.ID, episode.Ep, episode.Sort, episode.Type, episode.Disc,
			episode.Airdate, episode.Name, episode.NameCN, episode.Duration, episode.DurationSeconds,
			episode.Description, episode.Comment, timestamp, timestamp,
		); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `
UPDATE anime_metadata
SET episodes_status = 'completed', episodes_error = '', episodes_fetched_at = ?
WHERE bangumi_id = ?`, timestamp, bangumiID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		if err != nil {
			return err
		}
		return errors.New("anime metadata record not found")
	}
	return tx.Commit()
}

func getActorImageState(ctx context.Context, db *sql.DB, actorID int64) (actorImageState, error) {
	var state actorImageState
	err := db.QueryRowContext(ctx, `
SELECT image_large_url, image_local_path, image_status
FROM actors WHERE actor_id = ?`, actorID).Scan(
		&state.ImageLargeURL, &state.ImagePath, &state.ImageStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return actorImageState{}, nil
	}
	if err != nil {
		return actorImageState{}, err
	}
	state.Exists = true
	return state, nil
}

func upsertActor(ctx context.Context, db *sql.DB, actor storedActor, now time.Time) error {
	timestamp := now.UTC().Unix()
	_, err := db.ExecContext(ctx, `
INSERT INTO actors(
    actor_id, name, short_summary, career_json, type, locked,
    image_large_url, image_local_path, image_status, image_error, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(actor_id) DO UPDATE SET
    name = excluded.name,
    short_summary = excluded.short_summary,
    career_json = excluded.career_json,
    type = excluded.type,
    locked = excluded.locked,
    image_large_url = excluded.image_large_url,
    image_local_path = excluded.image_local_path,
    image_status = excluded.image_status,
    image_error = excluded.image_error,
    updated_at = excluded.updated_at`,
		actor.ActorID, actor.Name, actor.ShortSummary, actor.CareerJSON, actor.Type, actor.Locked,
		actor.ImageLargeURL, actor.ImagePath, actor.ImageStatus, actor.ImageError, timestamp, timestamp,
	)
	return err
}

func markDetailFailed(ctx context.Context, db *sql.DB, bangumiID int64, runErr error) error {
	_, err := db.ExecContext(ctx, `
UPDATE anime_metadata SET detail_status = 'failed', detail_error = ? WHERE bangumi_id = ?`,
		truncateError(runErr), bangumiID)
	return err
}

func markCharactersFailed(ctx context.Context, db *sql.DB, bangumiID int64, runErr error) error {
	_, err := db.ExecContext(ctx, `
UPDATE anime_metadata SET characters_status = 'failed', characters_error = ? WHERE bangumi_id = ?`,
		truncateError(runErr), bangumiID)
	return err
}

func markEpisodesFailed(ctx context.Context, db *sql.DB, bangumiID int64, runErr error) error {
	_, err := db.ExecContext(ctx, `
UPDATE anime_metadata SET episodes_status = 'failed', episodes_error = ? WHERE bangumi_id = ?`,
		truncateError(runErr), bangumiID)
	return err
}

func extractAliases(infobox []infoboxItem) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range infobox {
		if item.Key != "别名" {
			continue
		}
		var values []struct {
			Value string `json:"v"`
		}
		if err := json.Unmarshal(item.Value, &values); err == nil {
			for _, value := range values {
				alias := strings.TrimSpace(value.Value)
				if alias != "" {
					if _, exists := seen[alias]; !exists {
						seen[alias] = struct{}{}
						result = append(result, alias)
					}
				}
			}
			continue
		}
		var value string
		if err := json.Unmarshal(item.Value, &value); err == nil {
			value = strings.TrimSpace(value)
			if value != "" {
				if _, exists := seen[value]; !exists {
					seen[value] = struct{}{}
					result = append(result, value)
				}
			}
		}
	}
	return result
}

func normalizedRawJSON(value json.RawMessage, fallback string) string {
	if len(value) == 0 || string(value) == "null" {
		return fallback
	}
	return string(value)
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}

func combineStageError(stage string, runErr, persistErr error) error {
	if persistErr == nil {
		return fmt.Errorf("%s: %w", stage, runErr)
	}
	return fmt.Errorf("%s: %v；保存失败状态: %w", stage, runErr, persistErr)
}
