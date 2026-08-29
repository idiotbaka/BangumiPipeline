package viewer

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
)

func TestFollowedAnimeUsesRegularEpisodeTotal(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "viewer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `
INSERT INTO viewer_users(id, username, password_hash, created_at, updated_at)
VALUES (1, 'alice', 'hash', 1, 1);
INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, eps, total_episodes, created_at)
VALUES (1001, 'https://bgm.tv/subject/1001', 'Regular Anime', '正片番剧', 12, 13, 1);
INSERT INTO viewer_anime_follows(user_id, bangumi_id, created_at, updated_at)
VALUES (1, 1001, 1, 1);
INSERT INTO subscription_items(id, item_key, title, bangumi_id, created_at, updated_at)
VALUES (1, 'episode-12', 'Episode 12', 1001, 1, 1);
INSERT INTO download_jobs(id, subscription_item_id, status, created_at, updated_at)
VALUES (1, 1, 'completed', 1, 1);
INSERT INTO media_jobs(
    id, download_job_id, subscription_item_id, bangumi_id, anime_name,
    season_number, episode_type, episode_number, status, output_path,
    created_at, updated_at, completed_at
) VALUES (1, 1, 1, 1001, '正片番剧', 1, 'episode', '12', 'completed', '/media/12.mp4', 1, 1, 1);`); err != nil {
		t.Fatal(err)
	}

	items, err := NewService(db, time.Hour).FollowedAnime(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TotalEpisodes != 12 || !items[0].IsCompleted {
		t.Fatalf("follow should use the regular episode total for display and completion: %+v", items)
	}
}

func TestFollowedAnimeCompletedRequiresFinalRegularEpisode(t *testing.T) {
	cases := []struct {
		name          string
		totalEpisodes int
		episodeType   string
		episodeNumber string
		want          bool
	}{
		{name: "unknown total", totalEpisodes: 0, episodeType: "episode", episodeNumber: "12"},
		{name: "earlier regular episode", totalEpisodes: 12, episodeType: "episode", episodeNumber: "11"},
		{name: "fractional regular episode", totalEpisodes: 13, episodeType: "episode", episodeNumber: "13.5"},
		{name: "sp final number", totalEpisodes: 12, episodeType: "sp", episodeNumber: "12"},
		{name: "ova final number", totalEpisodes: 12, episodeType: "ova", episodeNumber: "12"},
		{name: "oad final number", totalEpisodes: 12, episodeType: "oad", episodeNumber: "12"},
		{name: "final regular episode", totalEpisodes: 12, episodeType: "episode", episodeNumber: "12", want: true},
		{name: "legacy regular episode", totalEpisodes: 12, episodeType: "", episodeNumber: "12.0", want: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			media := []followedMedia{{ref: historyEpisodeRef{
				episodeType: test.episodeType, episodeNumber: test.episodeNumber,
			}}}
			if got := followedAnimeCompleted(test.totalEpisodes, media); got != test.want {
				t.Fatalf("followedAnimeCompleted() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSortFollowedAnimePlacesCompletedFollowsLast(t *testing.T) {
	items := []FollowedAnime{
		{BangumiID: 1, WatchCompleted: true, CaughtUp: true, LastWatchedAt: 400, FollowedAt: 1},
		{BangumiID: 2, WatchCompleted: false, CaughtUp: false, LastWatchedAt: 100, FollowedAt: 2},
		{BangumiID: 3, WatchCompleted: true, CaughtUp: false, LastWatchedAt: 300, FollowedAt: 3},
		{BangumiID: 4, WatchCompleted: false, CaughtUp: false, LastWatchedAt: 200, FollowedAt: 4},
	}

	sortFollowedAnime(items)

	want := []int64{3, 4, 2, 1}
	for index, bangumiID := range want {
		if items[index].BangumiID != bangumiID {
			t.Fatalf("unexpected follow order: got %+v, want bangumi ID %d at index %d", items, bangumiID, index)
		}
	}
}

func TestHomeFollowedAnimeFiltersCaughtUpAndPrioritizesNewEpisodes(t *testing.T) {
	items := []FollowedAnime{
		{BangumiID: 1, LastWatchedAt: 500},
		{BangumiID: 2, hasNewEpisode: true, latestUpdatedAt: 200},
		{BangumiID: 3, CaughtUp: true, LastWatchedAt: 600},
		{BangumiID: 4, hasNewEpisode: true, latestUpdatedAt: 300},
	}

	homeItems := homeFollowedAnime(items)

	want := []int64{4, 2, 1}
	if len(homeItems) != len(want) {
		t.Fatalf("unexpected home follow count: got %+v, want %d items", homeItems, len(want))
	}
	for index, bangumiID := range want {
		if homeItems[index].BangumiID != bangumiID {
			t.Fatalf("unexpected home follow order: got %+v, want bangumi ID %d at index %d", homeItems, bangumiID, index)
		}
	}
	if len(items) != 4 {
		t.Fatalf("home filtering changed the complete follows list: got %+v", items)
	}
}
