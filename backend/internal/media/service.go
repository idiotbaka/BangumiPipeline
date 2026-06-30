package media

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"bangumipipeline.local/server/internal/imageutil"
	"bangumipipeline.local/server/internal/subscription"
)

const (
	TaskKey = "process-downloaded-media"

	StatusPending     = "pending"
	StatusTranscoding = "transcoding"
	StatusCompleted   = "completed"
	StatusFailed      = "failed"

	downloadStatusCompleted = "completed"

	CoverStatusPending   = "pending"
	CoverStatusCompleted = "completed"
	CoverStatusFailed    = "failed"
)

var (
	ErrInvalidStatus    = errors.New("invalid media status")
	ErrMediaJobNotFound = errors.New("media job not found")
	ErrRetryNotAllowed  = errors.New("media job retry not allowed")
	ErrAnimeNotFound    = errors.New("anime not found")

	ErrInvalidStorageRoot    = errors.New("invalid media storage root")
	ErrStorageMoveInProgress = errors.New("media storage move is in progress")
	ErrAnimeTranscoding      = errors.New("anime has transcoding media jobs")
	ErrStorageTargetConflict = errors.New("media storage target already exists")
	ErrInvalidEpisodeTarget  = errors.New("invalid episode target")
)

type Config struct {
	MediaDir          string
	FFmpegPath        string
	FFprobePath       string
	DownloadCleaner   DownloadCleaner
	MetadataRefresher MetadataRefresher
}

type DownloadCleaner interface {
	CleanupCompletedQBitTask(context.Context, int64) error
}

type MetadataRefresher interface {
	RefreshSubject(context.Context, int64) error
}

type Service struct {
	db                *sql.DB
	logger            *slog.Logger
	mediaDir          string
	ffmpegPath        string
	ffprobePath       string
	cleaner           DownloadCleaner
	metadataRefresher MetadataRefresher
	now               func() time.Time
	storageMu         sync.Mutex
}

type JobPage struct {
	Items    []Job `json:"items"`
	Total    int   `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

type Job struct {
	ID                   int64   `json:"id"`
	DownloadJobID        int64   `json:"downloadJobId"`
	SubscriptionItemID   int64   `json:"subscriptionItemId"`
	Title                string  `json:"title"`
	BangumiID            int64   `json:"bangumiId"`
	AnimeName            string  `json:"animeName"`
	SeasonNumber         int     `json:"seasonNumber"`
	EpisodeType          string  `json:"episodeType"`
	EpisodeNumber        string  `json:"episodeNumber"`
	Status               string  `json:"status"`
	SourceFile           string  `json:"sourceFile"`
	SubtitleFile         string  `json:"subtitleFile"`
	OutputFile           string  `json:"outputFile"`
	CoverFile            string  `json:"coverFile"`
	CoverStatus          string  `json:"coverStatus"`
	CoverError           string  `json:"coverError"`
	VideoCodec           string  `json:"videoCodec"`
	AudioCodec           string  `json:"audioCodec"`
	HasInternalSubtitles bool    `json:"hasInternalSubtitles"`
	HasExternalSubtitles bool    `json:"hasExternalSubtitles"`
	NeedsTranscode       bool    `json:"needsTranscode"`
	Action               string  `json:"action"`
	Progress             float64 `json:"progress"`
	ProcessedDurationMS  int64   `json:"processedDurationMs"`
	TotalDurationMS      int64   `json:"totalDurationMs"`
	ErrorMessage         string  `json:"errorMessage"`
	ProgressUpdatedAt    *int64  `json:"progressUpdatedAt"`
	StartedAt            *int64  `json:"startedAt"`
	CompletedAt          *int64  `json:"completedAt"`
	FailedAt             *int64  `json:"failedAt"`
	CreatedAt            int64   `json:"createdAt"`
	UpdatedAt            int64   `json:"updatedAt"`
}

type pendingJob struct {
	ID                   int64
	DownloadJobID        int64
	SubscriptionItemID   int64
	BangumiID            int64
	AnimeName            string
	SeasonNumber         int
	EpisodeType          string
	EpisodeNumber        string
	SavePath             string
	StorageRoot          string
	SourcePath           string
	SubtitlePath         string
	OutputPath           string
	VideoCodec           string
	AudioCodec           string
	HasInternalSubtitles bool
	HasExternalSubtitles bool
	NeedsTranscode       bool
	Action               string
	TotalDurationMS      int64
}

type mediaFile struct {
	Path string
	Size int64
}

type probeResult struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeStream struct {
	Index     int    `json:"index"`
	CodecName string `json:"codec_name"`
	CodecType string `json:"codec_type"`
	Profile   string `json:"profile"`
	PixFmt    string `json:"pix_fmt"`
	Duration  string `json:"duration"`
}

type probeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
}

type mediaPlan struct {
	sourcePath           string
	subtitlePath         string
	outputPath           string
	videoCodec           string
	audioCodec           string
	hasInternalSubtitles bool
	hasExternalSubtitles bool
	needsTranscode       bool
	action               string
	totalDurationMS      int64
}

type processResult struct {
	action         string
	needsTranscode bool
}

type plannedJob struct {
	pendingJob
	plan mediaPlan
}

type pendingPlanningResult struct {
	planned        int
	planFailed     int
	copyJobs       []plannedJob
	ffmpegJob      *plannedJob
	deferredFFmpeg int
	stoppedOnError bool
}

type processLineResult struct {
	processed  int
	copied     int
	ffmpegJobs int
	err        error
}

type StorageMoveResult struct {
	BangumiID   int64  `json:"bangumiId"`
	StorageRoot string `json:"storageRoot"`
	StoragePath string `json:"storagePath"`
	Moved       bool   `json:"moved"`
}

type EpisodeReplacementCleanup struct {
	MediaJobsRemoved int64 `json:"mediaJobsRemoved"`
	FilesDeleted     int   `json:"filesDeleted"`
}

type animeStorageInfo struct {
	BangumiID    int64
	Name         string
	NameCN       string
	StorageRoot  string
	StoragePath  string
	StoredRoot   string
	CurrentRoot  string
	TargetRoot   string
	TargetStored string
}

type pathMove struct {
	Source string
	Target string
}

type episodeBindingMediaJob struct {
	ID          int64
	AnimeName   string
	Status      string
	OutputPath  string
	CoverPath   string
	CoverStatus string
}

type episodeBindingMovePlan struct {
	jobID      int64
	outputPath string
	coverPath  string
	moves      []fileMove
}

type fileMove struct {
	source string
	target string
}

type coverCandidate struct {
	ID          int64
	OutputPath  string
	CoverPath   string
	CoverStatus string
}

type episodeReplacementJob struct {
	ID         int64
	Status     string
	OutputPath string
	CoverPath  string
}

func NewService(db *sql.DB, logger *slog.Logger, config Config) *Service {
	mediaDir := strings.TrimSpace(config.MediaDir)
	if mediaDir == "" {
		mediaDir = "./data/bangumi"
	}
	if abs, err := filepath.Abs(mediaDir); err == nil {
		mediaDir = abs
	}
	ffmpegPath := strings.TrimSpace(config.FFmpegPath)
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	ffprobePath := strings.TrimSpace(config.FFprobePath)
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	return &Service{
		db: db, logger: logger, mediaDir: mediaDir,
		ffmpegPath: ffmpegPath, ffprobePath: ffprobePath,
		cleaner: config.DownloadCleaner, metadataRefresher: config.MetadataRefresher, now: time.Now,
	}
}

func (s *Service) DefaultMediaDir() string {
	return s.mediaDir
}

func (s *Service) Execute(ctx context.Context) error {
	s.logger.Info("媒体处理任务开始", "source", "media")
	enqueued, err := s.EnqueueCompletedDownloads(ctx)
	if err != nil {
		return fmt.Errorf("创建待处理媒体任务: %w", err)
	}
	if err := s.recoverInterruptedJobs(ctx); err != nil {
		return fmt.Errorf("恢复中断媒体任务: %w", err)
	}
	coverBackfilled, err := s.backfillMissingCovers(ctx)
	if err != nil {
		return fmt.Errorf("补齐视频封面图: %w", err)
	}

	planning, err := s.planPendingJobs(ctx)
	if err != nil {
		return err
	}
	if planning.planned > 0 || planning.planFailed > 0 || planning.deferredFFmpeg > 0 {
		s.logger.Info("媒体处理规划完成", "source", "media",
			"planned", planning.planned, "copy_queue", len(planning.copyJobs),
			"ffmpeg_selected", planning.ffmpegJob != nil, "ffmpeg_deferred", planning.deferredFFmpeg,
			"plan_failed", planning.planFailed)
	}

	processed := 0
	copied := 0
	ffmpegJobs := 0
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	resultCh := make(chan processLineResult, 2)
	lines := 0
	if len(planning.copyJobs) > 0 {
		lines++
		go func() {
			resultCh <- s.processCopyLine(runCtx, planning.copyJobs)
		}()
	}
	if planning.ffmpegJob != nil {
		lines++
		go func() {
			resultCh <- s.processFFmpegLine(runCtx, *planning.ffmpegJob)
		}()
	}
	var firstErr error
	for i := 0; i < lines; i++ {
		result := <-resultCh
		processed += result.processed
		copied += result.copied
		ffmpegJobs += result.ffmpegJobs
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			cancel()
		}
	}
	if firstErr != nil {
		return firstErr
	}

	if processed == 0 {
		s.logger.Info("媒体处理任务完成：没有可执行视频", "source", "media",
			"enqueued", enqueued, "planned", planning.planned, "plan_failed", planning.planFailed,
			"ffmpeg_deferred", planning.deferredFFmpeg, "cover_backfilled", coverBackfilled)
		return nil
	}
	s.logger.Info("媒体处理任务完成", "source", "media",
		"enqueued", enqueued, "planned", planning.planned, "plan_failed", planning.planFailed,
		"processed", processed, "copied", copied, "ffmpeg_jobs", ffmpegJobs,
		"copy_queue", len(planning.copyJobs), "ffmpeg_deferred", planning.deferredFFmpeg,
		"cover_backfilled", coverBackfilled)
	return nil
}

func (s *Service) planPendingJobs(ctx context.Context) (pendingPlanningResult, error) {
	jobs, err := s.pendingJobs(ctx)
	if err != nil {
		return pendingPlanningResult{}, err
	}
	result := pendingPlanningResult{copyJobs: make([]plannedJob, 0)}
	for _, candidate := range jobs {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		var (
			planned plannedJob
			reserve bool
		)
		s.storageMu.Lock()
		job, ok, err := s.pendingJobByID(ctx, candidate.ID)
		if err == nil && ok {
			plan, planErr := s.planForPendingJob(ctx, job)
			if planErr != nil {
				if markErr := s.markFailed(ctx, job.ID, planErr.Error()); markErr != nil {
					err = markErr
				} else {
					result.planFailed++
					result.stoppedOnError = true
					s.logger.Error("媒体处理规划失败", "source", "media", "media_job_id", job.ID, "error", planErr)
				}
			} else {
				result.planned++
				planned = plannedJob{pendingJob: job, plan: plan}
				if !plan.needsTranscode {
					reserve = true
				} else if result.ffmpegJob == nil {
					reserve = true
				} else {
					result.deferredFFmpeg++
				}
				if reserve {
					reserved, reserveErr := s.reservePendingJob(ctx, job.ID)
					if reserveErr != nil {
						err = reserveErr
					} else if reserved {
						if plan.needsTranscode {
							copy := planned
							result.ffmpegJob = &copy
						} else {
							result.copyJobs = append(result.copyJobs, planned)
						}
					}
				}
			}
		}
		s.storageMu.Unlock()

		if err != nil {
			return result, err
		}
		if result.stoppedOnError {
			break
		}
	}
	return result, nil
}

func (s *Service) pendingJobs(ctx context.Context) ([]pendingJob, error) {
	rows, err := s.db.QueryContext(ctx, pendingJobSelect+`
FROM media_jobs mj
JOIN download_jobs dj ON dj.id = mj.download_job_id
JOIN anime_metadata am ON am.bangumi_id = mj.bangumi_id
WHERE mj.status = ?
ORDER BY mj.created_at, mj.id`, s.mediaDir, StatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]pendingJob, 0)
	for rows.Next() {
		job, err := scanPendingJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Service) pendingJobByID(ctx context.Context, jobID int64) (pendingJob, bool, error) {
	job, err := scanPendingJob(s.db.QueryRowContext(ctx, pendingJobSelect+`
FROM media_jobs mj
JOIN download_jobs dj ON dj.id = mj.download_job_id
JOIN anime_metadata am ON am.bangumi_id = mj.bangumi_id
WHERE mj.id = ? AND mj.status = ?`, s.mediaDir, jobID, StatusPending))
	if errors.Is(err, sql.ErrNoRows) {
		return pendingJob{}, false, nil
	}
	if err != nil {
		return pendingJob{}, false, err
	}
	return job, true, nil
}

func (s *Service) planForPendingJob(ctx context.Context, job pendingJob) (mediaPlan, error) {
	if plan, ok := persistedPlan(job); ok {
		return plan, nil
	}
	plan, err := s.planJob(ctx, job)
	if err != nil {
		return mediaPlan{}, err
	}
	if err := s.persistPlan(ctx, job.ID, plan); err != nil {
		return mediaPlan{}, err
	}
	return plan, nil
}

func persistedPlan(job pendingJob) (mediaPlan, bool) {
	if strings.TrimSpace(job.Action) == "" || strings.TrimSpace(job.SourcePath) == "" || strings.TrimSpace(job.OutputPath) == "" {
		return mediaPlan{}, false
	}
	return mediaPlan{
		sourcePath:           job.SourcePath,
		subtitlePath:         job.SubtitlePath,
		outputPath:           job.OutputPath,
		videoCodec:           job.VideoCodec,
		audioCodec:           job.AudioCodec,
		hasInternalSubtitles: job.HasInternalSubtitles,
		hasExternalSubtitles: job.HasExternalSubtitles,
		needsTranscode:       job.NeedsTranscode,
		action:               job.Action,
		totalDurationMS:      job.TotalDurationMS,
	}, true
}

func (s *Service) reservePendingJob(ctx context.Context, jobID int64) (bool, error) {
	now := s.now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET status = ?, error_message = '', progress = 0, processed_duration_ms = 0,
    progress_updated_at = NULL, started_at = COALESCE(started_at, ?),
    failed_at = NULL, updated_at = ?
WHERE id = ? AND status = ?`, StatusTranscoding, now, now, jobID, StatusPending)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Service) processCopyLine(ctx context.Context, jobs []plannedJob) processLineResult {
	var result processLineResult
	for _, job := range jobs {
		select {
		case <-ctx.Done():
			result.err = ctx.Err()
			return result
		default:
		}
		processed, err := s.processPlannedJob(ctx, job)
		if err != nil {
			result.err = err
			return result
		}
		result.processed++
		if processed.action == "copy" {
			result.copied++
		}
	}
	return result
}

