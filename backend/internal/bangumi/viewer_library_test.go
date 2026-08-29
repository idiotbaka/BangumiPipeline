package bangumi_test

import (
	"context"
	"fmt"
	"testing"

	"bangumipipeline.local/server/internal/bangumi"
)

func TestViewerLibraryPageReturnsPageAndTotal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openDatabase(t, ctx)
	for id := 1; id <= 18; id++ {
		if _, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, name_cn, air_date, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
			id, fmt.Sprintf("https://bgm.tv/subject/%d", id), fmt.Sprintf("Anime %d", id),
			fmt.Sprintf("番剧 %d", id), fmt.Sprintf("2026-01-%02d", id), id,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
UPDATE anime_metadata SET eps = 12, total_episodes = 13 WHERE bangumi_id = 18;
UPDATE anime_metadata SET eps = 0, total_episodes = 3 WHERE bangumi_id = 17;
INSERT INTO anime_episodes(bangumi_id, episode_id, ep_number, sort_number, type, created_at, updated_at)
VALUES
    (17, 1701, 1, 1, 0, 1, 1),
    (17, 1702, 2, 2, 0, 1, 1),
    (17, 1703, 0, 2.5, 1, 1, 1);`); err != nil {
		t.Fatal(err)
	}

	catalog := bangumi.NewCatalog(db)
	firstPage, err := catalog.ViewerLibraryPage(ctx, "", nil, 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	if firstPage.Total != 18 || len(firstPage.Items) != 16 || firstPage.Items[0].BangumiID != 18 {
		t.Fatalf("unexpected first page: %+v", firstPage)
	}
	if firstPage.Items[0].TotalEpisodes != 12 {
		t.Fatalf("library total should use configured regular episodes: %+v", firstPage.Items[0])
	}
	if firstPage.Items[1].BangumiID != 17 || firstPage.Items[1].TotalEpisodes != 2 {
		t.Fatalf("library total fallback should count only regular episode rows: %+v", firstPage.Items[1])
	}
	secondPage, err := catalog.ViewerLibraryPage(ctx, "", nil, 2, 16)
	if err != nil {
		t.Fatal(err)
	}
	if secondPage.Total != 18 || len(secondPage.Items) != 2 || secondPage.Items[0].BangumiID != 2 {
		t.Fatalf("unexpected second page: %+v", secondPage)
	}
	legacy, err := catalog.ViewerLibrary(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Total != 18 || len(legacy.Items) != 18 {
		t.Fatalf("legacy request should remain unpaginated: %+v", legacy)
	}
}
