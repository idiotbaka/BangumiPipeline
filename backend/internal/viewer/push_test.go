package viewer

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"bangumipipeline.local/server/internal/database"
)

func TestPushServiceDeliversOnlyToSubscribedFollowers(t *testing.T) {
	ctx, db, mediaID := pushFixture(t)
	service := newPushTestService(db)
	sender := &pushSenderRecorder{statusCode: 201}
	service.sender = sender

	for _, userID := range []int64{1, 2} {
		if err := service.UpsertSubscription(ctx, userID, testPushSubscription(fmt.Sprintf("https://push.example.test/subscription/%d", userID))); err != nil {
			t.Fatal(err)
		}
	}

	if err := service.NotifyMediaCompleted(ctx, mediaID, 1001); err != nil {
		t.Fatal(err)
	}
	if len(sender.payloads) != 1 {
		t.Fatalf("expected one follower notification, got %d", len(sender.payloads))
	}
	var payload struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		URL   string `json:"url"`
	}
	if err := json.Unmarshal(sender.payloads[0], &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Title != "《测试番剧》更新了" || payload.Body != "第 3 话 现已可播放" || payload.URL != "/anime/1001?media=10" {
		t.Fatalf("unexpected push payload: %+v", payload)
	}
	var delivered int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM viewer_web_push_deliveries WHERE status = ?`, pushDeliveryDelivered,
	).Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	if delivered != 1 {
		t.Fatalf("expected one delivered notification, got %d", delivered)
	}
}

func TestPushServiceRemovesExpiredSubscription(t *testing.T) {
	ctx, db, mediaID := pushFixture(t)
	service := newPushTestService(db)
	service.sender = &pushSenderRecorder{statusCode: 410}
	if err := service.UpsertSubscription(ctx, 1, testPushSubscription("https://push.example.test/subscription/expired")); err != nil {
		t.Fatal(err)
	}

	if err := service.NotifyMediaCompleted(ctx, mediaID, 1001); err != nil {
		t.Fatal(err)
	}
	var subscriptions int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM viewer_web_push_subscriptions").Scan(&subscriptions); err != nil {
		t.Fatal(err)
	}
	if subscriptions != 0 {
		t.Fatalf("expected invalid subscription to be removed, got %d", subscriptions)
	}
}

type pushSenderRecorder struct {
	statusCode int
	err        error
	payloads   [][]byte
}

func (s *pushSenderRecorder) Send(_ context.Context, _ pushSubscription, payload []byte, _ vapidKeys) (int, error) {
	s.payloads = append(s.payloads, append([]byte(nil), payload...))
	return s.statusCode, s.err
}

func testPushSubscription(endpoint string) PushSubscriptionInput {
	var input PushSubscriptionInput
	input.Endpoint = endpoint
	input.Keys.P256dh = base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...))
	input.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	return input
}

func newPushTestService(db *sql.DB) *PushService {
	return NewPushService(db, slog.New(slog.NewTextHandler(io.Discard, nil)), "notify@example.test")
}

func pushFixture(t *testing.T) (context.Context, *sql.DB, int64) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "push.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, `
INSERT INTO viewer_users(id, username, password_hash, created_at, updated_at)
VALUES (1, 'alice', 'hash', 1, 1), (2, 'bob', 'hash', 2, 2);

INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, created_at)
VALUES (1001, 'https://bgm.tv/subject/1001', 'Original Anime', '测试番剧', 1);

INSERT INTO subscription_items(id, item_key, title, bangumi_id, created_at, updated_at)
VALUES (1, 'item-1', 'Episode 3', 1001, 1, 1);

INSERT INTO download_jobs(id, subscription_item_id, status, created_at, updated_at)
VALUES (1, 1, 'completed', 1, 1);

INSERT INTO media_jobs(
    id, download_job_id, subscription_item_id, bangumi_id, anime_name,
    season_number, episode_type, episode_number, status, output_path, created_at, updated_at
) VALUES (10, 1, 1, 1001, '测试番剧', 1, 'episode', '3', 'completed', '/media/3.mp4', 1, 1);

INSERT INTO viewer_anime_follows(user_id, bangumi_id, created_at, updated_at)
VALUES (1, 1001, 1, 1);`); err != nil {
		t.Fatal(err)
	}
	return ctx, db, 10
}