func (s *Service) processFFmpegLine(ctx context.Context, job plannedJob) processLineResult {
	processed, err := s.processPlannedJob(ctx, job)
	result := processLineResult{err: err}
	if err != nil {
		return result
	}
	result.processed = 1
	if processed.needsTranscode {
		result.ffmpegJobs = 1
	}
	return result
}

func (s *Service) ListJobs(ctx context.Context, page, pageSize int, status string) (JobPage, error) {
	if _, err := s.EnqueueCompletedDownloads(ctx); err != nil {
		return JobPage{}, err
	}
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
	where, args := mediaJobWhere(status)
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	query := mediaJobSelect + where + `
ORDER BY mj.updated_at DESC, mj.id DESC
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

func (s *Service) CountJobsByStatus(ctx context.Context, status string) (int, error) {
	counts, err := s.CountJobsByStatuses(ctx, status)
	if err != nil {
		return 0, err
	}
	return counts[normalizeStatus(status)], nil
}

func (s *Service) CountJobsByStatuses(ctx context.Context, statuses ...string) (map[string]int, error) {
	if _, err := s.EnqueueCompletedDownloads(ctx); err != nil {
		return nil, err
	}
	result := make(map[string]int, len(statuses))
	for _, status := range statuses {
		status = normalizeStatus(status)
		if status == "" || status == "invalid" {
			return nil, ErrInvalidStatus
		}
		where, args := mediaJobWhere(status)
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&count); err != nil {
			return nil, err
		}
		result[status] = count
	}
	return result, nil
}

func (s *Service) PrepareEpisodeReplacement(ctx context.Context, bangumiID int64, seasonNumber int, episodeType, episodeNumber string) (EpisodeReplacementCleanup, error) {
	episodeType = normalizeEpisodeType(episodeType)
	episodeNumber = strings.TrimSpace(episodeNumber)
	if bangumiID < 1 || seasonNumber < 1 || episodeNumber == "" {
		return EpisodeReplacementCleanup{}, ErrInvalidEpisodeTarget
	}

	s.storageMu.Lock()
	defer s.storageMu.Unlock()

	rows, err := s.db.QueryContext(ctx, `
SELECT id, status, output_path, cover_path
FROM media_jobs
WHERE bangumi_id = ?
  AND season_number = ?
  AND COALESCE(NULLIF(episode_type, ''), 'episode') = ?
  AND episode_number = ?`, bangumiID, seasonNumber, episodeType, episodeNumber)
	if err != nil {
		return EpisodeReplacementCleanup{}, err
	}
	jobs := make([]episodeReplacementJob, 0)
	for rows.Next() {
		var job episodeReplacementJob
		if err := rows.Scan(&job.ID, &job.Status, &job.OutputPath, &job.CoverPath); err != nil {
			rows.Close()
			return EpisodeReplacementCleanup{}, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Close(); err != nil {
		return EpisodeReplacementCleanup{}, err
	}
	for _, job := range jobs {
		if job.Status == StatusTranscoding {
			return EpisodeReplacementCleanup{}, ErrAnimeTranscoding
		}
	}

	paths := make(map[string]struct{})
	for _, job := range jobs {
		if path := strings.TrimSpace(job.OutputPath); path != "" {
			paths[path] = struct{}{}
			paths[coverPathForOutput(path)] = struct{}{}
		}
		if path := strings.TrimSpace(job.CoverPath); path != "" {
			paths[path] = struct{}{}
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EpisodeReplacementCleanup{}, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
DELETE FROM media_jobs
WHERE bangumi_id = ?
  AND season_number = ?
  AND COALESCE(NULLIF(episode_type, ''), 'episode') = ?
  AND episode_number = ?
  AND status != ?`, bangumiID, seasonNumber, episodeType, episodeNumber, StatusTranscoding)
	if err != nil {
		return EpisodeReplacementCleanup{}, err
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return EpisodeReplacementCleanup{}, err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM download_jobs
WHERE subscription_item_id IN (
    SELECT id
    FROM subscription_items
    WHERE binding_status = 'bound'
      AND bound_bangumi_id = ?
      AND bound_season_number = ?
      AND COALESCE(NULLIF(bound_episode_type, ''), 'episode') = ?
      AND bound_episode_number = ?
)`, bangumiID, seasonNumber, episodeType, episodeNumber); err != nil {
		return EpisodeReplacementCleanup{}, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE subscription_items
SET binding_status = 'pending',
    binding_note = '绑定已被手动单话替换覆盖',
    bound_bangumi_id = NULL,
    bound_anime_name = '',
    bound_season_number = NULL,
    bound_episode_type = '',
    bound_episode_number = '',
    bound_at = NULL,
    ignored_at = NULL,
    updated_at = ?
WHERE binding_status = 'bound'
  AND bound_bangumi_id = ?
  AND bound_season_number = ?
  AND COALESCE(NULLIF(bound_episode_type, ''), 'episode') = ?
  AND bound_episode_number = ?`, s.now().UTC().Unix(), bangumiID, seasonNumber, episodeType, episodeNumber); err != nil {
		return EpisodeReplacementCleanup{}, err
	}
	if err := tx.Commit(); err != nil {
		return EpisodeReplacementCleanup{}, err
	}

	filesDeleted := 0
	for path := range paths {
		exists, err := fileExists(path)
		if err != nil {
			return EpisodeReplacementCleanup{}, err
		}
		if !exists {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return EpisodeReplacementCleanup{}, err
		}
		filesDeleted++
	}
	if removed > 0 || filesDeleted > 0 {
		s.logger.Info("单话替换前旧媒体产物已清理", "source", "media",
			"bangumi_id", bangumiID, "season_number", seasonNumber, "episode_type", episodeType,
			"episode_number", episodeNumber, "media_jobs_removed", removed, "files_deleted", filesDeleted)
	}
	return EpisodeReplacementCleanup{MediaJobsRemoved: removed, FilesDeleted: filesDeleted}, nil
}

