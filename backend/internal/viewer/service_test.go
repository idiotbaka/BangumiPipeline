package viewer

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"

	"golang.org/x/crypto/bcrypt"
)

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "viewer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO viewer_users(id, username, password_hash, created_at, updated_at)
VALUES (1, 'alice', ?, 1, 1)`, string(passwordHash)); err != nil {
		t.Fatal(err)
	}

	service := NewService(db, time.Hour)
	service.now = func() time.Time { return time.Unix(100, 0) }
	if err := service.ChangePassword(ctx, 1, "old-password", "new-password", "different-password"); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected password mismatch, got %v", err)
	}
	if err := service.ChangePassword(ctx, 1, "old-password", "short", "short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected invalid password, got %v", err)
	}
	if err := service.ChangePassword(ctx, 1, "wrong-password", "new-password", "new-password"); !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("expected invalid current password, got %v", err)
	}
	if err := service.ChangePassword(ctx, 1, "old-password", "new-password", "new-password"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := service.Login(ctx, "alice", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected old password login to fail, got %v", err)
	}
	user, _, err := service.Login(ctx, "alice", "new-password")
	if err != nil {
		t.Fatalf("expected new password login to succeed: %v", err)
	}
	if user.ID != 1 || user.Username != "alice" {
		t.Fatalf("unexpected user after password change: %+v", user)
	}
	var updatedAt int64
	if err := db.QueryRowContext(ctx, "SELECT updated_at FROM viewer_users WHERE id = 1").Scan(&updatedAt); err != nil {
		t.Fatal(err)
	}
	if updatedAt != 100 {
		t.Fatalf("expected updated_at 100, got %d", updatedAt)
	}
}

func TestManagedUsersIncludeLastActivity(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "viewer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `
INSERT INTO viewer_users(id, username, password_hash, created_at, updated_at)
VALUES (1, 'alice', 'hash', 1, 1), (2, 'bob', 'hash', 2, 2);

INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, eps, total_episodes, created_at)
VALUES (1001, 'https://bgm.tv/subject/1001', 'Original Anime', '测试番剧', 12, 12, 1);

INSERT INTO anime_episodes(bangumi_id, episode_id, ep_number, sort_number, type, name, name_cn, created_at, updated_at)
VALUES
    (1001, 100101, 1, 1, 0, 'Episode One', '第一话', 1, 1),
    (1001, 100102, 2, 2, 0, 'Episode Two', '第二话', 1, 1);

INSERT INTO subscription_items(id, item_key, title, bangumi_id, created_at, updated_at)
VALUES
    (1, 'item-1', 'Episode 1', 1001, 1, 1),
    (2, 'item-2', 'Episode 2', 1001, 1, 1);

INSERT INTO download_jobs(id, subscription_item_id, status, created_at, updated_at)
VALUES
    (1, 1, 'completed', 1, 1),
    (2, 2, 'completed', 1, 1);

INSERT INTO media_jobs(
    id, download_job_id, subscription_item_id, bangumi_id, anime_name,
    season_number, episode_type, episode_number, status, output_path,
    cover_status, cover_path, created_at, updated_at
) VALUES
    (10, 1, 1, 1001, '测试番剧', 1, 'episode', '1', 'completed', '/media/1.mp4', 'completed', '/media/1.jpg', 1, 1),
    (11, 2, 2, 1001, '测试番剧', 1, 'episode', '2', 'completed', '/media/2.mp4', 'completed', '/media/2.jpg', 2, 2);

INSERT INTO viewer_watch_history(
    user_id, bangumi_id, media_job_id, position_seconds, duration_seconds,
    completed, last_watched_at, created_at, updated_at
) VALUES
    (1, 1001, 10, 120, 240, 0, 100, 100, 100),
    (1, 1001, 11, 240, 240, 1, 200, 200, 200);`); err != nil {
		t.Fatal(err)
	}

	service := NewService(db, time.Hour)
	page, err := service.ListUsers(ctx, 1, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected 2 users, got %d", len(page.Items))
	}
	var alice, bob ManagedUser
	for _, user := range page.Items {
		switch user.Username {
		case "alice":
			alice = user
		case "bob":
			bob = user
		}
	}
	if bob.LastActivity != nil {
		t.Fatalf("expected bob to have no activity, got %+v", bob.LastActivity)
	}
	if alice.LastActivity == nil {
		t.Fatal("expected alice to have a last activity")
	}
	if alice.LastActivity.MediaID != 11 || alice.LastActivity.EpisodeLabel != "第 2 话" ||
		alice.LastActivity.EpisodeTitle != "第二话" || alice.LastActivity.ProgressPercent != 100 {
		t.Fatalf("unexpected last activity: %+v", alice.LastActivity)
	}

	activities, err := service.ManagedUserActivities(ctx, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(activities) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(activities))
	}
	if activities[0].MediaID != 11 || activities[1].MediaID != 10 {
		t.Fatalf("activities are not ordered by latest watch time: %+v", activities)
	}
	if activities[0].LatestEpisodeLabel != "第 2 话" {
		t.Fatalf("expected latest episode label to be attached, got %q", activities[0].LatestEpisodeLabel)
	}
}
