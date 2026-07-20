package bangumi

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/system"
)

func TestViewerEpisodeCommentsResolvesMediaAndBuildsNewestFirstTree(t *testing.T) {
	ctx := context.Background()
	db := openCommentTestDatabase(t, ctx)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	mediaID := insertCommentTestMedia(t, ctx, db, 537904, 1561891, "1", now)
	syncer := NewEpisodeCommentSyncer(db, system.NewService(db), discardLogger(), EpisodeCommentSyncerConfig{})
	syncer.now = func() time.Time { return now }
	if err := syncer.EnqueueMediaCompleted(ctx, mediaID, 537904); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE bangumi_episode_comment_sync
SET status = 'completed', last_fetched_at = ?, last_comment_count = 2,
    next_stage = 6, next_fetch_at = NULL, completed_at = ?
WHERE episode_id = ?`, now.Unix(), now.Unix(), 1561891); err != nil {
		t.Fatal(err)
	}
	insertViewerComment(t, ctx, db, 537904, 1561891, 100, 0, 1000, 0, "旧主楼(bgm24)", "old", "旧用户", "small", "medium", "large", "旧签名")
	insertViewerComment(t, ctx, db, 537904, 1561891, 101, 100, 1100, 1, "较早回复", "reply-a", "回复甲", "", "", "large-a", "甲签名")
	insertViewerComment(t, ctx, db, 537904, 1561891, 102, 100, 1200, 2, "较新回复", "reply-b", "回复乙", "small-b", "", "", "乙签名")
	insertViewerComment(t, ctx, db, 537904, 1561891, 200, 0, 2000, 3, "新主楼[mask]剧透[/mask]", "new", "新用户", "", "medium-new", "", "新签名")

	result, err := NewCatalog(db).ViewerEpisodeComments(ctx, 537904, mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if result.EpisodeID != 1561891 || result.SyncStatus != "completed" || result.FetchedAt == nil || *result.FetchedAt != now.Unix() {
		t.Fatalf("unexpected episode comment metadata: %+v", result)
	}
	if result.CommentCount != 2 || result.TotalCount != 4 || len(result.Comments) != 2 {
		t.Fatalf("unexpected comment counts: %+v", result)
	}
	if result.Comments[0].CommentID != 200 || result.Comments[1].CommentID != 100 {
		t.Fatalf("root comments are not newest first: %d, %d", result.Comments[0].CommentID, result.Comments[1].CommentID)
	}
	old := result.Comments[1]
	if len(old.Replies) != 2 || old.Replies[0].CommentID != 102 || old.Replies[1].CommentID != 101 {
		t.Fatalf("replies are not newest first: %+v", old.Replies)
	}
	if old.User == nil || old.User.Nickname != "旧用户" || old.User.Sign != "旧签名" || old.User.AvatarURL != "medium" {
		t.Fatalf("comment user data missing: %+v", old.User)
	}
	if old.Replies[0].User == nil || old.Replies[0].User.AvatarURL != "small-b" {
		t.Fatalf("avatar fallback failed: %+v", old.Replies[0].User)
	}
	if _, err := NewCatalog(db).ViewerEpisodeComments(ctx, 537904, mediaID+999); !errors.Is(err, ErrAnimeNotFound) {
		t.Fatalf("expected unrelated media to be rejected, got %v", err)
	}
}

func TestBuildViewerCommentTreeBreaksInvalidParentCycles(t *testing.T) {
	nodes := []*ViewerEpisodeComment{
		{CommentID: 1, ParentCommentID: 2, CreatedAt: 1},
		{CommentID: 2, ParentCommentID: 1, CreatedAt: 2},
		{CommentID: 3, ParentCommentID: 999, CreatedAt: 3},
	}
	roots := buildViewerCommentTree(nodes)
	if len(roots) != 3 || roots[0].CommentID != 3 || roots[1].CommentID != 2 || roots[2].CommentID != 1 {
		t.Fatalf("invalid parent links should become safe roots: %+v", roots)
	}
}

func TestViewerAnimeDetailCountsStoredTopLevelComments(t *testing.T) {
	ctx := context.Background()
	db := openCommentTestDatabase(t, ctx)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	insertCommentTestMedia(t, ctx, db, 537904, 1561891, "1", now)
	if _, err := db.ExecContext(ctx, `UPDATE anime_episodes SET comment_count = 999 WHERE episode_id = ?`, 1561891); err != nil {
		t.Fatal(err)
	}
	insertViewerComment(t, ctx, db, 537904, 1561891, 100, 0, 1000, 0, "主楼一", "one", "用户一", "", "", "", "")
	insertViewerComment(t, ctx, db, 537904, 1561891, 101, 100, 1100, 1, "回复", "reply", "回复用户", "", "", "", "")
	insertViewerComment(t, ctx, db, 537904, 1561891, 200, 0, 1200, 2, "主楼二", "two", "用户二", "", "", "", "")

	detail, err := NewCatalog(db).ViewerAnimeDetail(ctx, 537904)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Episodes) != 1 || detail.Episodes[0].CommentCount != 2 {
		t.Fatalf("expected 2 stored top-level comments instead of stale metadata count: %+v", detail.Episodes)
	}
}

func insertViewerComment(
	t *testing.T,
	ctx context.Context,
	db execQuerier,
	bangumiID, episodeID, commentID, parentID, createdAt int64,
	sortOrder int,
	content, username, nickname, avatarSmall, avatarMedium, avatarLarge, sign string,
) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO bangumi_episode_comments(
    bangumi_id, episode_id, comment_id, parent_comment_id, main_id,
    source_created_at, content, sort_order,
    user_id, username, nickname, avatar_small_url, avatar_medium_url, avatar_large_url,
    user_sign, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		bangumiID, episodeID, commentID, parentID, episodeID,
		createdAt, content, sortOrder,
		commentID+1000, username, nickname, avatarSmall, avatarMedium, avatarLarge,
		sign, createdAt,
	); err != nil {
		t.Fatal(err)
	}
}

type execQuerier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}
