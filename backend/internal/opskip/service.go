package opskip

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/bits"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	TaskKey = "detect-media-openings"

	StatusPending  = "pending"
	StatusDetected = "detected"
	StatusNotFound = "not_found"
	StatusFailed   = "failed"

	minimumGroupEpisodes          = 3
	analysisPercent               = 0.25
	analysisLengthLimitSeconds    = 10 * 60
	minimumOpeningDurationSeconds = 15
	maximumOpeningDurationSeconds = 4 * 60

	samplesToSeconds                    = 0.128
	maximumFingerprintPointDifferences  = 6
	invertedIndexShift                  = 2
	maximumTimeSkipSeconds              = 3.5
	silenceDetectionMaximumNoiseDB      = -50
	silenceDetectionMinimumDurationSecs = 0.33
	maxFingerprintPointValue            = int64(1<<32 - 1)

	chromaprintVersion = "ffmpeg-chromaprint-raw-v1"
)

var silenceDetectionPattern = regexp.MustCompile(`silence_(start|end): ([0-9.]+)`)

type Config struct {
	FFmpegPath  string
	FFprobePath string
}

type Service struct {
	db          *sql.DB
	logger      *slog.Logger
	ffmpegPath  string
	ffprobePath string
	now         func() time.Time
}

type mediaEpisode struct {
	MediaJobID       int64
	BangumiID        int64
	AnimeName        string
	SeasonNumber     int
	EpisodeNumber    string
	OutputPath       string
	TotalDurationMS  int64
	CompletedAt      int64
	Fingerprint      []uint32
	FingerprintError error
}

type groupKey struct {
	BangumiID    int64
	SeasonNumber int
}

type analysisGroup struct {
	key      groupKey
	anime    string
	episodes []mediaEpisode
}

type taskSummary struct {
	groups     int
	episodes   int
	detected   int
	notFound   int
	failed     int
	cached     int
	generated  int
	silenceHit int
}

type groupSummary struct {
	detected   int
	notFound   int
	failed     int
	cached     int
	generated  int
	silenceHit int
}

type fingerprintResult struct {
	points    []uint32
	duration  float64
	end       float64
	fromCache bool
}

type openingDetection struct {
	Start          float64
	End            float64
	Confidence     float64
	MatchedMediaID int64
}

type timeRange struct {
	Start float64
	End   float64
}

func (r timeRange) Duration() float64 {
	return r.End - r.Start
}

func (r timeRange) intersects(other timeRange) bool {
	return (r.Start < other.Start && other.Start < r.End) ||
		(r.Start < other.End && other.End < r.End)
}

type probeResult struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeStream struct {
	CodecType string `json:"codec_type"`
	Duration  string `json:"duration"`
}

type probeFormat struct {
	Duration string `json:"duration"`
}

func NewService(db *sql.DB, logger *slog.Logger, config Config) *Service {
	ffmpegPath := strings.TrimSpace(config.FFmpegPath)
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	ffprobePath := strings.TrimSpace(config.FFprobePath)
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	return &Service{
		db: db, logger: logger, ffmpegPath: ffmpegPath, ffprobePath: ffprobePath, now: time.Now,
	}
}

func (s *Service) Execute(ctx context.Context) error {
	s.logger.Info("片头识别任务开始", "source", "opskip")
	if err := s.checkFFmpeg(ctx); err != nil {
		return err
	}
	groups, err := s.completedEpisodeGroups(ctx)
	if err != nil {
		return fmt.Errorf("读取待识别成品视频: %w", err)
	}
	if len(groups) == 0 {
		s.logger.Info("片头识别任务完成：没有大于 2 集的正片成品视频", "source", "opskip")
		return nil
	}

	summary := taskSummary{groups: len(groups)}
	for _, group := range groups {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		result, err := s.analyzeGroup(ctx, group)
		if err != nil {
			return err
		}
		summary.episodes += len(group.episodes)
		summary.detected += result.detected
		summary.notFound += result.notFound
		summary.failed += result.failed
		summary.cached += result.cached
		summary.generated += result.generated
		summary.silenceHit += result.silenceHit
	}

	s.logger.Info("片头识别任务完成", "source", "opskip",
		"groups", summary.groups, "episodes", summary.episodes,
		"detected", summary.detected, "not_found", summary.notFound, "failed", summary.failed,
		"fingerprint_cached", summary.cached, "fingerprint_generated", summary.generated,
		"silence_adjusted", summary.silenceHit)
	return nil
}

