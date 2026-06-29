package download

import (
	"log/slog"
	"path/filepath"
	"testing"
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
