package viewer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

const maxCarouselImageBytes = 10 << 20

var (
	ErrCarouselNotFound     = errors.New("viewer carousel item not found")
	ErrCarouselAnimeExists  = errors.New("viewer carousel anime already exists")
	ErrCarouselAnimeMissing = errors.New("viewer carousel anime not found")
	ErrInvalidCarouselImage = errors.New("carousel image must be a landscape JPEG or PNG up to 10 MiB")
)

type CarouselItem struct {
	ID             int64  `json:"id"`
	BangumiID      int64  `json:"bangumiId"`
	Title          string `json:"title"`
	SortOrder      int    `json:"sortOrder"`
	ImageUpdatedAt int64  `json:"imageUpdatedAt"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
}

type CarouselSlide struct {
	ID             int64    `json:"id"`
	BangumiID      int64    `json:"bangumiId"`
	Title          string   `json:"title"`
	Summary        string   `json:"summary"`
	AirDate        string   `json:"airDate"`
	RatingScore    *float64 `json:"ratingScore"`
	SortOrder      int      `json:"sortOrder"`
	ImageUpdatedAt int64    `json:"imageUpdatedAt"`
}

type CarouselItemInput struct {
	BangumiID int64
	SortOrder int
	ImageData []byte
}

type CarouselImage struct {
	Data        []byte
	ContentType string
	UpdatedAt   int64
}

func (s *Service) ListCarouselItems(ctx context.Context) ([]CarouselItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT ci.id,
       ci.bangumi_id,
       COALESCE(NULLIF(am.name_cn, ''), am.name),
       ci.sort_order,
       ci.image_updated_at,
       ci.created_at,
       ci.updated_at
FROM viewer_carousel_items ci
JOIN anime_metadata am ON am.bangumi_id = ci.bangumi_id
WHERE am.deleted_at IS NULL
ORDER BY ci.sort_order, ci.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]CarouselItem, 0)
	for rows.Next() {
		var item CarouselItem
		if err := rows.Scan(
			&item.ID, &item.BangumiID, &item.Title, &item.SortOrder,
			&item.ImageUpdatedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) CarouselSlides(ctx context.Context) ([]CarouselSlide, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT ci.id,
       ci.bangumi_id,
       COALESCE(NULLIF(am.name_cn, ''), am.name),
       COALESCE(NULLIF(am.summary_cn, ''), am.summary),
       am.air_date,
       am.rating_json,
       ci.sort_order,
       ci.image_updated_at
FROM viewer_carousel_items ci
JOIN anime_metadata am ON am.bangumi_id = ci.bangumi_id
WHERE am.deleted_at IS NULL
ORDER BY ci.sort_order, ci.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slides := make([]CarouselSlide, 0)
	for rows.Next() {
		var slide CarouselSlide
		var ratingJSON string
		if err := rows.Scan(
			&slide.ID, &slide.BangumiID, &slide.Title, &slide.Summary,
			&slide.AirDate, &ratingJSON, &slide.SortOrder, &slide.ImageUpdatedAt,
		); err != nil {
			return nil, err
		}
		slide.RatingScore = carouselRatingScore(ratingJSON)
		slides = append(slides, slide)
	}
	return slides, rows.Err()
}

func (s *Service) CreateCarouselItem(ctx context.Context, input CarouselItemInput) (CarouselItem, error) {
	contentType, err := validateCarouselImage(input.ImageData)
	if err != nil {
		return CarouselItem{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CarouselItem{}, err
	}
	defer tx.Rollback()
	if err := validateCarouselAnime(ctx, tx, input.BangumiID, 0); err != nil {
		return CarouselItem{}, err
	}
	now := s.now().UTC().Unix()
	result, err := tx.ExecContext(ctx, `
INSERT INTO viewer_carousel_items(
    bangumi_id, sort_order, image_data, image_content_type,
    image_updated_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		input.BangumiID, input.SortOrder, input.ImageData, contentType, now, now, now,
	)
	if err != nil {
		return CarouselItem{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return CarouselItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return CarouselItem{}, err
	}
	return s.carouselItem(ctx, id)
}

func (s *Service) UpdateCarouselItem(ctx context.Context, id int64, input CarouselItemInput) (CarouselItem, error) {
	var contentType string
	var err error
	if input.ImageData != nil {
		contentType, err = validateCarouselImage(input.ImageData)
		if err != nil {
			return CarouselItem{}, err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CarouselItem{}, err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM viewer_carousel_items WHERE id = ?)", id).Scan(&exists); err != nil {
		return CarouselItem{}, err
	}
	if !exists {
		return CarouselItem{}, ErrCarouselNotFound
	}
	if err := validateCarouselAnime(ctx, tx, input.BangumiID, id); err != nil {
		return CarouselItem{}, err
	}
	now := s.now().UTC().Unix()
	if input.ImageData == nil {
		_, err = tx.ExecContext(ctx, `
UPDATE viewer_carousel_items
SET bangumi_id = ?, sort_order = ?, updated_at = ?
WHERE id = ?`, input.BangumiID, input.SortOrder, now, id)
	} else {
		_, err = tx.ExecContext(ctx, `
UPDATE viewer_carousel_items
SET bangumi_id = ?, sort_order = ?, image_data = ?, image_content_type = ?,
    image_updated_at = ?, updated_at = ?
WHERE id = ?`, input.BangumiID, input.SortOrder, input.ImageData, contentType, now, now, id)
	}
	if err != nil {
		return CarouselItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return CarouselItem{}, err
	}
	return s.carouselItem(ctx, id)
}

func (s *Service) DeleteCarouselItem(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM viewer_carousel_items WHERE id = ?", id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrCarouselNotFound
	}
	return nil
}

