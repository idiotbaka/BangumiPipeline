package database_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"bangumipipeline.local/server/internal/database"
)

func TestOpenUpgradesLegacyScheduledTasksTable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.ExecContext(ctx, `
CREATE TABLE scheduled_tasks (
    task_key TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    schedule TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
INSERT INTO scheduled_tasks(task_key, name, description, schedule, enabled, created_at, updated_at)
VALUES ('bangumi-season-metadata', 'legacy task', 'legacy', '0 3 * * *', 1, 1, 1);
CREATE TABLE anime_metadata (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bangumi_id INTEGER NOT NULL UNIQUE,
    url TEXT NOT NULL,
    name TEXT NOT NULL,
    name_cn TEXT NOT NULL DEFAULT '',
    air_date TEXT NOT NULL DEFAULT '',
    air_weekday INTEGER NOT NULL DEFAULT 0,
    image_large_url TEXT NOT NULL DEFAULT '',
    image_local_path TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
INSERT INTO anime_metadata(bangumi_id, url, name, created_at)
VALUES (101, 'https://bgm.tv/subject/101', 'legacy anime', 1);`)
	if err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var enabled bool
	var interval int
	var status string
	if err := db.QueryRowContext(ctx, `
SELECT enabled, interval_minutes, last_status
FROM scheduled_tasks
WHERE task_key = 'bangumi-season-metadata'`).Scan(&enabled, &interval, &status); err != nil {
		t.Fatal(err)
	}
	if !enabled || interval != 15 || status != "idle" {
		t.Fatalf("legacy task was not upgraded correctly: enabled=%v interval=%d status=%q", enabled, interval, status)
	}
	var detailStatus, charactersStatus, episodesStatus, infobox string
	if err := db.QueryRowContext(ctx, `
SELECT detail_status, characters_status, episodes_status, infobox_json
FROM anime_metadata WHERE bangumi_id = 101`).Scan(&detailStatus, &charactersStatus, &episodesStatus, &infobox); err != nil {
		t.Fatal(err)
	}
	if detailStatus != "pending" || charactersStatus != "pending" || episodesStatus != "pending" || infobox != "[]" {
		t.Fatalf("legacy anime was not prepared for metadata synchronization: detail=%q characters=%q episodes=%q infobox=%q",
			detailStatus, charactersStatus, episodesStatus, infobox)
	}
	assertTableExists(t, db, "anime_episodes")
}

func TestVersion5MigrationQueuesCompletedCharactersForActorNormalization(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "version4.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.ExecContext(ctx, `
CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL);
INSERT INTO schema_migrations(version, applied_at) VALUES (1,1),(2,1),(3,1),(4,1);
CREATE TABLE anime_metadata (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bangumi_id INTEGER NOT NULL UNIQUE,
    url TEXT NOT NULL,
    name TEXT NOT NULL,
    name_cn TEXT NOT NULL DEFAULT '',
    air_date TEXT NOT NULL DEFAULT '',
    air_weekday INTEGER NOT NULL DEFAULT 0,
    image_large_url TEXT NOT NULL DEFAULT '',
    image_local_path TEXT NOT NULL DEFAULT '',
    detail_status TEXT NOT NULL DEFAULT 'pending',
    characters_status TEXT NOT NULL DEFAULT 'pending',
    characters_error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
INSERT INTO anime_metadata(
    bangumi_id, url, name, image_large_url, image_local_path,
    detail_status, characters_status, created_at
) VALUES (808, 'https://bgm.tv/subject/808', 'v4 anime', 'https://example/cover.jpg', 'cover.jpg', 'completed', 'completed', 1);
CREATE TABLE anime_characters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bangumi_id INTEGER NOT NULL,
    character_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    relation TEXT NOT NULL DEFAULT '',
    type INTEGER NOT NULL DEFAULT 0,
    image_large_url TEXT NOT NULL DEFAULT '',
    image_local_path TEXT NOT NULL DEFAULT '',
    actors_json TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (bangumi_id, character_id)
);
INSERT INTO anime_characters(
    bangumi_id, character_id, name, image_large_url, image_local_path, created_at, updated_at
) VALUES (808, 88, 'v4 character', 'https://example/character.jpg', 'character.jpg', 1, 1);`)
	if err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var characterStage, subjectImageStatus, characterImageStatus string
	if err := db.QueryRowContext(ctx, `
