package subscription

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/system"
)

type testSettingsProvider struct {
	network      system.NetworkSettings
	subscription system.SubscriptionSettings
}

func (p testSettingsProvider) GetNetworkSettings(context.Context) (system.NetworkSettings, error) {
	return p.network, nil
}

func (p testSettingsProvider) GetSubscriptionSettings(context.Context) (system.SubscriptionSettings, error) {
	return p.subscription, nil
}

func TestExecuteAppliesSubscriptionEpisodeOffset(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	insertAnime(t, ctx, db, 1001, now)
	setAnimeEpisodeOffset(t, ctx, db, 1001, -12)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<item><guid isPermaLink="false">ep13</guid><title>[字幕组] 原作标题 [13][MP4][AVC AAC]</title><link>https://mikanani.me/Home/Episode/ep13</link><enclosure type="application/x-bittorrent" length="100" url="https://mikanani.me/Download/ep13.torrent"/></item>
</channel></rss>`)
	}))
	t.Cleanup(server.Close)

	service := NewService(db, testSettingsProvider{
		subscription: system.SubscriptionSettings{RSSURL: server.URL + "/rss.xml"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.now = func() time.Time { return now }

	if err := service.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	var episodeNumber, reason string
	if err := db.QueryRowContext(ctx, `
SELECT episode_number, match_reason
FROM subscription_items
WHERE item_key = 'ep13' AND match_status = ?`, matchStatusMatched).Scan(&episodeNumber, &reason); err != nil {
		t.Fatal(err)
	}
	if episodeNumber != "1" {
		t.Fatalf("expected offset episode 1, got %q", episodeNumber)
	}
	if reason == "" || !strings.Contains(reason, "番剧索引偏移 -12") {
		t.Fatalf("expected offset in match reason, got %q", reason)
	}
}

func TestOffsetEpisodeNumberClampsToOne(t *testing.T) {
	got, ok := offsetEpisodeNumber("03", -12)
	if !ok {
		t.Fatal("expected numeric episode to be offset")
	}
	if got != "1" {
		t.Fatalf("expected clamped episode 1, got %q", got)
	}
}

func TestSyncHistoryManualRSSAllowsChangedTitleWithFilters(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	insertAnime(t, ctx, db, 1001, now)
	insertBoundSource(t, ctx, db, 1001, "[字幕组][原作标题][12][繁体内嵌]", "12", now)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<item><guid isPermaLink="false">ep10</guid><title>[字幕组][标题变体][10][繁体内嵌]</title><link>https://mikanani.me/Home/Episode/ep10</link><enclosure type="application/x-bittorrent" length="100" url="https://mikanani.me/Download/ep10.torrent"/></item>
<item><guid isPermaLink="false">ep11</guid><title>[字幕组][标题变体][11][简繁外挂]</title><link>https://mikanani.me/Home/Episode/ep11</link><enclosure type="application/x-bittorrent" length="100" url="https://mikanani.me/Download/ep11.torrent"/></item>
<item><guid isPermaLink="false">ep12</guid><title>[字幕组][标题变体][12][繁体内嵌]</title><link>https://mikanani.me/Home/Episode/ep12</link><enclosure type="application/x-bittorrent" length="100" url="https://mikanani.me/Download/ep12.torrent"/></item>
</channel></rss>`)
	}))
	t.Cleanup(server.Close)

	service := NewService(db, testSettingsProvider{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.now = func() time.Time { return now }

	result, err := service.SyncHistory(ctx, 1001, HistorySyncOptions{
		RSSURL:       server.URL + "/RSS/Bangumi?bangumiId=3926&subgroupid=6",
		ExcludeTitle: "外挂",
		IncludeTitle: "繁体",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 3 || result.Inserted != 1 || result.Bound != 1 || result.SkippedExisting != 1 || result.SkippedUnmatched != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	var title, episodeNumber string
	if err := db.QueryRowContext(ctx, `
SELECT title, bound_episode_number
FROM subscription_items
WHERE item_key = 'ep10' AND binding_status = 'bound'`,
	).Scan(&title, &episodeNumber); err != nil {
		t.Fatal(err)
	}
	if title != "[字幕组][标题变体][10][繁体内嵌]" || episodeNumber != "10" {
		t.Fatalf("unexpected bound item: title=%q episode=%q", title, episodeNumber)
	}
	assertItemMissing(t, ctx, db, "ep11")
	assertItemMissing(t, ctx, db, "ep12")
}

func TestSyncHistoryWithoutBoundSourceRequiresManualRSS(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	insertAnime(t, ctx, db, 1001, now)

	service := NewService(db, testSettingsProvider{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err = service.SyncHistory(ctx, 1001, HistorySyncOptions{})
	if !errors.Is(err, ErrHistoryRSSURLRequired) {
		t.Fatalf("expected ErrHistoryRSSURLRequired, got %v", err)
	}
}

func TestSyncHistoryManualRSSWithoutBoundSource(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	insertAnime(t, ctx, db, 1001, now)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<item><guid isPermaLink="false">ep01</guid><title>[字幕组][原作标题][01][繁体内嵌]</title><link>https://mikanani.me/Home/Episode/ep01</link><enclosure type="application/x-bittorrent" length="100" url="https://mikanani.me/Download/ep01.torrent"/></item>
<item><guid isPermaLink="false">ep02</guid><title>[字幕组][原作标题][02][繁体内嵌]</title><link>https://mikanani.me/Home/Episode/ep02</link><enclosure type="application/x-bittorrent" length="100" url="https://mikanani.me/Download/ep02.torrent"/></item>
<item><guid isPermaLink="false">credit</guid><title>[字幕组][原作标题][NCOP][繁体内嵌]</title><link>https://mikanani.me/Home/Episode/credit</link><enclosure type="application/x-bittorrent" length="100" url="https://mikanani.me/Download/credit.torrent"/></item>
</channel></rss>`)
	}))
	t.Cleanup(server.Close)

	service := NewService(db, testSettingsProvider{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.now = func() time.Time { return now }

	result, err := service.SyncHistory(ctx, 1001, HistorySyncOptions{RSSURL: server.URL + "/RSS/Bangumi?bangumiId=3926&subgroupid=6"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 3 || result.Inserted != 2 || result.Bound != 2 || result.SkippedUnmatched != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.SourceTitle != "" || result.SearchTitle != "" {
		t.Fatalf("manual RSS without source should not report source/search title: %+v", result)
	}

	var boundCount int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM subscription_items
WHERE binding_status = ? AND bound_bangumi_id = ?`, BindingStatusBound, 1001).Scan(&boundCount); err != nil {
		t.Fatal(err)
	}
	if boundCount != 2 {
		t.Fatalf("expected 2 bound history items, got %d", boundCount)
	}

	var animeName, episodeNumber string
	if err := db.QueryRowContext(ctx, `
SELECT bound_anime_name, bound_episode_number
FROM subscription_items
WHERE item_key = 'ep01' AND binding_status = 'bound'`,
	).Scan(&animeName, &episodeNumber); err != nil {
		t.Fatal(err)
	}
	if animeName != "原作标题" || episodeNumber != "01" {
		t.Fatalf("unexpected bound item: anime=%q episode=%q", animeName, episodeNumber)
	}
	assertItemMissing(t, ctx, db, "credit")
}

func TestSyncManualEpisodeReplacesExistingBindingAndQueuesDownload(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	insertAnime(t, ctx, db, 1001, now)
	insertBoundSource(t, ctx, db, 1001, "[字幕组][原作标题][03][繁体内嵌]", "03", now)

	service := NewService(db, testSettingsProvider{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.now = func() time.Time { return now }
	magnet := "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=episode03"

	result, err := service.SyncManualEpisode(ctx, 1001, ManualEpisodeInput{
		MagnetURL: magnet, SeasonNumber: 1, EpisodeType: "", EpisodeNumber: "03",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DownloadJobID < 1 {
		t.Fatalf("expected download job id, got %d", result.DownloadJobID)
	}
	if result.Item.BindingStatus != BindingStatusBound || result.Item.BoundEpisodeNumber != "03" {
		t.Fatalf("unexpected manual item: %+v", result.Item)
	}
	if result.Item.Link != magnet || result.Item.TorrentURL != magnet {
		t.Fatalf("manual item should use magnet as link and torrent url: %+v", result.Item)
	}

	var oldStatus string
	var oldBangumiID sql.NullInt64
	if err := db.QueryRowContext(ctx, `
SELECT binding_status, bound_bangumi_id
FROM subscription_items
WHERE item_key = 'source'`).Scan(&oldStatus, &oldBangumiID); err != nil {
		t.Fatal(err)
	}
	if oldStatus != BindingStatusPending || oldBangumiID.Valid {
		t.Fatalf("old binding should be unbound, got status=%q bangumi_valid=%v", oldStatus, oldBangumiID.Valid)
	}

	var jobStatus, sourceURL string
	if err := db.QueryRowContext(ctx, `
SELECT status, source_url
FROM download_jobs
WHERE id = ?`, result.DownloadJobID).Scan(&jobStatus, &sourceURL); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "pending" || sourceURL != magnet {
		t.Fatalf("unexpected download job: status=%q source=%q", jobStatus, sourceURL)
	}
}

func insertAnime(t *testing.T, ctx context.Context, db *sql.DB, bangumiID int64, now time.Time) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, created_at)
VALUES (?, ?, ?, ?, ?)`, bangumiID, "https://bgm.tv/subject/1001", "Original Anime", "原作标题", now.Unix())
	if err != nil {
		t.Fatal(err)
	}
}

func setAnimeEpisodeOffset(t *testing.T, ctx context.Context, db *sql.DB, bangumiID int64, offset int) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
UPDATE anime_metadata
SET subscription_episode_offset = ?
WHERE bangumi_id = ?`, offset, bangumiID); err != nil {
		t.Fatal(err)
	}
}

func insertBoundSource(t *testing.T, ctx context.Context, db *sql.DB, bangumiID int64, title, episodeNumber string, now time.Time) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
INSERT INTO subscription_items(
    item_key, guid, title, match_status, bangumi_id, matched_name, parsed_name,
    season_number, episode_type, episode_number, binding_status, bound_bangumi_id,
    bound_anime_name, bound_season_number, bound_episode_type, bound_episode_number,
    binding_note, bound_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"source", "source", title, matchStatusMatched, bangumiID, "原作标题", "原作标题",
		1, "episode", episodeNumber, BindingStatusBound, bangumiID,
		"原作标题", 1, "episode", episodeNumber,
		"手动绑定", now.Unix(), now.Unix(), now.Unix())
	if err != nil {
		t.Fatal(err)
	}
}

func assertItemMissing(t *testing.T, ctx context.Context, db *sql.DB, itemKey string) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_items WHERE item_key = ?", itemKey).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected item %q to be skipped, got count %d", itemKey, count)
	}
}
