package download

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"bangumipipeline.local/server/internal/system"
)

const (
	TaskKey = "download-bound-episodes"

	StatusPending     = "pending"
	StatusDownloading = "downloading"
	StatusCompleted   = "completed"
	StatusFailed      = "failed"

	minFreeDiskBytes uint64 = 10 * 1024 * 1024 * 1024
)

var (
	ErrInvalidStatus       = errors.New("invalid download status")
	ErrDownloadJobNotFound = errors.New("download job not found")
	ErrRetryNotAllowed     = errors.New("download job retry not allowed")
	ErrQBitUnavailable     = errors.New("qBittorrent unavailable")
)

type SettingsProvider interface {
	GetDownloadSettings(context.Context) (system.DownloadSettings, error)
}

type Config struct {
	DownloadDir string
}

type Service struct {
	db          *sql.DB
	settings    SettingsProvider
	logger      *slog.Logger
	downloadDir string
	now         func() time.Time
}

type ConnectionTestResult struct {
	Version string `json:"version"`
}

type JobPage struct {
	Items    []Job `json:"items"`
	Total    int   `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

type RetryResult struct {
	Job    Job    `json:"job"`
	Action string `json:"action"`
}

type Job struct {
	ID                 int64   `json:"id"`
	SubscriptionItemID int64   `json:"subscriptionItemId"`
	Title              string  `json:"title"`
	BangumiID          int64   `json:"bangumiId"`
	AnimeName          string  `json:"animeName"`
	SeasonNumber       int     `json:"seasonNumber"`
	EpisodeType        string  `json:"episodeType"`
	EpisodeNumber      string  `json:"episodeNumber"`
	Status             string  `json:"status"`
	FolderName         string  `json:"folderName"`
	QBitName           string  `json:"qbitName"`
	Progress           float64 `json:"progress"`
	TotalSize          int64   `json:"totalSize"`
	DownloadedSize     int64   `json:"downloadedSize"`
	DownloadSpeed      int64   `json:"downloadSpeed"`
	ErrorMessage       string  `json:"errorMessage"`
	StartedAt          *int64  `json:"startedAt"`
	CompletedAt        *int64  `json:"completedAt"`
	FailedAt           *int64  `json:"failedAt"`
	CreatedAt          int64   `json:"createdAt"`
	UpdatedAt          int64   `json:"updatedAt"`
}

type pendingCandidate struct {
	JobID              int64
	SubscriptionItemID int64
	Title              string
	SourceURL          string
	BangumiID          int64
	AnimeName          string
	SeasonNumber       int
	EpisodeType        string
	EpisodeNumber      string
}

type activeJob struct {
	ID                 int64
	SubscriptionItemID int64
	QBitHash           string
	SavePath           string
	Status             string
	StartedAt          *int64
}

func NewService(db *sql.DB, settings SettingsProvider, logger *slog.Logger, config Config) *Service {
	downloadDir := strings.TrimSpace(config.DownloadDir)
	if downloadDir == "" {
		downloadDir = "./data/downloads"
	}
	if abs, err := filepath.Abs(downloadDir); err == nil {
		downloadDir = abs
	}
	return &Service{db: db, settings: settings, logger: logger, downloadDir: downloadDir, now: time.Now}
}

func (s *Service) Execute(ctx context.Context) error {
	settings, err := s.settings.GetDownloadSettings(ctx)
	if err != nil {
		return fmt.Errorf("读取下载设置: %w", err)
	}
	client, err := newQBitClient(settings)
	if err != nil {
		return err
	}
	if err := client.login(ctx, settings.Username, settings.Password); err != nil {
		return fmt.Errorf("连接 qBittorrent: %w", err)
	}

	synced, err := s.syncActiveJobs(ctx, client)
	if err != nil {
		return fmt.Errorf("同步 qBittorrent 下载状态: %w", err)
	}

	if err := os.MkdirAll(s.downloadDir, 0o755); err != nil {
		return fmt.Errorf("创建下载目录: %w", err)
	}
	freeBytes, err := freeDiskBytes(s.downloadDir)
	if err != nil {
		return fmt.Errorf("检查剩余磁盘空间: %w", err)
	}
	if freeBytes < minFreeDiskBytes {
		s.logger.Warn("下载任务跳过：剩余磁盘空间不足", "source", "download", "free_bytes", freeBytes, "min_free_bytes", minFreeDiskBytes)
		return nil
	}

	running, err := s.runningCount(ctx)
	if err != nil {
		return err
	}
	slots := settings.MaxConcurrentDownloads - running
	if slots <= 0 {
		s.logger.Info("下载任务跳过：并发下载数已达上限", "source", "download", "running", running, "max_concurrent", settings.MaxConcurrentDownloads, "synced", synced)
		return nil
	}

	candidates, err := s.nextPendingCandidates(ctx, slots)
	if err != nil {
		return err
	}
	started := 0
	for _, candidate := range candidates {
		if err := s.startCandidate(ctx, client, candidate); err != nil {
			return err
		}
		started++
	}
	s.logger.Info("下载番剧任务完成", "source", "download", "synced", synced, "started", started, "running_before_start", running, "available_slots", slots)
	return nil
}

func (s *Service) TestConnection(ctx context.Context, settings system.DownloadSettings) (ConnectionTestResult, error) {
	client, err := newQBitClient(settings)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	if err := client.login(ctx, settings.Username, settings.Password); err != nil {
		return ConnectionTestResult{}, err
	}
	version, err := client.version(ctx)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	return ConnectionTestResult{Version: version}, nil
}

func (s *Service) ListJobs(ctx context.Context, page, pageSize int, status string) (JobPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	status = normalizeStatus(status)
	if status == "invalid" {
		return JobPage{}, ErrInvalidStatus
	}

	result := JobPage{Items: make([]Job, 0), Page: page, PageSize: pageSize}
	where, args := jobListWhere(status)
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	query := jobListSelect + where + `
ORDER BY COALESCE(si.published_at, si.created_at) DESC, si.id DESC
LIMIT ? OFFSET ?`
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, job)
	}
	return result, rows.Err()
}

func (s *Service) RetryFailedJob(ctx context.Context, jobID int64) (RetryResult, error) {
	job, err := s.failedJob(ctx, jobID)
	if err != nil {
		return RetryResult{}, err
	}

	settings, err := s.settings.GetDownloadSettings(ctx)
	if err != nil {
		return RetryResult{}, err
	}
	client, err := newQBitClient(settings)
	if err != nil {
		return RetryResult{}, err
	}
	if err := client.login(ctx, settings.Username, settings.Password); err != nil {
		return RetryResult{}, fmt.Errorf("%w: %w", ErrQBitUnavailable, err)
	}
	torrents, err := client.torrents(ctx)
	if err != nil {
		return RetryResult{}, fmt.Errorf("%w: %w", ErrQBitUnavailable, err)
	}

	torrent, ok := matchTorrent(job, torrents, torrentsByHash(torrents))
	if !ok {
		updated, err := s.resetFailedJobToPending(ctx, jobID)
		if err != nil {
			return RetryResult{}, err
		}
		s.logger.Info("下载失败任务已重置为待下载", "source", "download", "job_id", jobID, "reason", "qbit_task_missing")
		return RetryResult{Job: updated, Action: "reset"}, nil
	}

	if statusFromTorrent(torrent) != StatusFailed {
		if err := s.updateFromTorrent(ctx, jobID, torrent); err != nil {
			return RetryResult{}, err
		}
		updated, err := s.jobByID(ctx, jobID)
		if err != nil {
			return RetryResult{}, err
		}
		s.logger.Info("下载失败任务状态已从 qBittorrent 纠正", "source", "download", "job_id", jobID, "qbit_hash", torrent.Hash, "status", updated.Status)
		return RetryResult{Job: updated, Action: "corrected"}, nil
	}

	if err := client.deleteTorrents(ctx, []string{torrent.Hash}, false); err != nil {
		return RetryResult{}, fmt.Errorf("%w: %w", ErrQBitUnavailable, err)
	}
	updated, err := s.resetFailedJobToPending(ctx, jobID)
	if err != nil {
		return RetryResult{}, err
	}
	s.logger.Info("qBittorrent 失败任务已删除并重置为待下载", "source", "download", "job_id", jobID, "qbit_hash", torrent.Hash)
	return RetryResult{Job: updated, Action: "deleted_reset"}, nil
}

func (s *Service) CleanupCompletedQBitTask(ctx context.Context, jobID int64) error {
	job, err := s.downloadJobForCleanup(ctx, jobID)
	if err != nil {
		return err
	}

	settings, err := s.settings.GetDownloadSettings(ctx)
	if err != nil {
		return err
	}
	client, err := newQBitClient(settings)
	if err != nil {
		return err
	}
	if err := client.login(ctx, settings.Username, settings.Password); err != nil {
		return fmt.Errorf("%w: %w", ErrQBitUnavailable, err)
	}
	torrents, err := client.torrents(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrQBitUnavailable, err)
	}

	tag := tagForItem(job.SubscriptionItemID)
	torrent, ok := matchTorrent(job, torrents, torrentsByHash(torrents))
	if ok {
		if err := client.deleteTorrents(ctx, []string{torrent.Hash}, true); err != nil {
			return fmt.Errorf("%w: %w", ErrQBitUnavailable, err)
		}
		s.logger.Info("qBittorrent 下载任务和文件已清理", "source", "download", "job_id", jobID, "qbit_hash", torrent.Hash)
	} else {
		s.logger.Info("qBittorrent 下载任务已不存在，跳过任务删除", "source", "download", "job_id", jobID)
	}
	if err := client.deleteTags(ctx, []string{tag}); err != nil {
		s.logger.Warn("qBittorrent 单集标签清理失败", "source", "download", "job_id", jobID, "tag", tag, "error", err)
	}
	return nil
}

func (s *Service) downloadJobForCleanup(ctx context.Context, jobID int64) (activeJob, error) {
	var job activeJob
	err := s.db.QueryRowContext(ctx, `
SELECT id, subscription_item_id, qbit_hash, save_path, status
FROM download_jobs
WHERE id = ?`, jobID).Scan(&job.ID, &job.SubscriptionItemID, &job.QBitHash, &job.SavePath, &job.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return activeJob{}, ErrDownloadJobNotFound
	}
	return job, err
}

func (s *Service) failedJob(ctx context.Context, jobID int64) (activeJob, error) {
	var job activeJob
	var startedAt sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT id, subscription_item_id, qbit_hash, save_path, status, started_at
FROM download_jobs
WHERE id = ?`, jobID).Scan(&job.ID, &job.SubscriptionItemID, &job.QBitHash, &job.SavePath, &job.Status, &startedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return activeJob{}, ErrDownloadJobNotFound
	}
	if err != nil {
		return activeJob{}, err
	}
	if job.Status != StatusFailed {
		return activeJob{}, ErrRetryNotAllowed
	}
	job.StartedAt = nullableInt64(startedAt)
	return job, nil
}

