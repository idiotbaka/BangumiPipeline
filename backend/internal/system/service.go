package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const (
	TaskStatusIdle      = "idle"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"

	MinIntervalMinutes = 1
	MaxIntervalMinutes = 43200

	MinConcurrentDownloads = 1
	MaxConcurrentDownloads = 50
)

var (
	ErrTaskNotFound            = errors.New("scheduled task not found")
	ErrTaskAlreadyRunning      = errors.New("scheduled task is already running")
	ErrInvalidInterval         = errors.New("interval minutes must be between 1 and 43200")
	ErrInvalidProxy            = errors.New("proxy must be an HTTP or HTTPS URL")
	ErrInvalidRSSURL           = errors.New("RSS URL must be an HTTP or HTTPS URL")
	ErrInvalidDownloadSettings = errors.New("invalid download settings")
	ErrInvalidMediaStoragePath = errors.New("invalid media storage path")
)

type ScheduledTask struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"intervalMinutes"`
	LastStatus      string `json:"lastStatus"`
	LastError       string `json:"lastError"`
	LastStartedAt   *int64 `json:"lastStartedAt"`
	LastFinishedAt  *int64 `json:"lastFinishedAt"`
	NextRunAt       *int64 `json:"nextRunAt"`
	CreatedAt       int64  `json:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt"`
}

type TaskUpdate struct {
	Enabled         *bool
	IntervalMinutes *int
}

type NetworkSettings struct {
	HTTPProxy  string `json:"httpProxy"`
	HTTPSProxy string `json:"httpsProxy"`
	UpdatedAt  int64  `json:"updatedAt"`
}

type SubscriptionSettings struct {
	RSSURL    string `json:"rssUrl"`
	UpdatedAt int64  `json:"updatedAt"`
}

type DownloadSettings struct {
	Host                   string `json:"host"`
	Port                   int    `json:"port"`
	Username               string `json:"username"`
	Password               string `json:"password"`
	MaxConcurrentDownloads int    `json:"maxConcurrentDownloads"`
	UpdatedAt              int64  `json:"updatedAt"`
}

type MediaStorageSettings struct {
	ExtraRoots []string `json:"extraRoots"`
	UpdatedAt  int64    `json:"updatedAt"`
}

type Service struct {
	db  *sql.DB
	now func() time.Time
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, now: time.Now}
}

func (s *Service) ListScheduledTasks(ctx context.Context) ([]ScheduledTask, error) {
	rows, err := s.db.QueryContext(ctx, taskSelect+" ORDER BY created_at, task_key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]ScheduledTask, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Service) ScheduledTask(ctx context.Context, key string) (ScheduledTask, error) {
	row := s.db.QueryRowContext(ctx, taskSelect+" WHERE task_key = ?", key)
	task, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ScheduledTask{}, ErrTaskNotFound
	}
	return task, err
}

func (s *Service) UpdateScheduledTask(ctx context.Context, key string, update TaskUpdate) (ScheduledTask, error) {
	if update.Enabled == nil && update.IntervalMinutes == nil {
		return ScheduledTask{}, errors.New("no scheduled task fields supplied")
	}
	if update.IntervalMinutes != nil && (*update.IntervalMinutes < MinIntervalMinutes || *update.IntervalMinutes > MaxIntervalMinutes) {
		return ScheduledTask{}, ErrInvalidInterval
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ScheduledTask{}, err
	}
	defer tx.Rollback()

	task, err := scanTask(tx.QueryRowContext(ctx, taskSelect+" WHERE task_key = ?", key))
	if errors.Is(err, sql.ErrNoRows) {
		return ScheduledTask{}, ErrTaskNotFound
	}
	if err != nil {
		return ScheduledTask{}, err
	}
	if update.Enabled != nil {
		task.Enabled = *update.Enabled
	}
	if update.IntervalMinutes != nil {
		task.IntervalMinutes = *update.IntervalMinutes
	}

	now := s.now().UTC().Unix()
	var nextRunAt any
	if task.Enabled && task.LastStatus != TaskStatusRunning {
		nextRunAt = now + int64(task.IntervalMinutes*60)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE scheduled_tasks
SET enabled = ?, interval_minutes = ?, next_run_at = ?, updated_at = ?
WHERE task_key = ?`, task.Enabled, task.IntervalMinutes, nextRunAt, now, key); err != nil {
		return ScheduledTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return ScheduledTask{}, err
	}
	return s.ScheduledTask(ctx, key)
}

func (s *Service) PrepareScheduler(ctx context.Context) error {
	now := s.now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx, `
UPDATE scheduled_tasks
SET last_status = ?, last_error = ?, last_finished_at = ?, next_run_at = NULL, updated_at = ?
WHERE last_status = ?`, TaskStatusFailed, "上次执行因服务重启而中断", now, now, TaskStatusRunning); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE scheduled_tasks
SET next_run_at = ? + interval_minutes * 60
WHERE enabled = 1 AND next_run_at IS NULL`, now)
	return err
}