func (s *Service) completedEpisodeGroups(ctx context.Context) ([]analysisGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT mj.id, mj.bangumi_id, mj.anime_name, mj.season_number,
       mj.episode_number, mj.output_path, mj.total_duration_ms,
       COALESCE(mj.completed_at, mj.updated_at, mj.created_at, 0)
FROM media_jobs mj
JOIN anime_metadata am ON am.bangumi_id = mj.bangumi_id
WHERE am.deleted_at IS NULL
  AND mj.status = 'completed'
  AND mj.output_path != ''
  AND LOWER(COALESCE(NULLIF(mj.episode_type, ''), 'episode')) = 'episode'
ORDER BY mj.bangumi_id, mj.season_number,
         CASE WHEN mj.episode_number GLOB '[0-9]*' THEN 0 ELSE 1 END,
         CAST(mj.episode_number AS REAL),
         mj.episode_number,
         mj.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groupIndex := make(map[groupKey]int)
	groups := make([]analysisGroup, 0)
	for rows.Next() {
		var episode mediaEpisode
		if err := rows.Scan(
			&episode.MediaJobID, &episode.BangumiID, &episode.AnimeName, &episode.SeasonNumber,
			&episode.EpisodeNumber, &episode.OutputPath, &episode.TotalDurationMS, &episode.CompletedAt,
		); err != nil {
			return nil, err
		}
		key := groupKey{BangumiID: episode.BangumiID, SeasonNumber: episode.SeasonNumber}
		index, ok := groupIndex[key]
		if !ok {
			index = len(groups)
			groupIndex[key] = index
			groups = append(groups, analysisGroup{key: key, anime: episode.AnimeName})
		}
		groups[index].episodes = append(groups[index].episodes, episode)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	filtered := groups[:0]
	for _, group := range groups {
		if len(group.episodes) < minimumGroupEpisodes {
			continue
		}
		filtered = append(filtered, group)
	}
	return filtered, nil
}

func (s *Service) analyzeGroup(ctx context.Context, group analysisGroup) (groupSummary, error) {
	s.logger.Info("开始识别番剧片头", "source", "opskip",
		"bangumi_id", group.key.BangumiID, "anime", group.anime,
		"season", group.key.SeasonNumber, "episodes", len(group.episodes))

	result := groupSummary{}
	valid := make([]mediaEpisode, 0, len(group.episodes))
	for index := range group.episodes {
		episode := group.episodes[index]
		fingerprint, err := s.fingerprint(ctx, episode)
		if err != nil {
			episode.FingerprintError = err
			if saveErr := s.saveSegment(ctx, episode, StatusFailed, openingDetection{}, len(group.episodes), err.Error()); saveErr != nil {
				return result, saveErr
			}
			result.failed++
			s.logger.Warn("片头识别指纹生成失败", "source", "opskip",
				"media_job_id", episode.MediaJobID, "bangumi_id", episode.BangumiID,
				"episode", episode.EpisodeNumber, "error", err)
			continue
		}
		if fingerprint.fromCache {
			result.cached++
		} else {
			result.generated++
		}
		episode.Fingerprint = fingerprint.points
		valid = append(valid, episode)
	}

	detections := detectOpenings(valid)
	for _, episode := range valid {
		detection, ok := detections[episode.MediaJobID]
		if !ok {
			if err := s.saveSegment(ctx, episode, StatusNotFound, openingDetection{}, len(group.episodes), ""); err != nil {
				return result, err
			}
			result.notFound++
			continue
		}
		adjusted, changed, err := s.adjustOpeningEndToSilence(ctx, episode, detection)
		if err != nil {
			s.logger.Warn("片头结束静音修正失败，将使用音频指纹结果", "source", "opskip",
				"media_job_id", episode.MediaJobID, "bangumi_id", episode.BangumiID,
				"episode", episode.EpisodeNumber, "error", err)
		} else if changed {
			detection = adjusted
			result.silenceHit++
		}
		if !validOpeningRange(detection.Start, detection.End) {
			if err := s.saveSegment(ctx, episode, StatusNotFound, openingDetection{}, len(group.episodes), ""); err != nil {
				return result, err
			}
			result.notFound++
			continue
		}
		if err := s.saveSegment(ctx, episode, StatusDetected, detection, len(group.episodes), ""); err != nil {
			return result, err
		}
		result.detected++
	}

	s.logger.Info("番剧片头识别完成", "source", "opskip",
		"bangumi_id", group.key.BangumiID, "anime", group.anime,
		"season", group.key.SeasonNumber, "detected", result.detected,
		"not_found", result.notFound, "failed", result.failed,
		"fingerprint_cached", result.cached, "fingerprint_generated", result.generated,
		"silence_adjusted", result.silenceHit)
	return result, nil
}

func detectOpenings(episodes []mediaEpisode) map[int64]openingDetection {
	detections := make(map[int64]openingDetection)
	for leftIndex := 0; leftIndex < len(episodes); leftIndex++ {
		for rightIndex := leftIndex + 1; rightIndex < len(episodes); rightIndex++ {
			left := episodes[leftIndex]
			right := episodes[rightIndex]
			leftRange, rightRange, ok := compareFingerprints(left.Fingerprint, right.Fingerprint)
			if !ok || !validOpeningRange(leftRange.Start, leftRange.End) || !validOpeningRange(rightRange.Start, rightRange.End) {
				continue
			}
			saveBestDetection(detections, left.MediaJobID, openingDetection{
				Start: leftRange.Start, End: leftRange.End,
				Confidence: leftRange.Duration(), MatchedMediaID: right.MediaJobID,
			})
			saveBestDetection(detections, right.MediaJobID, openingDetection{
				Start: rightRange.Start, End: rightRange.End,
				Confidence: rightRange.Duration(), MatchedMediaID: left.MediaJobID,
			})
		}
	}
	return detections
}

func saveBestDetection(detections map[int64]openingDetection, mediaJobID int64, candidate openingDetection) {
	if saved, ok := detections[mediaJobID]; !ok || candidate.Confidence > saved.Confidence {
		detections[mediaJobID] = candidate
	}
}

func compareFingerprints(left, right []uint32) (timeRange, timeRange, bool) {
	leftRanges, rightRanges := searchInvertedIndex(left, right)
	if len(leftRanges) == 0 || len(rightRanges) == 0 {
		return timeRange{}, timeRange{}, false
	}
	sort.Slice(leftRanges, func(i, j int) bool { return leftRanges[i].Duration() > leftRanges[j].Duration() })
	sort.Slice(rightRanges, func(i, j int) bool { return rightRanges[i].Duration() > rightRanges[j].Duration() })
	leftIntro := leftRanges[0]
	rightIntro := rightRanges[0]
	if leftIntro.Start <= 5 {
		leftIntro.Start = 0
	}
	if rightIntro.Start <= 5 {
		rightIntro.Start = 0
	}
	return leftIntro, rightIntro, true
}

func searchInvertedIndex(left, right []uint32) ([]timeRange, []timeRange) {
	leftRanges := make([]timeRange, 0)
	rightRanges := make([]timeRange, 0)
	leftIndex := createInvertedIndex(left)
	rightIndex := createInvertedIndex(right)
	shifts := make(map[int]struct{})

	for point, leftPosition := range leftIndex {
		for offset := -invertedIndexShift; offset <= invertedIndexShift; offset++ {
			modified := int64(point) + int64(offset)
			if modified < 0 || modified > maxFingerprintPointValue {
				continue
			}
			rightPosition, ok := rightIndex[uint32(modified)]
			if !ok {
				continue
			}
			shifts[rightPosition-leftPosition] = struct{}{}
		}
	}

	for shift := range shifts {
		leftRange, rightRange, ok := findContiguous(left, right, shift)
		if ok {
			leftRanges = append(leftRanges, leftRange)
			rightRanges = append(rightRanges, rightRange)
		}
	}
	return leftRanges, rightRanges
}

func createInvertedIndex(fingerprint []uint32) map[uint32]int {
	index := make(map[uint32]int, len(fingerprint))
	for position, point := range fingerprint {
		index[point] = position
	}
	return index
}

func findContiguous(left, right []uint32, shiftAmount int) (timeRange, timeRange, bool) {
	leftOffset := 0
	rightOffset := 0
	if shiftAmount < 0 {
		leftOffset -= shiftAmount
	} else {
		rightOffset += shiftAmount
	}
	upperLimit := min(len(left), len(right)) - abs(shiftAmount)
	if upperLimit <= 0 {
		return timeRange{}, timeRange{}, false
	}

	leftTimes := make([]float64, 0)
	rightTimes := make([]float64, 0)
	for index := 0; index < upperLimit; index++ {
		leftPosition := index + leftOffset
		rightPosition := index + rightOffset
		diff := left[leftPosition] ^ right[rightPosition]
		if bits.OnesCount32(diff) > maximumFingerprintPointDifferences {
			continue
		}
		leftTimes = append(leftTimes, float64(leftPosition)*samplesToSeconds)
		rightTimes = append(rightTimes, float64(rightPosition)*samplesToSeconds)
	}
	leftTimes = append(leftTimes, math.MaxFloat64)
	rightTimes = append(rightTimes, math.MaxFloat64)

	leftContiguous, ok := longestContiguousRange(leftTimes, maximumTimeSkipSeconds)
	if !ok || leftContiguous.Duration() < minimumOpeningDurationSeconds {
		return timeRange{}, timeRange{}, false
	}
	rightContiguous, ok := longestContiguousRange(rightTimes, maximumTimeSkipSeconds)
	if !ok {
		return timeRange{}, timeRange{}, false
	}

	if leftContiguous.Duration() >= 90 {
		leftContiguous.End -= 2 * maximumTimeSkipSeconds
		rightContiguous.End -= 2 * maximumTimeSkipSeconds
	} else if leftContiguous.Duration() >= 30 {
		leftContiguous.End -= maximumTimeSkipSeconds
		rightContiguous.End -= maximumTimeSkipSeconds
	}

	return leftContiguous, rightContiguous, true
}

func longestContiguousRange(times []float64, maximumDistance float64) (timeRange, bool) {
	if len(times) == 0 {
		return timeRange{}, false
	}
	sort.Float64s(times)
	ranges := make([]timeRange, 0)
	current := timeRange{Start: times[0], End: times[0]}
	for index := 0; index < len(times)-1; index++ {
		currentTime := times[index]
		next := times[index+1]
		if next-currentTime <= maximumDistance {
			current.End = next
			continue
		}
		ranges = append(ranges, current)
		current = timeRange{Start: next, End: next}
	}
	if len(ranges) == 0 {
		return timeRange{}, false
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Duration() > ranges[j].Duration() })
	return ranges[0], true
}

