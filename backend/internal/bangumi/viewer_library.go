package bangumi

import (
	"context"
	"fmt"
	"strings"
)

type ViewerLibrary struct {
	Items []ViewerScheduleCard `json:"items"`
	Total int                  `json:"total"`
}

// ViewerLibrary applies OR matching inside each tag group and AND matching
// between groups. Completed media is used only for ordering and progress; no
// filesystem path is exposed to the viewer client.
func (c *Catalog) ViewerLibrary(ctx context.Context, query string, filterGroups [][]string) (ViewerLibrary, error) {
	return c.ViewerLibraryPage(ctx, query, filterGroups, 0, 0)
}

// ViewerLibraryPage returns a page when pageSize is positive. Passing a zero
// pageSize preserves the legacy unpaginated behavior for existing clients.
func (c *Catalog) ViewerLibraryPage(ctx context.Context, query string, filterGroups [][]string, page, pageSize int) (ViewerLibrary, error) {
	where := []string{"am.deleted_at IS NULL"}
	args := make([]any, 0)
	query = strings.TrimSpace(query)
	if query != "" {
		like := "%" + query + "%"
		where = append(where, `(
    am.name LIKE ?
    OR am.name_cn LIKE ?
    OR EXISTS (
        SELECT 1 FROM anime_aliases alias
        WHERE alias.bangumi_id = am.bangumi_id AND alias.alias LIKE ?
    )
)`)
		args = append(args, like, like, like)
	}
	for _, group := range filterGroups {
		if len(group) == 0 {
			continue
		}
		placeholders := make([]string, len(group))
		for index, tag := range group {
			placeholders[index] = "?"
			args = append(args, tag)
		}
		where = append(where, fmt.Sprintf(`EXISTS (
    SELECT 1 FROM anime_tags tag
    WHERE tag.bangumi_id = am.bangumi_id
      AND tag.name IN (%s)
)`, strings.Join(placeholders, ",")))
	}

	whereSQL := strings.Join(where, "\n  AND ")
	var total int
	if err := c.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM anime_metadata am
WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ViewerLibrary{}, err
	}

	queryArgs := append([]any{}, args...)
	querySQL := `
SELECT am.bangumi_id,
       COALESCE(NULLIF(am.name_cn, ''), am.name),
       am.air_date,
       am.air_weekday,
       CASE WHEN am.total_episodes > 0 THEN am.total_episodes ELSE am.eps END,
       am.image_local_path != '',
       am.image_status
FROM anime_metadata am
WHERE ` + whereSQL + `
ORDER BY EXISTS (
             SELECT 1 FROM media_jobs media
             WHERE media.bangumi_id = am.bangumi_id
               AND media.status = 'completed'
               AND media.output_path != ''
         ) DESC,
         CASE WHEN am.air_date = '' THEN 1 ELSE 0 END,
         am.air_date DESC,
         COALESCE(NULLIF(am.name_cn, ''), am.name),
	         am.bangumi_id DESC`
	if pageSize > 0 {
		if page < 1 {
			page = 1
		}
		querySQL += "\nLIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	}
	rows, err := c.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return ViewerLibrary{}, err
	}
	items := make([]ViewerScheduleCard, 0)
	for rows.Next() {
		var item ViewerScheduleCard
		if err := rows.Scan(
			&item.BangumiID, &item.Title, &item.AirDate, &item.AirWeekday,
			&item.TotalEpisodes, &item.HasCover, &item.ImageStatus,
		); err != nil {
			rows.Close()
			return ViewerLibrary{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ViewerLibrary{}, err
	}
	if err := rows.Close(); err != nil {
		return ViewerLibrary{}, err
	}
	if err := c.attachViewerScheduleProgress(ctx, items); err != nil {
		return ViewerLibrary{}, err
	}
	return ViewerLibrary{Items: items, Total: total}, nil
}
