package viewer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxCommentFilterUsernames = 200

var (
	ErrInvalidCommentFilterUsername  = errors.New("comment filter username must contain 1 to 80 printable characters")
	ErrTooManyCommentFilterUsernames = errors.New("comment filter supports up to 200 usernames")
)

type CommentFilterSettings struct {
	Usernames []string `json:"usernames"`
}

func (s *Service) CommentFilterSettings(ctx context.Context) (CommentFilterSettings, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT username
FROM viewer_comment_username_filters
ORDER BY sort_order, username COLLATE BINARY`)
	if err != nil {
		return CommentFilterSettings{}, err
	}
	defer rows.Close()

	settings := CommentFilterSettings{Usernames: make([]string, 0)}
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return CommentFilterSettings{}, err
		}
		settings.Usernames = append(settings.Usernames, username)
	}
	if err := rows.Err(); err != nil {
		return CommentFilterSettings{}, err
	}
	return settings, nil
}

func (s *Service) UpdateCommentFilterSettings(ctx context.Context, usernames []string) (CommentFilterSettings, error) {
	normalized, err := normalizeCommentFilterUsernames(usernames)
	if err != nil {
		return CommentFilterSettings{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CommentFilterSettings{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM viewer_comment_username_filters"); err != nil {
		return CommentFilterSettings{}, err
	}
	now := s.now().UTC().Unix()
	for index, username := range normalized {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO viewer_comment_username_filters(username, sort_order, created_at)
VALUES (?, ?, ?)`, username, index, now); err != nil {
			return CommentFilterSettings{}, fmt.Errorf("save comment filter username %q: %w", username, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return CommentFilterSettings{}, err
	}
	return CommentFilterSettings{Usernames: normalized}, nil
}

func normalizeCommentFilterUsernames(usernames []string) ([]string, error) {
	if len(usernames) > maxCommentFilterUsernames {
		return nil, ErrTooManyCommentFilterUsernames
	}
	result := make([]string, 0, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, raw := range usernames {
		username := strings.TrimSpace(raw)
		if !validCommentFilterUsername(username) {
			return nil, ErrInvalidCommentFilterUsername
		}
		if _, exists := seen[username]; exists {
			continue
		}
		seen[username] = struct{}{}
		result = append(result, username)
	}
	return result, nil
}

func validCommentFilterUsername(username string) bool {
	length := utf8.RuneCountInString(username)
	if length < 1 || length > 80 {
		return false
	}
	for _, character := range username {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
