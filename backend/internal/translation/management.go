package translation

import (
	"context"
	"errors"
	"strings"

	"bangumipipeline.local/server/internal/bangumi"
)

var ErrInvalidProgressStatus = errors.New("invalid translation progress status")

type ProgressCount struct {
	Kind          string `json:"kind"`
	Total         int    `json:"total"`
	Completed     int    `json:"completed"`
	MissingSource int    `json:"missingSource"`
}

type MetadataStage struct {
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage"`
}

type AnimeProgress struct {
	BangumiID int64           `json:"bangumiId"`
	Name      string          `json:"name"`
	NameCN    string          `json:"nameCN"`
	Status    string          `json:"status"`
	Total     int             `json:"total"`
	Completed int             `json:"completed"`
	Counts    []ProgressCount `json:"counts"`
	Metadata  []MetadataStage `json:"metadata"`
}

type ProgressPage struct {
	Items    []AnimeProgress `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

type ProgressTarget struct {
	Kind   string `json:"kind"`
	ID     int64  `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

// candidates is deliberately shared by the aggregates and details. Counts use
// exactly the same nonempty-source / empty-Chinese predicates as nextTarget.
const progressTargetsSQL = `
, targets AS (
    SELECT bangumi_id, 'anime_title' AS kind, bangumi_id AS target_id,
           name AS label, name AS original, name_cn AS translated, 0 AS position
    FROM candidates
    UNION ALL
    SELECT bangumi_id, 'anime_summary', bangumi_id, name, summary, summary_cn, 0
    FROM candidates
    UNION ALL
    SELECT e.bangumi_id, 'episode_title', e.episode_id,
           printf('%s %g · %s', CASE WHEN e.type = 0 THEN '正片' ELSE '特殊分集' END,
                  e.sort_number, e.name), e.name, e.name_cn, e.sort_number
    FROM anime_episodes e JOIN candidates am ON am.bangumi_id = e.bangumi_id
    UNION ALL
    SELECT e.bangumi_id, 'episode_summary', e.episode_id,
           printf('%s %g · %s', CASE WHEN e.type = 0 THEN '正片' ELSE '特殊分集' END,
                  e.sort_number, e.name), e.description, e.description_cn, e.sort_number
    FROM anime_episodes e JOIN candidates am ON am.bangumi_id = e.bangumi_id
    UNION ALL
    SELECT c.bangumi_id, 'character_summary', c.character_id, c.name, c.summary, c.summary_cn, c.id
    FROM anime_characters c JOIN candidates am ON am.bangumi_id = c.bangumi_id
    UNION ALL
    SELECT ca.bangumi_id, 'actor_short_summary', a.actor_id, a.name,
           a.short_summary, a.short_summary_cn, a.actor_id
    FROM (SELECT DISTINCT ca.bangumi_id, ca.actor_id FROM character_actors ca
          JOIN candidates am ON am.bangumi_id = ca.bangumi_id) ca
    JOIN actors a ON a.actor_id = ca.actor_id
)
`

const progressAggregatesSQL = `
, counts AS (
    SELECT bangumi_id, kind,
           SUM(original != '') AS total,
           SUM(original != '' AND translated != '') AS completed,
           SUM(original = '') AS missing_source
    FROM targets GROUP BY bangumi_id, kind
), progress AS (
    SELECT am.*, SUM(c.total) AS total, SUM(c.completed) AS completed,
           CASE
             WHEN SUM(c.total) > SUM(c.completed) THEN 'pending'
             WHEN am.detail_status != 'completed' OR am.characters_status != 'completed'
                  OR am.episodes_status != 'completed' THEN 'metadata_pending'
             WHEN SUM(c.total) = 0 THEN 'no_content'
             ELSE 'completed'
           END AS progress_status
    FROM candidates am JOIN counts c ON c.bangumi_id = am.bangumi_id
    GROUP BY am.id
)
`

func emptyProgressCounts() []ProgressCount {
	return []ProgressCount{
		{Kind: string(targetAnimeTitle)}, {Kind: string(targetAnimeSummary)},
		{Kind: string(targetEpisodeTitle)}, {Kind: string(targetEpisodeSummary)},
		{Kind: string(targetCharacterSummary)}, {Kind: string(targetActorShortSummary)},
	}
}

func (s *Service) ListProgress(ctx context.Context, page, pageSize int, query, status string) (ProgressPage, error) {
	result := ProgressPage{Items: make([]AnimeProgress, 0)}
	switch status {
	case "", "all", "pending", "metadata_pending", "completed", "no_content":
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
	where, args := progressAnimeWhere(query)
	cte := `WITH candidates AS (
    SELECT am.id, am.bangumi_id, am.name, am.name_cn, am.summary, am.summary_cn, am.created_at,
           am.detail_status, am.detail_error, am.characters_status, am.characters_error,
           am.episodes_status, am.episodes_error
    FROM anime_metadata am WHERE ` + where + ")" + progressTargetsSQL + progressAggregatesSQL
	filter := "1 = 1"
	if status != "" && status != "all" {
		filter = "progress_status = ?"
		args = append(args, status)
	}
	if err := s.db.QueryRowContext(ctx, cte+" SELECT COUNT(*) FROM progress WHERE "+filter, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, cte+`
, selected AS (
    SELECT * FROM progress WHERE `+filter+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?
)
SELECT p.bangumi_id, p.name, p.name_cn, p.progress_status, p.total, p.completed,
       p.detail_status, p.detail_error, p.characters_status, p.characters_error,
       p.episodes_status, p.episodes_error, c.kind, c.total, c.completed, c.missing_source
FROM selected p JOIN counts c ON c.bangumi_id = p.bangumi_id
ORDER BY p.created_at DESC, p.id DESC, c.kind`, queryArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AnimeProgress
		var count ProgressCount
		stages := []MetadataStage{{Kind: "detail"}, {Kind: "characters"}, {Kind: "episodes"}}
		if err := rows.Scan(&item.BangumiID, &item.Name, &item.NameCN, &item.Status, &item.Total, &item.Completed,
			&stages[0].Status, &stages[0].ErrorMessage, &stages[1].Status, &stages[1].ErrorMessage,
			&stages[2].Status, &stages[2].ErrorMessage, &count.Kind, &count.Total, &count.Completed, &count.MissingSource); err != nil {
			return result, err
		}
		if len(result.Items) == 0 || result.Items[len(result.Items)-1].BangumiID != item.BangumiID {
			item.Counts, item.Metadata = emptyProgressCounts(), stages
			result.Items = append(result.Items, item)
		}
		last := &result.Items[len(result.Items)-1]
		for i := range last.Counts {
			if last.Counts[i].Kind == count.Kind {
				last.Counts[i] = count
				break
			}
		}
	}
	return result, rows.Err()
}

func (s *Service) ProgressDetails(ctx context.Context, bangumiID int64) ([]ProgressTarget, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM anime_metadata WHERE bangumi_id = ? AND deleted_at IS NULL)", bangumiID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, bangumi.ErrAnimeNotFound
	}
	rows, err := s.db.QueryContext(ctx, `WITH candidates AS (
    SELECT * FROM anime_metadata WHERE bangumi_id = ? AND deleted_at IS NULL
)`+progressTargetsSQL+`
SELECT kind, target_id, label,
       CASE WHEN original = '' THEN 'no_source' WHEN translated = '' THEN 'pending' ELSE 'completed' END
FROM targets ORDER BY kind, position, target_id`, bangumiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ProgressTarget, 0)
	for rows.Next() {
		var item ProgressTarget
		if err := rows.Scan(&item.Kind, &item.ID, &item.Label, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func progressAnimeWhere(query string) (string, []any) {
	where := "am.deleted_at IS NULL"
	if query = strings.TrimSpace(query); query != "" {
		where += ` AND (am.name LIKE ? OR am.name_cn LIKE ? OR EXISTS (
            SELECT 1 FROM anime_aliases aa WHERE aa.bangumi_id = am.bangumi_id AND aa.alias LIKE ?))`
		like := "%" + query + "%"
		return where, []any{like, like, like}
	}
	return where, nil
}
