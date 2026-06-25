package subscription

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/system"
)

type testSettingsProvider struct{}

func (testSettingsProvider) GetNetworkSettings(context.Context) (system.NetworkSettings, error) {
	return system.NetworkSettings{}, nil
}

func (testSettingsProvider) GetSubscriptionSettings(context.Context) (system.SubscriptionSettings, error) {
	return system.SubscriptionSettings{}, nil
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

func insertAnime(t *testing.T, ctx context.Context, db *sql.DB, bangumiID int64, now time.Time) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, created_at)
VALUES (?, ?, ?, ?, ?)`, bangumiID, "https://bgm.tv/subject/1001", "Original Anime", "原作标题", now.Unix())
	if err != nil {
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
