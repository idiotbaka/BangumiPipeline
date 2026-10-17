package translation

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var (
	ErrInvalidTranslationKind = errors.New("invalid translation kind")
	ErrTranslationNotFound    = errors.New("translation target not found")
	ErrTranslationConflict    = errors.New("translation content changed")
	ErrTranslationNoSource    = errors.New("translation source is empty")
	ErrInvalidTranslation     = errors.New("translation and expected content are required")
)

type TranslationContent struct {
	ProgressTarget
	OriginalText   string `json:"originalText"`
	TranslatedText string `json:"translatedText"`
	Shared         bool   `json:"shared"`
}

type UpdateTranslationInput struct {
	OriginalText           *string `json:"originalText"`
	ExpectedTranslatedText *string `json:"expectedTranslatedText"`
	TranslatedText         string  `json:"translatedText"`
}

type translationField struct {
	table, id, original, translated string
}

// SQL identifiers only come from this fixed allowlist, never request input.
func translationFieldFor(kind targetKind) (translationField, bool) {
	switch kind {
	case targetAnimeTitle:
		return translationField{"anime_metadata", "bangumi_id", "name", "name_cn"}, true
	case targetAnimeSummary:
		return translationField{"anime_metadata", "bangumi_id", "summary", "summary_cn"}, true
	case targetEpisodeTitle:
		return translationField{"anime_episodes", "episode_id", "name", "name_cn"}, true
	case targetEpisodeSummary:
		return translationField{"anime_episodes", "episode_id", "description", "description_cn"}, true
	case targetCharacterSummary:
		return translationField{"anime_characters", "character_id", "summary", "summary_cn"}, true
	case targetActorShortSummary:
		return translationField{"actors", "actor_id", "short_summary", "short_summary_cn"}, true
	default:
		return translationField{}, false
	}
}

func (s *Service) TranslationContent(ctx context.Context, bangumiID int64, kind string, id int64) (TranslationContent, error) {
	var item TranslationContent
	if _, ok := translationFieldFor(targetKind(kind)); !ok {
		return item, ErrInvalidTranslationKind
	}
	err := s.db.QueryRowContext(ctx, `WITH candidates AS (
    SELECT * FROM anime_metadata WHERE bangumi_id = ? AND deleted_at IS NULL
)`+progressTargetsSQL+`
SELECT kind, target_id, label,
       CASE WHEN original = '' THEN 'no_source' WHEN translated = '' THEN 'pending' ELSE 'completed' END,
       original, translated
FROM targets WHERE kind = ? AND target_id = ?`, bangumiID, kind, id).Scan(
		&item.Kind, &item.ID, &item.Label, &item.Status, &item.OriginalText, &item.TranslatedText)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrTranslationNotFound
	}
	item.Shared = kind == string(targetActorShortSummary)
	return item, err
}

func (s *Service) UpdateTranslation(ctx context.Context, bangumiID int64, kind string, id int64, input UpdateTranslationInput) (TranslationContent, error) {
	item, err := s.TranslationContent(ctx, bangumiID, kind, id)
	if err != nil {
		return item, err
	}
	text := strings.TrimSpace(input.TranslatedText)
	if text == "" || input.OriginalText == nil || input.ExpectedTranslatedText == nil {
		return item, ErrInvalidTranslation
	}
	if item.OriginalText != *input.OriginalText || item.TranslatedText != *input.ExpectedTranslatedText {
		return item, ErrTranslationConflict
	}
	if item.OriginalText == "" {
		return item, ErrTranslationNoSource
	}
	field, _ := translationFieldFor(targetKind(kind))
	saved, err := s.writeTranslation(ctx, field, bangumiID, id, item.OriginalText, item.TranslatedText, text, true)
	if err != nil {
		return item, err
	}
	if !saved {
		return item, ErrTranslationConflict
	}
	item.TranslatedText, item.Status = text, "completed"
	s.logger.Info("人工编辑元数据翻译完成", "source", "translation", "bangumi_id", bangumiID, "target_kind", kind, "target_id", id, "shared", item.Shared)
	return item, nil
}

// Compare both original and translation in the UPDATE itself. A slow LLM reply
// must not replace a human edit or a translation for refreshed source text.
// Stable Bangumi IDs also survive metadata refreshes that recreate local rows.
func (s *Service) writeTranslation(ctx context.Context, field translationField, bangumiID, id int64, original, expected, text string, scoped bool) (bool, error) {
	query := "UPDATE " + field.table + " SET " + field.translated + " = ?"
	args := []any{text}
	if field.table != "anime_metadata" {
		query += ", updated_at = ?"
		args = append(args, s.now().UTC().Unix())
	}
	query += " WHERE " + field.id + " = ? AND " + field.original + " = ? AND " + field.translated + " = ?"
	args = append(args, id, original, expected)
	if scoped {
		if field.table == "actors" {
			query += ` AND EXISTS (SELECT 1 FROM character_actors ca
                JOIN anime_metadata am ON am.bangumi_id = ca.bangumi_id
                WHERE ca.actor_id = actors.actor_id AND ca.bangumi_id = ? AND am.deleted_at IS NULL)`
		} else {
			query += ` AND bangumi_id = ? AND EXISTS (SELECT 1 FROM anime_metadata am
                WHERE am.bangumi_id = ` + field.table + `.bangumi_id AND am.deleted_at IS NULL)`
		}
		args = append(args, bangumiID)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	return changed > 0, err
}
