package viewer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

var ErrFollowAnimeNotFound = errors.New("follow anime not found")

type FollowedAnime struct {
	BangumiID           int64   `json:"bangumiId"`
	AnimeTitle          string  `json:"animeTitle"`
	TotalEpisodes       int     `json:"totalEpisodes"`
	MediaID             int64   `json:"mediaId"`
	EpisodeLabel        string  `json:"episodeLabel"`
	EpisodeTitle        string  `json:"episodeTitle"`
	HasCover            bool    `json:"hasCover"`
	HasWatchProgress    bool    `json:"hasWatchProgress"`
	WatchedEpisodeLabel string  `json:"watchedEpisodeLabel"`
	PositionSeconds     float64 `json:"positionSeconds"`
	DurationSeconds     float64 `json:"durationSeconds"`
	ProgressPercent     int     `json:"progressPercent"`
	WatchCompleted      bool    `json:"watchCompleted"`
	LatestEpisodeLabel  string  `json:"latestEpisodeLabel"`
	CaughtUp            bool    `json:"caughtUp"`
	LastWatchedAt       int64   `json:"lastWatchedAt"`
	FollowedAt          int64   `json:"followedAt"`
}

type followedAnimeBase struct {
	BangumiID     int64
	AnimeTitle    string
	TotalEpisodes int
	FollowedAt    int64
}

type followedMedia struct {
	id       int64
	ref      historyEpisodeRef
	title    string
	hasCover bool
}

type followedProgress struct {
	mediaID         int64
	ref             historyEpisodeRef
	positionSeconds float64
	durationSeconds float64
	completed       bool
	lastWatchedAt   int64
}

