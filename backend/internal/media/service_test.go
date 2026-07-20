package media

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
)

func TestPrepareEpisodeReplacementDeletesOutputAndMediaJob(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	outputPath, coverPath := insertCompletedMediaJob(t, ctx, db, 1001, 1, "episode", "03", now)

	service := NewService(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{MediaDir: t.TempDir()})
	result, err := service.PrepareEpisodeReplacement(ctx, 1001, 1, "episode", "03")
	if err != nil {
		t.Fatal(err)
	}
	if result.MediaJobsRemoved != 1 || result.FilesDeleted != 2 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("expected output file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(coverPath); !os.IsNotExist(err) {
		t.Fatalf("expected cover file to be removed, stat err=%v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_jobs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected media job to be removed, got %d", count)
	}
}

func TestPrepareEpisodeReplacementRejectsTranscodingJob(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	outputPath, coverPath := insertCompletedMediaJob(t, ctx, db, 1001, 1, "episode", "03", now)
	if _, err := db.ExecContext(ctx, "UPDATE media_jobs SET status = ?", StatusTranscoding); err != nil {
		t.Fatal(err)
	}

	service := NewService(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{MediaDir: t.TempDir()})
	if _, err := service.PrepareEpisodeReplacement(ctx, 1001, 1, "episode", "03"); err != ErrAnimeTranscoding {
		t.Fatalf("expected ErrAnimeTranscoding, got %v", err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected output file to remain, stat err=%v", err)
	}
	if _, err := os.Stat(coverPath); err != nil {
		t.Fatalf("expected cover file to remain, stat err=%v", err)
	}
}

func TestRefreshAnimeMetadataOncePerDay(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 7, 1, 10, 30, 0, 0, time.Local)
	if _, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, created_at)