func (s *Service) DueTaskKeys(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT task_key
FROM scheduled_tasks
WHERE enabled = 1 AND last_status != ? AND next_run_at IS NOT NULL AND next_run_at <= ?
ORDER BY next_run_at`, TaskStatusRunning, s.now().UTC().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Service) MarkTaskStarted(ctx context.Context, key string) (ScheduledTask, error) {
	now := s.now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `
UPDATE scheduled_tasks
SET last_status = ?, last_error = '', last_started_at = ?, last_finished_at = NULL,
    next_run_at = NULL, updated_at = ?
WHERE task_key = ? AND last_status != ?`, TaskStatusRunning, now, now, key, TaskStatusRunning)
	if err != nil {
		return ScheduledTask{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ScheduledTask{}, err
	}
	if affected == 0 {
		_, taskErr := s.ScheduledTask(ctx, key)
		if errors.Is(taskErr, ErrTaskNotFound) {
			return ScheduledTask{}, ErrTaskNotFound
		}
		if taskErr != nil {
			return ScheduledTask{}, taskErr
		}
		return ScheduledTask{}, ErrTaskAlreadyRunning
	}
	return s.ScheduledTask(ctx, key)
}

func (s *Service) MarkTaskFinished(ctx context.Context, key string, runErr error) error {
	status := TaskStatusCompleted
	errorMessage := ""
	if runErr != nil {
		status = TaskStatusFailed
		errorMessage = runErr.Error()
		if len(errorMessage) > 1000 {
			errorMessage = errorMessage[:1000]
		}
	}
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE scheduled_tasks
SET last_status = ?, last_error = ?, last_finished_at = ?,
    next_run_at = CASE WHEN enabled = 1 THEN ? + interval_minutes * 60 ELSE NULL END,
    updated_at = ?
WHERE task_key = ?`, status, errorMessage, now, now, now, key)
	return err
}

func (s *Service) GetNetworkSettings(ctx context.Context) (NetworkSettings, error) {
	var settings NetworkSettings
	err := s.db.QueryRowContext(ctx,
		"SELECT http_proxy, https_proxy, updated_at FROM network_settings WHERE id = 1",
	).Scan(&settings.HTTPProxy, &settings.HTTPSProxy, &settings.UpdatedAt)
	return settings, err
}

func (s *Service) UpdateNetworkSettings(ctx context.Context, httpProxy, httpsProxy string) (NetworkSettings, error) {
	httpProxy = strings.TrimSpace(httpProxy)
	httpsProxy = strings.TrimSpace(httpsProxy)
	if err := validateProxy(httpProxy); err != nil {
		return NetworkSettings{}, fmt.Errorf("HTTP proxy: %w", err)
	}
	if err := validateProxy(httpsProxy); err != nil {
		return NetworkSettings{}, fmt.Errorf("HTTPS proxy: %w", err)
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE network_settings
SET http_proxy = ?, https_proxy = ?, updated_at = ?
WHERE id = 1`, httpProxy, httpsProxy, s.now().UTC().Unix())
	if err != nil {
		return NetworkSettings{}, err
	}
	return s.GetNetworkSettings(ctx)
}

func (s *Service) GetSubscriptionSettings(ctx context.Context) (SubscriptionSettings, error) {
	var settings SubscriptionSettings
	err := s.db.QueryRowContext(ctx,
		"SELECT rss_url, updated_at FROM subscription_settings WHERE id = 1",
	).Scan(&settings.RSSURL, &settings.UpdatedAt)
	return settings, err
}

func (s *Service) UpdateSubscriptionSettings(ctx context.Context, rssURL string) (SubscriptionSettings, error) {
	rssURL = strings.TrimSpace(rssURL)
	if err := validateRSSURL(rssURL); err != nil {
		return SubscriptionSettings{}, err
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE subscription_settings
SET rss_url = ?, updated_at = ?
WHERE id = 1`, rssURL, s.now().UTC().Unix())
	if err != nil {
		return SubscriptionSettings{}, err
	}
	return s.GetSubscriptionSettings(ctx)
}

