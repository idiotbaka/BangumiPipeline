package viewer

import (
	"context"
	"errors"
)

var ErrInvalidSinglePage = errors.New("unknown single page")

// Keep this list in step with the viewer's built-in seasonal page components.
// Only keys from this list can be selected by an administrator.
var singlePages = []SinglePage{
	{Key: "202610-preview", Title: "10月秋番报告", Path: "/special/202610"},
}

type SinglePage struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Path  string `json:"path"`
}

type SinglePageConfig struct {
	Items     []SinglePage `json:"items"`
	ActiveKey string       `json:"activeKey"`
}

func singlePageByKey(key string) *SinglePage {
	for _, page := range singlePages {
		if page.Key == key {
			copy := page
			return &copy
		}
	}
	return nil
}

func (s *Service) SinglePageConfig(ctx context.Context) (SinglePageConfig, error) {
	var activeKey string
	if err := s.db.QueryRowContext(ctx, `SELECT active_single_page_key FROM viewer_site_settings WHERE id = 1`).Scan(&activeKey); err != nil {
		return SinglePageConfig{}, err
	}
	if singlePageByKey(activeKey) == nil {
		activeKey = ""
	}
	return SinglePageConfig{Items: append([]SinglePage(nil), singlePages...), ActiveKey: activeKey}, nil
}

func (s *Service) SetActiveSinglePage(ctx context.Context, key string) (SinglePageConfig, error) {
	if key != "" && singlePageByKey(key) == nil {
		return SinglePageConfig{}, ErrInvalidSinglePage
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE viewer_site_settings
SET active_single_page_key = ?, updated_at = ?
WHERE id = 1`, key, s.now().UTC().Unix()); err != nil {
		return SinglePageConfig{}, err
	}
	return s.SinglePageConfig(ctx)
}
