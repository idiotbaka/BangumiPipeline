package bangumi

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/system"
)

func TestDecodeEpisodeCommentsPreservesRichContentAndNestedReplies(t *testing.T) {
	const payload = `[{
        "id":1954711,"mainID":1561891,"creatorID":890738,"relatedID":0,"createdAt":1759768704,
		"content":"第一集看完(bgm38)\n[s]删除线[/s]\n[mask]剧透[/mask]\n[img]https://i.vgy.me/hs6IVG.jpg[/img]",
        "state":0,
		"replies":[{
			"id":1954712,"mainID":1561891,"creatorID":43847,"relatedID":1954711,"createdAt":1759768800,
			"content":"楼中楼回复(bgm99)","state":0,
			"replies":[{"id":1954713,"mainID":1561891,"creatorID":43848,"relatedID":1954712,"createdAt":1759768900,"content":"第二层回复","state":0,"user":{"id":43848,"username":"nested-user","nickname":"第二层","avatar":{"small":"n-small","medium":"n-medium","large":"n-large"},"group":10,"sign":"第二层签名","joinedAt":1310514365}}],
			"user":{"id":43847,"username":"reply-user","nickname":"回复者","avatar":{"small":"small-r","medium":"medium-r","large":"large-r"},"group":10,"sign":"回复签名","joinedAt":1310514364}
		}],
        "user":{"id":890738,"username":"main-user","nickname":"主评论者","avatar":{"small":"small","medium":"medium","large":"large"},"group":10,"sign":"个性签名","joinedAt":1720020786},
        "reactions":[{"users":[{"id":577694,"username":"reactor","nickname":"贴贴者"}],"value":54}],
        "futureField":{"kept":true}
    }]`
	comments, err := decodeEpisodeComments(537904, 1561891, rawMessages(t, payload))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 3 {
		t.Fatalf("expected main comment and nested replies, got %d", len(comments))
	}
	main, reply, nested := comments[0], comments[1], comments[2]
	wantContent := "第一集看完(bgm38)\n[s]删除线[/s]\n[mask]剧透[/mask]\n[img]https://i.vgy.me/hs6IVG.jpg[/img]"
	if main.Content != wantContent {
		t.Fatalf("rich content changed:\nwant %q\n got %q", wantContent, main.Content)
	}
	if main.UserSign != "个性签名" || main.AvatarLargeURL != "large" || !strings.Contains(main.ReactionsJSON, `"value":54`) {
		t.Fatalf("user or reaction data missing: %+v reactions=%s", main, main.ReactionsJSON)
	}
	if !strings.Contains(main.RawJSON, `"futureField"`) || !strings.Contains(main.RawJSON, `"replies"`) {
		t.Fatalf("raw JSON did not preserve unknown or nested fields: %s", main.RawJSON)
	}
	if reply.ParentCommentID != main.CommentID || reply.RelatedID != main.CommentID || reply.Depth != 1 || reply.UserSign != "回复签名" {
		t.Fatalf("nested reply was not preserved: %+v", reply)
	}
	if nested.ParentCommentID != reply.CommentID || nested.RelatedID != reply.CommentID || nested.Depth != 2 || nested.UserSign != "第二层签名" {
		t.Fatalf("second-level reply was not preserved: %+v", nested)
	}
}

