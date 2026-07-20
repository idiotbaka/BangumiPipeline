package bangumi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	EpisodeCommentTaskKey = "sync-bangumi-episode-comments"

	defaultCommentAPIBaseURL   = "https://next.bgm.tv"
	defaultCommentBatchSize    = 10
	maxCommentNestingDepth     = 20
	maxCommentsPerEpisode      = 50_000
	commentBackfillInsertLimit = defaultCommentBatchSize
	commentSmileRetryDelay     = time.Hour

	mediaEpisodeCommentJoin = `
JOIN anime_episodes ae ON ae.bangumi_id = mj.bangumi_id AND ae.episode_id = (
    SELECT episode.episode_id
    FROM anime_episodes episode
    WHERE episode.bangumi_id = mj.bangumi_id
      AND TRIM(mj.episode_number) != ''
      AND ABS(
          CASE
              WHEN LOWER(COALESCE(NULLIF(mj.episode_type, ''), 'episode')) = 'episode'
                  THEN CASE WHEN episode.type = 0 AND episode.ep_number > 0 THEN episode.ep_number ELSE episode.sort_number END
              ELSE episode.sort_number
          END - CAST(mj.episode_number AS REAL)
      ) < 0.0000001
      AND (
          (LOWER(COALESCE(NULLIF(mj.episode_type, ''), 'episode')) = 'episode' AND episode.type = 0)
          OR
          (LOWER(COALESCE(NULLIF(mj.episode_type, ''), 'episode')) != 'episode' AND episode.type != 0)
      )
    ORDER BY episode.type, episode.episode_id
    LIMIT 1
)`
)

var (
	commentMilestoneOffsets = []time.Duration{
		0,
		2 * time.Hour,
		12 * time.Hour,
		24 * time.Hour,
		3 * 24 * time.Hour,
		7 * 24 * time.Hour,
	}
	commentRetryBackoffs = []time.Duration{
		5 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
		12 * time.Hour,
		24 * time.Hour,
	}
)

type EpisodeCommentSyncerConfig struct {
	APIBaseURL          string
	UserAgent           string
	APIInterval         time.Duration
	RequestTimeout      time.Duration
	BatchSize           int
	RequestLimiter      *RequestLimiter
	SmileDir            string
	SmileBangumiBaseURL string
	SmileLainBaseURL    string
}

// EpisodeCommentSyncer persists complete snapshots from next.bgm.tv and owns
// the per-episode milestone schedule. Media completion only enqueues work; all
// network failures remain isolated from the media pipeline.
type EpisodeCommentSyncer struct {
	db       *sql.DB
	settings SettingsProvider
	logger   *slog.Logger
	config   EpisodeCommentSyncerConfig
	limiter  *RequestLimiter
	smiles   *BangumiSmileStore
	now      func() time.Time

	smileAssetsReady       atomic.Bool
	nextSmileCheckAt       atomic.Int64
	historicalBackfillDone atomic.Bool
}

type episodeCommentAPI struct {
	ID        int64               `json:"id"`
	MainID    int64               `json:"mainID"`
	CreatorID int64               `json:"creatorID"`
	RelatedID int64               `json:"relatedID"`
	CreatedAt int64               `json:"createdAt"`
	Content   string              `json:"content"`
	State     int                 `json:"state"`
	Replies   []json.RawMessage   `json:"replies"`
	User      *episodeCommentUser `json:"user"`
	Reactions json.RawMessage     `json:"reactions"`
}

type episodeCommentUser struct {
	ID       int64                `json:"id"`
	Username string               `json:"username"`
	Nickname string               `json:"nickname"`
	Avatar   episodeCommentAvatar `json:"avatar"`
	Group    int                  `json:"group"`
	Sign     string               `json:"sign"`
	JoinedAt int64                `json:"joinedAt"`
}

type episodeCommentAvatar struct {
	Small  string `json:"small"`
	Medium string `json:"medium"`
	Large  string `json:"large"`
}

