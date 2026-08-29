package bangumi_test

import (
	"context"
	"testing"

	"bangumipipeline.local/server/internal/bangumi"
)

func TestViewerScheduleIncludesLatestEpisodeUpdatedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	if _, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, air_date, air_weekday, eps, total_episodes, created_at)
VALUES
    (7001, 'https://bgm.tv/subject/7001', 'Updated Anime', '更新番剧', '2026-07-01', 3, 2, 0, 1),
    (7002, 'https://bgm.tv/subject/7002', 'Special Anime', '特别篇番剧', '2026-07-02', 4, 2, 2, 1);

INSERT INTO anime_tags(bangumi_id, name, count)
VALUES (7001, '2026年7月', 1), (7002, '2026年7月', 1);

INSERT INTO subscription_items(id, item_key, title, bangumi_id, created_at, updated_at)
VALUES
    (7001, 'schedule-1', 'Episode 1', 7001, 1, 1),
    (7002, 'schedule-2', 'Episode 2', 7001, 1, 1),
    (7003, 'schedule-special', 'Special 2', 7002, 1, 1);

INSERT INTO download_jobs(id, subscription_item_id, status, created_at, updated_at)
VALUES
    (7001, 7001, 'completed', 1, 1),
    (7002, 7002, 'completed', 1, 1),
    (7003, 7003, 'completed', 1, 1);

INSERT INTO media_jobs(
    id, download_job_id, subscription_item_id, bangumi_id, anime_name,
    season_number, episode_type, episode_number, status, output_path,
    created_at, updated_at, completed_at
) VALUES
    (7001, 7001, 7001, 7001, '更新番剧', 1, 'episode', '1', 'completed', '/media/1.mp4', 100, 100, 100),
    (7002, 7002, 7002, 7001, '更新番剧', 1, 'episode', '2', 'completed', '/media/2.mp4', 200, 200, 200),
    (7003, 7003, 7003, 7002, '特别篇番剧', 1, 'sp', '2', 'completed', '/media/sp-2.mp4', 300, 300, 300);`); err != nil {
		t.Fatal(err)
	}

	schedule, err := bangumi.NewCatalog(db).ViewerSchedule(ctx, 2026, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(schedule.Items) != 2 {
		t.Fatalf("expected two schedule items, got %+v", schedule.Items)
	}
	items := make(map[int64]bangumi.ViewerScheduleCard, len(schedule.Items))
	for _, item := range schedule.Items {
		items[item.BangumiID] = item
	}
	updated := items[7001]
	if updated.LatestEpisodeLabel != "第 2 话" || updated.LatestEpisodeUpdatedAt == nil || *updated.LatestEpisodeUpdatedAt != 200 {
		t.Fatalf("latest episode update time was not attached: %+v", updated)
	}
	if !updated.IsCompleted {
		t.Fatalf("anime with its final regular episode should be completed: %+v", updated)
	}
	specialOnly := items[7002]
	if specialOnly.LatestEpisodeLabel != "SP 2" || specialOnly.LatestEpisodeUpdatedAt == nil || *specialOnly.LatestEpisodeUpdatedAt != 300 {
		t.Fatalf("special episode progress was not attached: %+v", specialOnly)
	}
	if specialOnly.IsCompleted {
		t.Fatalf("a special episode matching the final episode number must not complete an anime: %+v", specialOnly)
	}
}