func (s *Service) UpdateEpisodeBinding(ctx context.Context, bangumiID int64, sourceInput, targetInput subscription.EpisodeBindingIdentity) (subscription.EpisodeBindingMutationResult, error) {
	if bangumiID < 1 {
		return subscription.EpisodeBindingMutationResult{}, subscription.ErrInvalidBinding
	}
	sourceInput, err := subscription.NormalizeEpisodeBindingIdentity(sourceInput)
	if err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	targetInput, err = subscription.NormalizeEpisodeBindingIdentity(targetInput)
	if err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	result := subscription.EpisodeBindingMutationResult{
		BangumiID: bangumiID,
		Source:    sourceInput,
		Target:    &targetInput,
	}

	s.storageMu.Lock()
	defer s.storageMu.Unlock()

	animeName, storageRoot, err := s.episodeBindingAnime(ctx, bangumiID)
	if err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	count, err := s.boundEpisodeBindingCount(ctx, bangumiID, sourceInput)
	if err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	if count == 0 {
		return subscription.EpisodeBindingMutationResult{}, subscription.ErrEpisodeBindingNotFound
	}
	if err := s.ensureEpisodeBindingEditable(ctx, bangumiID, sourceInput); err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	if !sameEpisodeBindingIdentity(sourceInput, targetInput) {
		exists, err := s.episodeBindingTargetExists(ctx, bangumiID, sourceInput, targetInput)
		if err != nil {
			return subscription.EpisodeBindingMutationResult{}, err
		}
		if exists {
			return subscription.EpisodeBindingMutationResult{}, subscription.ErrEpisodeBindingExists
		}
	}

	jobs, err := s.episodeBindingMediaJobs(ctx, bangumiID, sourceInput)
	if err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	plans, err := s.planEpisodeBindingMoves(jobs, animeName, storageRoot, targetInput)
	if err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	for _, plan := range plans {
		for _, move := range plan.moves {
			if err := moveFileNoOverwrite(move.source, move.target); err != nil {
				return subscription.EpisodeBindingMutationResult{}, err
			}
		}
	}

	now := s.now().UTC().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	defer tx.Rollback()

	titleRules, err := tx.ExecContext(ctx, `
UPDATE subscription_title_rules
SET season_number = ?, episode_type = ?, updated_at = ?
WHERE created_from_item_id IN (
    SELECT id
    FROM subscription_items
    WHERE binding_status = ?
      AND bound_bangumi_id = ?
      AND bound_season_number = ?
      AND COALESCE(NULLIF(bound_episode_type, ''), 'episode') = ?
      AND bound_episode_number = ?
)`, targetInput.SeasonNumber, targetInput.EpisodeType, now,
		subscription.BindingStatusBound, bangumiID, sourceInput.SeasonNumber, sourceInput.EpisodeType, sourceInput.EpisodeNumber)
	if err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	result.UpdatedTitleRules, _ = titleRules.RowsAffected()

	for _, plan := range plans {
		updated, err := tx.ExecContext(ctx, `
UPDATE media_jobs
SET season_number = ?, episode_type = ?, episode_number = ?,
    output_path = ?, cover_path = ?, updated_at = ?
WHERE id = ? AND status != ?`, targetInput.SeasonNumber, targetInput.EpisodeType, targetInput.EpisodeNumber,
			plan.outputPath, plan.coverPath, now, plan.jobID, StatusTranscoding)
		if err != nil {
			return subscription.EpisodeBindingMutationResult{}, err
		}
		affected, err := updated.RowsAffected()
		if err != nil {
			return subscription.EpisodeBindingMutationResult{}, err
		}
		result.UpdatedMediaJobs += affected
	}

	items, err := tx.ExecContext(ctx, `
UPDATE subscription_items
SET bound_season_number = ?,
    bound_episode_type = ?,
    bound_episode_number = ?,
    binding_note = CASE
        WHEN binding_note = '' THEN '绑定集数标识已在番剧管理中修改'
        ELSE binding_note
    END,
    updated_at = ?
WHERE binding_status = ?
  AND bound_bangumi_id = ?
  AND bound_season_number = ?
  AND COALESCE(NULLIF(bound_episode_type, ''), 'episode') = ?
  AND bound_episode_number = ?`, targetInput.SeasonNumber, targetInput.EpisodeType, targetInput.EpisodeNumber, now,
		subscription.BindingStatusBound, bangumiID, sourceInput.SeasonNumber, sourceInput.EpisodeType, sourceInput.EpisodeNumber)
	if err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	result.UpdatedItems, _ = items.RowsAffected()
	if result.UpdatedItems == 0 {
		return subscription.EpisodeBindingMutationResult{}, subscription.ErrEpisodeBindingNotFound
	}
	if err := tx.Commit(); err != nil {
		return subscription.EpisodeBindingMutationResult{}, err
	}
	s.logger.Info("番剧绑定集数标识和媒体产物路径已修改", "source", "media",
		"bangumi_id", bangumiID, "from_season_number", sourceInput.SeasonNumber,
		"from_episode_type", sourceInput.EpisodeType, "from_episode_number", sourceInput.EpisodeNumber,
		"to_season_number", targetInput.SeasonNumber, "to_episode_type", targetInput.EpisodeType,
		"to_episode_number", targetInput.EpisodeNumber, "updated_items", result.UpdatedItems,
		"updated_media_jobs", result.UpdatedMediaJobs)
	return result, nil
}

func (s *Service) episodeBindingAnime(ctx context.Context, bangumiID int64) (animeName, storageRoot string, err error) {
	var name, nameCN, storedRoot string
	err = s.db.QueryRowContext(ctx, `
SELECT name, name_cn, media_storage_root
FROM anime_metadata
WHERE bangumi_id = ? AND deleted_at IS NULL`, bangumiID).Scan(&name, &nameCN, &storedRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrAnimeNotFound
	}
	if err != nil {
		return "", "", err
	}
	animeName = strings.TrimSpace(nameCN)
	if animeName == "" {
		animeName = strings.TrimSpace(name)
	}
	if animeName == "" {
		animeName = fmt.Sprintf("Bangumi-%d", bangumiID)
	}
	storageRoot = strings.TrimSpace(storedRoot)
	if storageRoot == "" {
		storageRoot = s.mediaDir
	}
	return animeName, storageRoot, nil
}

