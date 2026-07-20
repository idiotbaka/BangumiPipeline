package bangumi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/system"
)

const commentAvatarResponseLimit = 5 << 20

var commentAvatarRetryBackoffs = []time.Duration{
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	12 * time.Hour,
	24 * time.Hour,
}

type BangumiCommentAvatarSyncConfig struct {
	Directory      string
	UserAgent      string
	RequestTimeout time.Duration
	RequestLimiter *RequestLimiter
}

type BangumiCommentAvatarSyncResult struct {
	Due        int
	Downloaded int
	Cached     int
	NotFound   int
	Failed     int
}

type BangumiCommentAvatarStore struct {
	db     database.Executor
	logger *slog.Logger
	config BangumiCommentAvatarSyncConfig
	now    func() time.Time
}

type commentAvatarCandidate struct {
	UserID    int64
	MediumURL string
}

type commentAvatarJob struct {
	UserID    int64
	MediumURL string
	FileName  string
	Attempts  int
}

func NewBangumiCommentAvatarStore(db database.Executor, logger *slog.Logger, config BangumiCommentAvatarSyncConfig) *BangumiCommentAvatarStore {
	config.Directory = strings.TrimSpace(config.Directory)
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 10 * time.Second
	}
	return &BangumiCommentAvatarStore{db: db, logger: logger, config: config, now: time.Now}
}

