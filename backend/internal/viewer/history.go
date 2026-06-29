package viewer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var ErrWatchMediaNotFound = errors.New("watch media not found")

type WatchProgress struct {
	MediaID         int64   `json:"mediaId"`
	BangumiID       int64   `json:"bangumiId"`
	PositionSeconds float64 `json:"positionSeconds"`
	DurationSeconds float64 `json:"durationSeconds"`
	Completed       bool    `json:"completed"`
	UpdatedAt       int64   `json:"updatedAt"`
}

type WatchProgressInput struct {
	BangumiID       int64
	MediaID         int64
	PositionSeconds float64
	DurationSeconds float64
}

type WatchHistoryItem struct {
	BangumiID          int64   `json:"bangumiId"`
	MediaID            int64   `json:"mediaId"`
	AnimeTitle         string  `json:"animeTitle"`
	EpisodeLabel       string  `json:"episodeLabel"`
	EpisodeTitle       string  `json:"episodeTitle"`
	LatestEpisodeLabel string  `json:"latestEpisodeLabel"`
	TotalEpisodes      int     `json:"totalEpisodes"`
	PositionSeconds    float64 `json:"positionSeconds"`
	DurationSeconds    float64 `json:"durationSeconds"`
	ProgressPercent    int     `json:"progressPercent"`
	Completed          bool    `json:"completed"`
	HasCover           bool    `json:"hasCover"`
	LastWatchedAt      int64   `json:"lastWatchedAt"`
}

func (s *Service) RecordWatchProgress(ctx context.Context, userID int64, input WatchProgressInput) (*WatchProgress, error) {
	if userID < 1 || input.BangumiID < 1 || input.MediaID < 1 ||
		math.IsNaN(input.PositionSeconds) || math.IsInf(input.PositionSeconds, 0) ||
		math.IsNaN(input.DurationSeconds) || math.IsInf(input.DurationSeconds, 0) ||
		input.PositionSeconds < 0 || input.DurationSeconds <= 0 {
		return nil, ErrWatchMediaNotFound
	}
	if input.PositionSeconds <= 15 {
		return nil, nil
	}
	if input.PositionSeconds > input.DurationSeconds {
		input.PositionSeconds = input.DurationSeconds
	}
	completed := input.PositionSeconds/input.DurationSeconds >= 0.9
	var mediaExists bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM media_jobs
    WHERE id = ? AND bangumi_id = ? AND status = 'completed' AND output_path != ''
)`, input.MediaID, input.BangumiID).Scan(&mediaExists); err != nil {
		return nil, err
	}
	if !mediaExists {
		return nil, ErrWatchMediaNotFound
	}
	now := s.now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO viewer_watch_history(
    user_id, bangumi_id, media_job_id, position_seconds, duration_seconds,
    completed, last_watched_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, media_job_id) DO UPDATE SET
    bangumi_id = excluded.bangumi_id,
    position_seconds = excluded.position_seconds,
    duration_seconds = excluded.duration_seconds,
    completed = excluded.completed,
    last_watched_at = excluded.last_watched_at,
    updated_at = excluded.updated_at`,
		userID, input.BangumiID, input.MediaID, input.PositionSeconds, input.DurationSeconds,
		completed, now, now, now,
	); err != nil {
		return nil, err
	}
	return &WatchProgress{
		MediaID: input.MediaID, BangumiID: input.BangumiID,
		PositionSeconds: input.PositionSeconds, DurationSeconds: input.DurationSeconds,
		Completed: completed, UpdatedAt: now,
	}, nil
}