func (s *Service) boundEpisodeBindingCount(ctx context.Context, bangumiID int64, identity subscription.EpisodeBindingIdentity) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM subscription_items
WHERE binding_status = ?
  AND bound_bangumi_id = ?
  AND bound_season_number = ?
  AND COALESCE(NULLIF(bound_episode_type, ''), 'episode') = ?
  AND bound_episode_number = ?`, subscription.BindingStatusBound, bangumiID, identity.SeasonNumber, identity.EpisodeType, identity.EpisodeNumber).Scan(&count)
	return count, err
}

func (s *Service) ensureEpisodeBindingEditable(ctx context.Context, bangumiID int64, identity subscription.EpisodeBindingIdentity) error {
	var downloading int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM download_jobs dj
JOIN subscription_items si ON si.id = dj.subscription_item_id
WHERE si.binding_status = ?
  AND si.bound_bangumi_id = ?
  AND si.bound_season_number = ?
  AND COALESCE(NULLIF(si.bound_episode_type, ''), 'episode') = ?
  AND si.bound_episode_number = ?
  AND dj.status = 'downloading'`, subscription.BindingStatusBound, bangumiID, identity.SeasonNumber, identity.EpisodeType, identity.EpisodeNumber).Scan(&downloading); err != nil {
		return err
	}
	if downloading > 0 {
		return subscription.ErrEpisodeBindingBusy
	}

	var transcoding int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM media_jobs
WHERE bangumi_id = ?
  AND season_number = ?
  AND COALESCE(NULLIF(episode_type, ''), 'episode') = ?
  AND episode_number = ?
  AND status = ?`, bangumiID, identity.SeasonNumber, identity.EpisodeType, identity.EpisodeNumber, StatusTranscoding).Scan(&transcoding); err != nil {
		return err
	}
	if transcoding > 0 {
		return subscription.ErrEpisodeBindingBusy
	}
	return nil
}

func (s *Service) episodeBindingTargetExists(ctx context.Context, bangumiID int64, source, target subscription.EpisodeBindingIdentity) (bool, error) {
	var boundCount int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM subscription_items
WHERE binding_status = ?
  AND bound_bangumi_id = ?
  AND bound_season_number = ?
  AND COALESCE(NULLIF(bound_episode_type, ''), 'episode') = ?
  AND bound_episode_number = ?
  AND NOT (
    bound_season_number = ?
    AND COALESCE(NULLIF(bound_episode_type, ''), 'episode') = ?
    AND bound_episode_number = ?
  )`, subscription.BindingStatusBound, bangumiID, target.SeasonNumber, target.EpisodeType, target.EpisodeNumber,
		source.SeasonNumber, source.EpisodeType, source.EpisodeNumber).Scan(&boundCount); err != nil {
		return false, err
	}
	if boundCount > 0 {
		return true, nil
	}

	var mediaCount int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM media_jobs
WHERE bangumi_id = ?
  AND season_number = ?
  AND COALESCE(NULLIF(episode_type, ''), 'episode') = ?
  AND episode_number = ?
  AND NOT (
    season_number = ?
    AND COALESCE(NULLIF(episode_type, ''), 'episode') = ?
    AND episode_number = ?
  )`, bangumiID, target.SeasonNumber, target.EpisodeType, target.EpisodeNumber,
		source.SeasonNumber, source.EpisodeType, source.EpisodeNumber).Scan(&mediaCount); err != nil {
		return false, err
	}
	return mediaCount > 0, nil
}

func (s *Service) episodeBindingMediaJobs(ctx context.Context, bangumiID int64, identity subscription.EpisodeBindingIdentity) ([]episodeBindingMediaJob, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, anime_name, status, output_path, cover_path, cover_status
FROM media_jobs
WHERE bangumi_id = ?
  AND season_number = ?
  AND COALESCE(NULLIF(episode_type, ''), 'episode') = ?
  AND episode_number = ?`, bangumiID, identity.SeasonNumber, identity.EpisodeType, identity.EpisodeNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]episodeBindingMediaJob, 0)
	for rows.Next() {
		var job episodeBindingMediaJob
		if err := rows.Scan(&job.ID, &job.AnimeName, &job.Status, &job.OutputPath, &job.CoverPath, &job.CoverStatus); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Service) planEpisodeBindingMoves(jobs []episodeBindingMediaJob, animeName, storageRoot string, target subscription.EpisodeBindingIdentity) ([]episodeBindingMovePlan, error) {
	plans := make([]episodeBindingMovePlan, 0, len(jobs))
	seenTargets := make(map[string]int64)
	for _, job := range jobs {
		if job.Status == StatusTranscoding {
			return nil, subscription.ErrEpisodeBindingBusy
		}
		plan := episodeBindingMovePlan{jobID: job.ID, outputPath: job.OutputPath, coverPath: job.CoverPath}
		targetAnimeName := strings.TrimSpace(job.AnimeName)
		if targetAnimeName == "" {
			targetAnimeName = animeName
		}
		if strings.TrimSpace(job.OutputPath) != "" {
			targetJob := pendingJob{
				AnimeName: targetAnimeName, SeasonNumber: target.SeasonNumber,
				EpisodeType: target.EpisodeType, EpisodeNumber: target.EpisodeNumber,
			}
			plan.outputPath = finalOutputPath(storageRoot, targetJob)
			if strings.TrimSpace(job.CoverPath) != "" {
				plan.coverPath = coverPathForOutput(plan.outputPath)
			}
			if err := ensureUniqueEpisodeBindingTarget(seenTargets, plan.outputPath, job.ID); err != nil {
				return nil, err
			}
			if err := appendEpisodeBindingMove(&plan.moves, job.OutputPath, plan.outputPath); err != nil {
				return nil, err
			}
			if strings.TrimSpace(job.CoverPath) != "" {
				if err := ensureUniqueEpisodeBindingTarget(seenTargets, plan.coverPath, job.ID); err != nil {
					return nil, err
				}
				if err := appendEpisodeBindingMove(&plan.moves, job.CoverPath, plan.coverPath); err != nil {
					return nil, err
				}
			}
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func ensureUniqueEpisodeBindingTarget(seen map[string]int64, path string, jobID int64) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	key := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	if existing, ok := seen[key]; ok && existing != jobID {
		return ErrStorageTargetConflict
	}
	seen[key] = jobID
	return nil
}

func appendEpisodeBindingMove(moves *[]fileMove, source, target string) error {
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)
	if source == "" || target == "" || samePath(source, target) {
		return nil
	}
	sourceExists, err := fileExists(source)
	if err != nil {
		return err
	}
	targetExists, err := fileExists(target)
	if err != nil {
		return err
	}
	if targetExists {
		return ErrStorageTargetConflict
	}
	if sourceExists {
		*moves = append(*moves, fileMove{source: source, target: target})
	}
	return nil
}

func sameEpisodeBindingIdentity(left, right subscription.EpisodeBindingIdentity) bool {
	return left.SeasonNumber == right.SeasonNumber &&
		left.EpisodeType == right.EpisodeType &&
		left.EpisodeNumber == right.EpisodeNumber
}

func (s *Service) MoveAnimeStorage(ctx context.Context, bangumiID int64, targetRoot string) (StorageMoveResult, error) {
	targetRoot, err := s.normalizeStorageRoot(targetRoot)
	if err != nil {
		return StorageMoveResult{}, err
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()

	info, err := s.animeStorageInfo(ctx, bangumiID, targetRoot)
	if err != nil {
		return StorageMoveResult{}, err
	}
	transcoding, err := s.animeTranscodingCount(ctx, bangumiID)
	if err != nil {
		return StorageMoveResult{}, err
	}
	if transcoding > 0 {
		return StorageMoveResult{}, ErrAnimeTranscoding
	}
	if err := os.MkdirAll(info.TargetRoot, 0o755); err != nil {
		return StorageMoveResult{}, err
	}
	if samePath(info.CurrentRoot, info.TargetRoot) {
		if err := s.persistAnimeStorage(ctx, info, nil); err != nil {
			return StorageMoveResult{}, err
		}
		return StorageMoveResult{
			BangumiID: info.BangumiID, StorageRoot: info.TargetRoot, StoragePath: info.StoragePath, Moved: false,
		}, nil
	}

	sourceDirs, err := s.animeOutputDirs(ctx, bangumiID, info.CurrentRoot)
	if err != nil {
		return StorageMoveResult{}, err
	}
	fallbackSource := storagePathForNames(info.CurrentRoot, info.NameCN, info.Name)
	if len(sourceDirs) == 0 {
		if exists, err := dirExists(fallbackSource); err != nil {
			return StorageMoveResult{}, err
		} else if exists {
			sourceDirs = append(sourceDirs, fallbackSource)
		}
	}

	mappings := make([]pathMove, 0, len(sourceDirs))
	moved := false
	for _, sourceDir := range sourceDirs {
		targetDir := info.StoragePath
		mappings = append(mappings, pathMove{Source: sourceDir, Target: targetDir})
		exists, err := dirExists(sourceDir)
		if err != nil {
			return StorageMoveResult{}, err
		}
		if !exists || samePath(sourceDir, targetDir) {
			continue
		}
		if err := moveDirectory(sourceDir, targetDir); err != nil {
			return StorageMoveResult{}, err
		}
		moved = true
	}

	if err := s.persistAnimeStorage(ctx, info, mappings); err != nil {
		return StorageMoveResult{}, err
	}
	s.logger.Info("番剧成品存储路径已移动", "source", "media", "bangumi_id", bangumiID,
		"from", info.CurrentRoot, "to", info.TargetRoot, "moved", moved)
	return StorageMoveResult{
		BangumiID: info.BangumiID, StorageRoot: info.TargetRoot, StoragePath: info.StoragePath, Moved: moved,
	}, nil
}

func (s *Service) normalizeStorageRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = s.mediaDir
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return "", ErrInvalidStorageRoot
	}
	return root, nil
}

