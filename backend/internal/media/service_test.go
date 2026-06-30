package media

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
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

type metadataRefreshRecorder struct {
	ids []int64
}

func (r *metadataRefreshRecorder) RefreshSubject(_ context.Context, bangumiID int64) error {
	r.ids = append(r.ids, bangumiID)
	return nil
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
