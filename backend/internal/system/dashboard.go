package system

import (
	"context"
	"fmt"
	"time"
)

type DashboardBindingDay struct {
	Total     int `json:"total"`
	Automatic int `json:"automatic"`
	Manual    int `json:"manual"`
	Unknown   int `json:"unknown"`
}

type DashboardPipeline struct {
	GeneratedAt  int64  `json:"generatedAt"`
	Timezone     string `json:"timezone"`
	Today        string `json:"today"`
	Yesterday    string `json:"yesterday"`
	Subscription struct {
		PendingBindings int                 `json:"pendingBindings"`
		Today           DashboardBindingDay `json:"today"`
		Yesterday       DashboardBindingDay `json:"yesterday"`
	} `json:"subscription"`
	Download struct {
		Pending     int `json:"pending"`
		Downloading int `json:"downloading"`
		Failed      int `json:"failed"`
		Today       int `json:"today"`
		Yesterday   int `json:"yesterday"`
	} `json:"download"`
	Media struct {
		Pending     int `json:"pending"`
		Transcoding int `json:"transcoding"`
		Failed      int `json:"failed"`
		Today       int `json:"today"`
		Yesterday   int `json:"yesterday"`
	} `json:"media"`
}

// DashboardPipeline is a read-only snapshot. In particular it must never enqueue
// media work, unlike the management list's CountJobsByStatuses path.
func (s *Service) DashboardPipeline(ctx context.Context) (DashboardPipeline, error) {
	var result DashboardPipeline
	now := s.now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday, tomorrow := today.AddDate(0, 0, -1), today.AddDate(0, 0, 1)
	zone, _ := now.Zone()
	result.GeneratedAt, result.Today, result.Yesterday = now.Unix(), today.Format("2006-01-02"), yesterday.Format("2006-01-02")
	result.Timezone = fmt.Sprintf("%s (%s, UTC%s)", now.Location().String(), zone, now.Format("-07:00"))
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_items WHERE binding_status = 'pending'`).Scan(&result.Subscription.PendingBindings); err != nil {
		return result, err
	}
	// Notes are system-generated, not user-entered. Only known binding sources
	// are classified; legacy or unfamiliar notes remain visible as unknown.
	rows, err := s.db.QueryContext(ctx, `
SELECT bound_at >= ?, COUNT(*),
       SUM(binding_note IN ('标题记忆自动绑定', '历史话数同步自动绑定', '本地媒体文件夹同步自动绑定')),
       SUM(binding_note IN ('人工确认绑定', '人工手动绑定', '手动同步/替换单话'))
FROM subscription_items
WHERE binding_status = 'bound' AND bound_at >= ? AND bound_at < ?
GROUP BY bound_at >= ?`, today.Unix(), yesterday.Unix(), tomorrow.Unix(), today.Unix())
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var isToday bool
		var day DashboardBindingDay
		if err := rows.Scan(&isToday, &day.Total, &day.Automatic, &day.Manual); err != nil {
			rows.Close()
			return result, err
		}
		day.Unknown = day.Total - day.Automatic - day.Manual
		if isToday {
			result.Subscription.Today = day
		} else {
			result.Subscription.Yesterday = day
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	// Match download management, including bound items not yet enqueued and
	// excluding local imports, which are not qBittorrent downloads.
	if err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(COALESCE(dj.status, 'pending') = 'pending'), 0),
       COALESCE(SUM(dj.status = 'downloading'), 0), COALESCE(SUM(dj.status = 'failed'), 0)
FROM subscription_items si LEFT JOIN download_jobs dj ON dj.subscription_item_id = si.id
WHERE si.binding_status = 'bound' AND si.bound_bangumi_id IS NOT NULL
  AND si.bound_season_number IS NOT NULL AND si.bound_episode_number != ''
  AND COALESCE(NULLIF(dj.source_type, ''), 'download') = 'download'`).Scan(
		&result.Download.Pending, &result.Download.Downloading, &result.Download.Failed); err != nil {
		return result, err
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(dj.completed_at >= ?), 0), COALESCE(SUM(dj.completed_at < ?), 0)
FROM download_jobs dj JOIN subscription_items si ON si.id = dj.subscription_item_id
WHERE dj.status = 'completed' AND dj.completed_at >= ? AND dj.completed_at < ?
  AND COALESCE(NULLIF(dj.source_type, ''), 'download') = 'download'
  AND si.binding_status = 'bound' AND si.bound_bangumi_id IS NOT NULL
  AND si.bound_season_number IS NOT NULL AND si.bound_episode_number != ''`, today.Unix(), today.Unix(), yesterday.Unix(), tomorrow.Unix()).Scan(
		&result.Download.Today, &result.Download.Yesterday); err != nil {
		return result, err
	}
	return s.dashboardMedia(ctx, result, yesterday.Unix(), today.Unix(), tomorrow.Unix())
}

func (s *Service) dashboardMedia(ctx context.Context, result DashboardPipeline, start, today, end int64) (DashboardPipeline, error) {
	if err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(mj.status = 'pending'), 0), COALESCE(SUM(mj.status = 'transcoding'), 0),
       COALESCE(SUM(mj.status = 'failed'), 0)
FROM media_jobs mj JOIN subscription_items si ON si.id = mj.subscription_item_id
WHERE mj.status IN ('pending', 'transcoding', 'failed')`).Scan(
		&result.Media.Pending, &result.Media.Transcoding, &result.Media.Failed); err != nil {
		return result, err
	}
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(mj.completed_at >= ?), 0), COALESCE(SUM(mj.completed_at < ?), 0)
FROM media_jobs mj JOIN subscription_items si ON si.id = mj.subscription_item_id
WHERE mj.status = 'completed' AND mj.completed_at >= ? AND mj.completed_at < ?`, today, today, start, end).Scan(
		&result.Media.Today, &result.Media.Yesterday)
	return result, err
}
