package viewer

import "testing"

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