func (s *Service) IsAnimeFollowed(ctx context.Context, userID, bangumiID int64) (bool, error) {
	var followed bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM viewer_anime_follows
    WHERE user_id = ? AND bangumi_id = ?
)`, userID, bangumiID).Scan(&followed)
	return followed, err
}

func (s *Service) SetAnimeFollow(ctx context.Context, userID, bangumiID int64, followed bool) (bool, error) {
	if userID < 1 || bangumiID < 1 {
		return false, ErrFollowAnimeNotFound
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var animeExists bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM anime_metadata
    WHERE bangumi_id = ? AND deleted_at IS NULL
)`, bangumiID).Scan(&animeExists); err != nil {
		return false, err
	}
	if !animeExists {
		return false, ErrFollowAnimeNotFound
	}
	if followed {
		now := s.now().UTC().Unix()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO viewer_anime_follows(user_id, bangumi_id, created_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(user_id, bangumi_id) DO UPDATE SET updated_at = excluded.updated_at`,
			userID, bangumiID, now, now,
		); err != nil {
			return false, err
		}
	} else if _, err := tx.ExecContext(ctx, `
DELETE FROM viewer_anime_follows
WHERE user_id = ? AND bangumi_id = ?`, userID, bangumiID); err != nil {
		return false, err
	} else if _, err := tx.ExecContext(ctx, `
DELETE FROM viewer_web_push_deliveries
WHERE status = ?
  AND subscription_id IN (
      SELECT id FROM viewer_web_push_subscriptions WHERE user_id = ?
  )
  AND media_job_id IN (
      SELECT id FROM media_jobs WHERE bangumi_id = ?
  )`, pushDeliveryPending, userID, bangumiID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return followed, nil
}

func (s *Service) FollowedAnime(ctx context.Context, userID int64) ([]FollowedAnime, error) {
	bases, err := s.followedAnimeBases(ctx, userID)
	if err != nil || len(bases) == 0 {
		return []FollowedAnime{}, err
	}
	ids := make([]int64, 0, len(bases))
	for _, base := range bases {
		ids = append(ids, base.BangumiID)
	}
	media, err := s.followedAnimeMedia(ctx, ids)
	if err != nil {
		return nil, err
	}
	progress, err := s.followedAnimeProgress(ctx, userID, ids)
	if err != nil {
		return nil, err
	}

	items := make([]FollowedAnime, 0, len(bases))
	for _, base := range bases {
		item := FollowedAnime{
			BangumiID: base.BangumiID, AnimeTitle: base.AnimeTitle,
			TotalEpisodes: base.TotalEpisodes, FollowedAt: base.FollowedAt,
		}
		mediaItems := media[base.BangumiID]
		var earliest, latest followedMedia
		if len(mediaItems) > 0 {
			earliest = mediaItems[0]
			latest = mediaItems[0]
			for _, candidate := range mediaItems[1:] {
				if historyEpisodeLess(candidate.ref, earliest.ref) {
					earliest = candidate
				}
				if historyEpisodeLess(latest.ref, candidate.ref) {
					latest = candidate
				}
			}
			item.LatestEpisodeLabel = watchEpisodeLabel(
				latest.ref.season, latest.ref.episodeType, latest.ref.episodeNumber,
			)
		}
		watch, hasWatch := progress[base.BangumiID]
		item.HasWatchProgress = hasWatch
		if hasWatch {
			item.WatchedEpisodeLabel = watchEpisodeLabel(
				watch.ref.season, watch.ref.episodeType, watch.ref.episodeNumber,
			)
			item.PositionSeconds = watch.positionSeconds
			item.DurationSeconds = watch.durationSeconds
			item.WatchCompleted = watch.completed
			item.LastWatchedAt = watch.lastWatchedAt
			if watch.completed {
				item.ProgressPercent = 100
			} else if watch.durationSeconds > 0 {
				item.ProgressPercent = int(math.Round(watch.positionSeconds / watch.durationSeconds * 100))
				item.ProgressPercent = max(0, min(item.ProgressPercent, 100))
			}
			item.CaughtUp = len(mediaItems) > 0 && watch.completed && sameFollowEpisode(watch.ref, latest.ref)
		}

		var target followedMedia
		switch {
		case len(mediaItems) == 0:
		case !hasWatch:
			target = earliest
		case !watch.completed:
			for _, candidate := range mediaItems {
				if candidate.id == watch.mediaID {
					target = candidate
					break
				}
			}
			item.PositionSeconds = watch.positionSeconds
		case watch.mediaID != latest.id:
			target = latest
			item.PositionSeconds = 0
		default:
			target = latest
			item.PositionSeconds = 0
		}
		if target.id > 0 {
			item.MediaID = target.id
			item.EpisodeLabel = watchEpisodeLabel(
				target.ref.season, target.ref.episodeType, target.ref.episodeNumber,
			)
			item.EpisodeTitle = target.title
			item.HasCover = target.hasCover
		}
		items = append(items, item)
	}
	sortFollowedAnime(items)
	return items, nil
}

func sortFollowedAnime(items []FollowedAnime) {
	sort.SliceStable(items, func(i, j int) bool {
		leftFinished := items[i].WatchCompleted && items[i].CaughtUp
		rightFinished := items[j].WatchCompleted && items[j].CaughtUp
		if leftFinished != rightFinished {
			return !leftFinished
		}
		if items[i].LastWatchedAt != items[j].LastWatchedAt {
			return items[i].LastWatchedAt > items[j].LastWatchedAt
		}
		return items[i].FollowedAt > items[j].FollowedAt
	})
}

func (s *Service) followedAnimeBases(ctx context.Context, userID int64) ([]followedAnimeBase, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT follow.bangumi_id,
       COALESCE(NULLIF(anime.name_cn, ''), anime.name),
       CASE WHEN anime.total_episodes > 0 THEN anime.total_episodes ELSE anime.eps END,
       follow.created_at
FROM viewer_anime_follows follow
JOIN anime_metadata anime ON anime.bangumi_id = follow.bangumi_id
WHERE follow.user_id = ? AND anime.deleted_at IS NULL
ORDER BY follow.updated_at DESC, follow.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]followedAnimeBase, 0)
	for rows.Next() {
		var item followedAnimeBase
		if err := rows.Scan(&item.BangumiID, &item.AnimeTitle, &item.TotalEpisodes, &item.FollowedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) followedAnimeMedia(ctx context.Context, bangumiIDs []int64) (map[int64][]followedMedia, error) {
	placeholders, args := followPlaceholders(bangumiIDs)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT media.bangumi_id, media.id, media.season_number,
       COALESCE(NULLIF(media.episode_type, ''), 'episode'), media.episode_number,
       COALESCE((
           SELECT COALESCE(NULLIF(episode.name_cn, ''), episode.name)
           FROM anime_episodes episode
           WHERE episode.bangumi_id = media.bangumi_id
             AND episode.sort_number = CAST(media.episode_number AS REAL)
             AND ((LOWER(COALESCE(NULLIF(media.episode_type, ''), 'episode')) = 'episode' AND episode.type = 0)
               OR (LOWER(COALESCE(NULLIF(media.episode_type, ''), 'episode')) != 'episode' AND episode.type != 0))
           ORDER BY episode.type, episode.episode_id LIMIT 1
       ), ''),
       media.cover_status = 'completed' AND media.cover_path != '',
       COALESCE(media.completed_at, media.updated_at, media.created_at, 0)
FROM media_jobs media
WHERE media.status = 'completed' AND media.output_path != ''
  AND media.bangumi_id IN (%s)
ORDER BY media.bangumi_id, media.id`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64][]followedMedia, len(bangumiIDs))
	for rows.Next() {
		var bangumiID int64
		var item followedMedia
		if err := rows.Scan(
			&bangumiID, &item.id, &item.ref.season, &item.ref.episodeType,
			&item.ref.episodeNumber, &item.title, &item.hasCover, &item.ref.updatedAt,
		); err != nil {
			return nil, err
		}
		result[bangumiID] = append(result[bangumiID], item)
	}
	return result, rows.Err()
}

