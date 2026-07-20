package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/bangumi"
	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/httpapi"
	"bangumipipeline.local/server/internal/viewer"
)

func TestViewerEpisodeCommentsAndSmileAssetRequireViewerSession(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "viewer-comments-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Unix()
	mediaID := insertViewerCommentAPIData(t, ctx, db, now)

	smileDir := filepath.Join(t.TempDir(), "smiles")
	if err := os.MkdirAll(smileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	smileData := []byte("GIF89a\x01\x00\x01\x00")
	if err := os.WriteFile(filepath.Join(smileDir, "bgm24.gif"), smileData, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := bangumi.BangumiSmileManifest{
		Version: bangumi.BangumiSmileManifestVersion, Complete: true, ExpectedCount: bangumi.BangumiSmileAssetCount,
		Assets: map[string]bangumi.BangumiSmileAsset{
			"(bgm24)": {Code: "(bgm24)", File: "bgm24.gif", ContentType: "image/gif", SourceURL: "https://bangumi.tv/img/smiles/tv/01.gif"},
		},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(smileDir, "manifest.json"), manifestData, 0o644); err != nil {
		t.Fatal(err)
	}

	auth := viewer.NewService(db, time.Hour)
	if _, err := auth.UpdateSiteSettings(ctx, viewer.SiteSettingsUpdate{
		SiteName: "Test Viewer", RegistrationEnabled: true, InviteRequired: false,
	}); err != nil {
		t.Fatal(err)
	}
	_, session, err := auth.Register(ctx, "comment-viewer", "viewer-password-123", "")
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewViewerHandler(
		auth, nil, bangumi.NewCatalog(db), logger, false, t.TempDir(), smileDir,
	))
	defer server.Close()

	commentsURL := server.URL + "/api/anime/537904/media/" + stringID(mediaID) + "/comments"
	unauthorized, err := server.Client().Get(commentsURL)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized comments status = %d", unauthorized.StatusCode)
	}

	request, _ := http.NewRequest(http.MethodGet, commentsURL, nil)
	request.Header.Set("Authorization", "Bearer "+session.Token)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("comments status = %d body=%s", response.StatusCode, body)
	}
	var payload struct {
		Episode bangumi.ViewerEpisodeComments `json:"episode"`
		Smiles  map[string]string             `json:"smiles"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Episode.EpisodeID != 1561891 || payload.Episode.TotalCount != 1 || len(payload.Episode.Comments) != 1 {
		t.Fatalf("unexpected comments payload: %+v", payload.Episode)
	}
	if payload.Smiles["(bgm24)"] != "/api/bangumi-smiles/bgm24" {
		t.Fatalf("unexpected smile mapping: %+v", payload.Smiles)
	}

	smileRequest, _ := http.NewRequest(http.MethodGet, server.URL+payload.Smiles["(bgm24)"], nil)
	smileRequest.Header.Set("Authorization", "Bearer "+session.Token)
	smileResponse, err := server.Client().Do(smileRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer smileResponse.Body.Close()
	servedSmile, _ := io.ReadAll(smileResponse.Body)
	if smileResponse.StatusCode != http.StatusOK || smileResponse.Header.Get("Content-Type") != "image/gif" || string(servedSmile) != string(smileData) {
		t.Fatalf("unexpected smile response: status=%d type=%q body=%q", smileResponse.StatusCode, smileResponse.Header.Get("Content-Type"), servedSmile)
	}
	if strings.Contains(payload.Smiles["(bgm24)"], smileDir) {
		t.Fatal("viewer response exposed the local smile directory")
	}
}

func insertViewerCommentAPIData(t *testing.T, ctx context.Context, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, now int64) int64 {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT INTO anime_metadata(bangumi_id, url, name, created_at) VALUES (537904, 'https://bgm.tv/subject/537904', 'Test', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO anime_episodes(bangumi_id, episode_id, ep_number, sort_number, type, name, created_at, updated_at) VALUES (537904, 1561891, 1, 1, 0, 'Episode 1', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	item, err := db.ExecContext(ctx, `INSERT INTO subscription_items(item_key, title, created_at, updated_at) VALUES ('viewer-comment-api-item', 'Episode', ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	itemID, _ := item.LastInsertId()
	download, err := db.ExecContext(ctx, `INSERT INTO download_jobs(subscription_item_id, status, created_at, updated_at) VALUES (?, 'completed', ?, ?)`, itemID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	downloadID, _ := download.LastInsertId()
	media, err := db.ExecContext(ctx, `
INSERT INTO media_jobs(download_job_id, subscription_item_id, bangumi_id, anime_name, season_number,
    episode_type, episode_number, status, output_path, created_at, updated_at, completed_at)
VALUES (?, ?, 537904, 'Test', 1, 'episode', '1', 'completed', 'private/output.mp4', ?, ?, ?)`, downloadID, itemID, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	mediaID, _ := media.LastInsertId()
	if _, err := db.ExecContext(ctx, `
INSERT INTO bangumi_episode_comment_sync(episode_id, bangumi_id, anchor_media_job_id, anchor_at, status,
    next_stage, last_fetched_at, last_comment_count, completed_at, created_at, updated_at)
VALUES (1561891, 537904, ?, ?, 'completed', 6, ?, 1, ?, ?, ?)`, mediaID, now, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO bangumi_episode_comments(bangumi_id, episode_id, comment_id, main_id, source_created_at,
    content, user_id, username, nickname, user_sign, fetched_at)
VALUES (537904, 1561891, 1, 1561891, ?, '测试评论(bgm24)', 2, 'user', '用户', '签名', ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	return mediaID
}
