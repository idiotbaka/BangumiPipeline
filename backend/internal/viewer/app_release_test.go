package viewer

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
)

func TestAppReleaseUsesSemanticVersionOrdering(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "app-release.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewService(db, time.Hour)
	apk := []byte{'P', 'K', 0x03, 0x04, 'a', 'p', 'k'}
	for _, version := range []string{"1.9.0", "1.10.0"} {
		if _, err := service.PublishAppRelease(ctx, AppReleaseInput{
			Version: version, ReleaseNotes: "更新内容", APKData: apk,
		}); err != nil {
			t.Fatalf("publish %s: %v", version, err)
		}
	}

	latest, err := service.LatestAppRelease(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != "1.10.0" {
		t.Fatalf("expected semantic latest version 1.10.0, got %q", latest.Version)
	}
	items, err := service.ListAppReleases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Version != "1.10.0" || items[1].Version != "1.9.0" {
		t.Fatalf("unexpected release order: %#v", items)
	}

	file, err := service.AppReleaseAPK(ctx, latest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if file.Version != latest.Version || string(file.Data) != string(apk) || file.Size != int64(len(apk)) {
		t.Fatalf("unexpected release file: %#v", file)
	}
	if _, err := service.PublishAppRelease(ctx, AppReleaseInput{
		Version: "1.10.0", ReleaseNotes: "重复版本", APKData: apk,
	}); !errors.Is(err, ErrAppReleaseVersionExists) {
		t.Fatalf("expected duplicate version error, got %v", err)
	}
}

func TestAppReleaseValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "app-release-validation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewService(db, time.Hour)

	for _, input := range []AppReleaseInput{
		{Version: "1.0", ReleaseNotes: "更新内容", APKData: []byte{'P', 'K', 0x03, 0x04}},
		{Version: "1.0.0", ReleaseNotes: "", APKData: []byte{'P', 'K', 0x03, 0x04}},
		{Version: "1.0.0", ReleaseNotes: "更新内容", APKData: []byte("not an apk")},
	} {
		if _, err := service.PublishAppRelease(ctx, input); err == nil {
			t.Fatalf("expected validation error for %#v", input)
		}
	}
}
