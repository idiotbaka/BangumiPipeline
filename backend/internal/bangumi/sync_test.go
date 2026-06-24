package bangumi_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/bangumi"
	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/system"
)

func TestSyncStoresDetailCharactersImagesAndSkipsCompletedStages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	settings := system.NewService(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var calendarRequests, detailRequests, characterRequests, imageRequests atomic.Int32
	const userAgent = "test-user/BangumiPipeline/0.1"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != userAgent {
			t.Errorf("unexpected User-Agent: %q", r.Header.Get("User-Agent"))
		}
		switch r.URL.Path {
		case "/calendar":
			calendarRequests.Add(1)
			writeJSON(w, fmt.Sprintf(`[{"items":[
                    {"id":101,"url":"https://bgm.tv/subject/101","type":2,"name":"テストアニメ","name_cn":"测试动画","air_date":"2026-07-01","air_weekday":3,"images":{"large":%q}},
                    {"id":202,"url":"https://bgm.tv/subject/202","type":1,"name":"Book","images":{"large":%q}}
                ]}]`, server.URL+"/cover.jpg", server.URL+"/book.jpg"))
		case "/v0/subjects/101":
			detailRequests.Add(1)
			writeJSON(w, `{
                    "date":"2026-07-01","platform":"TV","summary":"详细简介","name":"テストアニメ","name_cn":"测试动画",
                    "tags":[{"name":"恋爱","count":12,"total_cont":2},{"name":"校园","count":8,"total_cont":1}],
                    "infobox":[{"key":"别名","value":[{"v":"Alias One"},{"v":"别名二"}]},{"key":"话数","value":"12"},{"key":"动态字段","value":{"nested":true}}],
                    "rating":{"rank":1,"score":8.5},"total_episodes":12,"collection":{"wish":100},
                    "id":101,"eps":12,"meta_tags":["TV","恋爱"],"volumes":1,"series":false,"locked":false,"nsfw":false,"type":2
                }`)
		case "/v0/subjects/101/characters":
			characterRequests.Add(1)
			writeJSON(w, fmt.Sprintf(`[{
                    "images":{"large":%q},"name":"角色一","summary":"角色简介","relation":"主角",
					"actors":[{"id":9001,"name":"声优一","career":["seiyu"],"images":{"large":%q}}],"type":1,"id":501
                }]`, server.URL+"/character.jpg", server.URL+"/actor.jpg"))
		case "/cover.jpg", "/character.jpg", "/actor.jpg":
			imageRequests.Add(1)
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fake-jpeg-data"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	coverDir := filepath.Join(t.TempDir(), "covers")
	syncer := newTestSyncer(db, settings, logger, server.URL, userAgent, coverDir)
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	var name, nameCN, summary, platform, detailStatus, characterStatus, infoboxJSON, imagePath string
	var eps, totalEpisodes int
	if err := db.QueryRowContext(ctx, `
SELECT name, name_cn, summary, platform, eps, total_episodes, detail_status, characters_status,
       infobox_json, image_local_path
FROM anime_metadata WHERE bangumi_id = 101`).Scan(
		&name, &nameCN, &summary, &platform, &eps, &totalEpisodes,
		&detailStatus, &characterStatus, &infoboxJSON, &imagePath,
	); err != nil {
		t.Fatal(err)
	}
	if name != "テストアニメ" || nameCN != "测试动画" || summary != "详细简介" || platform != "TV" || eps != 12 || totalEpisodes != 12 {
		t.Fatalf("unexpected subject detail: %q %q %q %q %d %d", name, nameCN, summary, platform, eps, totalEpisodes)
	}
	if detailStatus != "completed" || characterStatus != "completed" || !strings.Contains(infoboxJSON, "动态字段") {
		t.Fatalf("stages or infobox not stored: detail=%s characters=%s infobox=%s", detailStatus, characterStatus, infoboxJSON)
	}
	assertFileExists(t, imagePath)

	assertCount(t, db, "SELECT COUNT(*) FROM anime_tags WHERE bangumi_id = 101", 2)
	assertCount(t, db, "SELECT COUNT(*) FROM anime_aliases WHERE bangumi_id = 101", 2)
	var characterName, characterImage string
	if err := db.QueryRowContext(ctx, `
SELECT name, image_local_path
FROM anime_characters WHERE bangumi_id = 101 AND character_id = 501`).Scan(
		&characterName, &characterImage,
	); err != nil {
		t.Fatal(err)
	}
	if characterName != "角色一" {
		t.Fatalf("unexpected character: %q", characterName)
	}
	assertFileExists(t, characterImage)
	assertCount(t, db, "SELECT COUNT(*) FROM actors WHERE actor_id = 9001", 1)
	assertCount(t, db, "SELECT COUNT(*) FROM character_actors WHERE actor_id = 9001", 1)
	var actorName, actorImage, careerJSON string
	if err := db.QueryRowContext(ctx, "SELECT name, image_local_path, career_json FROM actors WHERE actor_id = 9001").Scan(&actorName, &actorImage, &careerJSON); err != nil {
		t.Fatal(err)
	}
	if actorName != "声优一" || !strings.Contains(careerJSON, "seiyu") {
		t.Fatalf("unexpected normalized actor: name=%q career=%s", actorName, careerJSON)
	}
	assertFileExists(t, actorImage)

	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if calendarRequests.Load() != 2 || detailRequests.Load() != 1 || characterRequests.Load() != 1 || imageRequests.Load() != 3 {
		t.Fatalf("completed stages were fetched again: calendar=%d detail=%d characters=%d images=%d",
			calendarRequests.Load(), detailRequests.Load(), characterRequests.Load(), imageRequests.Load())
	}
}

func TestActorsAreDeduplicatedAcrossSubjects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	settings := system.NewService(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var actorImageRequests atomic.Int32

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/calendar":
			writeJSON(w, `[{"items":[
                    {"id":1,"url":"https://bgm.tv/subject/1","type":2,"name":"Anime 1","images":{}},
                    {"id":2,"url":"https://bgm.tv/subject/2","type":2,"name":"Anime 2","images":{}}
                ]}]`)
		case "/v0/subjects/1", "/v0/subjects/2":
			id := strings.TrimPrefix(r.URL.Path, "/v0/subjects/")
			writeJSON(w, fmt.Sprintf(`{"id":%s,"name":"Anime %s","tags":[],"infobox":[],"meta_tags":[]}`, id, id))
		case "/v0/subjects/1/characters":
			writeJSON(w, fmt.Sprintf(`[{"id":11,"name":"Character 1","actors":[{"id":777,"name":"Shared Actor","images":{"large":%q}}]}]`, server.URL+"/shared-actor.jpg"))
		case "/v0/subjects/2/characters":
			writeJSON(w, fmt.Sprintf(`[{"id":22,"name":"Character 2","actors":[{"id":777,"name":"Shared Actor","images":{"large":%q}}]}]`, server.URL+"/shared-actor.jpg"))
		case "/shared-actor.jpg":
			actorImageRequests.Add(1)
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("actor-image"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	syncer := newTestSyncer(db, settings, logger, server.URL, "test/dedupe", filepath.Join(t.TempDir(), "covers"))
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	assertCount(t, db, "SELECT COUNT(*) FROM actors WHERE actor_id = 777", 1)
	assertCount(t, db, "SELECT COUNT(*) FROM character_actors WHERE actor_id = 777", 2)
	if actorImageRequests.Load() != 1 {
		t.Fatalf("shared actor image downloaded %d times", actorImageRequests.Load())
	}
}

func TestImage404IsTerminalAndNotRetried(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	settings := system.NewService(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var coverRequests, characterImageRequests, actorImageRequests atomic.Int32

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/calendar":
			writeJSON(w, fmt.Sprintf(`[{"items":[{"id":505,"url":"https://bgm.tv/subject/505","type":2,"name":"Missing Images","images":{"large":%q}}]}]`, server.URL+"/missing-cover.jpg"))
		case "/v0/subjects/505":
			writeJSON(w, `{"id":505,"name":"Missing Images","tags":[],"infobox":[],"meta_tags":[]}`)
		case "/v0/subjects/505/characters":
			writeJSON(w, fmt.Sprintf(`[{"id":55,"name":"Missing Character","images":{"large":%q},"actors":[{"id":555,"name":"Missing Actor","images":{"large":%q}}]}]`, server.URL+"/missing-character.jpg", server.URL+"/missing-actor.jpg"))
		case "/missing-cover.jpg":
			coverRequests.Add(1)
			http.NotFound(w, r)
		case "/missing-character.jpg":
			characterImageRequests.Add(1)
			http.NotFound(w, r)
		case "/missing-actor.jpg":
			actorImageRequests.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	syncer := newTestSyncer(db, settings, logger, server.URL, "test/not-found", filepath.Join(t.TempDir(), "covers"))
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	assertImageStatus(t, db, "anime_metadata", "bangumi_id", 505, "not_found")
	assertImageStatus(t, db, "anime_characters", "character_id", 55, "not_found")
	assertImageStatus(t, db, "actors", "actor_id", 555, "not_found")
	if coverRequests.Load() != 1 || characterImageRequests.Load() != 1 || actorImageRequests.Load() != 1 {
		t.Fatalf("404 images were retried: cover=%d character=%d actor=%d", coverRequests.Load(), characterImageRequests.Load(), actorImageRequests.Load())
	}
}

func TestTransientCharacterImageFailureRetriesOnNextRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	settings := system.NewService(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var imageRequests atomic.Int32

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/calendar":
			writeJSON(w, `[{"items":[{"id":606,"url":"https://bgm.tv/subject/606","type":2,"name":"Retry Image","images":{}}]}]`)
		case "/v0/subjects/606":
			writeJSON(w, `{"id":606,"name":"Retry Image","tags":[],"infobox":[],"meta_tags":[]}`)
		case "/v0/subjects/606/characters":
			writeJSON(w, fmt.Sprintf(`[{"id":66,"name":"Retry Character","images":{"large":%q},"actors":[]}]`, server.URL+"/retry-character.jpg"))
		case "/retry-character.jpg":
			if imageRequests.Add(1) == 1 {
				http.Error(w, "temporary", http.StatusBadGateway)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("recovered-image"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	syncer := newTestSyncer(db, settings, logger, server.URL, "test/retry-image", filepath.Join(t.TempDir(), "covers"))
	if err := syncer.Execute(ctx); err == nil {
		t.Fatal("expected transient image failure")
	}
	assertStageStatuses(t, db, 606, "completed", "failed")
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	assertStageStatuses(t, db, 606, "completed", "completed")
	assertImageStatus(t, db, "anime_characters", "character_id", 66, "downloaded")
	if imageRequests.Load() != 2 {
		t.Fatalf("transient character image was requested %d times", imageRequests.Load())
	}
}

func TestTransientAnimeCoverFailureIsPersistedAndRetried(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	settings := system.NewService(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var coverRequests atomic.Int32

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/calendar":
			writeJSON(w, fmt.Sprintf(`[{"items":[{"id":707,"url":"https://bgm.tv/subject/707","type":2,"name":"Retry Cover","images":{"large":%q}}]}]`, server.URL+"/retry-cover.jpg"))
		case "/v0/subjects/707":
			writeJSON(w, `{"id":707,"name":"Retry Cover","tags":[],"infobox":[],"meta_tags":[]}`)
		case "/v0/subjects/707/characters":
			writeJSON(w, `[]`)
		case "/retry-cover.jpg":
			if coverRequests.Add(1) == 1 {
				http.Error(w, "temporary", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("recovered-cover"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	syncer := newTestSyncer(db, settings, logger, server.URL, "test/retry-cover", filepath.Join(t.TempDir(), "covers"))
	if err := syncer.Execute(ctx); err == nil {
		t.Fatal("expected transient cover failure")
	}
	assertImageStatus(t, db, "anime_metadata", "bangumi_id", 707, "failed")
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	assertImageStatus(t, db, "anime_metadata", "bangumi_id", 707, "downloaded")
	if coverRequests.Load() != 2 {
		t.Fatalf("transient anime cover was requested %d times", coverRequests.Load())
	}
}

func TestFailedCharacterStageRetriesWithoutRefetchingCompletedDetail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	settings := system.NewService(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var detailRequests, characterRequests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/calendar":
			writeJSON(w, `[{"items":[{"id":303,"url":"https://bgm.tv/subject/303","type":2,"name":"Retry","images":{}}]}]`)
		case "/v0/subjects/303":
			detailRequests.Add(1)
			writeJSON(w, `{"id":303,"name":"Retry","tags":[],"infobox":[],"meta_tags":[]}`)
		case "/v0/subjects/303/characters":
			if characterRequests.Add(1) == 1 {
				http.Error(w, "temporary failure", http.StatusBadGateway)
				return
			}
			writeJSON(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	syncer := newTestSyncer(db, settings, logger, server.URL, "test/retry", filepath.Join(t.TempDir(), "covers"))
	if err := syncer.Execute(ctx); err == nil {
		t.Fatal("expected first character synchronization to fail")
	}
	assertStageStatuses(t, db, 303, "completed", "failed")
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	assertStageStatuses(t, db, 303, "completed", "completed")
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if detailRequests.Load() != 1 || characterRequests.Load() != 2 {
		t.Fatalf("unexpected retry counts: detail=%d characters=%d", detailRequests.Load(), characterRequests.Load())
	}
}

func TestSubjectAPIRequestsRespectConfiguredInterval(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	settings := system.NewService(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var mu sync.Mutex
	requestTimes := make([]time.Time, 0, 3)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestTimes = append(requestTimes, time.Now())
		mu.Unlock()
		switch r.URL.Path {
		case "/calendar":
			writeJSON(w, `[{"items":[{"id":404,"url":"https://bgm.tv/subject/404","type":2,"name":"Limited","images":{}}]}]`)
		case "/v0/subjects/404":
			writeJSON(w, `{"id":404,"name":"Limited","tags":[],"infobox":[],"meta_tags":[]}`)
		case "/v0/subjects/404/characters":
			writeJSON(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	const interval = 30 * time.Millisecond
	syncer := bangumi.NewSyncer(db, settings, logger, bangumi.SyncerConfig{
		APIBaseURL: server.URL, UserAgent: "test/limited", CoverDir: filepath.Join(t.TempDir(), "covers"),
		APIInterval: interval, RequestTimeout: 2 * time.Second,
	})
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	times := append([]time.Time(nil), requestTimes...)
	mu.Unlock()
	if len(times) != 3 {
		t.Fatalf("expected three API requests, got %d", len(times))
	}
	for index := 1; index < len(times); index++ {
		if gap := times[index].Sub(times[index-1]); gap < interval-5*time.Millisecond {
			t.Fatalf("API requests were not rate limited: gap %s", gap)
		}
	}
}

func TestSyncStoresOnlyFirstTenCharacters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	settings := system.NewService(db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/calendar":
			writeJSON(w, `[{"items":[{"id":919,"url":"https://bgm.tv/subject/919","type":2,"name":"Many Characters","images":{}}]}]`)
		case "/v0/subjects/919":
			writeJSON(w, `{"id":919,"name":"Many Characters","tags":[],"infobox":[],"meta_tags":[]}`)
		case "/v0/subjects/919/characters":
			characters := make([]string, 0, 12)
			for id := 1; id <= 12; id++ {
				characters = append(characters, fmt.Sprintf(`{"id":%d,"name":"Character %d","images":{},"actors":[]}`, id, id))
			}
			writeJSON(w, "["+strings.Join(characters, ",")+"]")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	syncer := newTestSyncer(db, settings, logger, server.URL, "test/character-limit", filepath.Join(t.TempDir(), "covers"))
	if err := syncer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	assertCount(t, db, "SELECT COUNT(*) FROM anime_characters WHERE bangumi_id = 919", 10)
	assertCount(t, db, "SELECT COUNT(*) FROM anime_characters WHERE bangumi_id = 919 AND character_id > 10", 0)
}

func newTestSyncer(db *sql.DB, settings *system.Service, logger *slog.Logger, baseURL, userAgent, coverDir string) *bangumi.Syncer {
	return bangumi.NewSyncer(db, settings, logger, bangumi.SyncerConfig{
		APIBaseURL: baseURL, UserAgent: userAgent, CoverDir: coverDir,
		APIInterval: time.Millisecond, RequestTimeout: 2 * time.Second,
	})
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, expected int) {
	t.Helper()
	var count int
	if err := db.QueryRow(query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != expected {
		t.Fatalf("expected count %d, got %d for %s", expected, count, query)
	}
}

func assertStageStatuses(t *testing.T, db *sql.DB, bangumiID int64, detail, characters string) {
	t.Helper()
	var gotDetail, gotCharacters string
	if err := db.QueryRow(`
SELECT detail_status, characters_status FROM anime_metadata WHERE bangumi_id = ?`, bangumiID).
		Scan(&gotDetail, &gotCharacters); err != nil {
		t.Fatal(err)
	}
	if gotDetail != detail || gotCharacters != characters {
		t.Fatalf("unexpected statuses: detail=%s characters=%s", gotDetail, gotCharacters)
	}
}

func assertImageStatus(t *testing.T, db *sql.DB, table, idColumn string, id int64, expected string) {
	t.Helper()
	var status string
	query := fmt.Sprintf("SELECT image_status FROM %s WHERE %s = ?", table, idColumn)
	if err := db.QueryRow(query, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != expected {
		t.Fatalf("expected %s image status %q, got %q", table, expected, status)
	}
}

func openDatabase(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