func (s *Service) animeStorageInfo(ctx context.Context, bangumiID int64, targetRoot string) (animeStorageInfo, error) {
	var info animeStorageInfo
	err := s.db.QueryRowContext(ctx, `
SELECT bangumi_id, name, name_cn, media_storage_root
FROM anime_metadata
WHERE bangumi_id = ? AND deleted_at IS NULL`, bangumiID).Scan(
		&info.BangumiID, &info.Name, &info.NameCN, &info.StoredRoot,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return animeStorageInfo{}, ErrAnimeNotFound
	}
	if err != nil {
		return animeStorageInfo{}, err
	}
	info.CurrentRoot = strings.TrimSpace(info.StoredRoot)
	if info.CurrentRoot == "" {
		info.CurrentRoot = s.mediaDir
	}
	info.TargetRoot = targetRoot
	if samePath(targetRoot, s.mediaDir) {
		info.TargetStored = ""
	} else {
		info.TargetStored = targetRoot
	}
	info.StorageRoot = info.TargetRoot
	info.StoragePath = storagePathForNames(info.TargetRoot, info.NameCN, info.Name)
	return info, nil
}

func (s *Service) animeTranscodingCount(ctx context.Context, bangumiID int64) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM media_jobs
WHERE bangumi_id = ? AND status = ?`, bangumiID, StatusTranscoding).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Service) animeOutputDirs(ctx context.Context, bangumiID int64, root string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT output_path, cover_path
FROM media_jobs
WHERE bangumi_id = ? AND (output_path != '' OR cover_path != '')`, bangumiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for rows.Next() {
		var outputPath, coverPath string
		if err := rows.Scan(&outputPath, &coverPath); err != nil {
			return nil, err
		}
		for _, mediaPath := range []string{outputPath, coverPath} {
			rel, ok := relativePath(root, mediaPath)
			if !ok || rel == "." {
				continue
			}
			first := strings.Split(rel, string(os.PathSeparator))[0]
			if first == "" || first == "." {
				continue
			}
			dir := filepath.Join(root, first)
			if _, exists := seen[dir]; exists {
				continue
			}
			seen[dir] = struct{}{}
			result = append(result, dir)
		}
	}
	return result, rows.Err()
}

func (s *Service) persistAnimeStorage(ctx context.Context, info animeStorageInfo, moves []pathMove) error {
	type mediaOutput struct {
		ID        int64
		Output    string
		CoverPath string
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, output_path, cover_path
FROM media_jobs
WHERE bangumi_id = ? AND (output_path != '' OR cover_path != '')`, info.BangumiID)
	if err != nil {
		return err
	}
	outputs := make([]mediaOutput, 0)
	for rows.Next() {
		var output mediaOutput
		if err := rows.Scan(&output.ID, &output.Output, &output.CoverPath); err != nil {
			rows.Close()
			return err
		}
		outputs = append(outputs, output)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
UPDATE anime_metadata
SET media_storage_root = ?
WHERE bangumi_id = ?`, info.TargetStored, info.BangumiID); err != nil {
		return err
	}
	for _, output := range outputs {
		updatedOutput := movedPath(output.Output, moves)
		updatedCover := movedPath(output.CoverPath, moves)
		if updatedOutput == output.Output && updatedCover == output.CoverPath {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE media_jobs
SET output_path = ?, cover_path = ?, updated_at = ?
WHERE id = ?`, updatedOutput, updatedCover, s.now().UTC().Unix(), output.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func movedPath(path string, moves []pathMove) string {
	updated := path
	if strings.TrimSpace(path) == "" {
		return updated
	}
	for _, move := range moves {
		if rel, ok := relativePath(move.Source, path); ok {
			updated = filepath.Join(move.Target, rel)
			break
		}
	}
	return updated
}

func (s *Service) RetryFailedJob(ctx context.Context, jobID int64) (Job, error) {
	now := s.now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET status = ?, source_path = '', subtitle_path = '', output_path = '',
    cover_path = '', cover_status = ?, cover_error = '',
    video_codec = '', audio_codec = '', has_internal_subtitles = 0,
    has_external_subtitles = 0, needs_transcode = 0, action = '',
    progress = 0, processed_duration_ms = 0, total_duration_ms = 0,
    progress_updated_at = NULL, error_message = '',
    started_at = NULL, completed_at = NULL, failed_at = NULL,
    updated_at = ?
WHERE id = ? AND status = ?`, StatusPending, CoverStatusPending, now, jobID, StatusFailed)
	if err != nil {
		return Job{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Job{}, err
	}
	if affected == 0 {
		if _, err := s.jobByID(ctx, jobID); errors.Is(err, ErrMediaJobNotFound) {
			return Job{}, ErrMediaJobNotFound
		}
		return Job{}, ErrRetryNotAllowed
	}
	job, err := s.jobByID(ctx, jobID)
	if err != nil {
		return Job{}, err
	}
	s.logger.Info("媒体处理失败任务已重置为待处理", "source", "media", "media_job_id", jobID)
	return job, nil
}

func (s *Service) jobByID(ctx context.Context, jobID int64) (Job, error) {
	job, err := scanJob(s.db.QueryRowContext(ctx, mediaJobSelect+`
FROM media_jobs mj
JOIN subscription_items si ON si.id = mj.subscription_item_id
WHERE mj.id = ?`, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrMediaJobNotFound
	}
	return job, err
}

func (s *Service) EnqueueCompletedDownloads(ctx context.Context) (int64, error) {
	now := s.now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO media_jobs(
    download_job_id, subscription_item_id, bangumi_id, anime_name, season_number,
    episode_type, episode_number, status, created_at, updated_at
)
SELECT dj.id, si.id, si.bound_bangumi_id,
       COALESCE(NULLIF(si.bound_anime_name, ''), NULLIF(am.name_cn, ''), NULLIF(am.name, ''), 'Bangumi-' || si.bound_bangumi_id),
       si.bound_season_number,
       COALESCE(NULLIF(si.bound_episode_type, ''), 'episode'),
       si.bound_episode_number,
       ?, ?, ?
FROM download_jobs dj
JOIN subscription_items si ON si.id = dj.subscription_item_id
LEFT JOIN anime_metadata am ON am.bangumi_id = si.bound_bangumi_id
WHERE dj.status = ?
  AND si.binding_status = 'bound'
  AND si.bound_bangumi_id IS NOT NULL
  AND si.bound_season_number IS NOT NULL
  AND si.bound_episode_number != ''`, StatusPending, now, now, downloadStatusCompleted)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Service) recoverInterruptedJobs(ctx context.Context) error {
	now := s.now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET status = ?, error_message = '上次处理被中断，已重新排队',
    progress = 0, processed_duration_ms = 0, progress_updated_at = NULL,
    started_at = NULL, updated_at = ?
WHERE status = ?`, StatusPending, now, StatusTranscoding)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		s.logger.Warn("媒体处理任务恢复中断任务", "source", "media", "count", affected)
	}
	return nil
}

func (s *Service) processPlannedJob(ctx context.Context, job plannedJob) (processResult, error) {
	plan := job.plan
	result := processResult{
		action:         plan.action,
		needsTranscode: plan.needsTranscode,
	}
	s.logger.Info("媒体处理开始", "source", "media", "media_job_id", job.ID, "action", plan.action, "source_file", filepath.Base(plan.sourcePath))
	var err error
	if plan.action == "copy" {
		err = copyToFinal(plan.sourcePath, plan.outputPath)
	} else {
		err = s.runFFmpeg(ctx, job.ID, plan)
	}
	if err != nil {
		_ = s.markFailed(ctx, job.ID, err.Error())
		s.logger.Error("媒体处理失败", "source", "media", "media_job_id", job.ID, "action", plan.action, "error", err)
		return result, nil
	}
	if err := s.generateCoverForJob(ctx, job.ID, plan.outputPath); err != nil {
		s.logger.Warn("视频封面图生成失败", "source", "media", "media_job_id", job.ID, "output_file", filepath.Base(plan.outputPath), "error", err)
	}
	if err := s.markCompleted(ctx, job.ID); err != nil {
		return result, err
	}
	if err := s.cleanupDownload(ctx, job.pendingJob); err != nil {
		message := "最终产物已完成，但 qBittorrent 下载清理失败: " + err.Error()
		_ = s.recordCompletionWarning(ctx, job.ID, message)
		s.logger.Warn("媒体处理完成后清理 qBittorrent 下载失败", "source", "media", "media_job_id", job.ID, "download_job_id", job.DownloadJobID, "error", err)
	} else {
		s.logger.Info("媒体处理完成后 qBittorrent 下载已清理", "source", "media", "media_job_id", job.ID, "download_job_id", job.DownloadJobID)
	}
	s.refreshAnimeMetadataOncePerDay(ctx, job.BangumiID)
	s.logger.Info("媒体处理成功", "source", "media", "media_job_id", job.ID, "action", plan.action, "output_file", filepath.Base(plan.outputPath))
	return result, nil
}

func (s *Service) cleanupDownload(ctx context.Context, job pendingJob) error {
	if s.cleaner == nil {
		return nil
	}
	return s.cleaner.CleanupCompletedQBitTask(ctx, job.DownloadJobID)
}

func (s *Service) refreshAnimeMetadataOncePerDay(ctx context.Context, bangumiID int64) {
	if s.metadataRefresher == nil || bangumiID <= 0 {
		return
	}
	now := s.now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	nowUnix := now.UTC().Unix()
	result, err := s.db.ExecContext(ctx, `
UPDATE anime_metadata
SET last_media_refresh_at = ?
WHERE bangumi_id = ?
  AND deleted_at IS NULL
  AND (last_media_refresh_at IS NULL OR last_media_refresh_at < ?)`, nowUnix, bangumiID, dayStart)
	if err != nil {
		s.logger.Warn("媒体处理完成后记录番剧元数据刷新时间失败", "source", "media", "bangumi_id", bangumiID, "error", err)
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		s.logger.Warn("媒体处理完成后确认番剧元数据刷新时间失败", "source", "media", "bangumi_id", bangumiID, "error", err)
		return
	}
	if affected == 0 {
		return
	}
	if err := s.metadataRefresher.RefreshSubject(ctx, bangumiID); err != nil {
		s.logger.Warn("媒体处理完成后刷新番剧元数据失败", "source", "media", "bangumi_id", bangumiID, "error", err)
		return
	}
	s.logger.Info("媒体处理完成后已刷新番剧元数据", "source", "media", "bangumi_id", bangumiID)
}

func (s *Service) planJob(ctx context.Context, job pendingJob) (mediaPlan, error) {
	video, err := findPrimaryVideo(job.SavePath)
	if err != nil {
		return mediaPlan{}, err
	}
	probe, err := s.probe(ctx, video.Path)
	if err != nil {
		return mediaPlan{}, err
	}
	videoStream, audioStream, subtitleStreams := classifyStreams(probe.Streams)
	if videoStream.CodecName == "" {
		return mediaPlan{}, errors.New("未找到视频流")
	}
	totalDurationMS := probeDurationMS(probe, videoStream)
	subtitlePath := findExternalSubtitle(video.Path)
	outputPath := finalOutputPath(job.StorageRoot, job)
	webPlayable := isBrowserPlayable(video.Path, videoStream, audioStream)
	hasInternal := len(subtitleStreams) > 0
	hasExternal := subtitlePath != ""

	action := "copy"
	needsTranscode := false
	switch {
	case hasInternal || hasExternal:
		action = "burn_subtitles"
		needsTranscode = true
	case !webPlayable && videoStream.CodecName == "h264" && (audioStream.CodecName == "" || audioStream.CodecName == "aac"):
		action = "remux"
		needsTranscode = true
	case !webPlayable:
		action = "transcode"
		needsTranscode = true
	}
	return mediaPlan{
		sourcePath: video.Path, subtitlePath: subtitlePath, outputPath: outputPath,
		videoCodec: videoStream.CodecName, audioCodec: audioStream.CodecName,
		hasInternalSubtitles: hasInternal, hasExternalSubtitles: hasExternal,
		needsTranscode: needsTranscode, action: action, totalDurationMS: totalDurationMS,
	}, nil
}

func (s *Service) probe(ctx context.Context, path string) (probeResult, error) {
	command := exec.CommandContext(ctx, s.ffprobePath,
		"-v", "error", "-print_format", "json", "-show_format", "-show_streams", path,
	)
	output, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return probeResult{}, fmt.Errorf("ffprobe 失败: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return probeResult{}, fmt.Errorf("ffprobe 失败: %w", err)
	}
	var result probeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return probeResult{}, fmt.Errorf("解析 ffprobe 输出失败: %w", err)
	}
	return result, nil
}