SELECT characters_status, image_status FROM anime_metadata WHERE bangumi_id = 808`).
		Scan(&characterStage, &subjectImageStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT image_status FROM anime_characters WHERE bangumi_id = 808 AND character_id = 88`).
		Scan(&characterImageStatus); err != nil {
		t.Fatal(err)
	}
	if characterStage != "pending" || subjectImageStatus != "downloaded" || characterImageStatus != "downloaded" {
		t.Fatalf("version 5 migration mismatch: stage=%q subject_image=%q character_image=%q", characterStage, subjectImageStatus, characterImageStatus)
	}
}

func TestVersion6MigrationTrimsCharactersAndOrphanActors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "version5.db")
	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
	})
	if _, err := db.ExecContext(ctx, `
INSERT INTO anime_metadata(bangumi_id, url, name, created_at)
VALUES (6060, 'https://bgm.tv/subject/6060', 'many characters', 1)`); err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 12; id++ {
		if _, err := db.ExecContext(ctx, `
INSERT INTO actors(actor_id, name, created_at, updated_at) VALUES (?, ?, 1, 1)`, id, "Actor"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `
INSERT INTO anime_characters(bangumi_id, character_id, name, created_at, updated_at)
VALUES (6060, ?, ?, ?, ?)`, id, "Character", id, id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `
INSERT INTO character_actors(bangumi_id, character_id, actor_id, sort_order)
VALUES (6060, ?, ?, 0)`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 6"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for table, expected := range map[string]int{
		"anime_characters": 10,
		"character_actors": 10,
		"actors":           10,
	} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != expected {
			t.Fatalf("expected %d rows in %s after migration, got %d", expected, table, count)
		}
	}
}

func TestVersion26MigrationAddsQBitDownloadDir(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "version25.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `
CREATE TABLE download_settings (
    id                       INTEGER PRIMARY KEY CHECK (id = 1),
    host                     TEXT NOT NULL DEFAULT '127.0.0.1',
    port                     INTEGER NOT NULL DEFAULT 8080,
    username                 TEXT NOT NULL DEFAULT '',
    password                 TEXT NOT NULL DEFAULT '',
    max_concurrent_downloads INTEGER NOT NULL DEFAULT 2,
    updated_at               INTEGER NOT NULL
);
INSERT INTO download_settings(id, host, port, username, password, max_concurrent_downloads, updated_at)
VALUES (1, 'qbittorrent', 8080, 'admin', 'secret', 3, 1);`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var qbitDownloadDir string
	if err := db.QueryRowContext(ctx, `
SELECT qbit_download_dir FROM download_settings WHERE id = 1`).Scan(&qbitDownloadDir); err != nil {
		t.Fatal(err)
	}
	if qbitDownloadDir != "" {
		t.Fatalf("expected empty default qBittorrent directory, got %q", qbitDownloadDir)
	}
	var applied bool
	if err := db.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = 26)`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("expected version 26 migration to be recorded")
	}
}

func TestVersion28MigrationAddsOpeningSkipTablesAndTask(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	assertTableExists(t, db, "media_op_fingerprints")
	assertTableExists(t, db, "media_op_segments")

	var name string
	var interval int
	var enabled bool
	if err := db.QueryRowContext(ctx, `
SELECT name, interval_minutes, enabled
FROM scheduled_tasks
WHERE task_key = 'detect-media-openings'`).Scan(&name, &interval, &enabled); err != nil {
		t.Fatal(err)
	}
	if name != "识别产物视频的片头曲（OP）" || interval != 1440 || enabled {
		t.Fatalf("unexpected opening skip task seed: name=%q interval=%d enabled=%v", name, interval, enabled)
	}
	var applied bool
	if err := db.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = 28)`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("expected version 28 migration to be recorded")
	}
}

func TestVersion31MigrationAddsEpisodeCommentTablesAndTask(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	assertTableExists(t, db, "bangumi_episode_comment_sync")
	assertTableExists(t, db, "bangumi_episode_comments")

	var name string
	var interval int
	var enabled bool
	if err := db.QueryRowContext(ctx, `
SELECT name, interval_minutes, enabled
FROM scheduled_tasks
WHERE task_key = 'sync-bangumi-episode-comments'`).Scan(&name, &interval, &enabled); err != nil {
		t.Fatal(err)
	}
	if name != "同步 Bangumi 剧集吐槽" || interval != 1 || !enabled {
		t.Fatalf("unexpected episode comment task seed: name=%q interval=%d enabled=%v", name, interval, enabled)
	}
	var applied bool
	if err := db.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = 31)`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("expected version 31 migration to be recorded")
	}
}

func assertTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var exists bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)", table).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("expected table %s to exist", table)
	}
}
