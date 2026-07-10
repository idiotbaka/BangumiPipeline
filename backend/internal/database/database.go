package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// A single writer is sufficient for this single-process application and
	// ensures connection-scoped SQLite pragmas are applied consistently.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply %q: %w", pragma, err)
		}
	}

	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL COLLATE NOCASE UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin      INTEGER NOT NULL DEFAULT 0 CHECK (is_admin IN (0, 1)),
    created_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash BLOB PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL
);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (1, unixepoch());

CREATE TABLE IF NOT EXISTS scheduled_tasks (
    task_key    TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    schedule    TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    interval_minutes INTEGER NOT NULL DEFAULT 15,
    last_status TEXT NOT NULL DEFAULT 'idle',
    last_error TEXT NOT NULL DEFAULT '',
    last_started_at INTEGER,
    last_finished_at INTEGER,
    next_run_at INTEGER,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

INSERT OR IGNORE INTO scheduled_tasks(
    task_key, name, description, schedule, enabled, created_at, updated_at
) VALUES (
    'bangumi-season-metadata',
    '从 bangumi.tv 抓取当季新番元数据',
    '从 Bangumi API 同步当季动画基础元数据、制作信息、角色信息和分集元数据。',
    'interval',
    0,
    unixepoch(),
    unixepoch()
);

CREATE TABLE IF NOT EXISTS network_settings (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    http_proxy  TEXT NOT NULL DEFAULT '',
    https_proxy TEXT NOT NULL DEFAULT '',
    updated_at  INTEGER NOT NULL
);

INSERT OR IGNORE INTO network_settings(id, http_proxy, https_proxy, updated_at)
VALUES (1, '', '', unixepoch());

CREATE TABLE IF NOT EXISTS llm_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    base_url   TEXT NOT NULL DEFAULT '',
    api_key    TEXT NOT NULL DEFAULT '',
    model      TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);

INSERT OR IGNORE INTO llm_settings(id, base_url, api_key, model, updated_at)
VALUES (1, '', '', '', unixepoch());

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (2, unixepoch());

