package opskip

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"bangumipipeline.local/server/internal/bangumi"
)

var ErrInvalidProgressStatus = errors.New("invalid opening progress status")

type AnimeProgress struct {
	BangumiID       int64  `json:"bangumiId"`
	Name            string `json:"name"`
	NameCN          string `json:"nameCN"`
	Status          string `json:"status"`
	Total           int    `json:"total"`
	Detected        int    `json:"detected"`
	NotFound        int    `json:"notFound"`
	Pending         int    `json:"pending"`
	WaitingEpisodes int    `json:"waitingEpisodes"`
	Failed          int    `json:"failed"`
	LastAnalyzedAt  *int64 `json:"lastAnalyzedAt"`
}

type ProgressPage struct {
	Items    []AnimeProgress `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

type EpisodeProgress struct {
	MediaID           int64   `json:"mediaId"`
	SeasonNumber      int     `json:"seasonNumber"`
	EpisodeNumber     string  `json:"episodeNumber"`
	Status            string  `json:"status"`
	GroupSize         int     `json:"groupSize"`
	MinimumGroupSize  int     `json:"minimumGroupSize"`
	AnalyzedGroupSize int     `json:"analyzedGroupSize"`
	StartSeconds      float64 `json:"startSeconds"`
	EndSeconds        float64 `json:"endSeconds"`
	ErrorMessage      string  `json:"errorMessage"`
	UpdatedAt         *int64  `json:"updatedAt"`
}

// Keep the candidate predicates aligned with completedEpisodeGroups: progress
// describes completed normal-episode products, not scheduled or future episodes.
func openingProgressSQL(where string) string {
	return fmt.Sprintf(`WITH episodes AS (
    SELECT am.id AS anime_id, am.bangumi_id, am.name, am.name_cn, am.created_at,
           mj.id AS media_id, mj.season_number, mj.episode_number,
           COUNT(*) OVER (PARTITION BY mj.bangumi_id, mj.season_number) AS group_size,
           COALESCE(mos.status, 'pending') AS result_status,
           COALESCE(mos.start_seconds, 0) AS start_seconds,
           COALESCE(mos.end_seconds, 0) AS end_seconds,
           COALESCE(mos.analyzed_group_size, 0) AS analyzed_group_size,
           REPLACE(COALESCE(mos.error_message, ''), mj.output_path, '[成品视频]') AS error_message,
           mos.updated_at
    FROM anime_metadata am JOIN media_jobs mj ON mj.bangumi_id = am.bangumi_id
    LEFT JOIN media_op_segments mos ON mos.media_job_id = mj.id
    WHERE am.deleted_at IS NULL AND mj.status = 'completed' AND mj.output_path != ''
      AND LOWER(COALESCE(NULLIF(mj.episode_type, ''), 'episode')) = 'episode'
      AND %s
), classified AS (
    SELECT *, CASE WHEN result_status = 'pending' AND group_size < %d
                   THEN 'waiting_episodes' ELSE result_status END AS progress_status
    FROM episodes
)
`, where, minimumGroupEpisodes)
}

const openingAggregatesSQL = `
, totals AS (
    SELECT anime_id, bangumi_id, name, name_cn, created_at, COUNT(*) AS total,
           SUM(progress_status = 'detected') AS detected,
           SUM(progress_status = 'not_found') AS not_found,
           SUM(progress_status = 'pending') AS pending,
           SUM(progress_status = 'waiting_episodes') AS waiting_episodes,
           SUM(progress_status = 'failed') AS failed,
           MAX(CASE WHEN result_status != 'pending' THEN updated_at END) AS last_analyzed_at
    FROM classified GROUP BY anime_id
), progress AS (
    SELECT *, CASE WHEN failed > 0 THEN 'failed'
                   WHEN pending > 0 THEN 'pending'
                   WHEN waiting_episodes > 0 THEN 'waiting_episodes'
                   ELSE 'completed' END AS status
    FROM totals
)
`

func (s *Service) ListProgress(ctx context.Context, page, pageSize int, query, status string) (ProgressPage, error) {
	result := ProgressPage{Items: make([]AnimeProgress, 0)}
	filter := "1 = 1"
	switch status {
	case "", "all":
	case "completed":
		filter = "status = 'completed'"
	case "pending":
		filter = "status != 'completed'"
	case "failed":
		filter = "failed > 0"
	case "waiting_episodes":
		filter = "waiting_episodes > 0"
	case "not_found":
		filter = "not_found > 0"
	default:
		return result, ErrInvalidProgressStatus
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	result.Page, result.PageSize = page, pageSize
	where := "1 = 1"
	var args []any
	if query = strings.TrimSpace(query); query != "" {
		where = ` (am.name LIKE ? OR am.name_cn LIKE ? OR EXISTS (
            SELECT 1 FROM anime_aliases aa WHERE aa.bangumi_id = am.bangumi_id AND aa.alias LIKE ?))`
		like := "%" + query + "%"
		args = []any{like, like, like}
	}
	cte := openingProgressSQL(where) + openingAggregatesSQL
	if err := s.db.QueryRowContext(ctx, cte+" SELECT COUNT(*) FROM progress WHERE "+filter, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, cte+`
SELECT bangumi_id, name, name_cn, status, total, detected, not_found, pending,
       waiting_episodes, failed, last_analyzed_at
FROM progress WHERE `+filter+` ORDER BY created_at DESC, anime_id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AnimeProgress
		if err := rows.Scan(&item.BangumiID, &item.Name, &item.NameCN, &item.Status, &item.Total,
			&item.Detected, &item.NotFound, &item.Pending, &item.WaitingEpisodes, &item.Failed, &item.LastAnalyzedAt); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func (s *Service) ProgressDetails(ctx context.Context, bangumiID int64) ([]EpisodeProgress, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM anime_metadata WHERE bangumi_id = ? AND deleted_at IS NULL)", bangumiID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, bangumi.ErrAnimeNotFound
	}
	rows, err := s.db.QueryContext(ctx, openingProgressSQL("am.bangumi_id = ?")+`
SELECT media_id, season_number, episode_number, progress_status, group_size,
       analyzed_group_size, start_seconds, end_seconds, error_message, updated_at
FROM classified ORDER BY season_number,
    CASE WHEN episode_number GLOB '[0-9]*' THEN 0 ELSE 1 END,
    CAST(episode_number AS REAL), episode_number, media_id`, bangumiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EpisodeProgress, 0)
	for rows.Next() {
		item := EpisodeProgress{MinimumGroupSize: minimumGroupEpisodes}
		if err := rows.Scan(&item.MediaID, &item.SeasonNumber, &item.EpisodeNumber, &item.Status, &item.GroupSize,
			&item.AnalyzedGroupSize, &item.StartSeconds, &item.EndSeconds, &item.ErrorMessage, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