func validOpeningRange(start, end float64) bool {
	duration := end - start
	return start >= 0 && end > start &&
		duration >= minimumOpeningDurationSeconds &&
		duration <= maximumOpeningDurationSeconds
}

func (s *Service) fingerprint(ctx context.Context, episode mediaEpisode) (fingerprintResult, error) {
	info, err := os.Stat(episode.OutputPath)
	if err != nil {
		return fingerprintResult{}, fmt.Errorf("访问成品视频失败: %w", err)
	}
	fileSize := info.Size()
	fileMTime := info.ModTime().UTC().Unix()
	if cached, ok, err := s.cachedFingerprint(ctx, episode.MediaJobID, fileSize, fileMTime); err != nil {
		return fingerprintResult{}, err
	} else if ok {
		return cached, nil
	}

	duration := float64(episode.TotalDurationMS) / 1000
	if duration <= 0 {
		duration, err = s.probeDuration(ctx, episode.OutputPath)
		if err != nil {
			return fingerprintResult{}, err
		}
	}
	if duration <= 0 {
		return fingerprintResult{}, errors.New("无法获取视频总时长")
	}
	fingerprintEnd := math.Min(duration*analysisPercent, analysisLengthLimitSeconds)
	if fingerprintEnd <= 0 {
		return fingerprintResult{}, errors.New("片头分析窗口无效")
	}

	points, err := s.runFingerprint(ctx, episode.OutputPath, fingerprintEnd)
	if err != nil {
		return fingerprintResult{}, err
	}
	if err := s.saveFingerprint(ctx, episode.MediaJobID, fileSize, fileMTime, duration, fingerprintEnd, points); err != nil {
		return fingerprintResult{}, err
	}
	return fingerprintResult{points: points, duration: duration, end: fingerprintEnd}, nil
}

