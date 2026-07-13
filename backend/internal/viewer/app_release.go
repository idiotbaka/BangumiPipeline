package viewer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxAppAPKBytes = 256 << 20

var (
	ErrAppReleaseNotFound      = errors.New("app release not found")
	ErrAppReleaseVersionExists = errors.New("app release version already exists")
	ErrInvalidAppVersion       = errors.New("app version must use major.minor.patch format")
	ErrInvalidAppReleaseNotes  = errors.New("app release notes must contain 1 to 10000 characters")
	ErrInvalidAppAPK           = errors.New("app apk must be a valid file up to 256 MiB")
)

type AppRelease struct {
	ID           int64  `json:"id"`
	Version      string `json:"version"`
	ReleaseNotes string `json:"releaseNotes"`
	APKSize      int64  `json:"apkSize"`
	APKSHA256    string `json:"apkSha256"`
	PublishedAt  int64  `json:"publishedAt"`
}

type AppReleaseInput struct {
	Version      string
	ReleaseNotes string
	APKData      []byte
}

type AppReleaseFile struct {
	Version     string
	Data        []byte
	Size        int64
	SHA256      string
	PublishedAt int64
}

func (s *Service) ListAppReleases(ctx context.Context) ([]AppRelease, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, version, release_notes, apk_size, apk_sha256, created_at
FROM viewer_app_releases
ORDER BY version_major DESC, version_minor DESC, version_patch DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AppRelease, 0)
	for rows.Next() {
		var item AppRelease
		if err := rows.Scan(
			&item.ID, &item.Version, &item.ReleaseNotes, &item.APKSize,
			&item.APKSHA256, &item.PublishedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) PublishAppRelease(ctx context.Context, input AppReleaseInput) (AppRelease, error) {
	version, major, minor, patch, releaseNotes, err := normalizeAppReleaseMetadata(input)
	if err != nil {
		return AppRelease{}, err
	}
	if !validAppAPK(input.APKData) {
		return AppRelease{}, ErrInvalidAppAPK
	}

	var exists bool
	if err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM viewer_app_releases WHERE version = ?)", version,
	).Scan(&exists); err != nil {
		return AppRelease{}, err
	}
	if exists {
		return AppRelease{}, ErrAppReleaseVersionExists
	}

	now := s.now().UTC().Unix()
	digest := sha256.Sum256(input.APKData)
	result, err := s.db.ExecContext(ctx, `
INSERT INTO viewer_app_releases(
    version, version_major, version_minor, version_patch,
    release_notes, apk_data, apk_size, apk_sha256, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		version, major, minor, patch, releaseNotes, input.APKData, len(input.APKData),
		hex.EncodeToString(digest[:]), now, now,
	)
	if err != nil {
		return AppRelease{}, fmt.Errorf("publish app release: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return AppRelease{}, err
	}
	return s.appRelease(ctx, id)
}

func (s *Service) UpdateAppRelease(ctx context.Context, id int64, input AppReleaseInput) (AppRelease, error) {
	version, major, minor, patch, releaseNotes, err := normalizeAppReleaseMetadata(input)
	if err != nil {
		return AppRelease{}, err
	}
	if input.APKData != nil && !validAppAPK(input.APKData) {
		return AppRelease{}, ErrInvalidAppAPK
	}

	var releaseExists, versionExists bool
	if err := s.db.QueryRowContext(ctx, `
SELECT
    EXISTS(SELECT 1 FROM viewer_app_releases WHERE id = ?),
    EXISTS(SELECT 1 FROM viewer_app_releases WHERE version = ? AND id <> ?)`,
		id, version, id,
	).Scan(&releaseExists, &versionExists); err != nil {
		return AppRelease{}, err
	}
	if !releaseExists {
		return AppRelease{}, ErrAppReleaseNotFound
	}
	if versionExists {
		return AppRelease{}, ErrAppReleaseVersionExists
	}

	now := s.now().UTC().Unix()
	if input.APKData == nil {
		_, err = s.db.ExecContext(ctx, `
UPDATE viewer_app_releases
SET version = ?, version_major = ?, version_minor = ?, version_patch = ?,
    release_notes = ?, updated_at = ?
WHERE id = ?`, version, major, minor, patch, releaseNotes, now, id)
	} else {
		digest := sha256.Sum256(input.APKData)
		_, err = s.db.ExecContext(ctx, `
UPDATE viewer_app_releases
SET version = ?, version_major = ?, version_minor = ?, version_patch = ?,
    release_notes = ?, apk_data = ?, apk_size = ?, apk_sha256 = ?, updated_at = ?
WHERE id = ?`,
			version, major, minor, patch, releaseNotes, input.APKData, len(input.APKData),
			hex.EncodeToString(digest[:]), now, id,
		)
	}
	if err != nil {
		return AppRelease{}, fmt.Errorf("update app release: %w", err)
	}
	return s.appRelease(ctx, id)
}

func (s *Service) DeleteAppRelease(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM viewer_app_releases WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete app release: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrAppReleaseNotFound
	}
	return nil
}

func (s *Service) LatestAppRelease(ctx context.Context) (AppRelease, error) {
	var item AppRelease
	err := s.db.QueryRowContext(ctx, `
SELECT id, version, release_notes, apk_size, apk_sha256, created_at
FROM viewer_app_releases
ORDER BY version_major DESC, version_minor DESC, version_patch DESC, id DESC
LIMIT 1`).Scan(
		&item.ID, &item.Version, &item.ReleaseNotes, &item.APKSize,
		&item.APKSHA256, &item.PublishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AppRelease{}, ErrAppReleaseNotFound
	}
	return item, err
}

func (s *Service) AppReleaseAPK(ctx context.Context, id int64) (AppReleaseFile, error) {
	var file AppReleaseFile
	err := s.db.QueryRowContext(ctx, `
SELECT version, apk_data, apk_size, apk_sha256, created_at
FROM viewer_app_releases
WHERE id = ?`, id).Scan(
		&file.Version, &file.Data, &file.Size, &file.SHA256, &file.PublishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AppReleaseFile{}, ErrAppReleaseNotFound
	}
	return file, err
}

func (s *Service) appRelease(ctx context.Context, id int64) (AppRelease, error) {
	var item AppRelease
	err := s.db.QueryRowContext(ctx, `
SELECT id, version, release_notes, apk_size, apk_sha256, created_at
FROM viewer_app_releases
WHERE id = ?`, id).Scan(
		&item.ID, &item.Version, &item.ReleaseNotes, &item.APKSize,
		&item.APKSHA256, &item.PublishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AppRelease{}, ErrAppReleaseNotFound
	}
	return item, err
}

func parseAppVersion(value string) (string, int64, int64, int64, error) {
	version := strings.TrimSpace(value)
	parts := strings.Split(version, ".")
	if len(parts) != 3 || len(version) > 32 {
		return "", 0, 0, 0, ErrInvalidAppVersion
	}
	numbers := make([]int64, 3)
	for index, part := range parts {
		value, err := strconv.ParseInt(part, 10, 32)
		if err != nil || value < 0 || strconv.FormatInt(value, 10) != part {
			return "", 0, 0, 0, ErrInvalidAppVersion
		}
		numbers[index] = value
	}
	return version, numbers[0], numbers[1], numbers[2], nil
}

func normalizeAppReleaseMetadata(input AppReleaseInput) (string, int64, int64, int64, string, error) {
	version, major, minor, patch, err := parseAppVersion(input.Version)
	if err != nil {
		return "", 0, 0, 0, "", err
	}
	releaseNotes := strings.TrimSpace(input.ReleaseNotes)
	if !utf8.ValidString(releaseNotes) || utf8.RuneCountInString(releaseNotes) < 1 || utf8.RuneCountInString(releaseNotes) > 10000 || strings.ContainsRune(releaseNotes, '\x00') {
		return "", 0, 0, 0, "", ErrInvalidAppReleaseNotes
	}
	return version, major, minor, patch, releaseNotes, nil
}

func validAppAPK(data []byte) bool {
	return len(data) >= 4 && len(data) <= MaxAppAPKBytes &&
		data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04
}