func TestEpisodeCommentSyncerStoresUpdatesAndRetriesWithoutLosingSnapshot(t *testing.T) {
	ctx := context.Background()
	db := openCommentTestDatabase(t, ctx)
	anchor := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	insertCommentTestMedia(t, ctx, db, 537904, 1561891, "1", anchor)

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/p1/episodes/1561891/comments" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch requests.Add(1) {
		case 1:
			_, _ = io.WriteString(w, `[{
                    "id":100,"mainID":1561891,"creatorID":10,"relatedID":0,"createdAt":1000,
                    "content":"首版(bgm38)[s]保留[/s][mask]隐藏[/mask][img]https://img.example/a.jpg[/img]","state":0,
                    "replies":[{"id":101,"mainID":1561891,"creatorID":11,"relatedID":100,"createdAt":1001,"content":"回复","state":0,"user":{"id":11,"username":"reply","nickname":"回复","avatar":{"small":"rs","medium":"rm","large":"rl"},"group":10,"sign":"回复签名","joinedAt":11}}],
                    "user":{"id":10,"username":"main","nickname":"主楼","avatar":{"small":"s","medium":"m","large":"l"},"group":10,"sign":"主楼签名","joinedAt":10},
                    "reactions":[{"users":[{"id":12,"username":"reactor","nickname":"反应者"}],"value":54}],
                    "unknown":"preserved"
                }]`)
		case 2:
			_, _ = io.WriteString(w, `[{
                    "id":100,"mainID":1561891,"creatorID":10,"relatedID":0,"createdAt":1000,
                    "content":"更新后的正文[mask]新剧透[/mask]","state":0,"replies":[],
                    "user":{"id":10,"username":"main","nickname":"主楼新昵称","avatar":{"small":"s2","medium":"m2","large":"l2"},"group":10,"sign":"更新签名","joinedAt":10},
                    "reactions":[]
                }]`)
		default:
			http.Error(w, "temporary failure", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	syncer := NewEpisodeCommentSyncer(db, system.NewService(db), discardLogger(), EpisodeCommentSyncerConfig{
		APIBaseURL: server.URL, UserAgent: "test/BangumiPipeline-comments",
		APIInterval: time.Millisecond, RequestTimeout: 2 * time.Second, BatchSize: 10,
	})
	syncer.now = func() time.Time { return anchor }
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	assertCommentSyncState(t, ctx, db, 1561891, "pending", 1, anchor.Add(2*time.Hour).Unix(), 0, 1)
	assertStoredComment(t, ctx, db, 1561891, 100, 0, "首版(bgm38)[s]保留[/s][mask]隐藏[/mask][img]https://img.example/a.jpg[/img]", "主楼签名", "l")
	assertStoredComment(t, ctx, db, 1561891, 101, 100, "回复", "回复签名", "rl")

	syncer.now = func() time.Time { return anchor.Add(2 * time.Hour) }
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	assertCommentSyncState(t, ctx, db, 1561891, "pending", 2, anchor.Add(12*time.Hour).Unix(), 0, 1)
	assertStoredComment(t, ctx, db, 1561891, 100, 0, "更新后的正文[mask]新剧透[/mask]", "更新签名", "l2")
	var replyCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bangumi_episode_comments WHERE episode_id = ? AND comment_id = ?", 1561891, 101).Scan(&replyCount); err != nil {
		t.Fatal(err)
	}
	if replyCount != 0 {
		t.Fatalf("expected removed reply to disappear from refreshed snapshot, got %d", replyCount)
	}

	failureAt := anchor.Add(12 * time.Hour)
	syncer.now = func() time.Time { return failureAt }
	if err := syncer.Execute(ctx); err == nil {
		t.Fatal("expected temporary API failure to fail scheduled run")
	}
	assertCommentSyncState(t, ctx, db, 1561891, "pending", 2, failureAt.Add(5*time.Minute).Unix(), 1, 1)
	assertStoredComment(t, ctx, db, 1561891, 100, 0, "更新后的正文[mask]新剧透[/mask]", "更新签名", "l2")
}

func TestEpisodeCommentSyncerBackfillsOldMediaOnlyOnce(t *testing.T) {
	ctx := context.Background()
	db := openCommentTestDatabase(t, ctx)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	insertCommentTestMedia(t, ctx, db, 101, 101001, "1", now.Add(-30*24*time.Hour))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(server.Close)
	syncer := NewEpisodeCommentSyncer(db, system.NewService(db), discardLogger(), EpisodeCommentSyncerConfig{
		APIBaseURL: server.URL, UserAgent: "test/BangumiPipeline-comments",
		APIInterval: time.Millisecond, RequestTimeout: 2 * time.Second,
	})
	syncer.now = func() time.Time { return now }
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	assertCommentSyncState(t, ctx, db, 101001, "completed", len(commentMilestoneOffsets), 0, 0, 0)
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one historical fetch, got %d", requests.Load())
	}
}

