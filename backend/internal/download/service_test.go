package download

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/system"
)

type staticDownloadSettings struct {
	settings system.DownloadSettings
}

func (s staticDownloadSettings) GetDownloadSettings(context.Context) (system.DownloadSettings, error) {
	return s.settings, nil
}

type cancelQBitRecorder struct {
	torrents           []qBitTorrent
	deleteStatus       int
	deletedHashes      string
	deleteFiles        string
	deletedTags        string
	deleteRequestCount int
}

func newCancelQBitServer(t *testing.T, recorder *cancelQBitRecorder) (system.DownloadSettings, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(recorder.torrents); err != nil {
				t.Errorf("encode torrents: %v", err)
			}
		case "/api/v2/torrents/delete":
			recorder.deleteRequestCount++
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse delete form: %v", err)
			}
			recorder.deletedHashes = r.Form.Get("hashes")
			recorder.deleteFiles = r.Form.Get("deleteFiles")
			if recorder.deleteStatus != 0 {
				w.WriteHeader(recorder.deleteStatus)
			}
		case "/api/v2/torrents/deleteTags":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse delete tags form: %v", err)
			}
			recorder.deletedTags = r.Form.Get("tags")
		default:
			http.NotFound(w, r)
		}
	}))
	parsed, err := url.Parse(server.URL)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return system.DownloadSettings{Host: host, Port: port, Username: "admin", Password: "secret", MaxConcurrentDownloads: 1}, server.Close
}

