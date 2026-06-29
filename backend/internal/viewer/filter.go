package viewer

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxFilterDimensions = 20
	maxFilterTags       = 50
	maxFilterNameRunes  = 40
	maxFilterTagRunes   = 80
)

var (
	ErrFilterDimensionNotFound = errors.New("viewer filter dimension not found")
	ErrFilterDimensionExists   = errors.New("viewer filter dimension already exists")
	ErrInvalidFilterDimension  = errors.New("invalid viewer filter dimension")
	ErrTooManyFilterDimensions = errors.New("too many viewer filter dimensions")
	ErrInvalidLibraryFilter    = errors.New("invalid viewer library filter")
)

type FilterDimension struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	SortOrder int      `json:"sortOrder"`
	Tags      []string `json:"tags"`
	CreatedAt int64    `json:"createdAt"`
	UpdatedAt int64    `json:"updatedAt"`
}

type FilterDimensionInput struct {
	Name      string
	SortOrder int
	Tags      []string
}

func (s *Service) ListFilterDimensions(ctx context.Context) ([]FilterDimension, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT dimension.id, dimension.name, dimension.sort_order,
       dimension.created_at, dimension.updated_at,
       tag.name
FROM viewer_filter_dimensions dimension
LEFT JOIN viewer_filter_tags tag ON tag.dimension_id = dimension.id
ORDER BY dimension.sort_order, dimension.id, tag.sort_order, tag.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]FilterDimension, 0)
	indexByID := make(map[int64]int)
	for rows.Next() {
		var item FilterDimension
		var tag sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Name, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt, &tag,
		); err != nil {
			return nil, err
		}
		index, exists := indexByID[item.ID]
		if !exists {
			item.Tags = make([]string, 0)
			items = append(items, item)
			index = len(items) - 1
			indexByID[item.ID] = index
		}
		if tag.Valid {
			items[index].Tags = append(items[index].Tags, tag.String)
		}
	}
	return items, rows.Err()
}

