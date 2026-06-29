package bangumi

import (
	"context"
	"fmt"
	"strings"
)

type ViewerSchedule struct {
	SeasonKey   string               `json:"seasonKey"`
	SeasonLabel string               `json:"seasonLabel"`
	Items       []ViewerScheduleCard `json:"items"`
}

type ViewerScheduleCard struct {
	BangumiID          int64  `json:"bangumiId"`
	Title              string `json:"title"`
	AirDate            string `json:"airDate"`
	AirWeekday         int    `json:"airWeekday"`
	TotalEpisodes      int    `json:"totalEpisodes"`
	HasCover           bool   `json:"hasCover"`
	ImageStatus        string `json:"imageStatus"`
	LatestEpisode      string `json:"latestEpisode"`
	LatestEpisodeLabel string `json:"latestEpisodeLabel"`
}

// ViewerSchedule returns every active anime carrying the exact Bangumi season
// tag. Media progress is attached only after the metadata rows are closed so
// the single-connection SQLite setup cannot deadlock on a nested query.
func (c *Catalog) ViewerSchedule(ctx context.Context, year, month int) (ViewerSchedule, error) {
	season := ViewerSchedule{
		SeasonKey:   fmt.Sprintf("%04d-%02d", year, month),
		SeasonLabel: fmt.Sprintf("%d年%d月", year, month),
		Items:       make([]ViewerScheduleCard, 0),
	}

	rows, err := c.db.QueryContext(ctx, `
SELECT am.bangumi_id,
       COALESCE(NULLIF(am.name_cn, ''), am.name),
       am.air_date,
       am.air_weekday,
       CASE WHEN am.total_episodes > 0 THEN am.total_episodes ELSE am.eps END,
       am.image_local_path != '',
       am.image_status
FROM anime_metadata am
WHERE am.deleted_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM anime_tags tag
      WHERE tag.bangumi_id = am.bangumi_id
        AND tag.name = ?
  )
ORDER BY CASE WHEN am.air_weekday BETWEEN 1 AND 7 THEN am.air_weekday ELSE 8 END,
         CASE WHEN am.air_date = '' THEN 1 ELSE 0 END,
         am.air_date,
         COALESCE(NULLIF(am.name_cn, ''), am.name),
         am.bangumi_id`, season.SeasonLabel)
	if err != nil {
		return ViewerSchedule{}, err
	}
	for rows.Next() {
		var item ViewerScheduleCard
		if err := rows.Scan(
			&item.BangumiID, &item.Title, &item.AirDate, &item.AirWeekday,
			&item.TotalEpisodes, &item.HasCover, &item.ImageStatus,
		); err != nil {
			rows.Close()
			return ViewerSchedule{}, err
		}
		season.Items = append(season.Items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ViewerSchedule{}, err
	}
	if err := rows.Close(); err != nil {
		return ViewerSchedule{}, err
	}

	if err := c.attachViewerScheduleProgress(ctx, season.Items); err != nil {
		return ViewerSchedule{}, err
	}
	return season, nil
}

func (c *Catalog) attachViewerScheduleProgress(ctx context.Context, items []ViewerScheduleCard) error {
	if len(items) == 0 {
		return nil
	}
	indexByID := make(map[int64]int, len(items))
	args := make([]any, 0, len(items))
	placeholders := make([]string, 0, len(items))
	for index, item := range items {
		indexByID[item.BangumiID] = index
		args = append(args, item.BangumiID)
		placeholders = append(placeholders, "?")
	}

	rows, err := c.db.QueryContext(ctx, fmt.Sprintf(`
SELECT bangumi_id,
       season_number,
       COALESCE(NULLIF(episode_type, ''), 'episode'),
       episode_number,
       COALESCE(completed_at, updated_at, created_at, 0)
FROM media_jobs
WHERE status = 'completed'
  AND output_path != ''
  AND bangumi_id IN (%s)
ORDER BY bangumi_id, id`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	progress := make(map[int64]viewerEpisodeRef, len(items))
	for rows.Next() {
		var bangumiID int64
		var episode viewerEpisodeRef
		if err := rows.Scan(
			&bangumiID, &episode.season, &episode.episodeType,
			&episode.episodeNumber, &episode.updatedAt,
		); err != nil {
			return err
		}
		current, exists := progress[bangumiID]
		if !exists || viewerEpisodeProgressLess(current, episode) {
			progress[bangumiID] = episode
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for bangumiID, episode := range progress {
		index := indexByID[bangumiID]
		items[index].LatestEpisode = episode.episodeNumber
		items[index].LatestEpisodeLabel = viewerEpisodeLabel(episode)
	}
	return nil
}