func (s *Service) backfillMissingCovers(ctx context.Context) (int, error) {
	s.storageMu.Lock()
	defer s.storageMu.Unlock()

	candidates, err := s.coverBackfillCandidates(ctx)
	if err != nil {
		return 0, err
	}
	generated := 0
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			return generated, ctx.Err()
		default:
		}
		if candidate.CoverStatus == CoverStatusCompleted && strings.TrimSpace(candidate.CoverPath) != "" {
			if exists, err := fileExists(candidate.CoverPath); err != nil {
				s.logger.Warn("检查视频封面图失败", "source", "media", "media_job_id", candidate.ID, "error", err)
			} else if exists {
				continue
			}
		}
		if err := s.generateCoverForJob(ctx, candidate.ID, candidate.OutputPath); err != nil {
			s.logger.Warn("历史视频封面图生成失败", "source", "media", "media_job_id", candidate.ID, "output_file", filepath.Base(candidate.OutputPath), "error", err)
			continue
		}
		generated++
	}
	return generated, nil
}

func (s *Service) coverBackfillCandidates(ctx context.Context) ([]coverCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, output_path, cover_path, cover_status
FROM media_jobs
WHERE status = ? AND output_path != ''
ORDER BY COALESCE(completed_at, updated_at), id`, StatusCompleted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]coverCandidate, 0)
	for rows.Next() {
		var candidate coverCandidate
		if err := rows.Scan(&candidate.ID, &candidate.OutputPath, &candidate.CoverPath, &candidate.CoverStatus); err != nil {
			return nil, err
		}
		if candidate.CoverStatus == "" || candidate.CoverStatus == CoverStatusPending ||
			candidate.CoverStatus == CoverStatusFailed || candidate.CoverPath == "" ||
			candidate.CoverStatus == CoverStatusCompleted {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, rows.Err()
}

func (s *Service) generateCoverForJob(ctx context.Context, jobID int64, outputPath string) error {
	coverPath := coverPathForOutput(outputPath)
	if err := s.generateCover(ctx, outputPath, coverPath); err != nil {
		_ = s.markCoverFailed(ctx, jobID, err.Error())
		return err
	}
	if err := s.markCoverCompleted(ctx, jobID, coverPath); err != nil {
		return err
	}
	return nil
}

func (s *Service) generateCover(ctx context.Context, outputPath, coverPath string) error {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return errors.New("缺少最终产物视频路径")
	}
	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("访问最终产物视频失败: %w", err)
	}
	durationMS, err := s.videoDurationMS(ctx, outputPath)
	if err != nil {
		return err
	}
	if durationMS <= 0 {
		return errors.New("无法获取视频总时长")
	}
	if err := os.MkdirAll(filepath.Dir(coverPath), 0o755); err != nil {
		return err
	}
	tempPNG := coverPath + ".tmp.png"
	tempJPG := coverPath + ".tmp.jpg"
	_ = os.Remove(tempPNG)
	_ = os.Remove(tempJPG)
	timestamp := strconv.FormatFloat(float64(durationMS)/2000, 'f', 3, 64)
	scaleFilter := "scale='if(gt(iw,ih),min(480,iw),-2)':'if(gt(iw,ih),-2,min(480,ih))'"
	command := exec.CommandContext(ctx, s.ffmpegPath,
		"-hide_banner", "-nostats", "-loglevel", "warning",
		"-y", "-ss", timestamp, "-i", outputPath,
		"-frames:v", "1", "-an", "-sn", "-vf", scaleFilter, tempPNG,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		if len(message) > 2000 {
			message = message[len(message)-2000:]
		}
		return fmt.Errorf("ffmpeg 截取封面图失败: %s", message)
	}
	if err := encodeJPEGCover(tempPNG, tempJPG); err != nil {
		return err
	}
	if err := replaceFile(tempJPG, coverPath); err != nil {
		return err
	}
	_ = os.Remove(tempPNG)
	return nil
}

func (s *Service) videoDurationMS(ctx context.Context, path string) (int64, error) {
	probe, err := s.probe(ctx, path)
	if err != nil {
		return 0, err
	}
	videoStream, _, _ := classifyStreams(probe.Streams)
	if videoStream.CodecName == "" {
		return 0, errors.New("未找到视频流")
	}
	return probeDurationMS(probe, videoStream), nil
}

func encodeJPEGCover(sourcePNG, destinationJPG string) error {
	return imageutil.EncodeJPEG(sourcePNG, destinationJPG, 80)
}

func (s *Service) markCoverCompleted(ctx context.Context, jobID int64, coverPath string) error {
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET cover_path = ?, cover_status = ?, cover_error = '', updated_at = ?
WHERE id = ?`, coverPath, CoverStatusCompleted, now, jobID)
	return err
}