CREATE TABLE IF NOT EXISTS anime_metadata (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    bangumi_id       INTEGER NOT NULL UNIQUE,
    url              TEXT NOT NULL,
    name             TEXT NOT NULL,
    name_cn          TEXT NOT NULL DEFAULT '',
    air_date         TEXT NOT NULL DEFAULT '',
    air_weekday      INTEGER NOT NULL DEFAULT 0,
    image_large_url  TEXT NOT NULL DEFAULT '',
    image_local_path TEXT NOT NULL DEFAULT '',
    image_status     TEXT NOT NULL DEFAULT 'pending',
    image_error      TEXT NOT NULL DEFAULT '',
    detail_date      TEXT NOT NULL DEFAULT '',
    platform         TEXT NOT NULL DEFAULT '',
    summary          TEXT NOT NULL DEFAULT '',
    summary_cn       TEXT NOT NULL DEFAULT '',
    eps              INTEGER NOT NULL DEFAULT 0,
    total_episodes   INTEGER NOT NULL DEFAULT 0,
    volumes          INTEGER NOT NULL DEFAULT 0,
    series           INTEGER NOT NULL DEFAULT 0,
    locked           INTEGER NOT NULL DEFAULT 0,
    nsfw             INTEGER NOT NULL DEFAULT 0,
    infobox_json     TEXT NOT NULL DEFAULT '[]',
    rating_json      TEXT NOT NULL DEFAULT '{}',
    collection_json  TEXT NOT NULL DEFAULT '{}',
    meta_tags_json   TEXT NOT NULL DEFAULT '[]',
    detail_status    TEXT NOT NULL DEFAULT 'pending',
    detail_error     TEXT NOT NULL DEFAULT '',
    detail_fetched_at INTEGER,
    characters_status TEXT NOT NULL DEFAULT 'pending',
    characters_error TEXT NOT NULL DEFAULT '',
    characters_fetched_at INTEGER,
    episodes_status TEXT NOT NULL DEFAULT 'pending',
    episodes_error TEXT NOT NULL DEFAULT '',
    episodes_fetched_at INTEGER,
    last_media_refresh_at INTEGER,
    media_storage_root TEXT NOT NULL DEFAULT '',
    subscription_episode_offset INTEGER NOT NULL DEFAULT 0,
    deleted_at       INTEGER,
    created_at       INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS anime_tags (
    bangumi_id  INTEGER NOT NULL REFERENCES anime_metadata(bangumi_id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    count       INTEGER NOT NULL DEFAULT 0,
    total_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (bangumi_id, name)
);

CREATE TABLE IF NOT EXISTS anime_aliases (
    bangumi_id INTEGER NOT NULL REFERENCES anime_metadata(bangumi_id) ON DELETE CASCADE,
    alias      TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (bangumi_id, alias)
);

CREATE TABLE IF NOT EXISTS anime_characters (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    bangumi_id       INTEGER NOT NULL REFERENCES anime_metadata(bangumi_id) ON DELETE CASCADE,
    character_id     INTEGER NOT NULL,
    name             TEXT NOT NULL,
    summary          TEXT NOT NULL DEFAULT '',
    summary_cn       TEXT NOT NULL DEFAULT '',
    relation         TEXT NOT NULL DEFAULT '',
    type             INTEGER NOT NULL DEFAULT 0,
    image_large_url  TEXT NOT NULL DEFAULT '',
    image_local_path TEXT NOT NULL DEFAULT '',
    image_status     TEXT NOT NULL DEFAULT 'pending',
    image_error      TEXT NOT NULL DEFAULT '',
    actors_json      TEXT NOT NULL DEFAULT '[]',
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    UNIQUE (bangumi_id, character_id)
);

CREATE TABLE IF NOT EXISTS actors (
    actor_id         INTEGER PRIMARY KEY,
    name             TEXT NOT NULL,
    short_summary    TEXT NOT NULL DEFAULT '',
    short_summary_cn TEXT NOT NULL DEFAULT '',
    career_json      TEXT NOT NULL DEFAULT '[]',
    type             INTEGER NOT NULL DEFAULT 0,
    locked           INTEGER NOT NULL DEFAULT 0,
    image_large_url  TEXT NOT NULL DEFAULT '',
    image_local_path TEXT NOT NULL DEFAULT '',
    image_status     TEXT NOT NULL DEFAULT 'pending',
    image_error      TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS character_actors (
    bangumi_id   INTEGER NOT NULL,
    character_id INTEGER NOT NULL,
    actor_id     INTEGER NOT NULL REFERENCES actors(actor_id) ON DELETE CASCADE,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (bangumi_id, character_id, actor_id),
    FOREIGN KEY (bangumi_id, character_id)
        REFERENCES anime_characters(bangumi_id, character_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_anime_characters_bangumi_id ON anime_characters(bangumi_id);
CREATE INDEX IF NOT EXISTS idx_character_actors_actor_id ON character_actors(actor_id);

CREATE TABLE IF NOT EXISTS anime_episodes (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    bangumi_id       INTEGER NOT NULL REFERENCES anime_metadata(bangumi_id) ON DELETE CASCADE,
    episode_id       INTEGER NOT NULL,
    ep_number        INTEGER NOT NULL DEFAULT 0,
    sort_number      REAL NOT NULL DEFAULT 0,
    type             INTEGER NOT NULL DEFAULT 0,
    disc             INTEGER NOT NULL DEFAULT 0,
    airdate          TEXT NOT NULL DEFAULT '',
    name             TEXT NOT NULL DEFAULT '',
    name_cn          TEXT NOT NULL DEFAULT '',
    duration         TEXT NOT NULL DEFAULT '',
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    description      TEXT NOT NULL DEFAULT '',
    description_cn   TEXT NOT NULL DEFAULT '',
    comment_count    INTEGER NOT NULL DEFAULT 0,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    UNIQUE (bangumi_id, episode_id)
);

CREATE INDEX IF NOT EXISTS idx_anime_episodes_bangumi_sort ON anime_episodes(bangumi_id, type, sort_number, episode_id);

CREATE TABLE IF NOT EXISTS bangumi_custom_search_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    tags_json  TEXT NOT NULL DEFAULT '[]',
    updated_at INTEGER NOT NULL
);

INSERT OR IGNORE INTO bangumi_custom_search_settings(id, tags_json, updated_at)
VALUES (1, '[]', unixepoch());

CREATE TABLE IF NOT EXISTS media_storage_settings (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    extra_roots_json TEXT NOT NULL DEFAULT '[]',
    updated_at       INTEGER NOT NULL
);

INSERT OR IGNORE INTO media_storage_settings(id, extra_roots_json, updated_at)
VALUES (1, '[]', unixepoch());

CREATE TABLE IF NOT EXISTS system_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    level       TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'system',
    message     TEXT NOT NULL,
    fields_json TEXT NOT NULL DEFAULT '{}',
    created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_system_logs_created_at ON system_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_system_logs_level_id ON system_logs(level, id);
`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	// Version 3 upgrades databases created before interval scheduling existed.
	columns := map[string]string{
		"interval_minutes": "INTEGER NOT NULL DEFAULT 15",
		"last_status":      "TEXT NOT NULL DEFAULT 'idle'",
		"last_error":       "TEXT NOT NULL DEFAULT ''",
		"last_started_at":  "INTEGER",
		"last_finished_at": "INTEGER",
		"next_run_at":      "INTEGER",
	}
	for name, definition := range columns {
		if err := ensureColumn(ctx, db, "scheduled_tasks", name, definition); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `
UPDATE scheduled_tasks
SET interval_minutes = 15
WHERE interval_minutes IS NULL OR interval_minutes < 1;
UPDATE scheduled_tasks
SET name = '从 bangumi.tv 抓取当季新番元数据',
    description = '从 Bangumi API 同步当季动画基础元数据、制作信息、角色信息和分集元数据。',
    schedule = 'interval'
WHERE task_key = 'bangumi-season-metadata';
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (3, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 3 migration: %w", err)
	}

	animeColumns := map[string]string{
		"detail_date":           "TEXT NOT NULL DEFAULT ''",
		"platform":              "TEXT NOT NULL DEFAULT ''",
		"summary":               "TEXT NOT NULL DEFAULT ''",
		"eps":                   "INTEGER NOT NULL DEFAULT 0",
		"total_episodes":        "INTEGER NOT NULL DEFAULT 0",
		"volumes":               "INTEGER NOT NULL DEFAULT 0",
		"series":                "INTEGER NOT NULL DEFAULT 0",
		"locked":                "INTEGER NOT NULL DEFAULT 0",
		"nsfw":                  "INTEGER NOT NULL DEFAULT 0",
		"infobox_json":          "TEXT NOT NULL DEFAULT '[]'",
		"rating_json":           "TEXT NOT NULL DEFAULT '{}'",
		"collection_json":       "TEXT NOT NULL DEFAULT '{}'",
		"meta_tags_json":        "TEXT NOT NULL DEFAULT '[]'",
		"detail_status":         "TEXT NOT NULL DEFAULT 'pending'",
		"detail_error":          "TEXT NOT NULL DEFAULT ''",
		"detail_fetched_at":     "INTEGER",
		"characters_status":     "TEXT NOT NULL DEFAULT 'pending'",
		"characters_error":      "TEXT NOT NULL DEFAULT ''",
		"characters_fetched_at": "INTEGER",
		"episodes_status":       "TEXT NOT NULL DEFAULT 'pending'",
		"episodes_error":        "TEXT NOT NULL DEFAULT ''",
		"episodes_fetched_at":   "INTEGER",
	}
	for name, definition := range animeColumns {
		if err := ensureColumn(ctx, db, "anime_metadata", name, definition); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS idx_anime_detail_status ON anime_metadata(detail_status);
CREATE INDEX IF NOT EXISTS idx_anime_characters_status ON anime_metadata(characters_status);
CREATE INDEX IF NOT EXISTS idx_anime_episodes_status ON anime_metadata(episodes_status);
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (4, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 4 migration: %w", err)
	}

	for name, definition := range map[string]string{
		"image_status": "TEXT NOT NULL DEFAULT 'pending'",
		"image_error":  "TEXT NOT NULL DEFAULT ''",
	} {
		if err := ensureColumn(ctx, db, "anime_metadata", name, definition); err != nil {
			return err
		}
		if err := ensureColumn(ctx, db, "anime_characters", name, definition); err != nil {
			return err
		}
	}
	applied, err := migrationApplied(ctx, db, 5)
	if err != nil {
		return err
	}
	if !applied {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `
UPDATE anime_metadata
SET image_status = CASE
    WHEN image_local_path != '' THEN 'downloaded'
    WHEN image_large_url = '' THEN 'not_found'
    ELSE 'pending'
END;
UPDATE anime_characters
SET image_status = CASE
    WHEN image_local_path != '' THEN 'downloaded'
    WHEN image_large_url = '' THEN 'not_found'
    ELSE 'pending'
END;
UPDATE anime_metadata
SET characters_status = 'pending', characters_error = ''
WHERE characters_status = 'completed';
INSERT INTO schema_migrations(version, applied_at) VALUES (5, unixepoch());`); err != nil {
			return fmt.Errorf("apply version 5 migration: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit version 5 migration: %w", err)
		}
	}

	applied, err = migrationApplied(ctx, db, 6)
	if err != nil {
		return err
	}
	if !applied {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		// Character rows preserve the order returned by Bangumi through their
		// auto-increment id. Keep only the first ten for databases synchronized
		// before the application introduced the same limit in the fetch path.
		if _, err := tx.ExecContext(ctx, `
DELETE FROM anime_characters
WHERE id IN (
    SELECT extra.id
    FROM anime_characters AS extra
    WHERE (
        SELECT COUNT(*)
        FROM anime_characters AS earlier
        WHERE earlier.bangumi_id = extra.bangumi_id
          AND earlier.id <= extra.id
    ) > 10
);
DELETE FROM actors
WHERE actor_id NOT IN (SELECT DISTINCT actor_id FROM character_actors);
INSERT INTO schema_migrations(version, applied_at) VALUES (6, unixepoch());`); err != nil {
			return fmt.Errorf("apply version 6 migration: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit version 6 migration: %w", err)
		}
	}

	if err := ensureColumn(ctx, db, "anime_metadata", "deleted_at", "INTEGER"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS idx_anime_deleted_created ON anime_metadata(deleted_at, created_at DESC, id DESC);
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (7, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 7 migration: %w", err)
	}

	applied, err = migrationApplied(ctx, db, 8)
	if err != nil {
		return err
	}
	if !applied {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS subscription_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    rss_url    TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);

INSERT OR IGNORE INTO subscription_settings(id, rss_url, updated_at)
VALUES (1, '', unixepoch());

CREATE TABLE IF NOT EXISTS subscription_items (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    item_key       TEXT NOT NULL UNIQUE,
    guid           TEXT NOT NULL DEFAULT '',
    title          TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    link           TEXT NOT NULL DEFAULT '',
    enclosure_url  TEXT NOT NULL DEFAULT '',
    torrent_url    TEXT NOT NULL DEFAULT '',
    content_length INTEGER NOT NULL DEFAULT 0,
    pub_date       TEXT NOT NULL DEFAULT '',
    published_at   INTEGER,
    match_status   TEXT NOT NULL DEFAULT 'unmatched',
    bangumi_id     INTEGER,
    matched_name   TEXT NOT NULL DEFAULT '',
    parsed_name    TEXT NOT NULL DEFAULT '',
    season_number  INTEGER,
    episode_type   TEXT NOT NULL DEFAULT '',
    episode_number TEXT NOT NULL DEFAULT '',
    match_score    REAL NOT NULL DEFAULT 0,
    match_reason   TEXT NOT NULL DEFAULT '',
    raw_json       TEXT NOT NULL DEFAULT '{}',
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    FOREIGN KEY (bangumi_id) REFERENCES anime_metadata(bangumi_id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_subscription_items_status_created ON subscription_items(match_status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_subscription_items_bangumi ON subscription_items(bangumi_id, created_at DESC);

INSERT OR IGNORE INTO scheduled_tasks(
    task_key, name, description, schedule, enabled, interval_minutes, created_at, updated_at
) VALUES (
    'subscription-rss-match',
    '抓取订阅和匹配番剧',
    '抓取 RSS 番剧订阅，入库新条目并根据本地番剧名称、中文名和别名进行规则匹配。',
    'interval',
    0,
    15,
    unixepoch(),
    unixepoch()
);

INSERT INTO schema_migrations(version, applied_at) VALUES (8, unixepoch());`); err != nil {
			return fmt.Errorf("apply version 8 migration: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit version 8 migration: %w", err)
		}
	}

	for name, definition := range map[string]string{
		"binding_status":       "TEXT NOT NULL DEFAULT 'pending'",
		"bound_bangumi_id":     "INTEGER",
		"bound_anime_name":     "TEXT NOT NULL DEFAULT ''",
		"bound_season_number":  "INTEGER",
		"bound_episode_type":   "TEXT NOT NULL DEFAULT ''",
		"bound_episode_number": "TEXT NOT NULL DEFAULT ''",
		"binding_note":         "TEXT NOT NULL DEFAULT ''",
		"bound_at":             "INTEGER",
		"ignored_at":           "INTEGER",
	} {
		if err := ensureColumn(ctx, db, "subscription_items", name, definition); err != nil {
			return err
		}
	}
	applied, err = migrationApplied(ctx, db, 9)
	if err != nil {
		return err
	}
	if !applied {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS subscription_title_rules (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    title_key            TEXT NOT NULL UNIQUE,
    bangumi_id           INTEGER NOT NULL REFERENCES anime_metadata(bangumi_id) ON DELETE CASCADE,
    anime_name           TEXT NOT NULL,
    season_number        INTEGER NOT NULL,
    episode_type         TEXT NOT NULL DEFAULT 'episode',
    created_from_item_id INTEGER REFERENCES subscription_items(id) ON DELETE SET NULL,
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_subscription_items_binding_status ON subscription_items(binding_status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_subscription_items_bound_target ON subscription_items(bound_bangumi_id, bound_season_number, bound_episode_type, bound_episode_number);
CREATE INDEX IF NOT EXISTS idx_subscription_title_rules_bangumi ON subscription_title_rules(bangumi_id);

INSERT INTO schema_migrations(version, applied_at) VALUES (9, unixepoch());`); err != nil {
			return fmt.Errorf("apply version 9 migration: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit version 9 migration: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS download_settings (
    id                       INTEGER PRIMARY KEY CHECK (id = 1),
    host                     TEXT NOT NULL DEFAULT '127.0.0.1',
    port                     INTEGER NOT NULL DEFAULT 8080,
    username                 TEXT NOT NULL DEFAULT '',
    password                 TEXT NOT NULL DEFAULT '',
    qbit_download_dir        TEXT NOT NULL DEFAULT '',
    max_concurrent_downloads INTEGER NOT NULL DEFAULT 2,
    updated_at               INTEGER NOT NULL
);

INSERT OR IGNORE INTO download_settings(
    id, host, port, username, password, max_concurrent_downloads, updated_at
) VALUES (1, '127.0.0.1', 8080, '', '', 2, unixepoch());

CREATE TABLE IF NOT EXISTS download_jobs (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_item_id INTEGER NOT NULL UNIQUE REFERENCES subscription_items(id) ON DELETE CASCADE,
    status               TEXT NOT NULL DEFAULT 'pending',
    source_url           TEXT NOT NULL DEFAULT '',
    folder_name          TEXT NOT NULL DEFAULT '',
    save_path            TEXT NOT NULL DEFAULT '',
    qbit_hash            TEXT NOT NULL DEFAULT '',
    qbit_name            TEXT NOT NULL DEFAULT '',
    progress             REAL NOT NULL DEFAULT 0,
    total_size           INTEGER NOT NULL DEFAULT 0,
    downloaded_size      INTEGER NOT NULL DEFAULT 0,
    download_speed       INTEGER NOT NULL DEFAULT 0,
    error_message        TEXT NOT NULL DEFAULT '',
    started_at           INTEGER,
    completed_at         INTEGER,
    failed_at            INTEGER,
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_download_jobs_status ON download_jobs(status, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_download_jobs_qbit_hash ON download_jobs(qbit_hash);

INSERT OR IGNORE INTO scheduled_tasks(
    task_key, name, description, schedule, enabled, interval_minutes, created_at, updated_at
) VALUES (
    'download-bound-episodes',
    '下载番剧',
    '将已确认绑定但尚未下载的话数提交到 qBittorrent，并同步下载进度。',
    'interval',
    0,
    1,
    unixepoch(),
    unixepoch()
);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (10, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 10 migration: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS media_jobs (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    download_job_id        INTEGER NOT NULL UNIQUE REFERENCES download_jobs(id) ON DELETE CASCADE,
    subscription_item_id   INTEGER NOT NULL REFERENCES subscription_items(id) ON DELETE CASCADE,
    bangumi_id             INTEGER NOT NULL,
    anime_name             TEXT NOT NULL,
    season_number          INTEGER NOT NULL,
    episode_type           TEXT NOT NULL DEFAULT 'episode',
    episode_number         TEXT NOT NULL,
    status                 TEXT NOT NULL DEFAULT 'pending',
    source_path            TEXT NOT NULL DEFAULT '',
    subtitle_path          TEXT NOT NULL DEFAULT '',
    output_path            TEXT NOT NULL DEFAULT '',
    cover_path             TEXT NOT NULL DEFAULT '',
    cover_status           TEXT NOT NULL DEFAULT 'pending',
    cover_error            TEXT NOT NULL DEFAULT '',
    video_codec            TEXT NOT NULL DEFAULT '',
    audio_codec            TEXT NOT NULL DEFAULT '',
    has_internal_subtitles INTEGER NOT NULL DEFAULT 0 CHECK (has_internal_subtitles IN (0, 1)),
    has_external_subtitles INTEGER NOT NULL DEFAULT 0 CHECK (has_external_subtitles IN (0, 1)),
    needs_transcode        INTEGER NOT NULL DEFAULT 0 CHECK (needs_transcode IN (0, 1)),
    action                 TEXT NOT NULL DEFAULT '',
    progress               REAL NOT NULL DEFAULT 0,
    processed_duration_ms  INTEGER NOT NULL DEFAULT 0,
    total_duration_ms      INTEGER NOT NULL DEFAULT 0,
    progress_updated_at    INTEGER,
    error_message          TEXT NOT NULL DEFAULT '',
    started_at             INTEGER,
    completed_at           INTEGER,
    failed_at              INTEGER,
    created_at             INTEGER NOT NULL,
    updated_at             INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_media_jobs_status_updated ON media_jobs(status, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_media_jobs_bangumi_episode ON media_jobs(bangumi_id, season_number, episode_type, episode_number);

INSERT OR IGNORE INTO scheduled_tasks(
    task_key, name, description, schedule, enabled, interval_minutes, created_at, updated_at
) VALUES (
    'process-downloaded-media',
    '处理和转码已下载完成的视频',
    '扫描已下载完成的话数，判断浏览器可播放性，必要时调用 ffmpeg 转码或压制字幕，并写入最终产物目录。',
    'interval',
    0,
    1,
    unixepoch(),
    unixepoch()
);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (11, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 11 migration: %w", err)
	}
	for name, definition := range map[string]string{
		"progress":              "REAL NOT NULL DEFAULT 0",
		"processed_duration_ms": "INTEGER NOT NULL DEFAULT 0",
		"total_duration_ms":     "INTEGER NOT NULL DEFAULT 0",
		"progress_updated_at":   "INTEGER",
	} {
		if err := ensureColumn(ctx, db, "media_jobs", name, definition); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (12, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 12 migration: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS anime_episodes (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    bangumi_id       INTEGER NOT NULL REFERENCES anime_metadata(bangumi_id) ON DELETE CASCADE,
    episode_id       INTEGER NOT NULL,
    ep_number        INTEGER NOT NULL DEFAULT 0,
    sort_number      REAL NOT NULL DEFAULT 0,
    type             INTEGER NOT NULL DEFAULT 0,
    disc             INTEGER NOT NULL DEFAULT 0,
    airdate          TEXT NOT NULL DEFAULT '',
    name             TEXT NOT NULL DEFAULT '',
    name_cn          TEXT NOT NULL DEFAULT '',
    duration         TEXT NOT NULL DEFAULT '',
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    description      TEXT NOT NULL DEFAULT '',
    comment_count    INTEGER NOT NULL DEFAULT 0,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    UNIQUE (bangumi_id, episode_id)
);
CREATE INDEX IF NOT EXISTS idx_anime_episodes_bangumi_sort ON anime_episodes(bangumi_id, type, sort_number, episode_id);
CREATE INDEX IF NOT EXISTS idx_anime_episodes_status ON anime_metadata(episodes_status);
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (13, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 13 migration: %w", err)
	}
	if err := ensureColumn(ctx, db, "anime_metadata", "media_storage_root", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS media_storage_settings (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    extra_roots_json TEXT NOT NULL DEFAULT '[]',
    updated_at       INTEGER NOT NULL
);
INSERT OR IGNORE INTO media_storage_settings(id, extra_roots_json, updated_at)
VALUES (1, '[]', unixepoch());
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (14, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 14 migration: %w", err)
	}
	for name, definition := range map[string]string{
		"cover_path":   "TEXT NOT NULL DEFAULT ''",
		"cover_status": "TEXT NOT NULL DEFAULT 'pending'",
		"cover_error":  "TEXT NOT NULL DEFAULT ''",
	} {
		if err := ensureColumn(ctx, db, "media_jobs", name, definition); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS idx_media_jobs_cover_status ON media_jobs(cover_status, updated_at DESC, id DESC);
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (15, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 15 migration: %w", err)
	}
	for _, column := range []struct {
		table      string
		name       string
		definition string
	}{
		{"anime_metadata", "summary_cn", "TEXT NOT NULL DEFAULT ''"},
		{"anime_episodes", "description_cn", "TEXT NOT NULL DEFAULT ''"},
		{"anime_characters", "summary_cn", "TEXT NOT NULL DEFAULT ''"},
		{"actors", "short_summary_cn", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := ensureColumn(ctx, db, column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS llm_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    base_url   TEXT NOT NULL DEFAULT '',
    api_key    TEXT NOT NULL DEFAULT '',
    model      TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);
INSERT OR IGNORE INTO llm_settings(id, base_url, api_key, model, updated_at)
VALUES (1, '', '', '', unixepoch());
INSERT OR IGNORE INTO scheduled_tasks(
    task_key, name, description, schedule, enabled, interval_minutes, created_at, updated_at
) VALUES (
    'translate-anime-metadata',
    '翻译新番元数据',
    '使用 OpenAI Chat 兼容的大模型将 Bangumi 抓取到的番剧简介、分集信息、角色和声优简介翻译为中文。',
    'interval',
    0,
    1,
    unixepoch(),
    unixepoch()
);
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (16, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 16 migration: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS bangumi_custom_search_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    tags_json  TEXT NOT NULL DEFAULT '[]',
    updated_at INTEGER NOT NULL
);
INSERT OR IGNORE INTO bangumi_custom_search_settings(id, tags_json, updated_at)
VALUES (1, '[]', unixepoch());
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (17, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 17 migration: %w", err)
	}
	if err := ensureColumn(ctx, db, "anime_metadata", "last_media_refresh_at", "INTEGER"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (18, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 18 migration: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS viewer_users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL COLLATE NOCASE UNIQUE,
    password_hash TEXT NOT NULL,
    disabled_at   INTEGER,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS viewer_sessions (
    token_hash BLOB PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES viewer_users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_viewer_sessions_user_id ON viewer_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_viewer_sessions_expires_at ON viewer_sessions(expires_at);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (19, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 19 migration: %w", err)
	}
	if err := ensureColumn(ctx, db, "viewer_users", "disabled_at", "INTEGER"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS viewer_site_settings (
    id                 INTEGER PRIMARY KEY CHECK (id = 1),
    site_name          TEXT NOT NULL DEFAULT 'BangumiPipeline Viewer',
    registration_enabled INTEGER NOT NULL DEFAULT 1 CHECK (registration_enabled IN (0, 1)),
    invite_required    INTEGER NOT NULL DEFAULT 0 CHECK (invite_required IN (0, 1)),
    favicon_png        BLOB,
    favicon_updated_at INTEGER,
    updated_at         INTEGER NOT NULL
);

INSERT OR IGNORE INTO viewer_site_settings(id, site_name, updated_at)
VALUES (1, 'BangumiPipeline Viewer', unixepoch());

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (20, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 20 migration: %w", err)
	}
	if err := ensureColumn(ctx, db, "viewer_site_settings", "registration_enabled", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "viewer_site_settings", "invite_required", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS viewer_invitation_codes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    code            TEXT NOT NULL UNIQUE,
    used_by_user_id INTEGER REFERENCES viewer_users(id) ON DELETE SET NULL,
    used_at         INTEGER,
    created_at      INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_viewer_invitation_codes_used ON viewer_invitation_codes(used_at, id DESC);
CREATE INDEX IF NOT EXISTS idx_viewer_invitation_codes_user ON viewer_invitation_codes(used_by_user_id);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (21, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 21 migration: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS viewer_carousel_items (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    bangumi_id         INTEGER NOT NULL UNIQUE REFERENCES anime_metadata(bangumi_id) ON DELETE CASCADE,
    sort_order         INTEGER NOT NULL DEFAULT 0,
    image_data         BLOB NOT NULL,
    image_content_type TEXT NOT NULL,
    image_updated_at   INTEGER NOT NULL,
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_viewer_carousel_sort
ON viewer_carousel_items(sort_order, id);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (22, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 22 migration: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS viewer_filter_dimensions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL COLLATE NOCASE UNIQUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS viewer_filter_tags (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    dimension_id INTEGER NOT NULL REFERENCES viewer_filter_dimensions(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    UNIQUE(dimension_id, name)
);

CREATE INDEX IF NOT EXISTS idx_viewer_filter_dimensions_sort
ON viewer_filter_dimensions(sort_order, id);

CREATE INDEX IF NOT EXISTS idx_viewer_filter_tags_dimension_sort
ON viewer_filter_tags(dimension_id, sort_order, id);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (23, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 23 migration: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS viewer_watch_history (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id          INTEGER NOT NULL REFERENCES viewer_users(id) ON DELETE CASCADE,
    bangumi_id       INTEGER NOT NULL REFERENCES anime_metadata(bangumi_id) ON DELETE CASCADE,
    media_job_id     INTEGER NOT NULL REFERENCES media_jobs(id) ON DELETE CASCADE,
    position_seconds REAL NOT NULL DEFAULT 0,
    duration_seconds REAL NOT NULL DEFAULT 0,
    completed        INTEGER NOT NULL DEFAULT 0 CHECK (completed IN (0, 1)),
    last_watched_at  INTEGER NOT NULL,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    UNIQUE(user_id, media_job_id)
);

CREATE INDEX IF NOT EXISTS idx_viewer_watch_history_user_updated
ON viewer_watch_history(user_id, last_watched_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_viewer_watch_history_user_anime
ON viewer_watch_history(user_id, bangumi_id, last_watched_at DESC);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (24, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 24 migration: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS viewer_anime_follows (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES viewer_users(id) ON DELETE CASCADE,
    bangumi_id INTEGER NOT NULL REFERENCES anime_metadata(bangumi_id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(user_id, bangumi_id)
);

CREATE INDEX IF NOT EXISTS idx_viewer_anime_follows_user_updated
ON viewer_anime_follows(user_id, updated_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_viewer_anime_follows_bangumi
ON viewer_anime_follows(bangumi_id);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (25, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 25 migration: %w", err)
	}
	if err := ensureColumn(ctx, db, "download_settings", "qbit_download_dir", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (26, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 26 migration: %w", err)
	}
	if err := ensureColumn(ctx, db, "anime_metadata", "subscription_episode_offset", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (27, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 27 migration: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS media_op_fingerprints (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    media_job_id            INTEGER NOT NULL UNIQUE REFERENCES media_jobs(id) ON DELETE CASCADE,
    file_size               INTEGER NOT NULL DEFAULT 0,
    file_mtime              INTEGER NOT NULL DEFAULT 0,
    duration_seconds        REAL NOT NULL DEFAULT 0,
    fingerprint_end_seconds REAL NOT NULL DEFAULT 0,
    fingerprint_points      BLOB NOT NULL,
    chromaprint_version     TEXT NOT NULL DEFAULT 'ffmpeg-chromaprint-raw-v1',
    created_at              INTEGER NOT NULL,
    updated_at              INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_media_op_fingerprints_updated
ON media_op_fingerprints(updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS media_op_segments (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    media_job_id        INTEGER NOT NULL UNIQUE REFERENCES media_jobs(id) ON DELETE CASCADE,
    bangumi_id          INTEGER NOT NULL,
    season_number       INTEGER NOT NULL,
    episode_type        TEXT NOT NULL DEFAULT 'episode',
    episode_number      TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'detected', 'not_found', 'failed')),
    start_seconds       REAL NOT NULL DEFAULT 0,
    end_seconds         REAL NOT NULL DEFAULT 0,
    confidence          REAL NOT NULL DEFAULT 0,
    analyzed_group_size INTEGER NOT NULL DEFAULT 0,
    matched_media_job_id INTEGER REFERENCES media_jobs(id) ON DELETE SET NULL,
    error_message       TEXT NOT NULL DEFAULT '',
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_media_op_segments_anime_episode
ON media_op_segments(bangumi_id, season_number, episode_type, episode_number);

CREATE INDEX IF NOT EXISTS idx_media_op_segments_status
ON media_op_segments(status, updated_at DESC, id DESC);

INSERT OR IGNORE INTO scheduled_tasks(
    task_key, name, description, schedule, enabled, interval_minutes, created_at, updated_at
) VALUES (
    'detect-media-openings',
    '识别产物视频的片头曲（OP）',
    '扫描已完成的正片成品视频，使用 ffmpeg Chromaprint 分析音频指纹并交叉识别可跳过的片头曲片段。',
    'interval',
    0,
    1440,
    unixepoch(),
    unixepoch()
);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (28, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 28 migration: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS viewer_web_push_settings (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    vapid_public_key  TEXT NOT NULL DEFAULT '',
    vapid_private_key TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS viewer_web_push_subscriptions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES viewer_users(id) ON DELETE CASCADE,
    endpoint    TEXT NOT NULL UNIQUE,
    p256dh      TEXT NOT NULL,
    auth        TEXT NOT NULL,
    expires_at  INTEGER,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_viewer_web_push_subscriptions_user
ON viewer_web_push_subscriptions(user_id, updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS viewer_web_push_deliveries (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id INTEGER NOT NULL REFERENCES viewer_web_push_subscriptions(id) ON DELETE CASCADE,
    media_job_id    INTEGER NOT NULL REFERENCES media_jobs(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL,
    error_message   TEXT NOT NULL DEFAULT '',
    delivered_at    INTEGER,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    UNIQUE(subscription_id, media_job_id)
);

CREATE INDEX IF NOT EXISTS idx_viewer_web_push_deliveries_pending
ON viewer_web_push_deliveries(status, next_attempt_at, id);

INSERT OR IGNORE INTO scheduled_tasks(
    task_key, name, description, schedule, enabled, interval_minutes, created_at, updated_at
) VALUES (
    'deliver-viewer-push-notifications',
    '投递观看端新集通知',
    '向已授权浏览器通知其追番的新成品视频；网络异常时会自动重试。',
    'interval',
    1,
    5,
    unixepoch(),
    unixepoch()
);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
VALUES (29, unixepoch());`); err != nil {
		return fmt.Errorf("finish version 29 migration: %w", err)
	}
	return nil
}

func migrationApplied(ctx context.Context, db *sql.DB, version int) (bool, error) {
	var applied bool
	if err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", version,
	).Scan(&applied); err != nil {
		return false, fmt.Errorf("check migration %d: %w", version, err)
	}
	return applied, nil
}

func ensureColumn(ctx context.Context, db *sql.DB, table, column, definition string) error {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("inspect %s columns: %w", table, err)
	}
	exists := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			exists = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}