func (s *Service) resetFailedJobToPending(ctx context.Context, jobID int64) (Job, error) {
	now := s.now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `
UPDATE download_jobs
SET status = ?, qbit_hash = '', qbit_name = '', progress = 0, total_size = 0, downloaded_size = 0,
    download_speed = 0, error_message = '', started_at = NULL, completed_at = NULL, failed_at = NULL,
    updated_at = ?
WHERE id = ? AND status = ?`, StatusPending, now, jobID, StatusFailed)
	if err != nil {
		return Job{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Job{}, err
	}
	if affected == 0 {
		return Job{}, ErrRetryNotAllowed
	}
	return s.jobByID(ctx, jobID)
}

func (s *Service) jobByID(ctx context.Context, jobID int64) (Job, error) {
	row := s.db.QueryRowContext(ctx, jobListSelect+`
FROM subscription_items si
JOIN download_jobs dj ON dj.subscription_item_id = si.id
WHERE dj.id = ?`, jobID)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrDownloadJobNotFound
	}
	return job, err
}

func (s *Service) syncActiveJobs(ctx context.Context, client *qBitClient) (int, error) {
	jobs, err := s.syncableJobs(ctx)
	if err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}
	torrents, err := client.torrents(ctx)
	if err != nil {
		return 0, err
	}
	byHash := torrentsByHash(torrents)

	synced := 0
	for _, job := range jobs {
		torrent, ok := matchTorrent(job, torrents, byHash)
		if !ok {
			if job.QBitHash != "" {
				if err := s.markFailed(ctx, job.ID, "qBittorrent 任务不存在"); err != nil {
					return synced, err
				}
			}
			continue
		}
		if err := s.updateFromTorrent(ctx, job.ID, torrent); err != nil {
			return synced, err
		}
		synced++
	}
	return synced, nil
}