VALUES (?, ?, ?, ?, ?)`, 1001, "https://bgm.tv/subject/1001", "Original Anime", "原作标题", now.Unix()); err != nil {
		t.Fatal(err)
	}
	refresher := &metadataRefreshRecorder{}
	service := NewService(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		MediaDir:          t.TempDir(),
		MetadataRefresher: refresher,
	})
	service.now = func() time.Time { return now }

	service.refreshAnimeMetadataOncePerDay(ctx, 1001)
	service.refreshAnimeMetadataOncePerDay(ctx, 1001)
	if len(refresher.ids) != 1 || refresher.ids[0] != 1001 {
		t.Fatalf("expected one refresh on first day, got %+v", refresher.ids)
	}

	service.now = func() time.Time { return now.Add(24 * time.Hour) }
	service.refreshAnimeMetadataOncePerDay(ctx, 1001)
	if len(refresher.ids) != 2 || refresher.ids[1] != 1001 {
		t.Fatalf("expected another refresh on next day, got %+v", refresher.ids)
	}
}

func TestCompletedMediaEnqueuesEpisodeCommentsWithoutRollingBackOnQueueError(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	jobID := insertPlannedPendingMediaJob(t, ctx, db, 2002, "01", "copy", false, now)
	sourcePath := filepath.Join(t.TempDir(), "source.mp4")
	writeTestFile(t, sourcePath)
	outputPath := filepath.Join(t.TempDir(), "output.mp4")
	recorder := &commentEnqueueRecorder{err: errors.New("queue unavailable")}
	service := NewService(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		MediaDir:        t.TempDir(),
		FFprobePath:     "missing-ffprobe-for-comment-enqueue-test",
		CommentEnqueuer: recorder,
	})
	service.now = func() time.Time { return now }
	_, err = service.processPlannedJob(ctx, plannedJob{
		pendingJob: pendingJob{ID: jobID, BangumiID: 2002},
		plan: mediaPlan{
			action: "copy", sourcePath: sourcePath, outputPath: outputPath,
			videoCodec: "h264", audioCodec: "aac",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.calls) != 1 || recorder.calls[0].mediaJobID != jobID || recorder.calls[0].bangumiID != 2002 {
		t.Fatalf("unexpected comment enqueue calls: %+v", recorder.calls)
	}
	assertMediaJobStatus(t, ctx, db, jobID, StatusCompleted)
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected completed output despite queue failure: %v", err)
	}
}

func TestPlanPendingJobsSplitsCopyAndSingleFFmpegLine(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	service := NewService(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{MediaDir: t.TempDir()})
	service.now = func() time.Time { return now }

	copyID := insertPlannedPendingMediaJob(t, ctx, db, 1001, "01", "copy", false, now)
	ffmpegID := insertPlannedPendingMediaJob(t, ctx, db, 1001, "02", "burn_subtitles", true, now.Add(time.Second))
	deferredID := insertPlannedPendingMediaJob(t, ctx, db, 1001, "03", "transcode", true, now.Add(2*time.Second))

	result, err := service.planPendingJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.planned != 3 || result.planFailed != 0 || result.deferredFFmpeg != 1 {
		t.Fatalf("unexpected planning counts: %+v", result)
	}
	if len(result.copyJobs) != 1 || result.copyJobs[0].ID != copyID {
		t.Fatalf("expected one copy job %d, got %+v", copyID, result.copyJobs)
	}
	if result.ffmpegJob == nil || result.ffmpegJob.ID != ffmpegID {
		t.Fatalf("expected ffmpeg job %d, got %+v", ffmpegID, result.ffmpegJob)
	}
	assertMediaJobStatus(t, ctx, db, copyID, StatusTranscoding)
	assertMediaJobStatus(t, ctx, db, ffmpegID, StatusTranscoding)
	assertMediaJobStatus(t, ctx, db, deferredID, StatusPending)
}

func TestCleanupCompletedDownloadsRetriesFailedQBitCleanup(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	insertCompletedMediaJob(t, ctx, db, 1001, 1, "episode", "03", now)
	var mediaJobID, downloadJobID int64
	if err := db.QueryRowContext(ctx, "SELECT id, download_job_id FROM media_jobs LIMIT 1").Scan(&mediaJobID, &downloadJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE media_jobs SET error_message = ? WHERE id = ?", completionCleanupWarningPrefix+" old error", mediaJobID); err != nil {
		t.Fatal(err)
	}

	cleaner := &batchDownloadCleanerRecorder{}
	service := NewService(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		MediaDir:        t.TempDir(),
		DownloadCleaner: cleaner,
	})
	cleaned, err := service.cleanupCompletedDownloads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cleaned != 1 {
		t.Fatalf("expected one cleaned download, got %d", cleaned)
	}
	if len(cleaner.batchIDs) != 1 || cleaner.batchIDs[0] != downloadJobID {
		t.Fatalf("unexpected cleaned ids: %+v, want %d", cleaner.batchIDs, downloadJobID)
	}
	var message string
	if err := db.QueryRowContext(ctx, "SELECT error_message FROM media_jobs WHERE id = ?", mediaJobID).Scan(&message); err != nil {
		t.Fatal(err)
	}
	if message != "" {
		t.Fatalf("expected cleanup warning to be cleared, got %q", message)
	}
}

func TestMP4MoovBeforeMdat(t *testing.T) {
	fastStartPath := writeMP4Boxes(t, []string{"ftyp", "moov", "mdat"})
	fastStart, err := mp4MoovBeforeMdat(fastStartPath)
	if err != nil {
		t.Fatal(err)
	}
	if !fastStart {
		t.Fatal("expected moov-before-mdat MP4 to be detected as faststart")
	}

	slowStartPath := writeMP4Boxes(t, []string{"ftyp", "mdat", "moov"})
	fastStart, err = mp4MoovBeforeMdat(slowStartPath)
	if err != nil {
		t.Fatal(err)
	}
	if fastStart {
		t.Fatal("expected mdat-before-moov MP4 to require remux")
	}

	structure, err := inspectMP4TopLevelStructure(writeMP4Boxes(t, []string{"ftyp", "moov", "mdat", "mdat"}))
	if err != nil {
		t.Fatal(err)
	}
	if !structure.MoovBeforeMdat {
		t.Fatal("expected multi-mdat fixture to still be faststart")
	}
	if structure.MdatCount != 2 {
		t.Fatalf("expected two mdat boxes, got %d", structure.MdatCount)
	}
}

func TestProblematicEditListWarning(t *testing.T) {
	message := `[mov,mp4,m4a,3gp,3g2,mj2 @ 00000191dd9b38c0] st: 0 edit list: 1 Missing key frame while searching for timestamp: 5999