func (s *Service) GetDownloadSettings(ctx context.Context) (DownloadSettings, error) {
	var settings DownloadSettings
	err := s.db.QueryRowContext(ctx, `
SELECT host, port, username, password, max_concurrent_downloads, updated_at
FROM download_settings
WHERE id = 1`).Scan(
		&settings.Host, &settings.Port, &settings.Username, &settings.Password,
		&settings.MaxConcurrentDownloads, &settings.UpdatedAt,
	)
	return settings, err
}

func (s *Service) UpdateDownloadSettings(ctx context.Context, settings DownloadSettings) (DownloadSettings, error) {
	settings.Host = strings.TrimSpace(settings.Host)
	settings.Username = strings.TrimSpace(settings.Username)
	if err := validateDownloadSettings(settings); err != nil {
		return DownloadSettings{}, err
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE download_settings
SET host = ?, port = ?, username = ?, password = ?, max_concurrent_downloads = ?, updated_at = ?
WHERE id = 1`, settings.Host, settings.Port, settings.Username, settings.Password,
		settings.MaxConcurrentDownloads, s.now().UTC().Unix())
	if err != nil {
		return DownloadSettings{}, err
	}
	return s.GetDownloadSettings(ctx)
}

func (s *Service) GetMediaStorageSettings(ctx context.Context) (MediaStorageSettings, error) {
	var settings MediaStorageSettings
	var rawJSON string
	if err := s.db.QueryRowContext(ctx, `
SELECT extra_roots_json, updated_at
FROM media_storage_settings
WHERE id = 1`).Scan(&rawJSON, &settings.UpdatedAt); err != nil {
		return MediaStorageSettings{}, err
	}
	if err := json.Unmarshal([]byte(rawJSON), &settings.ExtraRoots); err != nil {
		settings.ExtraRoots = make([]string, 0)
	}
	if settings.ExtraRoots == nil {
		settings.ExtraRoots = make([]string, 0)
	}
	return settings, nil
}

func (s *Service) UpdateMediaStorageSettings(ctx context.Context, extraRoots []string) (MediaStorageSettings, error) {
	normalized, err := normalizeMediaStorageRoots(extraRoots)
	if err != nil {
		return MediaStorageSettings{}, err
	}
	rawJSON, err := json.Marshal(normalized)
	if err != nil {
		return MediaStorageSettings{}, err
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE media_storage_settings
SET extra_roots_json = ?, updated_at = ?
WHERE id = 1`, string(rawJSON), s.now().UTC().Unix())
	if err != nil {
		return MediaStorageSettings{}, err
	}
	return s.GetMediaStorageSettings(ctx)
}

func validateDownloadSettings(settings DownloadSettings) error {
	if settings.Host == "" || strings.ContainsAny(settings.Host, "/\\") {
		return ErrInvalidDownloadSettings
	}
	if settings.Port < 1 || settings.Port > 65535 {
		return ErrInvalidDownloadSettings
	}
	if settings.MaxConcurrentDownloads < MinConcurrentDownloads || settings.MaxConcurrentDownloads > MaxConcurrentDownloads {
		return ErrInvalidDownloadSettings
	}
	parsed, err := url.Parse("//" + settings.Host)
	if err != nil || parsed.Host == "" {
		return ErrInvalidDownloadSettings
	}
	return nil
}

func normalizeMediaStorageRoots(paths []string) ([]string, error) {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		cleaned := filepath.Clean(path)
		if !filepath.IsAbs(cleaned) {
			return nil, ErrInvalidMediaStoragePath
		}
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result, nil
}

func validateProxy(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ErrInvalidProxy
	}
	return nil
}

func validateRSSURL(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ErrInvalidRSSURL
	}
	return nil
}

const taskSelect = `
SELECT task_key, name, description, enabled, interval_minutes, last_status, last_error,
       last_started_at, last_finished_at, next_run_at, created_at, updated_at
FROM scheduled_tasks`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(row rowScanner) (ScheduledTask, error) {
	var task ScheduledTask
	var startedAt, finishedAt, nextRunAt sql.NullInt64
	err := row.Scan(
		&task.Key, &task.Name, &task.Description, &task.Enabled, &task.IntervalMinutes,
		&task.LastStatus, &task.LastError, &startedAt, &finishedAt, &nextRunAt,
		&task.CreatedAt, &task.UpdatedAt,
	)
	if err != nil {
		return ScheduledTask{}, err
	}
	task.LastStartedAt = nullableInt64(startedAt)
	task.LastFinishedAt = nullableInt64(finishedAt)
	task.NextRunAt = nullableInt64(nextRunAt)
	return task, nil
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