type storedEpisodeComment struct {
	BangumiID       int64
	EpisodeID       int64
	CommentID       int64
	ParentCommentID int64
	MainID          int64
	CreatorID       int64
	RelatedID       int64
	SourceCreatedAt int64
	Content         string
	State           int
	Depth           int
	SortOrder       int
	UserID          int64
	Username        string
	Nickname        string
	AvatarSmallURL  string
	AvatarMediumURL string
	AvatarLargeURL  string
	UserGroup       int
	UserSign        string
	UserJoinedAt    int64
	ReactionsJSON   string
	RawJSON         string
}

type episodeCommentSyncJob struct {
	BangumiID  int64
	EpisodeID  int64
	AnchorAt   int64
	NextStage  int
	Attempts   int
	IsBackfill bool
}

type completedMediaCommentCandidate struct {
	MediaJobID int64
	BangumiID  int64
	EpisodeID  int64
	AnchorAt   int64
}

func NewEpisodeCommentSyncer(db *sql.DB, settings SettingsProvider, logger *slog.Logger, config EpisodeCommentSyncerConfig) *EpisodeCommentSyncer {
	config.APIBaseURL = strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/")
	if config.APIBaseURL == "" {
		config.APIBaseURL = defaultCommentAPIBaseURL
	}
	if config.APIInterval <= 0 {
		config.APIInterval = 2 * time.Second
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 20 * time.Second
	}
	if config.BatchSize <= 0 {
		config.BatchSize = defaultCommentBatchSize
	}
	limiter := config.RequestLimiter
	if limiter == nil {
		limiter = NewRequestLimiter(config.APIInterval)
	}
	syncer := &EpisodeCommentSyncer{
		db: db, settings: settings, logger: logger, config: config,
		limiter: limiter, now: time.Now,
	}
	if strings.TrimSpace(config.SmileDir) != "" {
		syncer.smiles = NewBangumiSmileStore(logger, BangumiSmileSyncConfig{
			Directory: config.SmileDir, BangumiBaseURL: config.SmileBangumiBaseURL,
			LainBaseURL: config.SmileLainBaseURL, UserAgent: config.UserAgent,
			RequestTimeout: config.RequestTimeout,
		})
	}
	return syncer
}

func (s *EpisodeCommentSyncer) Execute(ctx context.Context) error {
	smileErr := s.ensureSmileAssets(ctx)
	backfilled, err := s.enqueueHistoricalEpisodes(ctx)
	if err != nil {
		return errors.Join(smileErr, fmt.Errorf("发现待补抓历史剧集评论: %w", err))
	}
	jobs, err := s.dueCommentSyncJobs(ctx, s.config.BatchSize)
	if err != nil {
		return errors.Join(smileErr, fmt.Errorf("查询待同步剧集评论: %w", err))
	}
	if len(jobs) == 0 {
		s.logger.Info("Bangumi 剧集吐槽同步完成：没有到期任务", "source", "bangumi", "backfilled", backfilled)
		return smileErr
	}

	client, err := s.newCommentAPIClient(ctx)
	if err != nil {
		return errors.Join(smileErr, err)
	}
	defer client.close()

	succeeded := 0
	notFound := 0
	failures := make([]string, 0)
	if smileErr != nil {
		failures = append(failures, smileErr.Error())
	}
	for _, job := range jobs {
		outcome, runErr := s.syncEpisodeComments(ctx, client, job)
		switch outcome {
		case "completed":
			succeeded++
		case "not_found":
			notFound++
		}
		if runErr != nil {
			failures = append(failures, fmt.Sprintf("episode #%d: %v", job.EpisodeID, runErr))
		}
	}
	s.logger.Info("Bangumi 剧集吐槽同步完成", "source", "bangumi",
		"due", len(jobs), "succeeded", succeeded, "not_found", notFound,
		"failed", len(failures), "backfilled", backfilled)
	if len(failures) > 0 {
		shown := failures
		if len(shown) > 3 {
			shown = shown[:3]
		}
		return fmt.Errorf("同步存在 %d 个错误：%s", len(failures), strings.Join(shown, "；"))
	}
	return nil
}