func (s *Service) markCoverFailed(ctx context.Context, jobID int64, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET cover_path = '', cover_status = ?, cover_error = ?, updated_at = ?
WHERE id = ?`, CoverStatusFailed, message, now, jobID)
	return err
}

func (s *Service) runFFmpeg(ctx context.Context, jobID int64, plan mediaPlan) error {
	if err := os.MkdirAll(filepath.Dir(plan.outputPath), 0o755); err != nil {
		return err
	}
	tempPath := plan.outputPath + ".tmp.mp4"
	_ = os.Remove(tempPath)
	args := []string{
		"-hide_banner", "-nostats", "-loglevel", "warning", "-progress", "pipe:1",
		"-y", "-i", plan.sourcePath, "-map", "0:v:0", "-map", "0:a:0?",
	}
	switch plan.action {
	case "remux":
		args = append(args, "-c", "copy", "-movflags", "+faststart")
	default:
		if plan.hasExternalSubtitles {
			args = append(args, "-vf", "subtitles=filename="+ffmpegFilterPath(plan.subtitlePath))
		} else if plan.hasInternalSubtitles {
			args = append(args, "-vf", "subtitles=filename="+ffmpegFilterPath(plan.sourcePath)+":si=0")
		}
		args = append(args,
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-sn",
		)
	}
	args = append(args, tempPath)
	command := exec.CommandContext(ctx, s.ffmpegPath, args...)

	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动 ffmpeg 失败: %w", err)
	}

	stderrCh := make(chan string, 1)
	go func() {
		output, _ := io.ReadAll(stderr)
		stderrCh <- string(output)
	}()

	if plan.totalDurationMS > 0 {
		if err := s.updateProgress(ctx, jobID, 0, plan.totalDurationMS, false); err != nil {
			s.logger.Warn("更新转码进度失败", "source", "media", "media_job_id", jobID, "error", err)
		}
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	tracker := progressTracker{totalDurationMS: plan.totalDurationMS}
	for scanner.Scan() {
		update, ok := tracker.consume(scanner.Text(), s.now())
		if !ok {
			continue
		}
		if err := s.updateProgress(ctx, jobID, update.processedDurationMS, plan.totalDurationMS, update.force); err != nil {
			s.logger.Warn("更新转码进度失败", "source", "media", "media_job_id", jobID, "error", err)
		}
	}
	scanErr := scanner.Err()
	waitErr := command.Wait()
	stderrOutput := <-stderrCh
	if waitErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		message := strings.TrimSpace(stderrOutput)
		if message == "" {
			message = waitErr.Error()
		}
		if len(message) > 2000 {
			message = message[len(message)-2000:]
		}
		return fmt.Errorf("ffmpeg 失败: %s", message)
	}
	if scanErr != nil {
		return fmt.Errorf("读取 ffmpeg 进度失败: %w", scanErr)
	}

	if err := replaceFile(tempPath, plan.outputPath); err != nil {
		return err
	}
	return nil
}

type progressUpdate struct {
	processedDurationMS int64
	force               bool
}

type progressTracker struct {
	totalDurationMS     int64
	processedDurationMS int64
	lastWrite           time.Time
}

func (t *progressTracker) consume(line string, now time.Time) (progressUpdate, bool) {
	key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
	if !ok {
		return progressUpdate{}, false
	}
	switch key {
	case "out_time_us", "out_time_ms":
		if microseconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && microseconds >= 0 {
			t.processedDurationMS = microseconds / 1000
			return t.maybeUpdate(now, false)
		}
	case "out_time":
		if milliseconds := parseClockDurationMS(value); milliseconds >= 0 {
			t.processedDurationMS = milliseconds
			return t.maybeUpdate(now, false)
		}
	case "progress":
		if strings.TrimSpace(value) == "end" {
			if t.totalDurationMS > 0 && t.processedDurationMS < t.totalDurationMS {
				t.processedDurationMS = t.totalDurationMS
			}
			return progressUpdate{processedDurationMS: t.processedDurationMS, force: true}, true
		}
	}
	return progressUpdate{}, false
}

func (t *progressTracker) maybeUpdate(now time.Time, force bool) (progressUpdate, bool) {
	if !force && !t.lastWrite.IsZero() && now.Sub(t.lastWrite) < time.Second {
		return progressUpdate{}, false
	}
	t.lastWrite = now
	return progressUpdate{processedDurationMS: t.processedDurationMS, force: force}, true
}

func (s *Service) updateProgress(ctx context.Context, jobID int64, processedDurationMS, totalDurationMS int64, force bool) error {
	if processedDurationMS < 0 {
		processedDurationMS = 0
	}
	if totalDurationMS < 0 {
		totalDurationMS = 0
	}
	if totalDurationMS > 0 && processedDurationMS > totalDurationMS {
		processedDurationMS = totalDurationMS
	}
	progress := 0.0
	if totalDurationMS > 0 {
		progress = float64(processedDurationMS) / float64(totalDurationMS)
		if progress > 1 {
			progress = 1
		}
		if progress >= 1 && !force {
			progress = 0.999
		}
	}
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET progress = ?, processed_duration_ms = ?,
    total_duration_ms = CASE WHEN ? > 0 THEN ? ELSE total_duration_ms END,
    progress_updated_at = ?, updated_at = ?
WHERE id = ? AND status = ?`, progress, processedDurationMS, totalDurationMS, totalDurationMS, now, now, jobID, StatusTranscoding)
	return err
}

func (s *Service) persistPlan(ctx context.Context, jobID int64, plan mediaPlan) error {
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET source_path = ?, subtitle_path = ?, output_path = ?,
    cover_path = '', cover_status = ?, cover_error = '',
    video_codec = ?, audio_codec = ?,
    has_internal_subtitles = ?, has_external_subtitles = ?, needs_transcode = ?, action = ?,
    progress = 0, processed_duration_ms = 0, total_duration_ms = ?, progress_updated_at = NULL,
    updated_at = ?
WHERE id = ?`, plan.sourcePath, plan.subtitlePath, plan.outputPath, CoverStatusPending, plan.videoCodec, plan.audioCodec,
		plan.hasInternalSubtitles, plan.hasExternalSubtitles, plan.needsTranscode, plan.action,
		plan.totalDurationMS, now, jobID)
	return err
}

func (s *Service) markCompleted(ctx context.Context, jobID int64) error {
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET status = ?, progress = 1,
    processed_duration_ms = CASE WHEN total_duration_ms > 0 THEN total_duration_ms ELSE processed_duration_ms END,
    progress_updated_at = ?, error_message = '',
    completed_at = COALESCE(completed_at, ?), failed_at = NULL, updated_at = ?
WHERE id = ?`, StatusCompleted, now, now, now, jobID)
	return err
}

func (s *Service) recordCompletionWarning(ctx context.Context, jobID int64, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET error_message = ?, updated_at = ?
WHERE id = ? AND status = ?`, message, now, jobID, StatusCompleted)
	return err
}

func (s *Service) markFailed(ctx context.Context, jobID int64, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET status = ?, error_message = ?, failed_at = COALESCE(failed_at, ?), updated_at = ?
WHERE id = ?`, StatusFailed, message, now, now, jobID)
	return err
}

func findPrimaryVideo(root string) (mediaFile, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return mediaFile{}, errors.New("下载任务没有保存目录")
	}
	info, err := os.Stat(root)
	if err != nil {
		return mediaFile{}, fmt.Errorf("访问下载产物失败: %w", err)
	}
	if !info.IsDir() {
		if !isVideoFile(root) {
			return mediaFile{}, errors.New("下载产物不是可识别的视频文件")
		}
		return mediaFile{Path: root, Size: info.Size()}, nil
	}
	var best mediaFile
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isVideoFile(path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > best.Size {
			best = mediaFile{Path: path, Size: info.Size()}
		}
		return nil
	})
	if err != nil {
		return mediaFile{}, err
	}
	if best.Path == "" {
		return mediaFile{}, errors.New("下载目录中没有可识别的视频文件")
	}
	return best, nil
}

func findExternalSubtitle(videoPath string) string {
	dir := filepath.Dir(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	var fallback string
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isSubtitleFile(path) {
			return nil
		}
		if strings.EqualFold(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), base) {
			fallback = path
			return io.EOF
		}
		if fallback == "" {
			fallback = path
		}
		return nil
	})
	return fallback
}

func classifyStreams(streams []probeStream) (probeStream, probeStream, []probeStream) {
	var video probeStream
	var audio probeStream
	subtitles := make([]probeStream, 0)
	for _, stream := range streams {
		switch stream.CodecType {
		case "video":
			if video.CodecName == "" {
				video = stream
			}
		case "audio":
			if audio.CodecName == "" {
				audio = stream
			}
		case "subtitle":
			subtitles = append(subtitles, stream)
		}
	}
	return video, audio, subtitles
}

func probeDurationMS(probe probeResult, video probeStream) int64 {
	if milliseconds := parseSecondsDurationMS(probe.Format.Duration); milliseconds > 0 {
		return milliseconds
	}
	return parseSecondsDurationMS(video.Duration)
}

func parseSecondsDurationMS(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "N/A") {
		return 0
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return int64(seconds * 1000)
}

func parseClockDurationMS(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "N/A") {
		return -1
	}
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return -1
	}
	hours, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return -1
	}
	minutes, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return -1
	}
	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return -1
	}
	totalSeconds := float64(hours*3600+minutes*60) + seconds
	if totalSeconds < 0 {
		return -1
	}
	return int64(totalSeconds * 1000)
}

func isBrowserPlayable(path string, video, audio probeStream) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".mp4" && ext != ".m4v" {
		return false
	}
	if video.CodecName != "h264" {
		return false
	}
	if audio.CodecName != "" && audio.CodecName != "aac" {
		return false
	}
	if video.PixFmt != "" && video.PixFmt != "yuv420p" && video.PixFmt != "yuvj420p" {
		return false
	}
	return true
}

