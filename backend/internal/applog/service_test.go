package applog_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/applog"
	"bangumipipeline.local/server/internal/database"
)

func TestLogServiceFiltersOrdersAndPublishes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := applog.NewService(db)
	updates, unsubscribe := service.Subscribe(2)
	t.Cleanup(unsubscribe)

	first, err := service.Write(ctx, "INFO", "test", "first", map[string]any{"value": 1}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Write(ctx, "WARN", "test", "second", nil, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if (<-updates).ID != first.ID || (<-updates).ID != second.ID {
		t.Fatal("subscriber did not receive logs in insertion order")
	}

	warnings, err := service.List(ctx, []string{"WARNING"}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0].Message != "second" || warnings[0].Level != "WARNING" {
		t.Fatalf("unexpected filtered logs: %+v", warnings)
	}
	after, err := service.ListAfter(ctx, nil, first.ID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].ID != second.ID {
		t.Fatalf("unexpected logs after id: %+v", after)
	}
}