func (s *EpisodeCommentSyncer) ensureSmileAssets(ctx context.Context) error {
	if s.smiles == nil || s.smileAssetsReady.Load() {
		return nil
	}
	if s.smiles.HasCompleteManifest() {
		s.smileAssetsReady.Store(true)
		return nil
	}
	now := s.now().UTC().Unix()
	if nextCheckAt := s.nextSmileCheckAt.Load(); nextCheckAt > now {
		return nil
	}
	network, err := s.settings.GetNetworkSettings(ctx)
	if err != nil {
		s.nextSmileCheckAt.Store(now + int64(commentSmileRetryDelay/time.Second))
		return fmt.Errorf("读取 Bangumi 表情下载代理设置: %w", err)
	}
	result, err := s.smiles.Ensure(ctx, network)
	if err != nil {
		s.nextSmileCheckAt.Store(now + int64(commentSmileRetryDelay/time.Second))
		s.logger.Warn("Bangumi 评论表情资源同步未完成，评论抓取将继续", "source", "bangumi",
			"available", result.Available, "expected", result.Expected, "error", err)
		return fmt.Errorf("同步 Bangumi 评论表情资源: %w", err)
	}
	if result.Complete {
		s.smileAssetsReady.Store(true)
		s.nextSmileCheckAt.Store(0)
	}
	return nil
}

// EnqueueMediaCompleted implements media.EpisodeCommentEnqueuer. Existing
// completed schedules are intentionally not restarted when a video is replaced.
func (s *EpisodeCommentSyncer) EnqueueMediaCompleted(ctx context.Context, mediaJobID, bangumiID int64) error {
	if mediaJobID <= 0 || bangumiID <= 0 {
		return errors.New("媒体任务或 Bangumi ID 无效")
	}
	var candidate completedMediaCommentCandidate
	err := s.db.QueryRowContext(ctx, `
SELECT mj.id, mj.bangumi_id, ae.episode_id,
       COALESCE(mj.completed_at, mj.updated_at, mj.created_at)
FROM media_jobs mj
`+mediaEpisodeCommentJoin+`
WHERE mj.id = ? AND mj.bangumi_id = ?
  AND mj.status = 'completed' AND mj.output_path != ''`, mediaJobID, bangumiID).Scan(
		&candidate.MediaJobID, &candidate.BangumiID, &candidate.EpisodeID, &candidate.AnchorAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		err = fmt.Errorf("已完成媒体任务 #%d 尚未匹配到 Bangumi 剧集 ID", mediaJobID)
		if resetErr := s.markHistoricalBackfillPending(ctx); resetErr != nil {
			return errors.Join(err, resetErr)
		}
		return err
	}
	if err != nil {
		if resetErr := s.markHistoricalBackfillPending(ctx); resetErr != nil {
			return errors.Join(err, resetErr)
		}
		return err
	}
	_, err = s.enqueueEpisodeCommentSync(ctx, candidate, false)
	if err != nil {
		if resetErr := s.markHistoricalBackfillPending(ctx); resetErr != nil {
			return errors.Join(err, resetErr)
		}
	}
	return err
}

func (s *EpisodeCommentSyncer) newCommentAPIClient(ctx context.Context) (*apiClient, error) {
	settings, err := s.settings.GetNetworkSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取代理设置: %w", err)
	}
	return newAPIClient(settings, SyncerConfig{
		APIBaseURL: s.config.APIBaseURL, UserAgent: s.config.UserAgent,
		RequestTimeout: s.config.RequestTimeout,
	}, s.logger, s.limiter)
}