func TestEpisodeCommentSyncerEnqueuesCompletedMediaWithoutRestartingExistingSchedule(t *testing.T) {
	ctx := context.Background()
	db := openCommentTestDatabase(t, ctx)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	mediaJobID := insertCommentTestMedia(t, ctx, db, 303, 303001, "1", now)
	syncer := NewEpisodeCommentSyncer(db, system.NewService(db), discardLogger(), EpisodeCommentSyncerConfig{})
	syncer.now = func() time.Time { return now }
	if err := syncer.EnqueueMediaCompleted(ctx, mediaJobID, 303); err != nil {
		t.Fatal(err)
	}
	var anchorMediaJobID, anchorAt, nextFetchAt int64
	var isBackfill bool
	if err := db.QueryRowContext(ctx, `
SELECT anchor_media_job_id, anchor_at, next_fetch_at, is_backfill
FROM bangumi_episode_comment_sync WHERE episode_id = 303001`).Scan(
		&anchorMediaJobID, &anchorAt, &nextFetchAt, &isBackfill,
	); err != nil {
		t.Fatal(err)
	}
	if anchorMediaJobID != mediaJobID || anchorAt != now.Unix() || nextFetchAt != now.Unix() || isBackfill {
		t.Fatalf("unexpected live enqueue state: media=%d anchor=%d next=%d backfill=%v",
			anchorMediaJobID, anchorAt, nextFetchAt, isBackfill)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE bangumi_episode_comment_sync
SET status = 'completed', next_stage = 6, next_fetch_at = NULL, completed_at = ?
WHERE episode_id = 303001`, now.Unix()); err != nil {
		t.Fatal(err)
	}
	syncer.now = func() time.Time { return now.Add(30 * 24 * time.Hour) }
	if err := syncer.EnqueueMediaCompleted(ctx, mediaJobID, 303); err != nil {
		t.Fatal(err)
	}
	assertCommentSyncState(t, ctx, db, 303001, "completed", 6, 0, 0, 0)
}

func TestEpisodeCommentSyncerMarks404NotFound(t *testing.T) {
	ctx := context.Background()
	db := openCommentTestDatabase(t, ctx)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	insertCommentTestMedia(t, ctx, db, 202, 202001, "1", now)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	syncer := NewEpisodeCommentSyncer(db, system.NewService(db), discardLogger(), EpisodeCommentSyncerConfig{
		APIBaseURL: server.URL, UserAgent: "test/BangumiPipeline-comments",
		APIInterval: time.Millisecond, RequestTimeout: 2 * time.Second,
	})
	syncer.now = func() time.Time { return now }
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	assertCommentSyncState(t, ctx, db, 202001, "not_found", len(commentMilestoneOffsets), 0, 0, 0)
}

func TestNextEpisodeCommentMilestoneCollapsesMissedStages(t *testing.T) {
	anchor := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC).Unix()
	tests := []struct {
		name         string
		currentStage int
		now          int64
		wantStage    int
		wantAt       int64
		wantComplete bool
	}{
		{name: "first fetch", currentStage: 0, now: anchor, wantStage: 1, wantAt: anchor + int64(2*time.Hour/time.Second)},
		{name: "missed through one day", currentStage: 0, now: anchor + int64(30*time.Hour/time.Second), wantStage: 4, wantAt: anchor + int64(3*24*time.Hour/time.Second)},
		{name: "old backfill", currentStage: 0, now: anchor + int64(30*24*time.Hour/time.Second), wantStage: 6, wantComplete: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stage, at, complete := nextEpisodeCommentMilestone(anchor, test.currentStage, test.now)
			if stage != test.wantStage || complete != test.wantComplete {
				t.Fatalf("unexpected milestone: stage=%d complete=%v", stage, complete)
			}
			if test.wantComplete {
				if at != nil {
					t.Fatalf("expected no next time, got %d", *at)
				}
			} else if at == nil || *at != test.wantAt {
				t.Fatalf("unexpected next time: %v want %d", at, test.wantAt)
			}
		})
	}
}

func rawMessages(t *testing.T, value string) []json.RawMessage {
	t.Helper()
	var result []json.RawMessage
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func openCommentTestDatabase(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "comments.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertCommentTestMedia(t *testing.T, ctx context.Context, db *sql.DB, bangumiID, episodeID int64, episodeNumber string, completedAt time.Time) int64 {
	t.Helper()
	now := completedAt.Unix()
	if _, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, created_at)
VALUES (?, ?, ?, ?)`, bangumiID, "https://bgm.tv/subject/test", "Test Anime", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO anime_episodes(
    bangumi_id, episode_id, ep_number, sort_number, type, name, created_at, updated_at
) VALUES (?, ?, ?, ?, 0, ?, ?, ?)`, bangumiID, episodeID, 1, 1, "Episode 1", now, now); err != nil {
		t.Fatal(err)
	}
	itemResult, err := db.ExecContext(ctx, `
INSERT INTO subscription_items(item_key, title, created_at, updated_at)
VALUES (?, ?, ?, ?)`, "comment-test-item-"+episodeNumber+"-"+time.Unix(now, 0).Format(time.RFC3339Nano), "Test Episode", now, now)
	if err != nil {
		t.Fatal(err)
	}
	itemID, _ := itemResult.LastInsertId()
	downloadResult, err := db.ExecContext(ctx, `
INSERT INTO download_jobs(subscription_item_id, status, created_at, updated_at)
VALUES (?, 'completed', ?, ?)`, itemID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	downloadID, _ := downloadResult.LastInsertId()
	mediaResult, err := db.ExecContext(ctx, `
INSERT INTO media_jobs(
    download_job_id, subscription_item_id, bangumi_id, anime_name, season_number,
    episode_type, episode_number, status, output_path, created_at, updated_at, completed_at
) VALUES (?, ?, ?, 'Test Anime', 1, 'episode', ?, 'completed', ?, ?, ?, ?)`,
		downloadID, itemID, bangumiID, episodeNumber, filepath.Join("media", episodeNumber+".mp4"), now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	mediaID, _ := mediaResult.LastInsertId()
	return mediaID
}

func assertCommentSyncState(t *testing.T, ctx context.Context, db *sql.DB, episodeID int64, wantStatus string, wantStage int, wantNextAt int64, wantAttempts, wantCount int) {
	t.Helper()
	var status string
	var stage, attempts, count int
	var nextAt sql.NullInt64
	if err := db.QueryRowContext(ctx, `
SELECT status, next_stage, next_fetch_at, attempts, last_comment_count
FROM bangumi_episode_comment_sync WHERE episode_id = ?`, episodeID).Scan(
		&status, &stage, &nextAt, &attempts, &count,
	); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || stage != wantStage || attempts != wantAttempts || count != wantCount {
		t.Fatalf("unexpected sync state: status=%s stage=%d attempts=%d count=%d", status, stage, attempts, count)
	}
	if wantNextAt == 0 {
		if nextAt.Valid {
			t.Fatalf("expected NULL next_fetch_at, got %d", nextAt.Int64)
		}
	} else if !nextAt.Valid || nextAt.Int64 != wantNextAt {
		t.Fatalf("unexpected next_fetch_at: %+v want %d", nextAt, wantNextAt)
	}
}

func assertStoredComment(t *testing.T, ctx context.Context, db *sql.DB, episodeID, commentID, wantParent int64, wantContent, wantSign, wantAvatar string) {
	t.Helper()
	var parent int64
	var content, sign, avatar, reactionsJSON, rawJSON string
	if err := db.QueryRowContext(ctx, `
SELECT parent_comment_id, content, user_sign, avatar_large_url, reactions_json, raw_json
FROM bangumi_episode_comments WHERE episode_id = ? AND comment_id = ?`, episodeID, commentID).Scan(
		&parent, &content, &sign, &avatar, &reactionsJSON, &rawJSON,
	); err != nil {
		t.Fatal(err)
	}
	if parent != wantParent || content != wantContent || sign != wantSign || avatar != wantAvatar {
		t.Fatalf("unexpected stored comment: parent=%d content=%q sign=%q avatar=%q", parent, content, sign, avatar)
	}
	if rawJSON == "" || reactionsJSON == "" {
		t.Fatalf("expected raw and reaction JSON, got raw=%q reactions=%q", rawJSON, reactionsJSON)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
