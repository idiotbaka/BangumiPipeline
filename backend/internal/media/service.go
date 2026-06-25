package media

import (
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
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	TaskKey = "process-downloaded-media"

	StatusPending     = "pending"
	StatusTranscoding = "transcoding"
	StatusCompleted   = "completed"
	StatusFailed      = "failed"

	downloadStatusCompleted = "completed"
)

var (
	ErrInvalidStatus    = errors.New("invalid media status")
	ErrMediaJobNotFound = errors.New("media job not found")
	ErrRetryNotAllowed  = errors.New("media job retry not allowed")
)

type Config struct {
	MediaDir        string
	FFmpegPath      string
	FFprobePath     string
	DownloadCleaner DownloadCleaner
}

type DownloadCleaner interface {
	CleanupCompletedQBitTask(context.Context, int64) error
}

type Service struct {
	db          *sql.DB
	logger      *slog.Logger
	mediaDir    string
	ffmpegPath  string
	ffprobePath string
	cleaner     DownloadCleaner
	now         func() time.Time
}

type JobPage struct {
	Items    []Job `json:"items"`
	Total    int   `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

type Job struct {
	ID                   int64  `json:"id"`
	DownloadJobID        int64  `json:"downloadJobId"`
	SubscriptionItemID   int64  `json:"subscriptionItemId"`
	Title                string `json:"title"`
	BangumiID            int64  `json:"bangumiId"`
	AnimeName            string `json:"animeName"`
	SeasonNumber         int    `json:"seasonNumber"`
	EpisodeType          string `json:"episodeType"`
	EpisodeNumber        string `json:"episodeNumber"`
	Status               string `json:"status"`
	SourceFile           string `json:"sourceFile"`
	SubtitleFile         string `json:"subtitleFile"`
	OutputFile           string `json:"outputFile"`
	VideoCodec           string `json:"videoCodec"`
	AudioCodec           string `json:"audioCodec"`
	HasInternalSubtitles bool   `json:"hasInternalSubtitles"`
	HasExternalSubtitles bool   `json:"hasExternalSubtitles"`
	NeedsTranscode       bool   `json:"needsTranscode"`
	Action               string `json:"action"`
	ErrorMessage         string `json:"errorMessage"`
	StartedAt            *int64 `json:"startedAt"`
	CompletedAt          *int64 `json:"completedAt"`
	FailedAt             *int64 `json:"failedAt"`
	CreatedAt            int64  `json:"createdAt"`
	UpdatedAt            int64  `json:"updatedAt"`
}

type pendingJob struct {
	ID                 int64
	DownloadJobID      int64
	SubscriptionItemID int64
	BangumiID          int64
	AnimeName          string
	SeasonNumber       int
	EpisodeType        string
	EpisodeNumber      string
	SavePath           string
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
}

type probeFormat struct {
	FormatName string `json:"format_name"`
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
		cleaner: config.DownloadCleaner, now: time.Now,
	}
}

func (s *Service) Execute(ctx context.Context) error {
	s.logger.Info("媒体处理任务开始", "source", "media")
	if _, err := s.EnqueueCompletedDownloads(ctx); err != nil {
		return fmt.Errorf("创建待处理媒体任务: %w", err)
	}
	if err := s.recoverInterruptedJobs(ctx); err != nil {
		return fmt.Errorf("恢复中断媒体任务: %w", err)
	}
	job, ok, err := s.nextPendingJob(ctx)
	if err != nil {
		return err
	}
	if !ok {
		s.logger.Info("媒体处理任务完成：没有待处理视频", "source", "media")
		return nil
	}
	if err := s.processJob(ctx, job); err != nil {
		return err
	}
	s.logger.Info("媒体处理任务完成", "source", "media", "media_job_id", job.ID)
	return nil
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

func (s *Service) RetryFailedJob(ctx context.Context, jobID int64) (Job, error) {
	now := s.now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET status = ?, source_path = '', subtitle_path = '', output_path = '',
    video_codec = '', audio_codec = '', has_internal_subtitles = 0,
    has_external_subtitles = 0, needs_transcode = 0, action = '',
    error_message = '', started_at = NULL, completed_at = NULL, failed_at = NULL,
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
SET status = ?, error_message = '上次处理被中断，已重新排队', started_at = NULL, updated_at = ?
WHERE status = ?`, StatusPending, now, StatusTranscoding)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		s.logger.Warn("媒体处理任务恢复中断任务", "source", "media", "count", affected)
	}
	return nil
}

func (s *Service) nextPendingJob(ctx context.Context) (pendingJob, bool, error) {
	var job pendingJob
	err := s.db.QueryRowContext(ctx, `
SELECT mj.id, mj.download_job_id, mj.subscription_item_id, mj.bangumi_id, mj.anime_name,
       mj.season_number, mj.episode_type, mj.episode_number, dj.save_path
FROM media_jobs mj
JOIN download_jobs dj ON dj.id = mj.download_job_id
WHERE mj.status = ?
ORDER BY mj.created_at, mj.id
LIMIT 1`, StatusPending).Scan(
		&job.ID, &job.DownloadJobID, &job.SubscriptionItemID, &job.BangumiID,
		&job.AnimeName, &job.SeasonNumber, &job.EpisodeType, &job.EpisodeNumber, &job.SavePath,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return pendingJob{}, false, nil
	}
	if err != nil {
		return pendingJob{}, false, err
	}
	return job, true, nil
}

