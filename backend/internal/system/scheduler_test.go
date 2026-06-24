package system_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/system"
)

func TestSchedulerManualRunAndStatusLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := system.NewService(db)
	interval := 30
	enabled := true
	task, err := service.UpdateScheduledTask(ctx, "bangumi-season-metadata", system.TaskUpdate{
		Enabled: &enabled, IntervalMinutes: &interval,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.IntervalMinutes != 30 || task.NextRunAt == nil {
		t.Fatalf("unexpected task configuration: %+v", task)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scheduler := system.NewScheduler(service, logger, time.Hour)
	started := make(chan struct{})
	release := make(chan struct{})
	scheduler.Register("bangumi-season-metadata", system.ExecutorFunc(func(context.Context) error {
		close(started)
		<-release
		return nil
	}))
	if err := scheduler.Start(ctx); err != nil {
		t.Fatal(err)
	}
	running, err := scheduler.Trigger("bangumi-season-metadata")
	if err != nil {
		t.Fatal(err)
	}
	if running.LastStatus != system.TaskStatusRunning || running.LastStartedAt == nil {
		t.Fatalf("task was not marked running: %+v", running)
	}
	<-started
	if _, err := db.ExecContext(ctx, "UPDATE scheduled_tasks SET next_run_at = unixepoch() - 1 WHERE task_key = 'bangumi-season-metadata'"); err != nil {
		t.Fatal(err)
	}
	due, err := service.DueTaskKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("running task should be skipped even when its next execution time has passed: %v", due)
	}
	if _, err := scheduler.Trigger("bangumi-season-metadata"); !errors.Is(err, system.ErrTaskAlreadyRunning) {
		t.Fatalf("expected already running error, got %v", err)
	}
	close(release)
	waitForStatus(t, ctx, service, system.TaskStatusCompleted)

	scheduler.Register("bangumi-season-metadata", system.ExecutorFunc(func(context.Context) error {
		return errors.New("upstream unavailable")
	}))
	if _, err := scheduler.Trigger("bangumi-season-metadata"); err != nil {
		t.Fatal(err)
	}
	failed := waitForStatus(t, ctx, service, system.TaskStatusFailed)
	if failed.LastError != "upstream unavailable" || failed.LastFinishedAt == nil {
		t.Fatalf("failure details were not persisted: %+v", failed)
	}
}

func waitForStatus(t *testing.T, ctx context.Context, service *system.Service, status string) system.ScheduledTask {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		task, err := service.ScheduledTask(ctx, "bangumi-season-metadata")
		if err != nil {
			t.Fatal(err)
		}
		if task.LastStatus == status {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task did not reach status %s", status)
	return system.ScheduledTask{}
}