func (s *Service) CarouselImage(ctx context.Context, id int64) (CarouselImage, error) {
	var image CarouselImage
	err := s.db.QueryRowContext(ctx, `
SELECT image_data, image_content_type, image_updated_at
FROM viewer_carousel_items
WHERE id = ?`, id).Scan(&image.Data, &image.ContentType, &image.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CarouselImage{}, ErrCarouselNotFound
	}
	return image, err
}

func (s *Service) carouselItem(ctx context.Context, id int64) (CarouselItem, error) {
	var item CarouselItem
	err := s.db.QueryRowContext(ctx, `
SELECT ci.id,
       ci.bangumi_id,
       COALESCE(NULLIF(am.name_cn, ''), am.name),
       ci.sort_order,
       ci.image_updated_at,
       ci.created_at,
       ci.updated_at
FROM viewer_carousel_items ci
JOIN anime_metadata am ON am.bangumi_id = ci.bangumi_id
WHERE ci.id = ? AND am.deleted_at IS NULL`, id).Scan(
		&item.ID, &item.BangumiID, &item.Title, &item.SortOrder,
		&item.ImageUpdatedAt, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CarouselItem{}, ErrCarouselNotFound
	}
	return item, err
}

func validateCarouselAnime(ctx context.Context, tx *sql.Tx, bangumiID, excludeID int64) error {
	if bangumiID < 1 {
		return ErrCarouselAnimeMissing
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM anime_metadata
    WHERE bangumi_id = ? AND deleted_at IS NULL
)`, bangumiID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrCarouselAnimeMissing
	}
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM viewer_carousel_items
    WHERE bangumi_id = ? AND id != ?
)`, bangumiID, excludeID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrCarouselAnimeExists
	}
	return nil
}

func validateCarouselImage(data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxCarouselImageBytes {
		return "", ErrInvalidCarouselImage
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= config.Height {
		return "", ErrInvalidCarouselImage
	}
	switch strings.ToLower(format) {
	case "jpeg":
		return "image/jpeg", nil
	case "png":
		return "image/png", nil
	default:
		return "", ErrInvalidCarouselImage
	}
}

func carouselRatingScore(raw string) *float64 {
	var rating struct {
		Score float64 `json:"score"`
	}
	if json.Unmarshal([]byte(raw), &rating) != nil || rating.Score <= 0 {
		return nil
	}
	return &rating.Score
}