// EnqueueHistorical discovers the latest medium avatar URL for every user that
// already exists in a stored comment. Rows are fully materialized before the
// write transaction starts so the worker reader is never held during writes.
func (s *BangumiCommentAvatarStore) EnqueueHistorical(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT user_id, avatar_medium_url
FROM (
    SELECT user_id, avatar_medium_url,
           ROW_NUMBER() OVER (
               PARTITION BY user_id
               ORDER BY fetched_at DESC, source_created_at DESC, comment_id DESC
           ) AS row_number
    FROM bangumi_episode_comments INDEXED BY idx_bangumi_episode_comments_user_avatar
    WHERE user_id > 0 AND avatar_medium_url != ''
)
WHERE row_number = 1
ORDER BY user_id`)
	if err != nil {
		return 0, err
	}
	candidates := make([]commentAvatarCandidate, 0)
	for rows.Next() {
		var candidate commentAvatarCandidate
		if err := rows.Scan(&candidate.UserID, &candidate.MediumURL); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := s.now().UTC().Unix()
	for _, candidate := range candidates {
		if err := upsertCommentAvatarCandidate(ctx, tx, candidate.UserID, candidate.MediumURL, now); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(candidates), nil
}

// RequeueMissingFiles is intended for the manual backfill command. The regular
// minute task avoids this full cache scan and only reads indexed due rows.
func (s *BangumiCommentAvatarStore) RequeueMissingFiles(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT user_id, file_name
FROM bangumi_comment_user_avatars
WHERE status = 'downloaded'
ORDER BY user_id`)
	if err != nil {
		return 0, err
	}
	missing := make([]int64, 0)
	for rows.Next() {
		var userID int64
		var fileName string
		if err := rows.Scan(&userID, &fileName); err != nil {
			rows.Close()
			return 0, err
		}
		if !s.avatarFileExists(userID, fileName) {
			missing = append(missing, userID)
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(missing) == 0 {
		return 0, nil
	}

	now := s.now().UTC().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, userID := range missing {
		if _, err := tx.ExecContext(ctx, `
UPDATE bangumi_comment_user_avatars
SET file_name = '', content_type = '', status = 'pending', attempts = 0,
    next_retry_at = ?, last_error = '', downloaded_at = NULL, updated_at = ?
WHERE user_id = ?`, now, now, userID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(missing), nil
}

func (s *BangumiCommentAvatarStore) RetryFailuresNow(ctx context.Context) (int64, error) {
	now := s.now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `
UPDATE bangumi_comment_user_avatars
SET status = 'pending', attempts = 0, next_retry_at = ?, last_error = '', updated_at = ?
WHERE status = 'failed'`, now, now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *BangumiCommentAvatarStore) SyncPending(ctx context.Context, network system.NetworkSettings, limit int) (BangumiCommentAvatarSyncResult, error) {
	result := BangumiCommentAvatarSyncResult{}
	if strings.TrimSpace(s.config.Directory) == "" {
		return result, nil
	}
	jobs, err := s.dueJobs(ctx, limit)
	if err != nil {
		return result, err
	}
	result.Due = len(jobs)
	if len(jobs) == 0 {
		return result, nil
	}
	return s.syncJobs(ctx, network, jobs)
}

func (s *BangumiCommentAvatarStore) syncJobs(ctx context.Context, network system.NetworkSettings, jobs []commentAvatarJob) (BangumiCommentAvatarSyncResult, error) {
	result := BangumiCommentAvatarSyncResult{Due: len(jobs)}
	client, err := newAPIClient(network, SyncerConfig{
		UserAgent: s.config.UserAgent, RequestTimeout: s.config.RequestTimeout,
	}, s.logger, s.config.RequestLimiter)
	if err != nil {
		return result, err
	}
	defer client.close()

	failures := make([]error, 0)
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return result, errors.Join(err, errors.Join(failures...))
		}
		cached := s.avatarFileExists(job.UserID, job.FileName)
		if !cached && strings.TrimSpace(job.FileName) == "" {
			// A changed URL invalidates the old file even though the stable cache
			// key remains the same user ID.
			_ = os.Remove(filepath.Join(s.config.Directory, commentAvatarFileName(job.UserID)))
		}
		download, downloadErr := s.downloadMediumAvatar(ctx, client, job)
		switch download.Status {
		case imageStatusDownloaded:
			if err := s.markDownloaded(ctx, job, download.Path); err != nil {
				result.Failed++
				failures = append(failures, fmt.Errorf("用户 #%d 写入下载状态: %w", job.UserID, err))
				continue
			}
			if cached {
				result.Cached++
			} else {
				result.Downloaded++
			}
		case imageStatusNotFound:
			if err := s.markNotFound(ctx, job); err != nil {
				result.Failed++
				failures = append(failures, fmt.Errorf("用户 #%d 写入不存在状态: %w", job.UserID, err))
				continue
			}
			result.NotFound++
		default:
			if downloadErr == nil {
				downloadErr = errors.New("头像下载失败")
			}
			if err := s.markFailed(ctx, job, downloadErr); err != nil {
				downloadErr = errors.Join(downloadErr, err)
			}
			result.Failed++
			failures = append(failures, fmt.Errorf("用户 #%d: %w", job.UserID, downloadErr))
		}
	}
	return result, errors.Join(failures...)
}

func (s *BangumiCommentAvatarStore) dueJobs(ctx context.Context, limit int) ([]commentAvatarJob, error) {
	now := s.now().UTC().Unix()
	query := `
SELECT user_id, medium_url, file_name, attempts
FROM bangumi_comment_user_avatars INDEXED BY idx_bangumi_comment_user_avatars_due
WHERE status IN ('pending', 'failed')
  AND (next_retry_at IS NULL OR next_retry_at <= ?)
ORDER BY status, next_retry_at, user_id`
	arguments := []any{now}
	if limit > 0 {
		query += " LIMIT ?"
		arguments = append(arguments, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]commentAvatarJob, 0)
	for rows.Next() {
		var job commentAvatarJob
		if err := rows.Scan(&job.UserID, &job.MediumURL, &job.FileName, &job.Attempts); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *BangumiCommentAvatarStore) downloadMediumAvatar(ctx context.Context, client *apiClient, job commentAvatarJob) (imageDownload, error) {
	if strings.TrimSpace(job.MediumURL) == "" {
		return imageDownload{Status: imageStatusNotFound}, nil
	}
	if err := os.MkdirAll(s.config.Directory, 0o755); err != nil {
		return imageDownload{Status: imageStatusFailed}, fmt.Errorf("创建评论头像目录: %w", err)
	}
	destination := filepath.Join(s.config.Directory, commentAvatarFileName(job.UserID))
	if info, err := os.Stat(destination); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
		return imageDownload{Path: destination, Status: imageStatusDownloaded}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, job.MediumURL, nil)
	if err != nil {
		return imageDownload{Status: imageStatusFailed}, err
	}
	req.Header.Set("Accept", "image/jpeg")
	req.Header.Set("User-Agent", client.userAgent)
	response, err := client.httpClient.Do(req)
	if err != nil {
		return imageDownload{Status: imageStatusFailed}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return imageDownload{Status: imageStatusNotFound}, nil
	}
	if response.StatusCode != http.StatusOK {
		return imageDownload{Status: imageStatusFailed}, fmt.Errorf("HTTP %d", response.StatusCode)
	}

	temporary, err := os.CreateTemp(s.config.Directory, ".avatar-*.jpg")
	if err != nil {
		return imageDownload{Status: imageStatusFailed}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	written, copyErr := io.Copy(temporary, io.LimitReader(response.Body, commentAvatarResponseLimit+1))
	if copyErr != nil {
		temporary.Close()
		return imageDownload{Status: imageStatusFailed}, copyErr
	}
	if written == 0 || written > commentAvatarResponseLimit {
		temporary.Close()
		if written == 0 {
			return imageDownload{Status: imageStatusFailed}, errors.New("头像内容为空")
		}
		return imageDownload{Status: imageStatusFailed}, fmt.Errorf("头像超过 %d MiB 限制", commentAvatarResponseLimit>>20)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		temporary.Close()
		return imageDownload{Status: imageStatusFailed}, err
	}
	if _, err := jpeg.DecodeConfig(temporary); err != nil {
		temporary.Close()
		return imageDownload{Status: imageStatusFailed}, fmt.Errorf("头像不是有效 JPEG: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return imageDownload{Status: imageStatusFailed}, err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return imageDownload{Status: imageStatusFailed}, err
	}
	s.logger.Info("Bangumi 评论用户头像下载成功", "source", "bangumi",
		"user_id", job.UserID, "bytes", written, "path", destination)
	return imageDownload{Path: destination, Status: imageStatusDownloaded}, nil
}

func (s *BangumiCommentAvatarStore) markDownloaded(ctx context.Context, job commentAvatarJob, path string) error {
	fileName := filepath.Base(path)
	if fileName != commentAvatarFileName(job.UserID) {
		return fmt.Errorf("头像缓存文件名无效: %q", fileName)
	}
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE bangumi_comment_user_avatars
SET file_name = ?, content_type = 'image/jpeg', status = 'downloaded', attempts = 0,
    next_retry_at = NULL, last_error = '', downloaded_at = ?, updated_at = ?
WHERE user_id = ? AND medium_url = ?`, fileName, now, now, job.UserID, job.MediumURL)
	return err
}

func (s *BangumiCommentAvatarStore) markNotFound(ctx context.Context, job commentAvatarJob) error {
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE bangumi_comment_user_avatars
SET file_name = '', content_type = '', status = 'not_found', attempts = attempts + 1,
    next_retry_at = NULL, last_error = 'HTTP 404', downloaded_at = NULL, updated_at = ?
WHERE user_id = ? AND medium_url = ?`, now, job.UserID, job.MediumURL)
	return err
}

func (s *BangumiCommentAvatarStore) markFailed(ctx context.Context, job commentAvatarJob, runErr error) error {
	attempts := job.Attempts + 1
	backoffIndex := attempts - 1
	if backoffIndex >= len(commentAvatarRetryBackoffs) {
		backoffIndex = len(commentAvatarRetryBackoffs) - 1
	}
	now := s.now().UTC().Unix()
	message := runErr.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE bangumi_comment_user_avatars
SET status = 'failed', attempts = ?, next_retry_at = ?, last_error = ?, updated_at = ?
WHERE user_id = ? AND medium_url = ?`, attempts,
		now+int64(commentAvatarRetryBackoffs[backoffIndex]/time.Second), message, now,
		job.UserID, job.MediumURL)
	return err
}

func (s *BangumiCommentAvatarStore) avatarFileExists(userID int64, fileName string) bool {
	if strings.TrimSpace(s.config.Directory) == "" || fileName != commentAvatarFileName(userID) {
		return false
	}
	info, err := os.Stat(filepath.Join(s.config.Directory, fileName))
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func commentAvatarFileName(userID int64) string {
	return strconv.FormatInt(userID, 10) + imageOutputFileType
}

type commentAvatarExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func upsertCommentAvatarCandidate(ctx context.Context, exec commentAvatarExecutor, userID int64, mediumURL string, now int64) error {
	mediumURL = strings.TrimSpace(mediumURL)
	if userID <= 0 || mediumURL == "" {
		return nil
	}
	_, err := exec.ExecContext(ctx, `
INSERT INTO bangumi_comment_user_avatars(
    user_id, medium_url, status, next_retry_at, created_at, updated_at
) VALUES (?, ?, 'pending', ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
    file_name = CASE WHEN medium_url != excluded.medium_url THEN '' ELSE file_name END,
    content_type = CASE WHEN medium_url != excluded.medium_url THEN '' ELSE content_type END,
    status = CASE WHEN medium_url != excluded.medium_url THEN 'pending' ELSE status END,
    attempts = CASE WHEN medium_url != excluded.medium_url THEN 0 ELSE attempts END,
    next_retry_at = CASE WHEN medium_url != excluded.medium_url THEN excluded.next_retry_at ELSE next_retry_at END,
    last_error = CASE WHEN medium_url != excluded.medium_url THEN '' ELSE last_error END,
    downloaded_at = CASE WHEN medium_url != excluded.medium_url THEN NULL ELSE downloaded_at END,
    medium_url = excluded.medium_url,
    updated_at = excluded.updated_at`, userID, mediumURL, now, now, now)
	return err
}