func (s *Service) cachedFingerprint(ctx context.Context, mediaJobID, fileSize, fileMTime int64) (fingerprintResult, bool, error) {
	var cachedSize, cachedMTime int64
	var duration, fingerprintEnd float64
	var blob []byte
	err := s.db.QueryRowContext(ctx, `
SELECT file_size, file_mtime, duration_seconds, fingerprint_end_seconds, fingerprint_points
FROM media_op_fingerprints
WHERE media_job_id = ? AND chromaprint_version = ?`, mediaJobID, chromaprintVersion).Scan(
		&cachedSize, &cachedMTime, &duration, &fingerprintEnd, &blob,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return fingerprintResult{}, false, nil
	}
	if err != nil {
		return fingerprintResult{}, false, err
	}
	if cachedSize != fileSize || cachedMTime != fileMTime || len(blob) == 0 {
		return fingerprintResult{}, false, nil
	}
	points, err := decodeFingerprintPoints(blob)
	if err != nil {
		return fingerprintResult{}, false, err
	}
	return fingerprintResult{points: points, duration: duration, end: fingerprintEnd, fromCache: true}, true, nil
}

func (s *Service) saveFingerprint(ctx context.Context, mediaJobID, fileSize, fileMTime int64, duration, fingerprintEnd float64, points []uint32) error {
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO media_op_fingerprints(
    media_job_id, file_size, file_mtime, duration_seconds, fingerprint_end_seconds,
    fingerprint_points, chromaprint_version, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(media_job_id) DO UPDATE SET
    file_size = excluded.file_size,
    file_mtime = excluded.file_mtime,
    duration_seconds = excluded.duration_seconds,
    fingerprint_end_seconds = excluded.fingerprint_end_seconds,
    fingerprint_points = excluded.fingerprint_points,
    chromaprint_version = excluded.chromaprint_version,
    updated_at = excluded.updated_at`,
		mediaJobID, fileSize, fileMTime, duration, fingerprintEnd,
		encodeFingerprintPoints(points), chromaprintVersion, now, now)
	return err
}

func (s *Service) saveSegment(ctx context.Context, episode mediaEpisode, status string, detection openingDetection, groupSize int, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	now := s.now().UTC().Unix()
	var matched any
	if detection.MatchedMediaID > 0 {
		matched = detection.MatchedMediaID
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO media_op_segments(
    media_job_id, bangumi_id, season_number, episode_type, episode_number, status,
    start_seconds, end_seconds, confidence, analyzed_group_size, matched_media_job_id,
    error_message, created_at, updated_at
) VALUES (?, ?, ?, 'episode', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(media_job_id) DO UPDATE SET
    bangumi_id = excluded.bangumi_id,
    season_number = excluded.season_number,
    episode_type = excluded.episode_type,
    episode_number = excluded.episode_number,
    status = excluded.status,
    start_seconds = excluded.start_seconds,
    end_seconds = excluded.end_seconds,
    confidence = excluded.confidence,
    analyzed_group_size = excluded.analyzed_group_size,
    matched_media_job_id = excluded.matched_media_job_id,
    error_message = excluded.error_message,
    updated_at = excluded.updated_at`,
		episode.MediaJobID, episode.BangumiID, episode.SeasonNumber, episode.EpisodeNumber, status,
		detection.Start, detection.End, detection.Confidence, groupSize, matched,
		message, now, now)
	return err
}