func (s *Service) LastWatchProgress(ctx context.Context, userID, bangumiID int64) (*WatchProgress, error) {
	var progress WatchProgress
	err := s.db.QueryRowContext(ctx, `
SELECT history.media_job_id, history.bangumi_id,
       history.position_seconds, history.duration_seconds,
       history.completed, history.updated_at
FROM viewer_watch_history history
JOIN media_jobs media ON media.id = history.media_job_id
WHERE history.user_id = ? AND history.bangumi_id = ?
  AND media.status = 'completed' AND media.output_path != ''
ORDER BY history.last_watched_at DESC, history.id DESC
LIMIT 1`, userID, bangumiID).Scan(
		&progress.MediaID, &progress.BangumiID,
		&progress.PositionSeconds, &progress.DurationSeconds,
		&progress.Completed, &progress.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &progress, err
}

func (s *Service) WatchHistory(ctx context.Context, userID int64, limit int) ([]WatchHistoryItem, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT history.bangumi_id,
       history.media_job_id,
       COALESCE(NULLIF(anime.name_cn, ''), anime.name),
       media.season_number,
       COALESCE(NULLIF(media.episode_type, ''), 'episode'),
       media.episode_number,
       COALESCE((
           SELECT COALESCE(NULLIF(episode.name_cn, ''), episode.name)
           FROM anime_episodes episode
           WHERE episode.bangumi_id = media.bangumi_id
             AND episode.sort_number = CAST(media.episode_number AS REAL)
             AND (
                 (LOWER(COALESCE(NULLIF(media.episode_type, ''), 'episode')) = 'episode' AND episode.type = 0)
                 OR
                 (LOWER(COALESCE(NULLIF(media.episode_type, ''), 'episode')) != 'episode' AND episode.type != 0)
             )
           ORDER BY episode.type, episode.episode_id
           LIMIT 1
       ), ''),
       CASE WHEN anime.total_episodes > 0 THEN anime.total_episodes ELSE anime.eps END,
       history.position_seconds,
       history.duration_seconds,
       history.completed,
       media.cover_status = 'completed' AND media.cover_path != '',
       history.last_watched_at
FROM viewer_watch_history history
JOIN media_jobs media ON media.id = history.media_job_id
JOIN anime_metadata anime ON anime.bangumi_id = history.bangumi_id
WHERE history.user_id = ?
  AND anime.deleted_at IS NULL
  AND media.status = 'completed'
  AND media.output_path != ''
ORDER BY history.last_watched_at DESC, history.id DESC
LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	items := make([]WatchHistoryItem, 0)
	for rows.Next() {
		var item WatchHistoryItem
		var season int
		var episodeType, episodeNumber string
		if err := rows.Scan(
			&item.BangumiID, &item.MediaID, &item.AnimeTitle,
			&season, &episodeType, &episodeNumber, &item.EpisodeTitle,
			&item.TotalEpisodes, &item.PositionSeconds, &item.DurationSeconds,
			&item.Completed, &item.HasCover, &item.LastWatchedAt,
		); err != nil {
			rows.Close()
			return nil, err
		}
		item.EpisodeLabel = watchEpisodeLabel(season, episodeType, episodeNumber)
		if item.Completed {
			item.ProgressPercent = 100
		} else if item.DurationSeconds > 0 {
			item.ProgressPercent = int(math.Round(item.PositionSeconds / item.DurationSeconds * 100))
			if item.ProgressPercent < 0 {
				item.ProgressPercent = 0
			}
			if item.ProgressPercent > 100 {
				item.ProgressPercent = 100
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := s.attachHistoryLatestEpisodes(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

type historyEpisodeRef struct {
	season        int
	episodeType   string
	episodeNumber string
	updatedAt     int64
}

func (s *Service) attachHistoryLatestEpisodes(ctx context.Context, items []WatchHistoryItem) error {
	if len(items) == 0 {
		return nil
	}
	seenAnime := make(map[int64]struct{}, len(items))
	args := make([]any, 0, len(items))
	placeholders := make([]string, 0, len(items))
	for _, item := range items {
		if _, exists := seenAnime[item.BangumiID]; exists {
			continue
		}
		seenAnime[item.BangumiID] = struct{}{}
		args = append(args, item.BangumiID)
		placeholders = append(placeholders, "?")
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT bangumi_id, season_number,
       COALESCE(NULLIF(episode_type, ''), 'episode'),
       episode_number,
       COALESCE(completed_at, updated_at, created_at, 0)
FROM media_jobs
WHERE status = 'completed' AND output_path != ''
  AND bangumi_id IN (%s)
ORDER BY bangumi_id, id`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	latest := make(map[int64]historyEpisodeRef, len(seenAnime))
	for rows.Next() {
		var bangumiID int64
		var episode historyEpisodeRef
		if err := rows.Scan(
			&bangumiID, &episode.season, &episode.episodeType,
			&episode.episodeNumber, &episode.updatedAt,
		); err != nil {
			return err
		}
		current, exists := latest[bangumiID]
		if !exists || historyEpisodeLess(current, episode) {
			latest[bangumiID] = episode
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for index := range items {
		if episode, exists := latest[items[index].BangumiID]; exists {
			items[index].LatestEpisodeLabel = watchEpisodeLabel(
				episode.season, episode.episodeType, episode.episodeNumber,
			)
		}
	}
	return nil
}

func historyEpisodeLess(left, right historyEpisodeRef) bool {
	if left.season != right.season {
		return left.season < right.season
	}
	if leftRank, rightRank := historyEpisodeTypeRank(left.episodeType), historyEpisodeTypeRank(right.episodeType); leftRank != rightRank {
		return leftRank < rightRank
	}
	leftNumber, leftErr := strconv.ParseFloat(strings.TrimSpace(left.episodeNumber), 64)
	rightNumber, rightErr := strconv.ParseFloat(strings.TrimSpace(right.episodeNumber), 64)
	if leftErr == nil && rightErr == nil && leftNumber != rightNumber {
		return leftNumber < rightNumber
	}
	if (leftErr == nil) != (rightErr == nil) {
		return leftErr != nil
	}
	if left.episodeNumber != right.episodeNumber {
		return left.episodeNumber < right.episodeNumber
	}
	return left.updatedAt < right.updatedAt
}

func historyEpisodeTypeRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "episode":
		return 3
	case "ova", "oad", "sp":
		return 2
	default:
		return 1
	}
}

func watchEpisodeLabel(season int, episodeType, number string) string {
	number = strings.TrimSpace(number)
	if number == "" {
		number = "?"
	}
	typeName := strings.ToLower(strings.TrimSpace(episodeType))
	var label string
	if typeName == "" || typeName == "episode" {
		label = fmt.Sprintf("第 %s 话", number)
	} else {
		label = fmt.Sprintf("%s %s", strings.ToUpper(typeName), number)
	}
	if season > 1 {
		return fmt.Sprintf("S%02d %s", season, label)
	}
	return label
}
