package bangumi

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/system"
)

func TestCommentAvatarStoreBackfillsDownloadsAndCachesByUserID(t *testing.T) {
	ctx := context.Background()
	db := openCommentTestDatabase(t, ctx)
	insertCommentTestMedia(t, ctx, db, 537904, 1561891, "1", time.Unix(1, 0).UTC())
	var requests atomic.Int32
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var avatarJPEG bytes.Buffer
	if err := jpeg.Encode(&avatarJPEG, img, &jpeg.Options{Quality: 75}); err != nil {
		t.Fatal(err)
	}
	avatarBytes := avatarJPEG.Bytes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(avatarBytes)
	}))
	t.Cleanup(server.Close)

	if _, err := db.ExecContext(ctx, `
INSERT INTO bangumi_episode_comments(
    bangumi_id, episode_id, comment_id, main_id, user_id, avatar_medium_url, fetched_at
) VALUES
    (537904, 1561891, 1, 1561891, 100, ?, 1),
    (537904, 1561891, 2, 1561891, 100, ?, 2),
    (537904, 1561891, 3, 1561891, 200, ?, 2)`,
		server.URL+"/old", server.URL+"/latest", server.URL+"/missing"); err != nil {
		t.Fatal(err)
	}
	avatarDir := filepath.Join(t.TempDir(), "avatar")
	store := NewBangumiCommentAvatarStore(db, discardLogger(), BangumiCommentAvatarSyncConfig{
		Directory: avatarDir, UserAgent: "test/BangumiPipeline-avatar",
		RequestTimeout: 2 * time.Second,
	})
	store.now = func() time.Time { return time.Unix(100, 0).UTC() }
	discovered, err := store.EnqueueHistorical(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if discovered != 2 {
		t.Fatalf("expected two unique historical avatar users, got %d", discovered)
	}
	assertCommentAvatarQueue(t, ctx, db, 100, server.URL+"/latest", "pending")
	allDue, err := store.dueJobs(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(allDue) != 2 {
		t.Fatalf("unlimited due query should return every avatar, got %d", len(allDue))
	}

	result, err := store.SyncPending(ctx, system.NetworkSettings{}, 10)
	if err != nil {
		t.Fatalf("404 should not fail the batch: %v", err)
	}
	if result.Due != 2 || result.Downloaded != 1 || result.NotFound != 1 || result.Failed != 0 {
		t.Fatalf("unexpected avatar result: %+v", result)
	}
	avatarPath := filepath.Join(avatarDir, "100.jpg")
	if info, err := os.Stat(avatarPath); err != nil || info.Size() <= 0 {
		t.Fatalf("downloaded avatar missing: info=%v err=%v", info, err)
	}
	storedAvatar, err := os.ReadFile(avatarPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(storedAvatar, avatarBytes) {
		t.Fatal("downloaded avatar was unexpectedly recompressed or modified")
	}
	assertCommentAvatarQueue(t, ctx, db, 100, server.URL+"/latest", "downloaded")
	assertCommentAvatarQueue(t, ctx, db, 200, server.URL+"/missing", "not_found")

	second, err := store.SyncPending(ctx, system.NetworkSettings{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if second.Due != 0 || requests.Load() != 2 {
		t.Fatalf("downloaded avatars should be reused without new requests: result=%+v requests=%d", second, requests.Load())
	}
}

func TestCommentAvatarURLChangeInvalidatesCachedFile(t *testing.T) {
	ctx := context.Background()
	db := openCommentTestDatabase(t, ctx)
	now := time.Unix(200, 0).UTC()
	avatarDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(avatarDir, "300.jpg"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO bangumi_comment_user_avatars(
    user_id, medium_url, file_name, content_type, status, downloaded_at, created_at, updated_at
) VALUES (300, 'old-url', '300.jpg', 'image/jpeg', 'downloaded', 1, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := upsertCommentAvatarCandidate(ctx, db, 300, "new-url", now.Unix()); err != nil {
		t.Fatal(err)
	}
	var fileName, status string
	if err := db.QueryRowContext(ctx, `
SELECT file_name, status FROM bangumi_comment_user_avatars WHERE user_id = 300`).Scan(&fileName, &status); err != nil {
		t.Fatal(err)
	}
	if fileName != "" || status != "pending" {
		t.Fatalf("changed avatar URL did not invalidate cache: file=%q status=%q", fileName, status)
	}
}