func (s *Service) runFingerprint(ctx context.Context, path string, endSeconds float64) ([]uint32, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostats", "-loglevel", "warning",
		"-ss", "0", "-i", path, "-t", formatSeconds(endSeconds),
		"-vn", "-sn", "-dn", "-ac", "2",
		"-f", "chromaprint", "-fp_format", "raw", "-",
	}
	command := exec.CommandContext(commandCtx, s.ffmpegPath, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if commandCtx.Err() != nil {
			return nil, commandCtx.Err()
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("ffmpeg 生成音频指纹失败: %s", truncateTail(message, 2000))
	}
	raw := stdout.Bytes()
	if len(raw) == 0 || len(raw)%4 != 0 {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = fmt.Sprintf("chromaprint 输出长度异常: %d", len(raw))
		}
		return nil, errors.New(truncateTail(message, 2000))
	}
	return decodeFingerprintPoints(raw)
}

func (s *Service) adjustOpeningEndToSilence(ctx context.Context, episode mediaEpisode, detection openingDetection) (openingDetection, bool, error) {
	limit := math.Ceil(detection.End + 2)
	if limit <= 0 {
		return detection, false, nil
	}
	commandCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostats", "-loglevel", "info",
		"-vn", "-sn", "-dn", "-i", episode.OutputPath, "-t", formatSeconds(limit),
		"-af", fmt.Sprintf("silencedetect=noise=%ddB:duration=0.1", silenceDetectionMaximumNoiseDB),
		"-f", "null", "-",
	}
	output, err := exec.CommandContext(commandCtx, s.ffmpegPath, args...).CombinedOutput()
	if err != nil {
		if commandCtx.Err() != nil {
			return detection, false, commandCtx.Err()
		}
		return detection, false, fmt.Errorf("ffmpeg 静音检测失败: %s", truncateTail(strings.TrimSpace(string(output)), 2000))
	}
	silences := parseSilenceRanges(string(output))
	endingWindow := timeRange{Start: detection.End - 15, End: detection.End}
	for _, silence := range silences {
		if !endingWindow.intersects(silence) ||
			silence.Duration() < silenceDetectionMinimumDurationSecs ||
			silence.Start < detection.Start {
			continue
		}
		adjusted := detection
		adjusted.End = silence.Start
		return adjusted, true, nil
	}
	return detection, false, nil
}

