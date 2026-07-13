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

func TestUpdateAndDeleteAppRelease(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "app-release-update.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewService(db, time.Hour)
	currentTime := time.Unix(100, 0)
	service.now = func() time.Time { return currentTime }
	originalAPK := []byte{'P', 'K', 0x03, 0x04, 'o', 'l', 'd'}
	release, err := service.PublishAppRelease(ctx, AppReleaseInput{
		Version: "1.1.0", ReleaseNotes: "原更新日志", APKData: originalAPK,
	})
	if err != nil {
		t.Fatal(err)
	}
	originalSHA256 := release.APKSHA256

	currentTime = time.Unix(200, 0)
	updated, err := service.UpdateAppRelease(ctx, release.ID, AppReleaseInput{
		Version: "1.2.0", ReleaseNotes: "新更新日志",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != "1.2.0" || updated.ReleaseNotes != "新更新日志" {
		t.Fatalf("unexpected updated release: %#v", updated)
	}
	if updated.PublishedAt != release.PublishedAt {
		t.Fatalf("published time changed from %d to %d", release.PublishedAt, updated.PublishedAt)
	}
	if updated.APKSize != int64(len(originalAPK)) || updated.APKSHA256 != originalSHA256 {
		t.Fatalf("APK metadata changed without a replacement: %#v", updated)
	}
	file, err := service.AppReleaseAPK(ctx, release.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(file.Data) != string(originalAPK) {
		t.Fatalf("APK changed without a replacement: %q", file.Data)
	}

	replacementAPK := []byte{'P', 'K', 0x03, 0x04, 'n', 'e', 'w', 'e', 'r'}
	replaced, err := service.UpdateAppRelease(ctx, release.ID, AppReleaseInput{
		Version: "1.2.0", ReleaseNotes: "替换安装包", APKData: replacementAPK,
	})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.APKSize != int64(len(replacementAPK)) || replaced.APKSHA256 == originalSHA256 {
		t.Fatalf("APK metadata was not replaced: %#v", replaced)
	}
	file, err = service.AppReleaseAPK(ctx, release.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(file.Data) != string(replacementAPK) {
		t.Fatalf("unexpected replacement APK: %q", file.Data)
	}

	other, err := service.PublishAppRelease(ctx, AppReleaseInput{
		Version: "2.0.0", ReleaseNotes: "其他版本", APKData: originalAPK,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateAppRelease(ctx, release.ID, AppReleaseInput{
		Version: "2.0.0", ReleaseNotes: "重复版本",
	}); !errors.Is(err, ErrAppReleaseVersionExists) {
		t.Fatalf("expected duplicate version error, got %v", err)
	}
	if _, err := service.UpdateAppRelease(ctx, 99999, AppReleaseInput{
		Version: "3.0.0", ReleaseNotes: "不存在",
	}); !errors.Is(err, ErrAppReleaseNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}

	if err := service.DeleteAppRelease(ctx, release.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppReleaseAPK(ctx, release.ID); !errors.Is(err, ErrAppReleaseNotFound) {
		t.Fatalf("expected deleted APK to be unavailable, got %v", err)
	}
	if err := service.DeleteAppRelease(ctx, release.ID); !errors.Is(err, ErrAppReleaseNotFound) {
		t.Fatalf("expected repeated delete to return not found, got %v", err)
	}
	latest, err := service.LatestAppRelease(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != other.ID {
		t.Fatalf("unexpected latest release after deletion: %#v", latest)
	}
}