func TestQBitSavePathMapsDownloadRoot(t *testing.T) {
	downloadDir := filepath.Join(t.TempDir(), "downloads")
	service := NewService(nil, nil, slog.Default(), Config{DownloadDir: downloadDir})
	hostSavePath := filepath.Join(service.DownloadDir(), "episode-123")

	tests := []struct {
		name     string
		qbitRoot string
		want     string
	}{
		{name: "unchanged without mapping", want: hostSavePath},
		{name: "linux container path", qbitRoot: "/downloads/BangumiPipeline/data/downloads", want: "/downloads/BangumiPipeline/data/downloads/episode-123"},
		{name: "linux trailing separator", qbitRoot: "/downloads/", want: "/downloads/episode-123"},
		{name: "linux root", qbitRoot: "/", want: "/episode-123"},
		{name: "windows target path", qbitRoot: `D:\downloads`, want: `D:\downloads\episode-123`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.qBitSavePath(hostSavePath, tt.qbitRoot); got != tt.want {
				t.Fatalf("qBitSavePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchTorrentUsesMappedSavePath(t *testing.T) {
	job := activeJob{
		SubscriptionItemID: 42,
		SavePath:           "/opt/BangumiPipeline/data/downloads/episode-42",
		QBitSavePath:       "/downloads/BangumiPipeline/data/downloads/episode-42",
	}
	torrents := []qBitTorrent{{Hash: "mapped", SavePath: job.QBitSavePath}}

	torrent, ok := matchTorrent(job, torrents, nil)
	if !ok || torrent.Hash != "mapped" {
		t.Fatalf("expected torrent to match mapped save path, got ok=%v torrent=%+v", ok, torrent)
	}
}

func TestListJobsExcludesLocalMediaImports(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Unix(1_700_000_000, 0).Unix()
	if _, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, created_at)
VALUES (1001, 'https://bgm.tv/subject/1001', 'Anime', '番剧', ?);

INSERT INTO subscription_items(
    item_key, title, binding_status, bound_bangumi_id, bound_anime_name,
    bound_season_number, bound_episode_type, bound_episode_number, created_at, updated_at
) VALUES
    ('download-item', '番剧 S01E01', 'bound', 1001, '番剧', 1, 'episode', '01', ?, ?),
    ('local-item', '番剧 S01E02', 'bound', 1001, '番剧', 1, 'episode', '02', ?, ?);

INSERT INTO download_jobs(
    subscription_item_id, status, source_type, save_path, completed_at, created_at, updated_at
)
SELECT id, 'completed', 'download', 'download-path', ?, ?, ?
FROM subscription_items WHERE item_key = 'download-item';

INSERT INTO download_jobs(
    subscription_item_id, status, source_type, save_path, completed_at, created_at, updated_at
)
SELECT id, 'completed', 'local', 'upload-path', ?, ?, ?
FROM subscription_items WHERE item_key = 'local-item';`,
		now, now, now, now, now, now, now, now, now, now, now,
	); err != nil {
		t.Fatal(err)
	}

	service := NewService(db, nil, slog.Default(), Config{DownloadDir: t.TempDir()})
	page, err := service.ListJobs(ctx, 1, 50, StatusCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].EpisodeNumber != "01" {
		t.Fatalf("expected only the qBittorrent download job, got %+v", page)
	}
}

func TestCancelJobDeletesQBitTaskAndRevertsBinding(t *testing.T) {
	ctx := context.Background()
	db, itemID, jobID := seedCancelableJob(t, StatusDownloading)

	recorder := &cancelQBitRecorder{torrents: []qBitTorrent{{Hash: "torrent-hash", State: "downloading"}}}
	settings, closeServer := newCancelQBitServer(t, recorder)
	t.Cleanup(closeServer)
	service := NewService(db, staticDownloadSettings{settings: settings}, slog.Default(), Config{DownloadDir: t.TempDir()})

	result, err := service.CancelJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if result.JobID != jobID || result.SubscriptionItemID != itemID || !result.QBitTaskDeleted {
		t.Fatalf("unexpected cancel result: %+v", result)
	}
	if recorder.deletedHashes != "torrent-hash" || recorder.deleteFiles != "true" || recorder.deletedTags != tagForItem(itemID) {
		t.Fatalf("unexpected qBittorrent cleanup: %+v", recorder)
	}
	assertDownloadCanceled(t, db, itemID, jobID)
}

func TestCancelJobRevertsBindingWhenQBitTaskIsMissing(t *testing.T) {
	ctx := context.Background()
	db, itemID, jobID := seedCancelableJob(t, StatusFailed)

	recorder := &cancelQBitRecorder{}
	settings, closeServer := newCancelQBitServer(t, recorder)
	t.Cleanup(closeServer)
	service := NewService(db, staticDownloadSettings{settings: settings}, slog.Default(), Config{DownloadDir: t.TempDir()})

	result, err := service.CancelJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if result.QBitTaskDeleted || recorder.deleteRequestCount != 0 {
		t.Fatalf("missing qBittorrent task should not be deleted: result=%+v recorder=%+v", result, recorder)
	}
	assertDownloadCanceled(t, db, itemID, jobID)
}

func TestCancelJobKeepsBindingWhenQBitDeleteFails(t *testing.T) {
	ctx := context.Background()
	db, itemID, jobID := seedCancelableJob(t, StatusDownloading)

	recorder := &cancelQBitRecorder{
		torrents:     []qBitTorrent{{Hash: "torrent-hash", State: "downloading"}},
		deleteStatus: http.StatusInternalServerError,
	}
	settings, closeServer := newCancelQBitServer(t, recorder)
	t.Cleanup(closeServer)
	service := NewService(db, staticDownloadSettings{settings: settings}, slog.Default(), Config{DownloadDir: t.TempDir()})

	if _, err := service.CancelJob(ctx, jobID); !errors.Is(err, ErrQBitUnavailable) {
		t.Fatalf("CancelJob() error = %v, want ErrQBitUnavailable", err)
	}
	var jobCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM download_jobs WHERE id = ?", jobID).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	var bindingStatus string
	if err := db.QueryRowContext(ctx, "SELECT binding_status FROM subscription_items WHERE id = ?", itemID).Scan(&bindingStatus); err != nil {
		t.Fatal(err)
	}
	if jobCount != 1 || bindingStatus != "bound" {
		t.Fatalf("failed qBittorrent delete changed local state: job_count=%d binding_status=%q", jobCount, bindingStatus)
	}
}

func TestCancelJobRejectsCompletedJob(t *testing.T) {
	db, _, jobID := seedCancelableJob(t, StatusCompleted)
	service := NewService(db, nil, slog.Default(), Config{DownloadDir: t.TempDir()})
	if _, err := service.CancelJob(context.Background(), jobID); !errors.Is(err, ErrCancelNotAllowed) {
		t.Fatalf("CancelJob() error = %v, want ErrCancelNotAllowed", err)
	}
}

func seedCancelableJob(t *testing.T, status string) (*sql.DB, int64, int64) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "cancel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Unix(1_700_000_000, 0).Unix()
	if _, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, created_at)
VALUES (2001, 'https://bgm.tv/subject/2001', 'Cancelable Anime', '可撤销番剧', ?);

INSERT INTO subscription_items(
    item_key, title, match_status, bangumi_id, matched_name,
    season_number, episode_type, episode_number,
    binding_status, binding_note, bound_bangumi_id, bound_anime_name,
    bound_season_number, bound_episode_type, bound_episode_number, bound_at,
    created_at, updated_at
) VALUES (
    'cancel-item', '可撤销番剧 S01E01', 'matched', 2001, '可撤销番剧',
    1, 'episode', '01',
    'bound', '已绑定', 2001, '可撤销番剧', 1, 'episode', '01', ?, ?, ?
);`, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	var itemID int64
	if err := db.QueryRowContext(ctx, "SELECT id FROM subscription_items WHERE item_key = 'cancel-item'").Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	result, err := db.ExecContext(ctx, `
INSERT INTO download_jobs(
    subscription_item_id, status, source_type, qbit_hash, save_path, started_at,
    completed_at, failed_at, created_at, updated_at
) VALUES (?, ?, 'download', 'torrent-hash', 'download-path', ?, ?, ?, ?, ?)`,
		itemID, status, now, nullableTimestamp(status == StatusCompleted, now), nullableTimestamp(status == StatusFailed, now), now, now)
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return db, itemID, jobID
}

func nullableTimestamp(include bool, timestamp int64) any {
	if !include {
		return nil
	}
	return timestamp
}

func assertDownloadCanceled(t *testing.T, db *sql.DB, itemID, jobID int64) {
	t.Helper()
	ctx := context.Background()
	var jobCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM download_jobs WHERE id = ?", jobID).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if jobCount != 0 {
		t.Fatalf("download job still exists after cancellation: %d", jobCount)
	}

	var bindingStatus, bindingNote, boundAnimeName, boundEpisodeType, boundEpisodeNumber, matchStatus string
	var boundBangumiID, boundSeasonNumber, boundAt sql.NullInt64
	if err := db.QueryRowContext(ctx, `
SELECT binding_status, binding_note, bound_bangumi_id, bound_anime_name,
       bound_season_number, bound_episode_type, bound_episode_number, bound_at, match_status
FROM subscription_items
WHERE id = ?`, itemID).Scan(
		&bindingStatus, &bindingNote, &boundBangumiID, &boundAnimeName,
		&boundSeasonNumber, &boundEpisodeType, &boundEpisodeNumber, &boundAt, &matchStatus,
	); err != nil {
		t.Fatal(err)
	}
	if bindingStatus != "pending" || bindingNote != "下载已手动撤销，等待重新绑定" ||
		boundBangumiID.Valid || boundAnimeName != "" || boundSeasonNumber.Valid ||
		boundEpisodeType != "" || boundEpisodeNumber != "" || boundAt.Valid {
		t.Fatalf("subscription binding was not reset: status=%q note=%q bangumi=%v anime=%q season=%v type=%q episode=%q bound_at=%v",
			bindingStatus, bindingNote, boundBangumiID, boundAnimeName, boundSeasonNumber, boundEpisodeType, boundEpisodeNumber, boundAt)
	}
	if matchStatus != "matched" {
		t.Fatalf("automatic match state should be preserved, got %q", matchStatus)
	}
}