func (s *Service) followedAnimeProgress(ctx context.Context, userID int64, bangumiIDs []int64) (map[int64]followedProgress, error) {
	placeholders, args := followPlaceholders(bangumiIDs)
	args = append([]any{userID}, args...)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT history.bangumi_id, history.media_job_id,
       media.season_number, COALESCE(NULLIF(media.episode_type, ''), 'episode'), media.episode_number,
       history.position_seconds, history.duration_seconds, history.completed, history.last_watched_at
FROM viewer_watch_history history
JOIN media_jobs media ON media.id = history.media_job_id
WHERE history.user_id = ? AND history.bangumi_id IN (%s)
  AND media.status = 'completed' AND media.output_path != ''
ORDER BY history.bangumi_id, history.last_watched_at DESC, history.id DESC`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]followedProgress, len(bangumiIDs))
	for rows.Next() {
		var bangumiID int64
		var item followedProgress
		if err := rows.Scan(
			&bangumiID, &item.mediaID, &item.ref.season, &item.ref.episodeType,
			&item.ref.episodeNumber, &item.positionSeconds, &item.durationSeconds,
			&item.completed, &item.lastWatchedAt,
		); err != nil {
			return nil, err
		}
		if _, exists := result[bangumiID]; !exists {
			result[bangumiID] = item
		}
	}
	return result, rows.Err()
}

func followPlaceholders(ids []int64) (string, []any) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for index, id := range ids {
		placeholders[index] = "?"
		args[index] = id
	}
	return strings.Join(placeholders, ","), args
}

func sameFollowEpisode(left, right historyEpisodeRef) bool {
	return left.season == right.season &&
		historyEpisodeTypeRank(left.episodeType) == historyEpisodeTypeRank(right.episodeType) &&
		strings.TrimSpace(left.episodeNumber) == strings.TrimSpace(right.episodeNumber)
}