func (s *Service) syncableJobs(ctx context.Context) ([]activeJob, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, subscription_item_id, qbit_hash, save_path, started_at
FROM download_jobs
WHERE status IN (?, ?)`, StatusDownloading, StatusFailed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]activeJob, 0)
	for rows.Next() {
		var job activeJob
		var startedAt sql.NullInt64
		if err := rows.Scan(&job.ID, &job.SubscriptionItemID, &job.QBitHash, &job.SavePath, &startedAt); err != nil {
			return nil, err
		}
		job.StartedAt = nullableInt64(startedAt)
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Service) runningCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM download_jobs WHERE status = ?", StatusDownloading).Scan(&count)
	return count, err
}

func (s *Service) nextPendingCandidates(ctx context.Context, limit int) ([]pendingCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(dj.id, 0), si.id, si.title,
       COALESCE(NULLIF(si.enclosure_url, ''), NULLIF(si.torrent_url, ''), si.link) AS source_url,
       si.bound_bangumi_id, si.bound_anime_name, si.bound_season_number,
       COALESCE(NULLIF(si.bound_episode_type, ''), 'episode'), si.bound_episode_number
FROM subscription_items si
LEFT JOIN download_jobs dj ON dj.subscription_item_id = si.id
WHERE si.binding_status = 'bound'
  AND si.bound_bangumi_id IS NOT NULL
  AND si.bound_season_number IS NOT NULL
  AND si.bound_episode_number != ''
  AND (dj.id IS NULL OR dj.status = ?)
ORDER BY COALESCE(si.published_at, si.created_at), si.id
LIMIT ?`, StatusPending, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]pendingCandidate, 0)
	for rows.Next() {
		var item pendingCandidate
		if err := rows.Scan(
			&item.JobID, &item.SubscriptionItemID, &item.Title, &item.SourceURL,
			&item.BangumiID, &item.AnimeName, &item.SeasonNumber,
			&item.EpisodeType, &item.EpisodeNumber,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) startCandidate(ctx context.Context, client *qBitClient, candidate pendingCandidate) error {
	now := s.now().UTC().Unix()
	candidate.SourceURL = normalizeDownloadSourceURL(candidate.SourceURL)
	folderName := folderNameFor(candidate)
	savePath := filepath.Join(s.downloadDir, folderName)
	jobID, err := s.ensureJob(ctx, candidate, folderName, savePath, now)
	if err != nil {
		return err
	}
	if strings.TrimSpace(candidate.SourceURL) == "" {
		return s.markFailed(ctx, jobID, "订阅条目没有可用的 torrent 或 magnet URL")
	}
	if !isSupportedSourceURL(candidate.SourceURL) {
		return s.markFailed(ctx, jobID, "下载链接不是 HTTP/HTTPS torrent 或 magnet URL")
	}
	if err := os.MkdirAll(savePath, 0o755); err != nil {
		_ = s.markFailed(ctx, jobID, err.Error())
		return err
	}
	tags := []string{commonQBitTag, tagForItem(candidate.SubscriptionItemID)}
	if err := client.addURL(ctx, candidate.SourceURL, savePath, tags); err != nil {
		_ = s.markFailed(ctx, jobID, err.Error())
		return nil
	}
	torrent, ok, err := s.waitForTorrent(ctx, client, candidate.SubscriptionItemID, savePath)
	if err != nil {
		_ = s.markFailed(ctx, jobID, err.Error())
		return nil
	}
	if !ok {
		_ = s.markFailed(ctx, jobID, "qBittorrent 已接受任务，但未能查询到任务状态")
		return nil
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE download_jobs
SET status = ?, qbit_hash = ?, qbit_name = ?, progress = ?, total_size = ?, downloaded_size = ?,
    download_speed = ?, error_message = '', started_at = COALESCE(started_at, ?), updated_at = ?
WHERE id = ?`, StatusDownloading, torrent.Hash, torrent.Name, torrent.Progress, torrent.Size, torrent.Downloaded,
		torrent.DownloadSpeed, now, now, jobID)
	return err
}

func (s *Service) ensureJob(ctx context.Context, candidate pendingCandidate, folderName, savePath string, now int64) (int64, error) {
	if candidate.JobID > 0 {
		_, err := s.db.ExecContext(ctx, `
UPDATE download_jobs
SET source_url = ?, folder_name = ?, save_path = ?, updated_at = ?
WHERE id = ? AND status = ?`, candidate.SourceURL, folderName, savePath, now, candidate.JobID, StatusPending)
		return candidate.JobID, err
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO download_jobs(
    subscription_item_id, status, source_url, folder_name, save_path, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`, candidate.SubscriptionItemID, StatusPending, candidate.SourceURL, folderName, savePath, now, now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Service) waitForTorrent(ctx context.Context, client *qBitClient, itemID int64, savePath string) (qBitTorrent, bool, error) {
	for attempt := 0; attempt < 10; attempt++ {
		torrents, err := client.torrents(ctx)
		if err != nil {
			return qBitTorrent{}, false, err
		}
		job := activeJob{SubscriptionItemID: itemID, SavePath: savePath}
		if torrent, ok := matchTorrent(job, torrents, nil); ok {
			return torrent, true, nil
		}
		select {
		case <-ctx.Done():
			return qBitTorrent{}, false, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return qBitTorrent{}, false, nil
}

func matchTorrent(job activeJob, torrents []qBitTorrent, byHash map[string]qBitTorrent) (qBitTorrent, bool) {
	if job.QBitHash != "" && byHash != nil {
		if torrent, ok := byHash[strings.ToLower(job.QBitHash)]; ok {
			return torrent, true
		}
	}
	tag := tagForItem(job.SubscriptionItemID)
	for _, torrent := range torrents {
		if torrentHasTag(torrent, tag) || sameDownloadPath(torrent.SavePath, job.SavePath) {
			return torrent, true
		}
	}
	return qBitTorrent{}, false
}

func torrentsByHash(torrents []qBitTorrent) map[string]qBitTorrent {
	byHash := make(map[string]qBitTorrent, len(torrents))
	for _, torrent := range torrents {
		if torrent.Hash != "" {
			byHash[strings.ToLower(torrent.Hash)] = torrent
		}
	}
	return byHash
}

func sameDownloadPath(left, right string) bool {
	left = normalizeDownloadPath(left)
	right = normalizeDownloadPath(right)
	return left != "" && right != "" && strings.EqualFold(left, right)
}

func normalizeDownloadPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.TrimRight(value, `/\`)
	value = filepath.Clean(value)
	return strings.ReplaceAll(value, `\`, `/`)
}

func (s *Service) updateFromTorrent(ctx context.Context, jobID int64, torrent qBitTorrent) error {
	status := statusFromTorrent(torrent)
	now := s.now().UTC().Unix()
	errorMessage := ""
	if status == StatusFailed {
		errorMessage = "qBittorrent 状态异常: " + torrent.State
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE download_jobs
SET status = ?, qbit_hash = ?, qbit_name = ?, progress = ?, total_size = ?, downloaded_size = ?,
    download_speed = ?, error_message = ?,
    started_at = COALESCE(started_at, ?),
    completed_at = CASE WHEN ? = ? AND completed_at IS NULL THEN ? ELSE completed_at END,
    failed_at = CASE WHEN ? = ? THEN COALESCE(failed_at, ?) ELSE NULL END,
    updated_at = ?
WHERE id = ?`, status, torrent.Hash, torrent.Name, torrent.Progress, torrent.Size, torrent.Downloaded,
		torrent.DownloadSpeed, errorMessage, now, status, StatusCompleted, now, status, StatusFailed, now, now, jobID)
	return err
}

func (s *Service) markFailed(ctx context.Context, jobID int64, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE download_jobs
SET status = ?, error_message = ?, download_speed = 0,
    failed_at = COALESCE(failed_at, ?), updated_at = ?
WHERE id = ?`, StatusFailed, message, now, now, jobID)
	return err
}

func statusFromTorrent(torrent qBitTorrent) string {
	state := strings.ToLower(strings.TrimSpace(torrent.State))
	if state == "error" || state == "missingfiles" || strings.Contains(state, "error") {
		return StatusFailed
	}
	if torrent.Progress >= 1 || strings.HasSuffix(state, "up") || state == "uploading" || state == "stalledup" {
		return StatusCompleted
	}
	return StatusDownloading
}

func isSupportedSourceURL(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "magnet:") {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func normalizeDownloadSourceURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(strings.ToLower(value), "magnet:") {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	if !strings.EqualFold(parsed.Host, "mikanani.me") {
		return value
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 3 || !strings.EqualFold(parts[0], "Home") || !strings.EqualFold(parts[1], "Episode") {
		return value
	}
	hash := strings.TrimSpace(parts[2])
	if !isBTIH(hash) {
		return value
	}
	return "magnet:?xt=urn:btih:" + hash
}

func isBTIH(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func folderNameFor(candidate pendingCandidate) string {
	anime := candidate.AnimeName
	if anime == "" {
		anime = fmt.Sprintf("Bangumi-%d", candidate.BangumiID)
	}
	episodeType := strings.ToUpper(candidate.EpisodeType)
	if episodeType == "" || episodeType == "EPISODE" {
		episodeType = "E"
	} else {
		episodeType += "-"
	}
	label := fmt.Sprintf("%s S%02d %s%s item-%d", anime, candidate.SeasonNumber, episodeType, candidate.EpisodeNumber, candidate.SubscriptionItemID)
	return safePathSegment(label)
}

func safePathSegment(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	value = strings.Trim(value, " ._")
	if value == "" {
		value = "episode"
	}
	runes := []rune(value)
	if len(runes) > 120 {
		value = string(runes[:120])
	}
	return value
}

func normalizeStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "":
		return ""
	case StatusPending, StatusDownloading, StatusCompleted, StatusFailed:
		return status
	default:
		return "invalid"
	}
}

func jobListWhere(status string) (string, []any) {
	where := `
FROM subscription_items si
LEFT JOIN download_jobs dj ON dj.subscription_item_id = si.id
WHERE si.binding_status = 'bound'
  AND si.bound_bangumi_id IS NOT NULL
  AND si.bound_season_number IS NOT NULL
  AND si.bound_episode_number != ''`
	args := make([]any, 0, 1)
	switch status {
	case StatusPending:
		where += "\n  AND COALESCE(dj.status, 'pending') = ?"
		args = append(args, StatusPending)
	case StatusDownloading, StatusCompleted, StatusFailed:
		where += "\n  AND dj.status = ?"
		args = append(args, status)
	}
	return where, args
}

const jobListSelect = `
SELECT COALESCE(dj.id, 0), si.id, si.title, si.bound_bangumi_id, si.bound_anime_name,
       si.bound_season_number, COALESCE(NULLIF(si.bound_episode_type, ''), 'episode'), si.bound_episode_number,
       COALESCE(dj.status, 'pending'), COALESCE(dj.folder_name, ''), COALESCE(dj.qbit_name, ''),
       COALESCE(dj.progress, 0), COALESCE(dj.total_size, 0), COALESCE(dj.downloaded_size, 0),
       COALESCE(dj.download_speed, 0), COALESCE(dj.error_message, ''),
       dj.started_at, dj.completed_at, dj.failed_at,
       COALESCE(dj.created_at, si.created_at), COALESCE(dj.updated_at, si.updated_at)
`

func scanJob(row interface{ Scan(dest ...any) error }) (Job, error) {
	var job Job
	var startedAt, completedAt, failedAt sql.NullInt64
	if err := row.Scan(
		&job.ID, &job.SubscriptionItemID, &job.Title, &job.BangumiID, &job.AnimeName,
		&job.SeasonNumber, &job.EpisodeType, &job.EpisodeNumber, &job.Status,
		&job.FolderName, &job.QBitName, &job.Progress, &job.TotalSize, &job.DownloadedSize,
		&job.DownloadSpeed, &job.ErrorMessage, &startedAt, &completedAt, &failedAt,
		&job.CreatedAt, &job.UpdatedAt,
	); err != nil {
		return Job{}, err
	}
	job.StartedAt = nullableInt64(startedAt)
	job.CompletedAt = nullableInt64(completedAt)
	job.FailedAt = nullableInt64(failedAt)
	return job, nil
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