func finalOutputPath(mediaDir string, job pendingJob) string {
	animeName := safePathSegment(job.AnimeName)
	seasonFolder := seasonFolderName(job)
	fileName := finalFileName(job)
	return filepath.Join(mediaDir, animeName, seasonFolder, fileName)
}

func storagePathForNames(root, nameCN, name string) string {
	displayName := nameCN
	if strings.TrimSpace(displayName) == "" {
		displayName = name
	}
	return filepath.Join(root, safePathSegment(displayName))
}

func relativePath(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return rel, true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return rel, true
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !info.IsDir(), nil
}

func coverPathForOutput(outputPath string) string {
	ext := filepath.Ext(outputPath)
	if ext == "" {
		return outputPath + ".jpg"
	}
	return strings.TrimSuffix(outputPath, ext) + ".jpg"
}

func seasonFolderName(job pendingJob) string {
	episodeType := strings.ToLower(strings.TrimSpace(job.EpisodeType))
	if episodeType == "" || episodeType == "episode" {
		return fmt.Sprintf("Season %d", job.SeasonNumber)
	}
	switch episodeType {
	case "special":
		return "SP"
	default:
		return strings.ToUpper(episodeType)
	}
}

func finalFileName(job pendingJob) string {
	name := safePathSegment(job.AnimeName)
	episodeType := strings.ToLower(strings.TrimSpace(job.EpisodeType))
	if episodeType == "" || episodeType == "episode" {
		return safePathSegment(fmt.Sprintf("%s S%02dE%s", name, job.SeasonNumber, paddedEpisode(job.EpisodeNumber))) + ".mp4"
	}
	label := strings.ToUpper(episodeType)
	if label == "SPECIAL" {
		label = "SP"
	}
	return safePathSegment(fmt.Sprintf("%s %s%s", name, label, paddedEpisode(job.EpisodeNumber))) + ".mp4"
}

func paddedEpisode(value string) string {
	value = strings.TrimSpace(value)
	number, err := strconv.Atoi(value)
	if err != nil || number >= 100 || number < 0 {
		return value
	}
	return fmt.Sprintf("%02d", number)
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
		value = "media"
	}
	runes := []rune(value)
	if len(runes) > 120 {
		value = string(runes[:120])
	}
	return value
}

func normalizeEpisodeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "episode"
	}
	if value == "special" {
		return "sp"
	}
	return value
}

func isVideoFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".mkv", ".mov", ".avi", ".wmv", ".flv", ".ts", ".m2ts", ".webm":
		return true
	default:
		return false
	}
}

func isSubtitleFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ass", ".ssa", ".srt", ".vtt":
		return true
	default:
		return false
	}
}

func ffmpegFilterPath(path string) string {
	value := filepath.ToSlash(path)
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	value = strings.ReplaceAll(value, `:`, `\:`)
	return "'" + value + "'"
}

func copyToFinal(source, destination string) error {
	sourceAbs, _ := filepath.Abs(source)
	destAbs, _ := filepath.Abs(destination)
	if strings.EqualFold(filepath.Clean(sourceAbs), filepath.Clean(destAbs)) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	tempPath := destination + ".tmp"
	_ = os.Remove(tempPath)
	if err := copyFile(source, tempPath); err != nil {
		return err
	}
	if err := replaceFile(tempPath, destination); err != nil {
		return err
	}
	return nil
}

func moveDirectory(source, destination string) error {
	if samePath(source, destination) {
		return nil
	}
	if rel, ok := relativePath(source, destination); ok && rel != "." {
		return ErrStorageTargetConflict
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(destination); err == nil {
		if !info.IsDir() {
			return ErrStorageTargetConflict
		}
		if err := copyDirectoryContents(source, destination); err != nil {
			return err
		}
		return os.RemoveAll(source)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(source, destination); err == nil {
		return nil
	}
	if err := copyDirectory(source, destination); err != nil {
		return err
	}
	return os.RemoveAll(source)
}

func copyDirectory(source, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	return copyDirectoryContents(source, destination)
}

func copyDirectoryContents(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(destination, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if info.Mode()&os.ModeType != 0 {
			return fmt.Errorf("不支持移动特殊文件: %s", filepath.Base(path))
		}
		return copyFileNoOverwrite(path, target, info.Mode())
	})
}

func copyFileNoOverwrite(source, destination string, mode os.FileMode) error {
	if _, err := os.Stat(destination); err == nil {
		return ErrStorageTargetConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func moveFileNoOverwrite(source, destination string) error {
	if samePath(source, destination) {
		return nil
	}
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return ErrStorageTargetConflict
	}
	if _, err := os.Stat(destination); err == nil {
		return ErrStorageTargetConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err == nil {
		return nil
	}
	if err := copyFileNoOverwrite(source, destination, info.Mode()); err != nil {
		return err
	}
	return os.Remove(source)
}

func replaceFile(source, destination string) error {
	_ = os.Remove(destination)
	if err := os.Rename(source, destination); err != nil {
		return err
	}
	return nil
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func normalizeStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "":
		return ""
	case StatusPending, StatusTranscoding, StatusCompleted, StatusFailed:
		return status
	default:
		return "invalid"
	}
}

func mediaJobWhere(status string) (string, []any) {
	where := `
FROM media_jobs mj
JOIN subscription_items si ON si.id = mj.subscription_item_id`
	args := make([]any, 0, 1)
	if status != "" {
		where += "\nWHERE mj.status = ?"
		args = append(args, status)
	}
	return where, args
}

const mediaJobSelect = `
SELECT mj.id, mj.download_job_id, mj.subscription_item_id, si.title, mj.bangumi_id,
       mj.anime_name, mj.season_number, mj.episode_type, mj.episode_number, mj.status,
       mj.source_path, mj.subtitle_path, mj.output_path, mj.cover_path, mj.cover_status, mj.cover_error,
       mj.video_codec, mj.audio_codec,
       mj.has_internal_subtitles, mj.has_external_subtitles, mj.needs_transcode,
       mj.action, mj.progress, mj.processed_duration_ms, mj.total_duration_ms,
       mj.error_message, mj.progress_updated_at, mj.started_at, mj.completed_at, mj.failed_at,
       mj.created_at, mj.updated_at
`

const pendingJobSelect = `
SELECT mj.id, mj.download_job_id, mj.subscription_item_id, mj.bangumi_id,
       mj.anime_name, mj.season_number, mj.episode_type, mj.episode_number, dj.save_path,
       COALESCE(NULLIF(am.media_storage_root, ''), ?),
       mj.source_path, mj.subtitle_path, mj.output_path, mj.video_codec, mj.audio_codec,
       mj.has_internal_subtitles, mj.has_external_subtitles, mj.needs_transcode,
       mj.action, mj.total_duration_ms
`

func scanJob(row interface{ Scan(dest ...any) error }) (Job, error) {
	var job Job
	var progressUpdatedAt, startedAt, completedAt, failedAt sql.NullInt64
	if err := row.Scan(
		&job.ID, &job.DownloadJobID, &job.SubscriptionItemID, &job.Title, &job.BangumiID,
		&job.AnimeName, &job.SeasonNumber, &job.EpisodeType, &job.EpisodeNumber, &job.Status,
		&job.SourceFile, &job.SubtitleFile, &job.OutputFile, &job.CoverFile, &job.CoverStatus, &job.CoverError,
		&job.VideoCodec, &job.AudioCodec,
		&job.HasInternalSubtitles, &job.HasExternalSubtitles, &job.NeedsTranscode,
		&job.Action, &job.Progress, &job.ProcessedDurationMS, &job.TotalDurationMS,
		&job.ErrorMessage, &progressUpdatedAt, &startedAt, &completedAt, &failedAt,
		&job.CreatedAt, &job.UpdatedAt,
	); err != nil {
		return Job{}, err
	}
	job.SourceFile = baseName(job.SourceFile)
	job.SubtitleFile = baseName(job.SubtitleFile)
	job.OutputFile = baseName(job.OutputFile)
	job.CoverFile = baseName(job.CoverFile)
	job.ProgressUpdatedAt = nullableInt64(progressUpdatedAt)
	job.StartedAt = nullableInt64(startedAt)
	job.CompletedAt = nullableInt64(completedAt)
	job.FailedAt = nullableInt64(failedAt)
	return job, nil
}

func scanPendingJob(row interface{ Scan(dest ...any) error }) (pendingJob, error) {
	var job pendingJob
	if err := row.Scan(
		&job.ID, &job.DownloadJobID, &job.SubscriptionItemID, &job.BangumiID,
		&job.AnimeName, &job.SeasonNumber, &job.EpisodeType, &job.EpisodeNumber,
		&job.SavePath, &job.StorageRoot, &job.SourcePath, &job.SubtitlePath,
		&job.OutputPath, &job.VideoCodec, &job.AudioCodec, &job.HasInternalSubtitles,
		&job.HasExternalSubtitles, &job.NeedsTranscode, &job.Action, &job.TotalDurationMS,
	); err != nil {
		return pendingJob{}, err
	}
	return job, nil
}

func baseName(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Base(path)
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
