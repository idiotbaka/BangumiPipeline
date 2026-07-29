package download

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
)

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
