package viewer

import "context"

type RecentActivity struct {
	ID              int64  `json:"id"`
	UserID          int64  `json:"userId"`
	Username        string `json:"username"`
	BangumiID       int64  `json:"bangumiId"`
	AnimeTitle      string `json:"animeTitle"`
	EpisodeLabel    string `json:"episodeLabel"`
	EpisodeTitle    string `json:"episodeTitle"`
	ProgressPercent int    `json:"progressPercent"`
	Completed       bool   `json:"completed"`
	LastWatchedAt   int64  `json:"lastWatchedAt"`
}

// RecentActivities is an administrator-only cross-user view of the same
// per-user/per-media snapshots shown in user management, not a playback log.
func (s *Service) RecentActivities(ctx context.Context) ([]RecentActivity, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT history.id, history.user_id, users.username, history.bangumi_id,
       COALESCE(NULLIF(anime.name_cn, ''), anime.name), media.season_number,
       COALESCE(NULLIF(media.episode_type, ''), 'episode'), media.episode_number,
       COALESCE((
           SELECT COALESCE(NULLIF(episode.name_cn, ''), episode.name)
           FROM anime_episodes episode
           WHERE episode.bangumi_id = media.bangumi_id
             AND episode.sort_number = CAST(media.episode_number AS REAL)
             AND ((LOWER(COALESCE(NULLIF(media.episode_type, ''), 'episode')) = 'episode' AND episode.type = 0)
               OR (LOWER(COALESCE(NULLIF(media.episode_type, ''), 'episode')) != 'episode' AND episode.type != 0))
           ORDER BY episode.type, episode.episode_id LIMIT 1
       ), ''), history.position_seconds, history.duration_seconds,
       history.completed, history.last_watched_at
FROM viewer_watch_history history
JOIN viewer_users users ON users.id = history.user_id
JOIN media_jobs media ON media.id = history.media_job_id AND media.bangumi_id = history.bangumi_id
JOIN anime_metadata anime ON anime.bangumi_id = history.bangumi_id
WHERE anime.deleted_at IS NULL AND media.status = 'completed' AND media.output_path != ''
ORDER BY history.last_watched_at DESC, history.id DESC LIMIT 5`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]RecentActivity, 0, 5)
	for rows.Next() {
		var item RecentActivity
		var progress WatchHistoryItem
		var season int
		var episodeType, number string
		if err := rows.Scan(&item.ID, &item.UserID, &item.Username, &item.BangumiID,
			&item.AnimeTitle, &season, &episodeType, &number, &item.EpisodeTitle,
			&progress.PositionSeconds, &progress.DurationSeconds, &progress.Completed, &item.LastWatchedAt); err != nil {
			return nil, err
		}
		finalizeWatchHistoryItem(&progress, season, episodeType, number)
		item.EpisodeLabel, item.ProgressPercent, item.Completed = progress.EpisodeLabel, progress.ProgressPercent, progress.Completed
		items = append(items, item)
	}
	return items, rows.Err()
}
