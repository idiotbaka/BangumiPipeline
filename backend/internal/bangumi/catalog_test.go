package bangumi_test

import (
	"context"
	"testing"

	"bangumipipeline.local/server/internal/bangumi"
)

func TestCatalogListsAndLoadsAnimeWithAtMostTenCharacters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	_, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(
    bangumi_id, url, name, name_cn, air_date, air_weekday, summary,
    eps, total_episodes, series, image_local_path, image_status,
    detail_status, characters_status, infobox_json, rating_json,
    collection_json, meta_tags_json, created_at
) VALUES (
    1234, 'https://bgm.tv/subject/1234', 'Original', '中文标题', '2026-07-01', 3, 'Summary',
    12, 12, 0, 'cover.jpg', 'downloaded', 'completed', 'completed',
    '[{"key":"话数","value":"12"}]', '{"score":8.2}', '{}', '["TV"]', 100
);
INSERT INTO anime_tags(bangumi_id, name, count) VALUES (1234, '动画', 10);
INSERT INTO anime_aliases(bangumi_id, alias, sort_order) VALUES (1234, 'Alias', 0);`)
	if err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 12; id++ {
		_, err := db.ExecContext(ctx, `
INSERT INTO anime_characters(
    bangumi_id, character_id, name, image_status, created_at, updated_at
) VALUES (1234, ?, ?, 'not_found', ?, ?)`, id, "Character", id, id)
		if err != nil {
			t.Fatal(err)
		}
	}

	catalog := bangumi.NewCatalog(db)
	page, err := catalog.List(ctx, 1, 24)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].NameCN != "中文标题" || !page.Items[0].HasCover {
		t.Fatalf("unexpected catalog page: %+v", page)
	}
	detail, err := catalog.Detail(ctx, 1234)
	if err != nil {
		t.Fatal(err)
	}
	if detail.NameCN != "中文标题" || len(detail.Tags) != 1 || len(detail.Aliases) != 1 || len(detail.Characters) != 10 {
		t.Fatalf("unexpected anime detail: %+v", detail)
	}
}

func TestCatalogListIncludesOnlyBoundSubscriptionEpisodes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	_, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, created_at)
VALUES (4321, 'https://bgm.tv/subject/4321', 'Bound Anime', '已绑定番剧', 100);
INSERT INTO subscription_items(
    item_key, title, match_status, bangumi_id, season_number, episode_type, episode_number,
    binding_status, bound_bangumi_id, bound_anime_name, bound_season_number,
    bound_episode_type, bound_episode_number, created_at, updated_at
) VALUES
    ('bound-7', 'Bound Anime 07', 'matched', 4321, 1, 'episode', '7', 'bound', 4321, '已绑定番剧', 1, 'episode', '7', 101, 101),
    ('bound-7-duplicate', 'Bound Anime 07 duplicate', 'matched', 4321, 1, 'episode', '7', 'bound', 4321, '已绑定番剧', 1, 'episode', '7', 102, 102),
    ('pending-8', 'Bound Anime 08', 'matched', 4321, 1, 'episode', '8', 'pending', NULL, '', NULL, '', '', 103, 103),
    ('bound-10', 'Bound Anime 10', 'matched', 4321, 1, 'episode', '10', 'bound', 4321, '已绑定番剧', 1, 'episode', '10', 104, 104);`)
	if err != nil {
		t.Fatal(err)
	}

	page, err := bangumi.NewCatalog(db).List(ctx, 1, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("unexpected catalog page: %+v", page)
	}
	episodes := page.Items[0].MatchedEpisodes
	if len(episodes) != 2 {
		t.Fatalf("expected two bound episodes, got %+v", episodes)
	}
	if episodes[0].EpisodeNumber != "7" || episodes[1].EpisodeNumber != "10" {
		t.Fatalf("episodes were not deduplicated and numerically sorted: %+v", episodes)
	}
	for _, episode := range episodes {
		if episode.Status != "matched" || episode.SeasonNumber != 1 || episode.EpisodeType != "episode" {
			t.Fatalf("unexpected episode payload: %+v", episode)
		}
	}
}