func (s *Service) CreateFilterDimension(ctx context.Context, input FilterDimensionInput) (FilterDimension, error) {
	input, err := normalizeFilterDimension(input)
	if err != nil {
		return FilterDimension{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FilterDimension{}, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM viewer_filter_dimensions").Scan(&count); err != nil {
		return FilterDimension{}, err
	}
	if count >= maxFilterDimensions {
		return FilterDimension{}, ErrTooManyFilterDimensions
	}
	if exists, err := filterDimensionNameExists(ctx, tx, input.Name, 0); err != nil {
		return FilterDimension{}, err
	} else if exists {
		return FilterDimension{}, ErrFilterDimensionExists
	}
	now := s.now().UTC().Unix()
	result, err := tx.ExecContext(ctx, `
INSERT INTO viewer_filter_dimensions(name, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?)`, input.Name, input.SortOrder, now, now)
	if err != nil {
		return FilterDimension{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return FilterDimension{}, err
	}
	if err := replaceFilterTags(ctx, tx, id, input.Tags); err != nil {
		return FilterDimension{}, err
	}
	if err := tx.Commit(); err != nil {
		return FilterDimension{}, err
	}
	return s.filterDimension(ctx, id)
}

func (s *Service) UpdateFilterDimension(ctx context.Context, id int64, input FilterDimensionInput) (FilterDimension, error) {
	input, err := normalizeFilterDimension(input)
	if err != nil {
		return FilterDimension{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FilterDimension{}, err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM viewer_filter_dimensions WHERE id = ?)", id).Scan(&exists); err != nil {
		return FilterDimension{}, err
	}
	if !exists {
		return FilterDimension{}, ErrFilterDimensionNotFound
	}
	if exists, err := filterDimensionNameExists(ctx, tx, input.Name, id); err != nil {
		return FilterDimension{}, err
	} else if exists {
		return FilterDimension{}, ErrFilterDimensionExists
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE viewer_filter_dimensions
SET name = ?, sort_order = ?, updated_at = ?
WHERE id = ?`, input.Name, input.SortOrder, s.now().UTC().Unix(), id); err != nil {
		return FilterDimension{}, err
	}
	if err := replaceFilterTags(ctx, tx, id, input.Tags); err != nil {
		return FilterDimension{}, err
	}
	if err := tx.Commit(); err != nil {
		return FilterDimension{}, err
	}
	return s.filterDimension(ctx, id)
}

func (s *Service) DeleteFilterDimension(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM viewer_filter_dimensions WHERE id = ?", id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrFilterDimensionNotFound
	}
	return nil
}

// ResolveLibraryFilters validates selections against the administrator-managed
// dimensions. Each returned inner slice is one dimension (OR); slices are
// combined by the catalog query as AND conditions.
func (s *Service) ResolveLibraryFilters(ctx context.Context, selections map[int64][]string) ([][]string, error) {
	if len(selections) == 0 {
		return nil, nil
	}
	dimensions, err := s.ListFilterDimensions(ctx)
	if err != nil {
		return nil, err
	}
	allowed := make(map[int64]map[string]struct{}, len(dimensions))
	for _, dimension := range dimensions {
		tags := make(map[string]struct{}, len(dimension.Tags))
		for _, tag := range dimension.Tags {
			tags[tag] = struct{}{}
		}
		allowed[dimension.ID] = tags
	}
	groups := make([][]string, 0, len(selections))
	resolvedDimensions := 0
	for _, dimension := range dimensions {
		selected, ok := selections[dimension.ID]
		if !ok {
			continue
		}
		resolvedDimensions++
		seen := make(map[string]struct{}, len(selected))
		group := make([]string, 0, len(selected))
		for _, tag := range selected {
			if _, ok := allowed[dimension.ID][tag]; !ok {
				return nil, ErrInvalidLibraryFilter
			}
			if _, duplicate := seen[tag]; duplicate {
				continue
			}
			seen[tag] = struct{}{}
			group = append(group, tag)
		}
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}
	if resolvedDimensions != len(selections) {
		return nil, ErrInvalidLibraryFilter
	}
	return groups, nil
}

func (s *Service) filterDimension(ctx context.Context, id int64) (FilterDimension, error) {
	items, err := s.ListFilterDimensions(ctx)
	if err != nil {
		return FilterDimension{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return FilterDimension{}, ErrFilterDimensionNotFound
}

func normalizeFilterDimension(input FilterDimensionInput) (FilterDimensionInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > maxFilterNameRunes || strings.IndexFunc(input.Name, unicode.IsControl) >= 0 {
		return FilterDimensionInput{}, ErrInvalidFilterDimension
	}
	seen := make(map[string]struct{}, len(input.Tags))
	tags := make([]string, 0, len(input.Tags))
	for _, value := range input.Tags {
		tag := strings.TrimSpace(value)
		if tag == "" || utf8.RuneCountInString(tag) > maxFilterTagRunes || strings.IndexFunc(tag, unicode.IsControl) >= 0 {
			return FilterDimensionInput{}, ErrInvalidFilterDimension
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	if len(tags) == 0 || len(tags) > maxFilterTags {
		return FilterDimensionInput{}, ErrInvalidFilterDimension
	}
	input.Tags = tags
	return input, nil
}

func filterDimensionNameExists(ctx context.Context, tx *sql.Tx, name string, excludeID int64) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM viewer_filter_dimensions
    WHERE name = ? COLLATE NOCASE AND id != ?
)`, name, excludeID).Scan(&exists)
	return exists, err
}

func replaceFilterTags(ctx context.Context, tx *sql.Tx, dimensionID int64, tags []string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM viewer_filter_tags WHERE dimension_id = ?", dimensionID); err != nil {
		return err
	}
	for index, tag := range tags {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO viewer_filter_tags(dimension_id, name, sort_order)
VALUES (?, ?, ?)`, dimensionID, tag, index); err != nil {
			return err
		}
	}
	return nil
}
