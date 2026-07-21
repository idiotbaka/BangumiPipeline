package viewer

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
)

func TestCommentFilterSettingsReplaceAndNormalizeUsernames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "viewer-comment-filter.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewService(db, time.Hour)
	initial, err := service.CommentFilterSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Usernames == nil || len(initial.Usernames) != 0 {
		t.Fatalf("unexpected initial settings: %+v", initial)
	}

	updated, err := service.UpdateCommentFilterSettings(ctx, []string{"  exact-user  ", "CaseUser", "exact-user"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"exact-user", "CaseUser"}; !reflect.DeepEqual(updated.Usernames, want) {
		t.Fatalf("updated usernames = %#v, want %#v", updated.Usernames, want)
	}

	stored, err := service.CommentFilterSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.Usernames, updated.Usernames) {
		t.Fatalf("stored usernames = %#v, want %#v", stored.Usernames, updated.Usernames)
	}

	cleared, err := service.UpdateCommentFilterSettings(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Usernames == nil || len(cleared.Usernames) != 0 {
		t.Fatalf("unexpected cleared settings: %+v", cleared)
	}
}

func TestCommentFilterSettingsValidateUsernames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "viewer-comment-filter-validation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewService(db, time.Hour)

	if _, err := service.UpdateCommentFilterSettings(ctx, []string{"\n"}); !errors.Is(err, ErrInvalidCommentFilterUsername) {
		t.Fatalf("expected invalid username error, got %v", err)
	}
	tooMany := make([]string, maxCommentFilterUsernames+1)
	for index := range tooMany {
		tooMany[index] = strings.Repeat("u", 4) + string(rune(index+0x100))
	}
	if _, err := service.UpdateCommentFilterSettings(ctx, tooMany); !errors.Is(err, ErrTooManyCommentFilterUsernames) {
		t.Fatalf("expected too many usernames error, got %v", err)
	}
}