[mov,mp4,m4a,3gp,3g2,mj2 @ 00000191dd9b38c0] st: 0 edit list 1 Cannot find an index entry before timestamp: 5999.`
	if !hasProblematicEditListWarning(message) {
		t.Fatal("expected problematic edit list warning to be detected")
	}
	if hasProblematicEditListWarning("some unrelated warning") {
		t.Fatal("unexpected unrelated warning match")
	}
}

func TestVideoBitRateBPSReadsStreamBitRateAndBPSTag(t *testing.T) {
	if got := videoBitRateBPS(probeStream{BitRate: "1680609"}); got != 1_680_609 {
		t.Fatalf("unexpected stream bit rate: got %d", got)
	}
	if got := videoBitRateBPS(probeStream{Tags: map[string]string{"BPS": "7997469"}}); got != 7_997_469 {
		t.Fatalf("unexpected BPS tag bit rate: got %d", got)
	}
	if got := videoBitRateBPS(probeStream{BitRate: "N/A"}); got != 0 {
		t.Fatalf("expected unknown bit rate to be 0, got %d", got)
	}
}

func TestFFmpegArgsLimitHighBitRateTranscode(t *testing.T) {
	args := ffmpegArgs(mediaPlan{
		sourcePath:      "source.mkv",
		outputPath:      "output.mp4",
		action:          "transcode",
		videoBitRateBPS: maxTranscodedVideoBitRateBPS + 1,
	}, "output.mp4.tmp.mp4")
	if !hasArgPair(args, "-maxrate", maxTranscodedVideoRateArg) {
		t.Fatalf("expected maxrate cap in args: %v", args)
	}
	if !hasArgPair(args, "-bufsize", maxTranscodedVideoBufSizeArg) {
		t.Fatalf("expected bufsize cap in args: %v", args)
	}
}

func TestFFmpegArgsDoNotLimitLowBitRateTranscode(t *testing.T) {
	args := ffmpegArgs(mediaPlan{
		sourcePath:      "source.mkv",
		outputPath:      "output.mp4",
		action:          "burn_subtitles",
		videoBitRateBPS: maxTranscodedVideoBitRateBPS,
	}, "output.mp4.tmp.mp4")
	if hasArg(args, "-maxrate") || hasArg(args, "-bufsize") {
		t.Fatalf("did not expect bitrate cap for low-bitrate source: %v", args)
	}
}

func TestFFmpegArgsDoNotLimitRemux(t *testing.T) {
	args := ffmpegArgs(mediaPlan{
		sourcePath:      "source.mkv",
		outputPath:      "output.mp4",
		action:          "remux",
		videoBitRateBPS: maxTranscodedVideoBitRateBPS + 1,
	}, "output.mp4.tmp.mp4")
	if hasArg(args, "-maxrate") || hasArg(args, "-bufsize") {
		t.Fatalf("did not expect bitrate cap for remux: %v", args)
	}
	if !hasArgPair(args, "-c", "copy") {
		t.Fatalf("expected remux to copy streams: %v", args)
	}
}

func TestFFmpegArgsScaleOversizedTranscode(t *testing.T) {
	args := ffmpegArgs(mediaPlan{
		sourcePath:      "source.mkv",
		outputPath:      "output.mp4",
		action:          "transcode",
		videoWidth:      3840,
		videoHeight:     2160,
		videoBitRateBPS: maxTranscodedVideoBitRateBPS,
	}, "output.mp4.tmp.mp4")
	if !hasArgContaining(args, "scale=") {
		t.Fatalf("expected scale filter for oversized video: %v", args)
	}
}

func TestFFmpegArgsDoNotScale1080pTranscode(t *testing.T) {
	args := ffmpegArgs(mediaPlan{
		sourcePath:      "source.mkv",
		outputPath:      "output.mp4",
		action:          "transcode",
		videoWidth:      1920,
		videoHeight:     1080,
		videoBitRateBPS: maxTranscodedVideoBitRateBPS,
	}, "output.mp4.tmp.mp4")
	if hasArgContaining(args, "scale=") {
		t.Fatalf("did not expect scale filter for 1080p video: %v", args)
	}
}

func TestFFmpegArgsCombineScaleAndSubtitleFilters(t *testing.T) {
	args := ffmpegArgs(mediaPlan{
		sourcePath:            "source.mkv",
		outputPath:            "output.mp4",
		action:                "burn_subtitles",
		hasInternalSubtitles:  true,
		internalSubtitleIndex: 1,
		videoWidth:            3840,
		videoHeight:           2160,
		videoBitRateBPS:       maxTranscodedVideoBitRateBPS,
	}, "output.mp4.tmp.mp4")
	filter, ok := argValue(args, "-vf")
	if !ok {
		t.Fatalf("expected vf filter in args: %v", args)
	}
	if !strings.Contains(filter, "scale=") || !strings.Contains(filter, "subtitles=") || !strings.Contains(filter, ":si=1") {
		t.Fatalf("expected scale and subtitle filters in one vf arg, got %q from %v", filter, args)
	}
	if countArg(args, "-vf") != 1 {
		t.Fatalf("expected one vf arg, got %v", args)
	}
}

func TestSelectSubtitlePrefersChineseExternalSubtitle(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "bangumi.mkv")
	writeTestFile(t, videoPath)
	zhTWPath := filepath.Join(dir, "bangumi.zh-TW.srt")
	zhCNPath := filepath.Join(dir, "bangumi.zh-CN.ass")
	writeTestFile(t, zhTWPath)
	writeTestFile(t, zhCNPath)

	selection := selectSubtitle(videoPath, []probeStream{
		{CodecName: "ass", CodecType: "subtitle", Tags: map[string]string{"title": "Chinese Traditional"}},
	})
	if selection.externalPath != zhCNPath {
		t.Fatalf("expected zh-CN external subtitle %q, got %+v", zhCNPath, selection)
	}
	if !selection.hasExternal || !selection.hasInternal {
		t.Fatalf("expected both subtitle kinds to be detected, got %+v", selection)
	}
}

func TestSelectSubtitlePrefersChineseInternalBeforeFallbackExternal(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "bangumi.mkv")
	fallbackPath := filepath.Join(dir, "bangumi.srt")
	writeTestFile(t, videoPath)
	writeTestFile(t, fallbackPath)

	selection := selectSubtitle(videoPath, []probeStream{
		{CodecName: "ass", CodecType: "subtitle", Tags: map[string]string{"language": "eng"}},
		{CodecName: "ass", CodecType: "subtitle", Tags: map[string]string{"title": "繁體中文"}},
	})
	if selection.externalPath != "" || selection.internalIndex != 1 {
		t.Fatalf("expected Chinese internal subtitle index 1, got %+v", selection)
	}
}

func TestSelectSubtitleFallsBackToExternalSubtitle(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "bangumi.mkv")
	fallbackPath := filepath.Join(dir, "bangumi.srt")
	writeTestFile(t, videoPath)
	writeTestFile(t, fallbackPath)

	selection := selectSubtitle(videoPath, []probeStream{
		{CodecName: "ass", CodecType: "subtitle", Tags: map[string]string{"language": "eng"}},
	})
	if selection.externalPath != fallbackPath {
		t.Fatalf("expected fallback external subtitle %q, got %+v", fallbackPath, selection)
	}
}

func TestFFmpegArgsUseSelectedInternalSubtitleIndex(t *testing.T) {
	args := ffmpegArgs(mediaPlan{
		sourcePath:            "source.mkv",
		outputPath:            "output.mp4",
		action:                "burn_subtitles",
		hasInternalSubtitles:  true,
		internalSubtitleIndex: 2,
		videoBitRateBPS:       maxTranscodedVideoBitRateBPS,
	}, "output.mp4.tmp.mp4")
	if !hasArgContaining(args, ":si=2") {
		t.Fatalf("expected selected subtitle index in args: %v", args)
	}
}

type metadataRefreshRecorder struct {
	ids []int64
}

type commentEnqueueCall struct {
	mediaJobID int64
	bangumiID  int64
}

type commentEnqueueRecorder struct {
	calls []commentEnqueueCall
	err   error
}

func (r *commentEnqueueRecorder) EnqueueMediaCompleted(_ context.Context, mediaJobID, bangumiID int64) error {
	r.calls = append(r.calls, commentEnqueueCall{mediaJobID: mediaJobID, bangumiID: bangumiID})
	return r.err
}

func (r *metadataRefreshRecorder) RefreshSubject(_ context.Context, bangumiID int64) error {
	r.ids = append(r.ids, bangumiID)
	return nil
}

type batchDownloadCleanerRecorder struct {
	batchIDs []int64
}

func (r *batchDownloadCleanerRecorder) CleanupCompletedQBitTask(_ context.Context, jobID int64) error {
	r.batchIDs = append(r.batchIDs, jobID)
	return nil
}

func (r *batchDownloadCleanerRecorder) CleanupCompletedQBitTasks(_ context.Context, jobIDs []int64) ([]int64, error) {
	r.batchIDs = append(r.batchIDs, jobIDs...)
	return append([]int64(nil), jobIDs...), nil
}

func writeMP4Boxes(t *testing.T, boxTypes []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.mp4")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, boxType := range boxTypes {
		if len(boxType) != 4 {
			t.Fatalf("invalid box type %q", boxType)
		}
		var header [8]byte
		binary.BigEndian.PutUint32(header[0:4], 12)
		copy(header[4:8], boxType)
		if _, err := file.Write(header[:]); err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte{0, 0, 0, 0}); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func assertMediaJobStatus(t *testing.T, ctx context.Context, db *sql.DB, jobID int64, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(ctx, "SELECT status FROM media_jobs WHERE id = ?", jobID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("media job %d status = %q, want %q", jobID, got, want)
	}
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func hasArgPair(args []string, key, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}

func argValue(args []string, key string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1], true
		}
	}
	return "", false
}

func countArg(args []string, want string) int {
	count := 0
	for _, arg := range args {
		if arg == want {
			count++
		}
	}
	return count
}

func hasArgContaining(args []string, want string) bool {
	for _, arg := range args {
		if strings.Contains(arg, want) {
			return true
		}
	}
	return false
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func insertPlannedPendingMediaJob(t *testing.T, ctx context.Context, db *sql.DB, bangumiID int64, episodeNumber, action string, needsTranscode bool, now time.Time) int64 {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT OR IGNORE INTO anime_metadata(bangumi_id, url, name, name_cn, created_at)
VALUES (?, ?, ?, ?, ?)`, bangumiID, fmt.Sprintf("https://bgm.tv/subject/%d", bangumiID), "Original Anime", "原作标题", now.Unix()); err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("planned-%s", episodeNumber)
	_, err := db.ExecContext(ctx, `
INSERT INTO subscription_items(
    item_key, guid, title, match_status, bangumi_id, matched_name, parsed_name,
    season_number, episode_type, episode_number, binding_status, bound_bangumi_id,
    bound_anime_name, bound_season_number, bound_episode_type, bound_episode_number,
    binding_note, bound_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key, key, key, "matched", bangumiID, "原作标题", "原作标题",
		1, "episode", episodeNumber, "bound", bangumiID,
		"原作标题", 1, "episode", episodeNumber,
		"手动绑定", now.Unix(), now.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	var itemID int64
	if err := db.QueryRowContext(ctx, "SELECT id FROM subscription_items WHERE item_key = ?", key).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	result, err := db.ExecContext(ctx, `
INSERT INTO download_jobs(subscription_item_id, status, source_url, save_path, created_at, updated_at)
VALUES (?, 'completed', 'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567', ?, ?, ?)`,
		itemID, filepath.Join(t.TempDir(), "download-"+episodeNumber), now.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	downloadJobID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	needs := 0
	if needsTranscode {
		needs = 1
	}
	result, err = db.ExecContext(ctx, `
INSERT INTO media_jobs(
    download_job_id, subscription_item_id, bangumi_id, anime_name, season_number,
    episode_type, episode_number, status, source_path, output_path,
    video_codec, audio_codec, has_internal_subtitles, has_external_subtitles,
    needs_transcode, action, total_duration_ms, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		downloadJobID, itemID, bangumiID, "原作标题", 1,
		"episode", episodeNumber, StatusPending,
		filepath.Join(t.TempDir(), "source-"+episodeNumber+".mp4"),
		filepath.Join(t.TempDir(), "output-"+episodeNumber+".mp4"),
		"h264", "aac", 0, 0, needs, action, int64(90_000), now.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return jobID
}

func insertCompletedMediaJob(t *testing.T, ctx context.Context, db *sql.DB, bangumiID int64, seasonNumber int, episodeType, episodeNumber string, now time.Time) (string, string) {
	t.Helper()
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "episode.mp4")
	coverPath := filepath.Join(dir, "episode.jpg")
	if err := os.WriteFile(outputPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverPath, []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, created_at)
VALUES (?, ?, ?, ?, ?)`, bangumiID, "https://bgm.tv/subject/1001", "Original Anime", "原作标题", now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	result, err := db.ExecContext(ctx, `
INSERT INTO subscription_items(
    item_key, guid, title, match_status, bangumi_id, matched_name, parsed_name,
    season_number, episode_type, episode_number, binding_status, bound_bangumi_id,
    bound_anime_name, bound_season_number, bound_episode_type, bound_episode_number,
    binding_note, bound_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"source", "source", "source", "matched", bangumiID, "原作标题", "原作标题",
		seasonNumber, episodeType, episodeNumber, "bound", bangumiID,
		"原作标题", seasonNumber, episodeType, episodeNumber,
		"手动绑定", now.Unix(), now.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	result, err = db.ExecContext(ctx, `
INSERT INTO download_jobs(subscription_item_id, status, source_url, created_at, updated_at)
VALUES (?, 'completed', 'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567', ?, ?)`,
		itemID, now.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	downloadJobID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO media_jobs(
    download_job_id, subscription_item_id, bangumi_id, anime_name, season_number,
    episode_type, episode_number, status, output_path, cover_path, cover_status,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		downloadJobID, itemID, bangumiID, "原作标题", seasonNumber,
		episodeType, episodeNumber, StatusCompleted, outputPath, coverPath, CoverStatusCompleted,
		now.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	return outputPath, coverPath
}