func (s *Service) processJob(ctx context.Context, job pendingJob) error {
	now := s.now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET status = ?, error_message = '', started_at = COALESCE(started_at, ?), failed_at = NULL, updated_at = ?
WHERE id = ? AND status = ?`, StatusTranscoding, now, now, job.ID, StatusPending); err != nil {
		return err
	}

	plan, err := s.planJob(ctx, job)
	if err != nil {
		_ = s.markFailed(ctx, job.ID, err.Error())
		return nil
	}
	if err := s.persistPlan(ctx, job.ID, plan); err != nil {
		return err
	}

	s.logger.Info("媒体处理开始", "source", "media", "media_job_id", job.ID, "action", plan.action, "source_file", filepath.Base(plan.sourcePath))
	if plan.action == "copy" {
		err = copyToFinal(plan.sourcePath, plan.outputPath)
	} else {
		err = s.runFFmpeg(ctx, plan)
	}
	if err != nil {
		_ = s.markFailed(ctx, job.ID, err.Error())
		s.logger.Error("媒体处理失败", "source", "media", "media_job_id", job.ID, "action", plan.action, "error", err)
		return nil
	}
	if err := s.markCompleted(ctx, job.ID); err != nil {
		return err
	}
	if err := s.cleanupDownload(ctx, job); err != nil {
		message := "最终产物已完成，但 qBittorrent 下载清理失败: " + err.Error()
		_ = s.recordCompletionWarning(ctx, job.ID, message)
		s.logger.Warn("媒体处理完成后清理 qBittorrent 下载失败", "source", "media", "media_job_id", job.ID, "download_job_id", job.DownloadJobID, "error", err)
	} else {
		s.logger.Info("媒体处理完成后 qBittorrent 下载已清理", "source", "media", "media_job_id", job.ID, "download_job_id", job.DownloadJobID)
	}
	s.logger.Info("媒体处理成功", "source", "media", "media_job_id", job.ID, "action", plan.action, "output_file", filepath.Base(plan.outputPath))
	return nil
}

func (s *Service) cleanupDownload(ctx context.Context, job pendingJob) error {
	if s.cleaner == nil {
		return nil
	}
	return s.cleaner.CleanupCompletedQBitTask(ctx, job.DownloadJobID)
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
	subtitlePath := findExternalSubtitle(video.Path)
	outputPath := finalOutputPath(s.mediaDir, job)
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
		needsTranscode: needsTranscode, action: action,
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

func (s *Service) runFFmpeg(ctx context.Context, plan mediaPlan) error {
	if err := os.MkdirAll(filepath.Dir(plan.outputPath), 0o755); err != nil {
		return err
	}
	tempPath := plan.outputPath + ".tmp.mp4"
	_ = os.Remove(tempPath)
	args := []string{"-y", "-i", plan.sourcePath, "-map", "0:v:0", "-map", "0:a:0?"}
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
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 2000 {
			message = message[len(message)-2000:]
		}
		return fmt.Errorf("ffmpeg 失败: %s", message)
	}
	if err := replaceFile(tempPath, plan.outputPath); err != nil {
		return err
	}
	return nil
}

func (s *Service) persistPlan(ctx context.Context, jobID int64, plan mediaPlan) error {
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET source_path = ?, subtitle_path = ?, output_path = ?, video_codec = ?, audio_codec = ?,
    has_internal_subtitles = ?, has_external_subtitles = ?, needs_transcode = ?, action = ?,
    updated_at = ?
WHERE id = ?`, plan.sourcePath, plan.subtitlePath, plan.outputPath, plan.videoCodec, plan.audioCodec,
		plan.hasInternalSubtitles, plan.hasExternalSubtitles, plan.needsTranscode, plan.action, now, jobID)
	return err
}

func (s *Service) markCompleted(ctx context.Context, jobID int64) error {
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE media_jobs
SET status = ?, error_message = '', completed_at = COALESCE(completed_at, ?), failed_at = NULL, updated_at = ?
WHERE id = ?`, StatusCompleted, now, now, jobID)
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
       mj.source_path, mj.subtitle_path, mj.output_path, mj.video_codec, mj.audio_codec,
       mj.has_internal_subtitles, mj.has_external_subtitles, mj.needs_transcode,
       mj.action, mj.error_message, mj.started_at, mj.completed_at, mj.failed_at,
       mj.created_at, mj.updated_at
`

func scanJob(row interface{ Scan(dest ...any) error }) (Job, error) {
	var job Job
	var startedAt, completedAt, failedAt sql.NullInt64
	if err := row.Scan(
		&job.ID, &job.DownloadJobID, &job.SubscriptionItemID, &job.Title, &job.BangumiID,
		&job.AnimeName, &job.SeasonNumber, &job.EpisodeType, &job.EpisodeNumber, &job.Status,
		&job.SourceFile, &job.SubtitleFile, &job.OutputFile, &job.VideoCodec, &job.AudioCodec,
		&job.HasInternalSubtitles, &job.HasExternalSubtitles, &job.NeedsTranscode,
		&job.Action, &job.ErrorMessage, &startedAt, &completedAt, &failedAt,
		&job.CreatedAt, &job.UpdatedAt,
	); err != nil {
		return Job{}, err
	}
	job.SourceFile = baseName(job.SourceFile)
	job.SubtitleFile = baseName(job.SubtitleFile)
	job.OutputFile = baseName(job.OutputFile)
	job.StartedAt = nullableInt64(startedAt)
	job.CompletedAt = nullableInt64(completedAt)
	job.FailedAt = nullableInt64(failedAt)
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
