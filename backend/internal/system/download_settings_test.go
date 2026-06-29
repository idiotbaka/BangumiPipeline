package system_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/system"
)

func TestDownloadSettingsPersistQBitDownloadDir(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := system.NewService(db)
	updated, err := service.UpdateDownloadSettings(ctx, system.DownloadSettings{
		Host:                   "qbittorrent",
		Port:                   8080,
		Username:               "admin",
		Password:               "secret",
		QBitDownloadDir:        "/downloads/BangumiPipeline/data/downloads",
		MaxConcurrentDownloads: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.QBitDownloadDir != "/downloads/BangumiPipeline/data/downloads" {
		t.Fatalf("unexpected qBittorrent download directory: %q", updated.QBitDownloadDir)
	}

	updated.QBitDownloadDir = "relative/downloads"
	if _, err := service.UpdateDownloadSettings(ctx, updated); !errors.Is(err, system.ErrInvalidDownloadSettings) {
		t.Fatalf("expected relative qBittorrent path to be rejected, got %v", err)
	}
}