func (s *EpisodeCommentSyncer) syncEpisodeComments(ctx context.Context, client *apiClient, job episodeCommentSyncJob) (string, error) {
	path := fmt.Sprintf("/p1/episodes/%d/comments", job.EpisodeID)
	var payload []json.RawMessage
	if err := client.getJSON(ctx, path, &payload); err != nil {
		var httpErr *apiHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			if markErr := s.markCommentSyncNotFound(ctx, job, err); markErr != nil {
				return "", errors.Join(err, markErr)
			}
			return "not_found", nil
		}
		if markErr := s.recordCommentSyncFailure(ctx, job, err); markErr != nil {
			return "", errors.Join(err, markErr)
		}
		return "", err
	}
	if payload == nil {
		err := fmt.Errorf("episode #%d 评论响应不是 JSON 数组", job.EpisodeID)
		if markErr := s.recordCommentSyncFailure(ctx, job, err); markErr != nil {
			return "", errors.Join(err, markErr)
		}
		return "", err
	}
	comments, err := decodeEpisodeComments(job.BangumiID, job.EpisodeID, payload)
	if err != nil {
		if markErr := s.recordCommentSyncFailure(ctx, job, err); markErr != nil {
			return "", errors.Join(err, markErr)
		}
		return "", err
	}
	if err := s.saveEpisodeCommentSnapshot(ctx, job, comments, len(payload)); err != nil {
		if markErr := s.recordCommentSyncFailure(ctx, job, err); markErr != nil {
			return "", errors.Join(err, markErr)
		}
		return "", err
	}
	s.logger.Info("Bangumi 剧集吐槽抓取成功", "source", "bangumi",
		"bangumi_id", job.BangumiID, "episode_id", job.EpisodeID,
		"top_level_comments", len(payload), "stored_comments", len(comments), "stage", job.NextStage)
	return "completed", nil
}