func parseSilenceRanges(output string) []timeRange {
	ranges := make([]timeRange, 0)
	current := timeRange{}
	for _, match := range silenceDetectionPattern.FindAllStringSubmatch(output, -1) {
		if len(match) != 3 {
			continue
		}
		value, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			continue
		}
		if match[1] == "start" {
			current.Start = value
			continue
		}
		current.End = value
		ranges = append(ranges, current)
	}
	return ranges
}

func (s *Service) probeDuration(ctx context.Context, path string) (float64, error) {
	commandCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, s.ffprobePath,
		"-v", "error", "-print_format", "json", "-show_format", "-show_streams", path,
	).CombinedOutput()
	if err != nil {
		if commandCtx.Err() != nil {
			return 0, commandCtx.Err()
		}
		return 0, fmt.Errorf("ffprobe 获取视频时长失败: %s", truncateTail(strings.TrimSpace(string(output)), 2000))
	}
	var probe probeResult
	if err := json.Unmarshal(output, &probe); err != nil {
		return 0, fmt.Errorf("解析 ffprobe 输出失败: %w", err)
	}
	if duration, ok := parsePositiveFloat(probe.Format.Duration); ok {
		return duration, nil
	}
	for _, stream := range probe.Streams {
		if stream.CodecType != "video" {
			continue
		}
		if duration, ok := parsePositiveFloat(stream.Duration); ok {
			return duration, nil
		}
	}
	for _, stream := range probe.Streams {
		if duration, ok := parsePositiveFloat(stream.Duration); ok {
			return duration, nil
		}
	}
	return 0, errors.New("ffprobe 输出中缺少视频时长")
}

func (s *Service) checkFFmpeg(ctx context.Context) error {
	if err := s.checkFFmpegOutput(ctx, []string{"-hide_banner", "-muxers"}, "chromaprint"); err != nil {
		return fmt.Errorf("当前 ffmpeg 不支持 chromaprint muxer，无法识别片头: %w", err)
	}
	if err := s.checkFFmpegOutput(ctx, []string{"-hide_banner", "-h", "muxer=chromaprint"}, "fp_format", "raw"); err != nil {
		return fmt.Errorf("当前 ffmpeg 的 chromaprint muxer 不支持 raw 指纹输出: %w", err)
	}
	if err := s.checkFFmpegOutput(ctx, []string{"-hide_banner", "-h", "filter=silencedetect"}, "silencedetect"); err != nil {
		return fmt.Errorf("当前 ffmpeg 不支持 silencedetect filter，无法修正片头结束点: %w", err)
	}
	return nil
}

func (s *Service) checkFFmpegOutput(ctx context.Context, args []string, required ...string) error {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, s.ffmpegPath, args...).CombinedOutput()
	if err != nil {
		if commandCtx.Err() != nil {
			return commandCtx.Err()
		}
		return fmt.Errorf("执行 %s 失败: %s", s.ffmpegPath, truncateTail(strings.TrimSpace(string(output)), 2000))
	}
	lower := strings.ToLower(string(output))
	for _, value := range required {
		if !strings.Contains(lower, strings.ToLower(value)) {
			return fmt.Errorf("输出缺少 %q", value)
		}
	}
	return nil
}

func encodeFingerprintPoints(points []uint32) []byte {
	raw := make([]byte, len(points)*4)
	for index, point := range points {
		binary.LittleEndian.PutUint32(raw[index*4:], point)
	}
	return raw
}

func decodeFingerprintPoints(raw []byte) ([]uint32, error) {
	if len(raw) == 0 || len(raw)%4 != 0 {
		return nil, fmt.Errorf("chromaprint 指纹长度异常: %d", len(raw))
	}
	points := make([]uint32, len(raw)/4)
	for index := range points {
		points[index] = binary.LittleEndian.Uint32(raw[index*4:])
	}
	return points, nil
}

func parsePositiveFloat(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

func formatSeconds(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}

func truncateTail(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
