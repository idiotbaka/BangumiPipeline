package viewer

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
)

func TestSinglePageSelection(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "single-pages.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewService(db, time.Hour)

	settings, err := service.SiteSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.SinglePage != nil {
		t.Fatalf("new site unexpectedly has an active page: %+v", settings.SinglePage)
	}
	config, err := service.SinglePageConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if config.ActiveKey != "" || len(config.Items) != 1 || config.Items[0].Title != "10月秋番报告" {
		t.Fatalf("unexpected page catalog: %+v", config)
	}

	if _, err := service.SetActiveSinglePage(ctx, "unknown"); !errors.Is(err, ErrInvalidSinglePage) {
		t.Fatalf("unknown page error = %v", err)
	}
	config, err = service.SetActiveSinglePage(ctx, "202610-preview")
	if err != nil {
		t.Fatal(err)
	}
	settings, err = service.SiteSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if config.ActiveKey != "202610-preview" || settings.SinglePage == nil || settings.SinglePage.Path != "/special/202610" {
		t.Fatalf("page was not activated: config=%+v settings=%+v", config, settings)
	}

	config, err = service.SetActiveSinglePage(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	settings, err = service.SiteSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if config.ActiveKey != "" || settings.SinglePage != nil {
		t.Fatalf("page was not disabled: config=%+v settings=%+v", config, settings)
	}
}