func decodeEpisodeComments(bangumiID, episodeID int64, payload []json.RawMessage) ([]storedEpisodeComment, error) {
	result := make([]storedEpisodeComment, 0, len(payload))
	seen := make(map[int64]struct{})
	var walk func(json.RawMessage, int64, int) error
	walk = func(raw json.RawMessage, nestedParentID int64, depth int) error {
		if depth > maxCommentNestingDepth {
			return fmt.Errorf("episode #%d 评论嵌套超过 %d 层", episodeID, maxCommentNestingDepth)
		}
		if len(result) >= maxCommentsPerEpisode {
			return fmt.Errorf("episode #%d 评论数量超过 %d", episodeID, maxCommentsPerEpisode)
		}
		var source episodeCommentAPI
		if err := json.Unmarshal(raw, &source); err != nil {
			return fmt.Errorf("episode #%d 评论 JSON 无效: %w", episodeID, err)
		}
		if source.ID <= 0 {
			return fmt.Errorf("episode #%d 评论缺少有效 id", episodeID)
		}
		if source.MainID != episodeID {
			return fmt.Errorf("episode #%d 评论 #%d mainID=%d 不匹配", episodeID, source.ID, source.MainID)
		}
		if _, exists := seen[source.ID]; exists {
			return fmt.Errorf("episode #%d 评论 id #%d 重复", episodeID, source.ID)
		}
		seen[source.ID] = struct{}{}

		parentID := source.RelatedID
		if parentID == 0 {
			parentID = nestedParentID
		}
		reactionsJSON := strings.TrimSpace(string(source.Reactions))
		if reactionsJSON == "" || reactionsJSON == "null" {
			reactionsJSON = "[]"
		}
		stored := storedEpisodeComment{
			BangumiID: bangumiID, EpisodeID: episodeID, CommentID: source.ID,
			ParentCommentID: parentID, MainID: source.MainID, CreatorID: source.CreatorID,
			RelatedID: source.RelatedID, SourceCreatedAt: source.CreatedAt,
			Content: source.Content, State: source.State, Depth: depth, SortOrder: len(result),
			ReactionsJSON: reactionsJSON, RawJSON: string(raw),
		}
		if source.User != nil {
			stored.UserID = source.User.ID
			stored.Username = source.User.Username
			stored.Nickname = source.User.Nickname
			stored.AvatarSmallURL = source.User.Avatar.Small
			stored.AvatarMediumURL = source.User.Avatar.Medium
			stored.AvatarLargeURL = source.User.Avatar.Large
			stored.UserGroup = source.User.Group
			stored.UserSign = source.User.Sign
			stored.UserJoinedAt = source.User.JoinedAt
		}
		result = append(result, stored)
		for _, reply := range source.Replies {
			if err := walk(reply, source.ID, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, raw := range payload {
		if err := walk(raw, 0, 0); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *EpisodeCommentSyncer) saveEpisodeCommentSnapshot(ctx context.Context, job episodeCommentSyncJob, comments []storedEpisodeComment, topLevelCount int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM bangumi_episode_comments WHERE episode_id = ?", job.EpisodeID); err != nil {
		return err
	}
	now := s.now().UTC().Unix()
	for _, comment := range comments {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO bangumi_episode_comments(
    bangumi_id, episode_id, comment_id, parent_comment_id, main_id, creator_id, related_id,
    source_created_at, content, state, depth, sort_order,
    user_id, username, nickname, avatar_small_url, avatar_medium_url, avatar_large_url,
    user_group, user_sign, user_joined_at, reactions_json, raw_json, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			comment.BangumiID, comment.EpisodeID, comment.CommentID, comment.ParentCommentID,
			comment.MainID, comment.CreatorID, comment.RelatedID, comment.SourceCreatedAt,
			comment.Content, comment.State, comment.Depth, comment.SortOrder,
			comment.UserID, comment.Username, comment.Nickname,
			comment.AvatarSmallURL, comment.AvatarMediumURL, comment.AvatarLargeURL,
			comment.UserGroup, comment.UserSign, comment.UserJoinedAt,
			comment.ReactionsJSON, comment.RawJSON, now,
		); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE anime_episodes SET comment_count = ?, updated_at = ?
WHERE bangumi_id = ? AND episode_id = ?`, topLevelCount, now, job.BangumiID, job.EpisodeID); err != nil {
		return err
	}
	nextStage, nextFetchAt, complete := nextEpisodeCommentMilestone(job.AnchorAt, job.NextStage, now)
	status := "pending"
	var completedAt any
	if complete {
		status = "completed"
		completedAt = now
	}
	result, err := tx.ExecContext(ctx, `
UPDATE bangumi_episode_comment_sync
SET status = ?, next_stage = ?, next_fetch_at = ?, last_fetched_at = ?,
    last_comment_count = ?, attempts = 0, last_error = '', completed_at = ?, updated_at = ?
WHERE episode_id = ?`, status, nextStage, nullableUnix(nextFetchAt), now,
		topLevelCount, completedAt, now, job.EpisodeID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("episode #%d 评论同步状态不存在", job.EpisodeID)
	}
	return tx.Commit()
}

func nextEpisodeCommentMilestone(anchorAt int64, currentStage int, now int64) (int, *int64, bool) {
	for stage := currentStage + 1; stage < len(commentMilestoneOffsets); stage++ {
		at := anchorAt + int64(commentMilestoneOffsets[stage]/time.Second)
		if at > now {
			return stage, &at, false
		}
	}
	return len(commentMilestoneOffsets), nil, true
}

func nullableUnix(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func (s *EpisodeCommentSyncer) recordCommentSyncFailure(ctx context.Context, job episodeCommentSyncJob, runErr error) error {
	attempts := job.Attempts + 1
	backoffIndex := attempts - 1
	if backoffIndex >= len(commentRetryBackoffs) {
		backoffIndex = len(commentRetryBackoffs) - 1
	}
	now := s.now().UTC().Unix()
	nextFetchAt := now + int64(commentRetryBackoffs[backoffIndex]/time.Second)
	message := runErr.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE bangumi_episode_comment_sync
SET status = 'pending', attempts = ?, last_error = ?, next_fetch_at = ?, updated_at = ?
WHERE episode_id = ?`, attempts, message, nextFetchAt, now, job.EpisodeID)
	return err
}

func (s *EpisodeCommentSyncer) markCommentSyncNotFound(ctx context.Context, job episodeCommentSyncJob, runErr error) error {
	now := s.now().UTC().Unix()
	message := runErr.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE bangumi_episode_comment_sync
SET status = 'not_found', next_stage = ?, next_fetch_at = NULL,
    last_error = ?, completed_at = ?, updated_at = ?
WHERE episode_id = ?`, len(commentMilestoneOffsets), message, now, now, job.EpisodeID)
	return err
}

func (s *EpisodeCommentSyncer) dueCommentSyncJobs(ctx context.Context, limit int) ([]episodeCommentSyncJob, error) {
	if limit < 1 {
		return []episodeCommentSyncJob{}, nil
	}
	now := s.now().UTC().Unix()
	jobs, err := s.dueCommentSyncJobsBySource(ctx, now, false, limit)
	if err != nil {
		return nil, err
	}
	if len(jobs) < limit {
		backfill, err := s.dueCommentSyncJobsBySource(ctx, now, true, limit-len(jobs))
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, backfill...)
	}
	return jobs, nil
}

func (s *EpisodeCommentSyncer) dueCommentSyncJobsBySource(ctx context.Context, now int64, backfill bool, limit int) ([]episodeCommentSyncJob, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT bangumi_id, episode_id, anchor_at, next_stage, attempts, is_backfill
FROM bangumi_episode_comment_sync INDEXED BY idx_bangumi_episode_comment_sync_ready
WHERE status = 'pending' AND next_fetch_at IS NOT NULL AND next_fetch_at <= ? AND is_backfill = ?
ORDER BY next_fetch_at, episode_id
LIMIT ?`, now, backfill, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]episodeCommentSyncJob, 0)
	for rows.Next() {
		var job episodeCommentSyncJob
		if err := rows.Scan(&job.BangumiID, &job.EpisodeID, &job.AnchorAt, &job.NextStage, &job.Attempts, &job.IsBackfill); err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

func (s *EpisodeCommentSyncer) enqueueHistoricalEpisodes(ctx context.Context) (int, error) {
	completed, err := s.historicalBackfillCompleted(ctx)
	if err != nil {
		return 0, err
	}
	if completed {
		return 0, nil
	}
	now := s.now().UTC().Unix()
	rows, err := s.db.QueryContext(ctx, `
SELECT mj.id, mj.bangumi_id, ae.episode_id,
       COALESCE(mj.completed_at, mj.updated_at, mj.created_at)
FROM media_jobs mj INDEXED BY idx_media_jobs_comment_backfill
`+mediaEpisodeCommentJoin+`
WHERE mj.status = 'completed' AND mj.output_path != '' AND mj.comment_sync_enqueued = 0
ORDER BY mj.id DESC
LIMIT ?`, commentBackfillInsertLimit)
	if err != nil {
		return 0, err
	}
	candidates := make([]completedMediaCommentCandidate, 0)
	for rows.Next() {
		var candidate completedMediaCommentCandidate
		if err := rows.Scan(&candidate.MediaJobID, &candidate.BangumiID, &candidate.EpisodeID, &candidate.AnchorAt); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	inserted := 0
	for _, candidate := range candidates {
		created, err := s.enqueueEpisodeCommentSync(ctx, candidate, true)
		if err != nil {
			return inserted, err
		}
		if created {
			inserted++
		}
	}
	if len(candidates) < commentBackfillInsertLimit {
		hasUnqueuedMedia, err := s.hasUnqueuedCompletedMedia(ctx)
		if err != nil {
			return inserted, err
		}
		if !hasUnqueuedMedia {
			if _, err := s.db.ExecContext(ctx, `
UPDATE bangumi_episode_comment_task_state
SET historical_backfill_completed = 1, updated_at = ?
WHERE id = 1`, now); err != nil {
				return inserted, err
			}
			s.historicalBackfillDone.Store(true)
		}
	}
	return inserted, nil
}

func (s *EpisodeCommentSyncer) historicalBackfillCompleted(ctx context.Context) (bool, error) {
	if s.historicalBackfillDone.Load() {
		return true, nil
	}
	var completed bool
	if err := s.db.QueryRowContext(ctx, `
SELECT historical_backfill_completed
FROM bangumi_episode_comment_task_state
WHERE id = 1`).Scan(&completed); err != nil {
		return false, err
	}
	if completed {
		hasUnqueuedMedia, err := s.hasUnqueuedCompletedMedia(ctx)
		if err != nil {
			return false, err
		}
		if hasUnqueuedMedia {
			if err := s.markHistoricalBackfillPending(ctx); err != nil {
				return false, err
			}
			return false, nil
		}
		s.historicalBackfillDone.Store(true)
	}
	return completed, nil
}

func (s *EpisodeCommentSyncer) hasUnqueuedCompletedMedia(ctx context.Context) (bool, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM media_jobs INDEXED BY idx_media_jobs_comment_backfill
    WHERE status = 'completed' AND output_path != '' AND comment_sync_enqueued = 0
    LIMIT 1
)`).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *EpisodeCommentSyncer) markHistoricalBackfillPending(ctx context.Context) error {
	s.historicalBackfillDone.Store(false)
	_, err := s.db.ExecContext(ctx, `
UPDATE bangumi_episode_comment_task_state
SET historical_backfill_completed = 0, updated_at = ?
WHERE id = 1`, s.now().UTC().Unix())
	if err != nil {
		return fmt.Errorf("重新启用历史剧集评论兜底扫描: %w", err)
	}
	return nil
}

func (s *EpisodeCommentSyncer) enqueueEpisodeCommentSync(ctx context.Context, candidate completedMediaCommentCandidate, backfill bool) (bool, error) {
	if candidate.MediaJobID <= 0 || candidate.BangumiID <= 0 || candidate.EpisodeID <= 0 {
		return false, errors.New("评论同步候选缺少有效标识")
	}
	now := s.now().UTC().Unix()
	anchorAt := candidate.AnchorAt
	if anchorAt <= 0 {
		anchorAt = now
	}
	created := false
	if backfill {
		result, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO bangumi_episode_comment_sync(
    episode_id, bangumi_id, anchor_media_job_id, anchor_at, status,
    next_stage, next_fetch_at, is_backfill, created_at, updated_at
) VALUES (?, ?, ?, ?, 'pending', 0, ?, 1, ?, ?)`,
			candidate.EpisodeID, candidate.BangumiID, candidate.MediaJobID, anchorAt, now, now, now)
		if err != nil {
			return false, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		created = affected > 0
	} else {
		result, err := s.db.ExecContext(ctx, `
INSERT INTO bangumi_episode_comment_sync(
    episode_id, bangumi_id, anchor_media_job_id, anchor_at, status,
    next_stage, next_fetch_at, is_backfill, created_at, updated_at
) VALUES (?, ?, ?, ?, 'pending', 0, ?, 0, ?, ?)
ON CONFLICT(episode_id) DO UPDATE SET
    anchor_media_job_id = COALESCE(bangumi_episode_comment_sync.anchor_media_job_id, excluded.anchor_media_job_id),
    is_backfill = 0,
    updated_at = excluded.updated_at`,
			candidate.EpisodeID, candidate.BangumiID, candidate.MediaJobID, anchorAt, now, now, now)
		if err != nil {
			return false, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		created = affected > 0
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET comment_sync_enqueued = 1
WHERE id = ? AND bangumi_id = ?`, candidate.MediaJobID, candidate.BangumiID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, fmt.Errorf("媒体任务 #%d 不存在，无法记录评论入队状态", candidate.MediaJobID)
	}
	return created, nil
}
