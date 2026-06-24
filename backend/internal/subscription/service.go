package subscription

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"bangumipipeline.local/server/internal/system"
)

const (
	TaskKey = "subscription-rss-match"

	matchStatusMatched   = "matched"
	matchStatusUnmatched = "unmatched"

	BindingStatusPending = "pending"
	BindingStatusBound   = "bound"
	BindingStatusIgnored = "ignored"

	defaultRequestTimeout = 20 * time.Second
	rssResponseLimit      = 10 << 20
	minMatchScore         = 0.72
	mikanSearchURL        = "https://mikanani.me/RSS/Search"
)

var (
	ErrItemNotFound          = errors.New("subscription item not found")
	ErrInvalidBinding        = errors.New("invalid subscription binding")
	ErrHistorySourceNotFound = errors.New("history source not found")
	ErrInvalidHistorySearch  = errors.New("invalid history search")
)

type SettingsProvider interface {
	GetNetworkSettings(context.Context) (system.NetworkSettings, error)
	GetSubscriptionSettings(context.Context) (system.SubscriptionSettings, error)
}

type Service struct {
	db       *sql.DB
	settings SettingsProvider
	logger   *slog.Logger
	timeout  time.Duration
	now      func() time.Time
}

func NewService(db *sql.DB, settings SettingsProvider, logger *slog.Logger) *Service {
	return &Service{
		db: db, settings: settings, logger: logger,
		timeout: defaultRequestTimeout, now: time.Now,
	}
}

type ItemPage struct {
	Items    []Item `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type HistorySyncResult struct {
	BangumiID        int64  `json:"bangumiId"`
	SourceTitle      string `json:"sourceTitle"`
	SearchTitle      string `json:"searchTitle"`
	Fetched          int    `json:"fetched"`
	Inserted         int    `json:"inserted"`
	Bound            int    `json:"bound"`
	SkippedExisting  int    `json:"skippedExisting"`
	SkippedIgnored   int    `json:"skippedIgnored"`
	SkippedUnmatched int    `json:"skippedUnmatched"`
}

type BindingInput struct {
	BangumiID     int64
	SeasonNumber  int
	EpisodeType   string
	EpisodeNumber string
}

type Item struct {
	ID                 int64   `json:"id"`
	GUID               string  `json:"guid"`
	Title              string  `json:"title"`
	Description        string  `json:"description"`
	Link               string  `json:"link"`
	EnclosureURL       string  `json:"enclosureUrl"`
	TorrentURL         string  `json:"torrentUrl"`
	ContentLength      int64   `json:"contentLength"`
	PubDate            string  `json:"pubDate"`
	PublishedAt        *int64  `json:"publishedAt"`
	MatchStatus        string  `json:"matchStatus"`
	BangumiID          *int64  `json:"bangumiId"`
	MatchedName        string  `json:"matchedName"`
	ParsedName         string  `json:"parsedName"`
	SeasonNumber       *int    `json:"seasonNumber"`
	EpisodeType        string  `json:"episodeType"`
	EpisodeNumber      string  `json:"episodeNumber"`
	MatchScore         float64 `json:"matchScore"`
	MatchReason        string  `json:"matchReason"`
	BindingStatus      string  `json:"bindingStatus"`
	BoundBangumiID     *int64  `json:"boundBangumiId"`
	BoundAnimeName     string  `json:"boundAnimeName"`
	BoundSeasonNumber  *int    `json:"boundSeasonNumber"`
	BoundEpisodeType   string  `json:"boundEpisodeType"`
	BoundEpisodeNumber string  `json:"boundEpisodeNumber"`
	BindingNote        string  `json:"bindingNote"`
	BoundAt            *int64  `json:"boundAt"`
	IgnoredAt          *int64  `json:"ignoredAt"`
	CreatedAt          int64   `json:"createdAt"`
	UpdatedAt          int64   `json:"updatedAt"`
}

type rawItem struct {
	Key           string `json:"-"`
	GUID          string `json:"guid"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Link          string `json:"link"`
	EnclosureURL  string `json:"enclosureUrl"`
	TorrentURL    string `json:"torrentUrl"`
	ContentLength int64  `json:"contentLength"`
	PubDate       string `json:"pubDate"`
	PublishedAt   *int64 `json:"publishedAt,omitempty"`
}

type historySource struct {
	BangumiID     int64
	ItemID        int64
	Title         string
	AnimeName     string
	SeasonNumber  int
	EpisodeType   string
	EpisodeNumber string
}

type episodeIdentity struct {
	SeasonNumber  int
	EpisodeType   string
	EpisodeNumber string
}

type matchResult struct {
	Status             string
	BangumiID          *int64
	MatchedName        string
	ParsedName         string
	SeasonNumber       *int
	EpisodeType        string
	EpisodeNumber      string
	Score              float64
	Reason             string
	BindingStatus      string
	BoundBangumiID     *int64
	BoundAnimeName     string
	BoundSeasonNumber  *int
	BoundEpisodeType   string
	BoundEpisodeNumber string
	BindingNote        string
}

type scoreResult struct {
	Score  float64
	Method string
}

func (s *Service) Execute(ctx context.Context) error {
	settings, err := s.settings.GetSubscriptionSettings(ctx)
	if err != nil {
		return fmt.Errorf("读取订阅设置: %w", err)
	}
	if settings.RSSURL == "" {
		s.logger.Info("订阅抓取跳过：RSS URL 未配置", "source", "subscription")
		return nil
	}
	network, err := s.settings.GetNetworkSettings(ctx)
	if err != nil {
		return fmt.Errorf("读取代理设置: %w", err)
	}
	client, err := s.httpClient(network)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()

	items, err := s.fetch(ctx, client, settings.RSSURL)
	if err != nil {
		return err
	}
	candidates, err := s.loadCandidates(ctx)
	if err != nil {
		return fmt.Errorf("读取本地番剧候选: %w", err)
	}

	inserted, skipped, matched, unmatched := 0, 0, 0, 0
	currentKeys := make([]string, 0, len(items))
	for _, item := range items {
		currentKeys = append(currentKeys, item.Key)
		result := matchItem(item.Title, candidates)
		result, err = s.applyTitleRule(ctx, item, result)
		if err != nil {
			return fmt.Errorf("应用订阅标题记忆失败: %w", err)
		}
		result, err = s.rejectDuplicateMatch(ctx, "", result)
		if err != nil {
			return fmt.Errorf("检查重复订阅条目失败: %w", err)
		}
		created, err := s.insertItem(ctx, item, result)
		if err != nil {
			return fmt.Errorf("订阅条目入库失败: %w", err)
		}
		if created {
			inserted++
			if result.Status == matchStatusMatched {
				matched++
			} else {
				unmatched++
			}
		} else {
			skipped++
		}
	}
	rematched, err := s.rematchUnmatched(ctx, candidates, currentKeys)
	if err != nil {
		return fmt.Errorf("重新匹配未识别条目失败: %w", err)
	}
	matched += rematched
	s.logger.Info("RSS 订阅抓取完成", "source", "subscription",
		"inserted", inserted, "skipped", skipped, "matched", matched, "unmatched", unmatched, "rematched", rematched)
	return nil
}

func (s *Service) SyncHistory(ctx context.Context, bangumiID int64) (HistorySyncResult, error) {
	if bangumiID < 1 {
		return HistorySyncResult{}, ErrInvalidBinding
	}
	source, err := s.latestHistorySource(ctx, bangumiID)
	if err != nil {
		return HistorySyncResult{}, err
	}
	searchTitle, err := historySearchTitle(source.Title, source.EpisodeNumber)
	if err != nil {
		return HistorySyncResult{}, err
	}
	searchURL := buildMikanHistorySearchURL(searchTitle)
	sourceKey := titleMemoryKey(source.Title)
	if sourceKey == "" {
		return HistorySyncResult{}, ErrInvalidHistorySearch
	}

	network, err := s.settings.GetNetworkSettings(ctx)
	if err != nil {
		return HistorySyncResult{}, fmt.Errorf("读取代理设置: %w", err)
	}
	client, err := s.httpClient(network)
	if err != nil {
		return HistorySyncResult{}, err
	}
	defer client.CloseIdleConnections()

	s.logger.Info("历史话数同步开始", "source", "subscription", "bangumi_id", bangumiID, "source_item_id", source.ItemID)
	items, err := s.fetch(ctx, client, searchURL)
	if err != nil {
		return HistorySyncResult{}, err
	}
	result := HistorySyncResult{
		BangumiID: bangumiID, SourceTitle: source.Title, SearchTitle: searchTitle, Fetched: len(items),
	}
	existing, err := s.boundEpisodeSet(ctx, bangumiID)
	if err != nil {
		return result, err
	}
	for _, item := range items {
		identity, ok := historyEpisodeIdentity(item.Title)
		if !ok || identity.SeasonNumber != source.SeasonNumber || identity.EpisodeType != source.EpisodeType {
			result.SkippedUnmatched++
			continue
		}
		if sourceKey != "" && titleMemoryKey(item.Title) != sourceKey {
			result.SkippedUnmatched++
			continue
		}
		if _, ok := existing[identity]; ok {
			result.SkippedExisting++
			continue
		}
		status, err := s.upsertHistoryItem(ctx, item, source, identity)
		if err != nil {
			return result, err
		}
		switch status {
		case "inserted":
			result.Inserted++
			result.Bound++
			existing[identity] = struct{}{}
		case "bound":
			result.Bound++
			existing[identity] = struct{}{}
		case "ignored":
			result.SkippedIgnored++
		default:
			result.SkippedExisting++
		}
	}
	s.logger.Info("历史话数同步完成", "source", "subscription", "bangumi_id", bangumiID,
		"fetched", result.Fetched, "inserted", result.Inserted, "bound", result.Bound,
		"skipped_existing", result.SkippedExisting, "skipped_ignored", result.SkippedIgnored,
		"skipped_unmatched", result.SkippedUnmatched)
	return result, nil
}

func (s *Service) latestHistorySource(ctx context.Context, bangumiID int64) (historySource, error) {
	var source historySource
	err := s.db.QueryRowContext(ctx, `
SELECT id, title, bound_anime_name, bound_season_number,
       COALESCE(NULLIF(bound_episode_type, ''), 'episode'), bound_episode_number
FROM subscription_items
WHERE binding_status = ?
  AND bound_bangumi_id = ?
  AND bound_season_number IS NOT NULL
  AND bound_episode_number != ''
ORDER BY bound_season_number DESC,
         CASE WHEN COALESCE(NULLIF(bound_episode_type, ''), 'episode') = 'episode' THEN 0 ELSE 1 END,
         CASE WHEN bound_episode_number GLOB '[0-9]*' THEN CAST(bound_episode_number AS REAL) ELSE -1 END DESC,
         COALESCE(published_at, created_at) DESC,
         id DESC
LIMIT 1`, BindingStatusBound, bangumiID).Scan(
		&source.ItemID, &source.Title, &source.AnimeName, &source.SeasonNumber,
		&source.EpisodeType, &source.EpisodeNumber,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return historySource{}, ErrHistorySourceNotFound
	}
	if err != nil {
		return historySource{}, err
	}
	source.BangumiID = bangumiID
	source.EpisodeType = normalizeEpisodeType(source.EpisodeType)
	source.EpisodeNumber = strings.TrimSpace(source.EpisodeNumber)
	if source.SeasonNumber < 1 || source.EpisodeNumber == "" {
		return historySource{}, ErrHistorySourceNotFound
	}
	if strings.TrimSpace(source.AnimeName) == "" {
		animeName, err := animeNameByDB(ctx, s.db, bangumiID)
		if err != nil {
			return historySource{}, err
		}
		source.AnimeName = animeName
	}
	return source, nil
}

func animeNameByDB(ctx context.Context, db *sql.DB, bangumiID int64) (string, error) {
	var name, nameCN string
	err := db.QueryRowContext(ctx, `
SELECT name, name_cn
FROM anime_metadata
WHERE bangumi_id = ? AND deleted_at IS NULL`, bangumiID).Scan(&name, &nameCN)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidBinding
	}
	if err != nil {
		return "", err
	}
	if nameCN != "" {
		return nameCN, nil
	}
	return name, nil
}

func (s *Service) boundEpisodeSet(ctx context.Context, bangumiID int64) (map[episodeIdentity]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT bound_season_number, COALESCE(NULLIF(bound_episode_type, ''), 'episode'), bound_episode_number
FROM subscription_items
WHERE binding_status = ?
  AND bound_bangumi_id = ?
  AND bound_season_number IS NOT NULL
  AND bound_episode_number != ''`, BindingStatusBound, bangumiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[episodeIdentity]struct{})
	for rows.Next() {
		var identity episodeIdentity
		if err := rows.Scan(&identity.SeasonNumber, &identity.EpisodeType, &identity.EpisodeNumber); err != nil {
			return nil, err
		}
		identity.EpisodeType = normalizeEpisodeType(identity.EpisodeType)
		identity.EpisodeNumber = strings.TrimSpace(identity.EpisodeNumber)
		if identity.SeasonNumber > 0 && identity.EpisodeNumber != "" {
			result[identity] = struct{}{}
		}
	}
	return result, rows.Err()
}

func historyEpisodeIdentity(title string) (episodeIdentity, bool) {
	parsed := parseTitle(title)
	if parsed.SeasonNumber == nil || parsed.EpisodeNumber == "" {
		return episodeIdentity{}, false
	}
	return episodeIdentity{
		SeasonNumber:  *parsed.SeasonNumber,
		EpisodeType:   normalizeEpisodeType(parsed.EpisodeType),
		EpisodeNumber: strings.TrimSpace(parsed.EpisodeNumber),
	}, true
}

func (s *Service) upsertHistoryItem(ctx context.Context, item rawItem, source historySource, identity episodeIdentity) (string, error) {
	parsed := parseTitle(item.Title)
	bangumiID := source.BangumiID
	seasonNumber := identity.SeasonNumber
	rawJSON, _ := json.Marshal(item)
	now := s.now().UTC().Unix()
	created, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO subscription_items(
    item_key, guid, title, description, link, enclosure_url, torrent_url, content_length,
    pub_date, published_at, match_status, bangumi_id, matched_name, parsed_name,
    season_number, episode_type, episode_number, match_score, match_reason,
    binding_status, bound_bangumi_id, bound_anime_name, bound_season_number,
    bound_episode_type, bound_episode_number, binding_note, bound_at, ignored_at,
    raw_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.Key, item.GUID, item.Title, item.Description, item.Link, item.EnclosureURL, item.TorrentURL, item.ContentLength,
		item.PubDate, item.PublishedAt, matchStatusMatched, bangumiID, source.AnimeName, parsed.ParsedName,
		seasonNumber, identity.EpisodeType, identity.EpisodeNumber, 1.0, "历史话数搜索自动匹配",
		BindingStatusBound, bangumiID, source.AnimeName, seasonNumber,
		identity.EpisodeType, identity.EpisodeNumber, "历史话数同步自动绑定", now, nil,
		string(rawJSON), now, now,
	)
	if err != nil {
		return "", err
	}
	if affected, err := created.RowsAffected(); err != nil {
		return "", err
	} else if affected > 0 {
		return "inserted", nil
	}

	updated, err := s.db.ExecContext(ctx, `
UPDATE subscription_items
SET match_status = ?, bangumi_id = ?, matched_name = ?, parsed_name = ?,
    season_number = ?, episode_type = ?, episode_number = ?, match_score = ?, match_reason = ?,
    binding_status = ?, bound_bangumi_id = ?, bound_anime_name = ?, bound_season_number = ?,
    bound_episode_type = ?, bound_episode_number = ?, binding_note = ?, bound_at = ?, ignored_at = NULL,
    raw_json = ?, updated_at = ?
WHERE item_key = ? AND binding_status = ?`,
		matchStatusMatched, bangumiID, source.AnimeName, parsed.ParsedName,
		seasonNumber, identity.EpisodeType, identity.EpisodeNumber, 1.0, "历史话数搜索自动匹配",
		BindingStatusBound, bangumiID, source.AnimeName, seasonNumber,
		identity.EpisodeType, identity.EpisodeNumber, "历史话数同步自动绑定", now,
		string(rawJSON), now, item.Key, BindingStatusPending,
	)
	if err != nil {
		return "", err
	}
	if affected, err := updated.RowsAffected(); err != nil {
		return "", err
	} else if affected > 0 {
		return "bound", nil
	}

	var status string
	if err := s.db.QueryRowContext(ctx, "SELECT binding_status FROM subscription_items WHERE item_key = ?", item.Key).Scan(&status); err != nil {
		return "", err
	}
	if status == BindingStatusIgnored {
		return "ignored", nil
	}
	return "existing", nil
}

func (s *Service) ListItems(ctx context.Context, page, pageSize int, bindingStatus string) (ItemPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	bindingStatus = strings.TrimSpace(bindingStatus)
	if bindingStatus != "" && bindingStatus != BindingStatusPending && bindingStatus != BindingStatusBound && bindingStatus != BindingStatusIgnored {
		bindingStatus = ""
	}
	result := ItemPage{Items: make([]Item, 0), Page: page, PageSize: pageSize}
	countQuery := "SELECT COUNT(*) FROM subscription_items"
	args := make([]any, 0, 3)
	if bindingStatus != "" {
		countQuery += " WHERE binding_status = ?"
		args = append(args, bindingStatus)
	}
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	query := itemSelect
	if bindingStatus != "" {
		query += "\nWHERE binding_status = ?"
	}
	query += `
ORDER BY COALESCE(published_at, created_at) DESC, id DESC
LIMIT ? OFFSET ?`
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

const itemSelect = `
SELECT id, guid, title, description, link, enclosure_url, torrent_url, content_length,
       pub_date, published_at, match_status, bangumi_id, matched_name, parsed_name,
       season_number, episode_type, episode_number, match_score, match_reason,
       binding_status, bound_bangumi_id, bound_anime_name, bound_season_number,
       bound_episode_type, bound_episode_number, binding_note, bound_at, ignored_at,
       created_at, updated_at
FROM subscription_items`

func (s *Service) ConfirmBinding(ctx context.Context, itemID int64) (Item, error) {
	item, err := s.Item(ctx, itemID)
	if err != nil {
		return Item{}, err
	}
	if item.MatchStatus != matchStatusMatched || item.BangumiID == nil || item.SeasonNumber == nil || item.EpisodeNumber == "" {
		return Item{}, ErrInvalidBinding
	}
	return s.bindItem(ctx, itemID, BindingInput{
		BangumiID: *item.BangumiID, SeasonNumber: *item.SeasonNumber,
		EpisodeType: item.EpisodeType, EpisodeNumber: item.EpisodeNumber,
	}, "人工确认绑定")
}

func (s *Service) BindManually(ctx context.Context, itemID int64, input BindingInput) (Item, error) {
	return s.bindItem(ctx, itemID, input, "人工手动绑定")
}

func (s *Service) IgnoreItem(ctx context.Context, itemID int64) (Item, error) {
	now := s.now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `
UPDATE subscription_items
SET binding_status = ?, binding_note = '已忽略',
    bound_bangumi_id = NULL, bound_anime_name = '', bound_season_number = NULL,
    bound_episode_type = '', bound_episode_number = '', bound_at = NULL,
    ignored_at = ?, updated_at = ?
WHERE id = ?`, BindingStatusIgnored, now, now, itemID)
	if err != nil {
		return Item{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Item{}, err
	}
	if affected == 0 {
		return Item{}, ErrItemNotFound
	}
	return s.Item(ctx, itemID)
}

func (s *Service) Item(ctx context.Context, itemID int64) (Item, error) {
	item, err := scanItem(s.db.QueryRowContext(ctx, itemSelect+`
WHERE id = ?`, itemID))
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrItemNotFound
	}
	return item, err
}

func (s *Service) bindItem(ctx context.Context, itemID int64, input BindingInput, note string) (Item, error) {
	input.EpisodeType = normalizeEpisodeType(input.EpisodeType)
	input.EpisodeNumber = strings.TrimSpace(input.EpisodeNumber)
	if itemID < 1 || input.BangumiID < 1 || input.SeasonNumber < 1 || input.EpisodeNumber == "" {
		return Item{}, ErrInvalidBinding
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()

	var title string
	if err := tx.QueryRowContext(ctx, "SELECT title FROM subscription_items WHERE id = ?", itemID).Scan(&title); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Item{}, ErrItemNotFound
		}
		return Item{}, err
	}
	animeName, err := animeNameByID(ctx, tx, input.BangumiID)
	if err != nil {
		return Item{}, err
	}

	now := s.now().UTC().Unix()
	if _, err := tx.ExecContext(ctx, `
UPDATE subscription_items
SET binding_status = ?, binding_note = ?,
    bound_bangumi_id = NULL, bound_anime_name = '', bound_season_number = NULL,
    bound_episode_type = '', bound_episode_number = '', bound_at = NULL,
    ignored_at = NULL, updated_at = ?
WHERE binding_status = ?
  AND bound_bangumi_id = ?
  AND bound_season_number = ?
  AND bound_episode_type = ?
  AND bound_episode_number = ?
  AND id != ?`,
		BindingStatusPending, fmt.Sprintf("绑定已被订阅条目 #%d 覆盖", itemID), now,
		BindingStatusBound, input.BangumiID, input.SeasonNumber, input.EpisodeType, input.EpisodeNumber, itemID,
	); err != nil {
		return Item{}, err
	}

	result, err := tx.ExecContext(ctx, `
UPDATE subscription_items
SET binding_status = ?, bound_bangumi_id = ?, bound_anime_name = ?,
    bound_season_number = ?, bound_episode_type = ?, bound_episode_number = ?,
    binding_note = ?, bound_at = ?, ignored_at = NULL, updated_at = ?
WHERE id = ?`, BindingStatusBound, input.BangumiID, animeName, input.SeasonNumber,
		input.EpisodeType, input.EpisodeNumber, note, now, now, itemID)
	if err != nil {
		return Item{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Item{}, err
	}
	if affected == 0 {
		return Item{}, ErrItemNotFound
	}
	if err := saveTitleRule(ctx, tx, itemID, title, input, animeName, now); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	return s.Item(ctx, itemID)
}

func normalizeEpisodeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "episode"
	}
	if value == "special" {
		return "sp"
	}
	return value
}

func animeNameByID(ctx context.Context, tx *sql.Tx, bangumiID int64) (string, error) {
	var name, nameCN string
	err := tx.QueryRowContext(ctx, `
SELECT name, name_cn
FROM anime_metadata
WHERE bangumi_id = ? AND deleted_at IS NULL`, bangumiID).Scan(&name, &nameCN)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidBinding
	}
	if err != nil {
		return "", err
	}
	if nameCN != "" {
		return nameCN, nil
	}
	return name, nil
}

func saveTitleRule(ctx context.Context, tx *sql.Tx, itemID int64, title string, input BindingInput, animeName string, now int64) error {
	key := titleMemoryKey(title)
	if key == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO subscription_title_rules(
    title_key, bangumi_id, anime_name, season_number, episode_type,
    created_from_item_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(title_key) DO UPDATE SET
    bangumi_id = excluded.bangumi_id,
    anime_name = excluded.anime_name,
    season_number = excluded.season_number,
    episode_type = excluded.episode_type,
    created_from_item_id = excluded.created_from_item_id,
    updated_at = excluded.updated_at`,
		key, input.BangumiID, animeName, input.SeasonNumber, input.EpisodeType, itemID, now, now)
	return err
}

func historySearchTitle(title, episodeNumber string) (string, error) {
	title = strings.TrimSpace(title)
	episodeNumber = strings.TrimSpace(episodeNumber)
	if title == "" || episodeNumber == "" {
		return "", ErrInvalidHistorySearch
	}
	quoted := regexp.QuoteMeta(episodeNumber)
	patterns := []struct {
		pattern string
		group   int
	}{
		{pattern: `(?i)\bS\d{1,2}\s*E\s*(` + quoted + `)(?:v\d+)?\b`, group: 1},
		{pattern: `第\s*(` + quoted + `)\s*[話话集]`, group: 1},
		{pattern: `(?i)(^|[\s\-_【\[\(（])(` + quoted + `)(?:v\d+)?($|[\]】\)）\s])`, group: 2},
	}
	for _, candidate := range patterns {
		result, ok, err := removeLastEpisodeNumber(title, candidate.pattern, candidate.group)
		if err != nil {
			return "", err
		}
		if ok {
			return result, nil
		}
	}
	return "", ErrInvalidHistorySearch
}

func removeLastEpisodeNumber(title, pattern string, group int) (string, bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", false, fmt.Errorf("%w: %v", ErrInvalidHistorySearch, err)
	}
	matches := re.FindAllStringSubmatchIndex(title, -1)
	if len(matches) == 0 {
		return "", false, nil
	}
	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		startIndex := group * 2
		endIndex := startIndex + 1
		if len(match) <= endIndex || match[startIndex] < 0 || match[endIndex] < 0 {
			continue
		}
		result := strings.TrimSpace(title[:match[startIndex]] + title[match[endIndex]:])
		if result != "" && result != title {
			return result, true, nil
		}
	}
	return "", false, nil
}

func buildMikanHistorySearchURL(searchTitle string) string {
	parsed, _ := url.Parse(mikanSearchURL)
	query := parsed.Query()
	query.Set("searchstr", searchTitle)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
func (s *Service) httpClient(settings system.NetworkSettings) (*http.Client, error) {
	httpProxy, err := parseOptionalURL(settings.HTTPProxy)
	if err != nil {
		return nil, fmt.Errorf("HTTP 代理配置无效: %w", err)
	}
	httpsProxy, err := parseOptionalURL(settings.HTTPSProxy)
	if err != nil {
		return nil, fmt.Errorf("HTTPS 代理配置无效: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = s.timeout
	transport.Proxy = func(request *http.Request) (*url.URL, error) {
		if request.URL.Scheme == "https" {
			return httpsProxy, nil
		}
		return httpProxy, nil
	}
	return &http.Client{Transport: transport, Timeout: s.timeout}, nil
}

func parseOptionalURL(value string) (*url.URL, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func (s *Service) fetch(ctx context.Context, client *http.Client, rssURL string) ([]rawItem, error) {
	s.logger.Info("RSS 订阅抓取中", "source", "subscription", "url", logURL(rssURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rssURL, nil)
	if err != nil {
		s.logger.Error("RSS 订阅抓取失败", "source", "subscription", "url", logURL(rssURL), "error", err)
		return nil, err
	}
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml;q=0.9, */*;q=0.8")
	req.Header.Set("User-Agent", "BangumiPipeline/0.1")
	response, err := client.Do(req)
	if err != nil {
		s.logger.Error("RSS 订阅抓取失败", "source", "subscription", "url", logURL(rssURL), "error", err)
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		err := fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
		s.logger.Error("RSS 订阅抓取失败", "source", "subscription", "url", logURL(rssURL), "error", err)
		return nil, err
	}
	var feed rssFeed
	if err := xml.NewDecoder(io.LimitReader(response.Body, rssResponseLimit)).Decode(&feed); err != nil {
		s.logger.Error("RSS 订阅解析失败", "source", "subscription", "url", logURL(rssURL), "error", err)
		return nil, err
	}
	items := make([]rawItem, 0, len(feed.Channel.Items))
	for _, source := range feed.Channel.Items {
		item := source.toRaw()
		if item.Title == "" && item.EnclosureURL == "" && item.Link == "" {
			continue
		}
		item.Key = itemKey(item)
		items = append(items, item)
	}
	s.logger.Info("RSS 订阅抓取成功", "source", "subscription", "url", logURL(rssURL), "items", len(items))
	return items, nil
}

func logURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return "<invalid>"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func itemKey(item rawItem) string {
	if item.GUID != "" {
		return item.GUID
	}
	sum := sha256.Sum256([]byte(item.Title + "\n" + item.Link + "\n" + item.EnclosureURL))
	return hex.EncodeToString(sum[:])
}

func (s *Service) insertItem(ctx context.Context, item rawItem, result matchResult) (bool, error) {
	rawJSON, _ := json.Marshal(item)
	now := s.now().UTC().Unix()
	bindingStatus := result.BindingStatus
	if bindingStatus == "" {
		bindingStatus = BindingStatusPending
	}
	var boundAt any
	if bindingStatus == BindingStatusBound {
		boundAt = now
	}
	created, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO subscription_items(
    item_key, guid, title, description, link, enclosure_url, torrent_url, content_length,
    pub_date, published_at, match_status, bangumi_id, matched_name, parsed_name,
    season_number, episode_type, episode_number, match_score, match_reason,
    binding_status, bound_bangumi_id, bound_anime_name, bound_season_number,
    bound_episode_type, bound_episode_number, binding_note, bound_at, ignored_at,
    raw_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.Key, item.GUID, item.Title, item.Description, item.Link, item.EnclosureURL, item.TorrentURL, item.ContentLength,
		item.PubDate, item.PublishedAt, result.Status, result.BangumiID, result.MatchedName, result.ParsedName,
		result.SeasonNumber, result.EpisodeType, result.EpisodeNumber, result.Score, result.Reason,
		bindingStatus, result.BoundBangumiID, result.BoundAnimeName, result.BoundSeasonNumber,
		result.BoundEpisodeType, result.BoundEpisodeNumber, result.BindingNote, boundAt, nil,
		string(rawJSON), now, now,
	)
	if err != nil {
		return false, err
	}
	affected, err := created.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Service) rematchUnmatched(ctx context.Context, candidates []candidate, currentKeys []string) (int, error) {
	current := make(map[string]struct{}, len(currentKeys))
	for _, key := range currentKeys {
		current[key] = struct{}{}
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT item_key, title
FROM subscription_items
WHERE match_status = ? AND binding_status = ?
ORDER BY created_at DESC, id DESC`, matchStatusUnmatched, BindingStatusPending)
	if err != nil {
		return 0, err
	}
	type pendingItem struct {
		key   string
		title string
	}
	pending := make([]pendingItem, 0)
	for rows.Next() {
		var item pendingItem
		if err := rows.Scan(&item.key, &item.title); err != nil {
			rows.Close()
			return 0, err
		}
		pending = append(pending, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	matched := 0
	now := s.now().UTC().Unix()
	for _, item := range pending {
		if _, exists := current[item.key]; !exists {
			continue
		}
		result := matchItem(item.title, candidates)
		result, err = s.applyTitleRule(ctx, rawItem{Key: item.key, Title: item.title}, result)
		if err != nil {
			return matched, err
		}
		result, err = s.rejectDuplicateMatch(ctx, item.key, result)
		if err != nil {
			return matched, err
		}
		bindingStatus := result.BindingStatus
		if bindingStatus == "" {
			bindingStatus = BindingStatusPending
		}
		var boundAt any
		if bindingStatus == BindingStatusBound {
			boundAt = now
		}
		if _, err := s.db.ExecContext(ctx, `
UPDATE subscription_items
SET match_status = ?, bangumi_id = ?, matched_name = ?, parsed_name = ?,
    season_number = ?, episode_type = ?, episode_number = ?,
    match_score = ?, match_reason = ?,
    binding_status = ?, bound_bangumi_id = ?, bound_anime_name = ?,
    bound_season_number = ?, bound_episode_type = ?, bound_episode_number = ?,
    binding_note = ?, bound_at = ?, ignored_at = NULL, updated_at = ?
WHERE item_key = ?`, result.Status, result.BangumiID, result.MatchedName, result.ParsedName,
			result.SeasonNumber, result.EpisodeType, result.EpisodeNumber, result.Score, result.Reason,
			bindingStatus, result.BoundBangumiID, result.BoundAnimeName, result.BoundSeasonNumber,
			result.BoundEpisodeType, result.BoundEpisodeNumber, result.BindingNote, boundAt, now, item.key); err != nil {
			return matched, err
		}
		if result.Status == matchStatusMatched {
			matched++
		}
	}
	return matched, nil
}

func (s *Service) rejectDuplicateMatch(ctx context.Context, currentKey string, result matchResult) (matchResult, error) {
	if result.Status != matchStatusMatched || result.BangumiID == nil || result.SeasonNumber == nil || result.EpisodeNumber == "" {
		return result, nil
	}
	query := `
SELECT id, title
FROM subscription_items
WHERE binding_status = ?
  AND bound_bangumi_id = ?
  AND bound_season_number = ?
  AND bound_episode_type = ?
  AND bound_episode_number = ?
  AND item_key != ?
ORDER BY created_at, id
LIMIT 1`
	var existingID int64
	var existingTitle string
	err := s.db.QueryRowContext(ctx, query,
		BindingStatusBound, *result.BangumiID, *result.SeasonNumber,
		result.EpisodeType, result.EpisodeNumber, currentKey,
	).Scan(&existingID, &existingTitle)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}

	matchedName := result.MatchedName
	if matchedName == "" {
		matchedName = result.ParsedName
	}
	result.Status = matchStatusUnmatched
	result.BangumiID = nil
	result.MatchedName = ""
	result.BindingStatus = BindingStatusPending
	result.BoundBangumiID = nil
	result.BoundAnimeName = ""
	result.BoundSeasonNumber = nil
	result.BoundEpisodeType = ""
	result.BoundEpisodeNumber = ""
	result.Reason = fmt.Sprintf(
		"已存在相同番剧季数集数的匹配条目：%s %s（订阅条目 #%d：%s）",
		matchedName, formatParsedEpisode(result), existingID, existingTitle,
	)
	result.BindingNote = "自动绑定失败：" + result.Reason
	return result, nil
}

func (s *Service) applyTitleRule(ctx context.Context, item rawItem, result matchResult) (matchResult, error) {
	if result.EpisodeNumber == "" {
		return result, nil
	}
	key := titleMemoryKey(item.Title)
	if key == "" {
		return result, nil
	}
	var rule struct {
		BangumiID    int64
		AnimeName    string
		SeasonNumber int
		EpisodeType  string
	}
	err := s.db.QueryRowContext(ctx, `
SELECT bangumi_id, anime_name, season_number, episode_type
FROM subscription_title_rules
WHERE title_key = ?`, key).Scan(&rule.BangumiID, &rule.AnimeName, &rule.SeasonNumber, &rule.EpisodeType)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	episodeType := normalizeEpisodeType(rule.EpisodeType)
	bangumiID := rule.BangumiID
	seasonNumber := rule.SeasonNumber
	result.Status = matchStatusMatched
	result.BangumiID = &bangumiID
	result.MatchedName = rule.AnimeName
	result.SeasonNumber = &seasonNumber
	result.EpisodeType = episodeType
	result.Score = 1
	result.Reason = "标题记忆自动匹配"
	result.BindingStatus = BindingStatusBound
	result.BoundBangumiID = &bangumiID
	result.BoundAnimeName = rule.AnimeName
	result.BoundSeasonNumber = &seasonNumber
	result.BoundEpisodeType = episodeType
	result.BoundEpisodeNumber = result.EpisodeNumber
	result.BindingNote = "标题记忆自动绑定"
	return result, nil
}

func formatParsedEpisode(result matchResult) string {
	season := "季数未定"
	if result.SeasonNumber != nil {
		season = fmt.Sprintf("S%02d", *result.SeasonNumber)
	}
	episodeType := result.EpisodeType
	if episodeType == "" || episodeType == "episode" {
		return fmt.Sprintf("%s E%s", season, result.EpisodeNumber)
	}
	return fmt.Sprintf("%s %s %s", season, strings.ToUpper(episodeType), result.EpisodeNumber)
}

func scanItem(row interface{ Scan(dest ...any) error }) (Item, error) {
	var item Item
	var publishedAt, bangumiID, seasonNumber sql.NullInt64
	var boundAt, ignoredAt, boundBangumiID, boundSeasonNumber sql.NullInt64
	if err := row.Scan(
		&item.ID, &item.GUID, &item.Title, &item.Description, &item.Link,
		&item.EnclosureURL, &item.TorrentURL, &item.ContentLength, &item.PubDate,
		&publishedAt, &item.MatchStatus, &bangumiID, &item.MatchedName, &item.ParsedName,
		&seasonNumber, &item.EpisodeType, &item.EpisodeNumber, &item.MatchScore,
		&item.MatchReason, &item.BindingStatus, &boundBangumiID, &item.BoundAnimeName,
		&boundSeasonNumber, &item.BoundEpisodeType, &item.BoundEpisodeNumber,
		&item.BindingNote, &boundAt, &ignoredAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return Item{}, err
	}
	if publishedAt.Valid {
		value := publishedAt.Int64
		item.PublishedAt = &value
	}
	if bangumiID.Valid {
		value := bangumiID.Int64
		item.BangumiID = &value
	}
	if seasonNumber.Valid {
		value := int(seasonNumber.Int64)
		item.SeasonNumber = &value
	}
	if boundBangumiID.Valid {
		value := boundBangumiID.Int64
		item.BoundBangumiID = &value
	}
	if boundSeasonNumber.Valid {
		value := int(boundSeasonNumber.Int64)
		item.BoundSeasonNumber = &value
	}
	if boundAt.Valid {
		value := boundAt.Int64
		item.BoundAt = &value
	}
	if ignoredAt.Valid {
		value := ignoredAt.Int64
		item.IgnoredAt = &value
	}
	return item, nil
}

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	GUID        string       `xml:"guid"`
	Link        string       `xml:"link"`
	Title       string       `xml:"title"`
	Description string       `xml:"description"`
	PubDate     string       `xml:"pubDate"`
	Torrent     rssTorrent   `xml:"torrent"`
	Enclosure   rssEnclosure `xml:"enclosure"`
}

type rssTorrent struct {
	Link          string `xml:"link"`
	ContentLength int64  `xml:"contentLength"`
	PubDate       string `xml:"pubDate"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

func (item rssItem) toRaw() rawItem {
	pubDate := strings.TrimSpace(item.PubDate)
	if pubDate == "" {
		pubDate = strings.TrimSpace(item.Torrent.PubDate)
	}
	publishedAt := parsePubDate(pubDate)
	contentLength := item.Enclosure.Length
	if contentLength == 0 {
		contentLength = item.Torrent.ContentLength
	}
	return rawItem{
		GUID: strings.TrimSpace(item.GUID), Title: strings.TrimSpace(item.Title),
		Description: strings.TrimSpace(item.Description), Link: strings.TrimSpace(item.Link),
		EnclosureURL: strings.TrimSpace(item.Enclosure.URL), TorrentURL: strings.TrimSpace(item.Torrent.Link),
		ContentLength: contentLength, PubDate: pubDate, PublishedAt: publishedAt,
	}
}

func parsePubDate(value string) *int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	layouts := []string{
		time.RFC1123Z, time.RFC1123, time.RFC3339Nano,
		"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			timestamp := parsed.UTC().Unix()
			return &timestamp
		}
	}
	return nil
}

type candidate struct {
	BangumiID int64
	Display   string
	Aliases   []string
	Keys      []string
}

func (s *Service) loadCandidates(ctx context.Context) ([]candidate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT am.bangumi_id, am.name, am.name_cn, COALESCE(aa.alias, '')
FROM anime_metadata am
LEFT JOIN anime_aliases aa ON aa.bangumi_id = am.bangumi_id
WHERE am.deleted_at IS NULL
ORDER BY am.created_at DESC, am.bangumi_id, aa.sort_order, aa.alias`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := make(map[int64]*candidate)
	order := make([]int64, 0)
	for rows.Next() {
		var id int64
		var name, nameCN, alias string
		if err := rows.Scan(&id, &name, &nameCN, &alias); err != nil {
			return nil, err
		}
		entry := byID[id]
		if entry == nil {
			display := nameCN
			if display == "" {
				display = name
			}
			entry = &candidate{BangumiID: id, Display: display}
			entry.addAlias(name)
			entry.addAlias(nameCN)
			byID[id] = entry
			order = append(order, id)
		}
		entry.addAlias(alias)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]candidate, 0, len(order))
	for _, id := range order {
		entry := byID[id]
		entry.Keys = make([]string, 0, len(entry.Aliases))
		seen := make(map[string]struct{})
		for _, alias := range entry.Aliases {
			key := normalize(alias)
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			entry.Keys = append(entry.Keys, key)
		}
		result = append(result, *entry)
	}
	return result, nil
}

func (c *candidate) addAlias(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, alias := range c.Aliases {
		if alias == value {
			return
		}
	}
	c.Aliases = append(c.Aliases, value)
}

func matchItem(title string, candidates []candidate) matchResult {
	parsed := parseTitle(title)
	result := matchResult{
		Status: matchStatusUnmatched, ParsedName: parsed.ParsedName,
		SeasonNumber: parsed.SeasonNumber, EpisodeType: parsed.EpisodeType,
		EpisodeNumber: parsed.EpisodeNumber, Reason: "未找到足够相似的本地番剧",
	}
	if parsed.EpisodeType != "" && parsed.EpisodeNumber == "" {
		result.Reason = "已识别为特殊集，但未解析到集数"
	}
	if parsed.EpisodeNumber == "" {
		if result.Reason == "未找到足够相似的本地番剧" {
			result.Reason = "未解析到集数"
		}
		return result
	}
	bestScore := 0.0
	bestMethod := ""
	var best *candidate
	for _, name := range parsed.Names {
		key := normalize(name)
		if key == "" {
			continue
		}
		for i := range candidates {
			score := candidates[i].score(key)
			if score.Score > bestScore {
				bestScore = score.Score
				bestMethod = score.Method
				best = &candidates[i]
			}
		}
	}
	result.Score = math.Round(bestScore*100) / 100
	if best != nil && bestScore >= minMatchScore {
		bangumiID := best.BangumiID
		result.Status = matchStatusMatched
		result.BangumiID = &bangumiID
		result.MatchedName = best.Display
		if bestMethod == "fuzzy" {
			result.Reason = "模糊匹配成功"
		} else {
			result.Reason = "规则匹配成功"
		}
	}
	return result
}

func titleMemoryKey(title string) string {
	parsed := parseTitle(title)
	keys := make([]string, 0, len(parsed.Names))
	seen := make(map[string]struct{})
	for _, name := range parsed.Names {
		key := normalize(name)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	episodeType := parsed.EpisodeType
	if episodeType == "" {
		episodeType = "episode"
	}
	return strings.Join(keys, "|") + "|type=" + normalizeEpisodeType(episodeType)
}

func (c candidate) score(key string) scoreResult {
	best := scoreResult{}
	for _, candidateKey := range c.Keys {
		score := similarity(key, candidateKey)
		if score.Score > best.Score {
			best = score
		}
	}
	return best
}

type parsedTitle struct {
	Names         []string
	ParsedName    string
	SeasonNumber  *int
	EpisodeType   string
	EpisodeNumber string
}

var (
	sxePattern            = regexp.MustCompile(`(?i)\bS(\d{1,2})\s*E(\d+(?:\.\d+)?)\b`)
	chineseEpisodePattern = regexp.MustCompile(`第\s*(\d+(?:\.\d+)?)\s*[話话集]`)
	specialPattern        = regexp.MustCompile(`(?i)(?:^|[\s\-_])((?:OVA|OAD|SP|SPECIAL))\s*(\d+(?:\.\d+)?)?`)
	trailingEpisode       = regexp.MustCompile(`(?i)(?:^|[\s\-_【\[\(（])(\d+(?:\.\d+)?)(?:v\d+)?(?:[\]】\)）\s]|$)`)
	seasonPattern         = regexp.MustCompile(`(?i)(?:season|第)\s*([0-9一二三四五六七八九十]+)\s*[季期]?`)
	standaloneSPattern    = regexp.MustCompile(`(?i)(?:^|[\s\-_])S(\d{1,2})(?:[\s\-_]|$)`)
	suffixSeasonPattern   = regexp.MustCompile(`\s+([2-9])$`)
	monthMarkerPattern    = regexp.MustCompile(`★\s*\d{1,2}\s*月新番\s*★`)
)

func parseTitle(title string) parsedTitle {
	cleaned := normalizeDecorations(title)
	seasonNumber, episodeType, episodeNumber := parseEpisode(cleaned)
	names := extractNames(cleaned, episodeNumber, episodeType)
	parsedName := ""
	if len(names) > 0 {
		parsedName = names[0]
	}
	if seasonNumber == nil {
		seasonNumber = inferSeasonFromNames(names)
	}
	if seasonNumber == nil {
		season := 1
		seasonNumber = &season
	}
	if episodeType == "" {
		episodeType = "episode"
	}
	return parsedTitle{
		Names: names, ParsedName: parsedName, SeasonNumber: seasonNumber,
		EpisodeType: episodeType, EpisodeNumber: episodeNumber,
	}
}

func normalizeDecorations(title string) string {
	result := strings.TrimSpace(title)
	if strings.HasPrefix(result, "[") || strings.HasPrefix(result, "【") {
		if end := strings.IndexAny(result, "]】"); end >= 0 && end+1 < len(result) {
			openLen := 1
			if strings.HasPrefix(result, "【") {
				openLen = len("【")
			}
			content := result[openLen:end]
			if isLikelyReleaseGroup(content) {
				closeLen := 1
				if strings.HasPrefix(result[end:], "】") {
					closeLen = len("】")
				}
				result = strings.TrimSpace(result[end+closeLen:])
			}
		}
	}
	if end := strings.Index(result, "】"); end >= 0 && end < 24 && !strings.HasPrefix(result, "【") {
		result = strings.TrimSpace(result[end+len("】"):])
	}
	result = monthMarkerPattern.ReplaceAllString(result, " ")
	result = strings.NewReplacer("／", "/", "｜", "/", "＿", "_", "　", " ").Replace(result)
	for _, token := range []string{"[1080P]", "[1080p]", "[720P]", "[2160P]", "[WEB-DL]", "[MP4]", "[MKV]", "[Baha]", "[AAC AVC]", "[HEVC]"} {
		result = strings.ReplaceAll(result, token, " ")
	}
	return compactSpaces(result)
}

func isLikelyReleaseGroup(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "/") || strings.Contains(value, "／") {
		return false
	}
	return len([]rune(value)) <= 16
}

func parseEpisode(title string) (*int, string, string) {
	if matches := sxePattern.FindStringSubmatch(title); len(matches) == 3 {
		season := atoi(matches[1])
		return &season, "episode", matches[2]
	}
	if matches := specialPattern.FindStringSubmatch(title); len(matches) >= 2 {
		episodeType := strings.ToLower(matches[1])
		episodeNumber := "1"
		if len(matches) >= 3 && matches[2] != "" {
			episodeNumber = matches[2]
		}
		return nil, episodeType, episodeNumber
	}
	if matches := chineseEpisodePattern.FindStringSubmatch(title); len(matches) == 2 {
		return nil, "episode", matches[1]
	}
	matches := trailingEpisode.FindAllStringSubmatch(title, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		value := matches[i][1]
		if isEpisodeNumber(value) {
			return nil, "episode", value
		}
	}
	return nil, "episode", ""
}

func extractNames(title, episodeNumber, episodeType string) []string {
	withoutTags := removeKnownBracketTags(title)
	if episodeNumber != "" {
		withoutTags = removeEpisodeSuffix(withoutTags, episodeNumber, episodeType)
	}
	parts := strings.FieldsFunc(withoutTags, func(r rune) bool { return r == '/' || r == '／' })
	names := make([]string, 0, len(parts)*2)
	for _, part := range parts {
		clean := cleanupNamePart(part)
		if clean == "" {
			continue
		}
		names = appendUnique(names, clean)
		if stripped := stripSeasonSuffix(clean); stripped != clean {
			names = appendUnique(names, stripped)
		}
	}
	if len(names) == 0 {
		clean := cleanupNamePart(withoutTags)
		if clean != "" {
			names = appendUnique(names, clean)
		}
	}
	sort.SliceStable(names, func(i, j int) bool { return len([]rune(names[i])) > len([]rune(names[j])) })
	return names
}

func removeKnownBracketTags(title string) string {
	var builder strings.Builder
	for index := 0; index < len(title); {
		open, close := "", ""
		if strings.HasPrefix(title[index:], "[") {
			open, close = "[", "]"
		} else if strings.HasPrefix(title[index:], "【") {
			open, close = "【", "】"
		}
		if open != "" {
			start := index + len(open)
			end := strings.Index(title[start:], close)
			if end >= 0 {
				content := title[start : start+end]
				if isNoiseTag(content) {
					builder.WriteByte(' ')
				} else {
					builder.WriteByte(' ')
					builder.WriteString(content)
					builder.WriteByte(' ')
				}
				index = start + end + len(close)
				continue
			}
		}
		builder.WriteByte(title[index])
		index++
	}
	return builder.String()
}

func isNoiseTag(value string) bool {
	key := strings.ToLower(strings.TrimSpace(value))
	if key == "" {
		return true
	}
	if _, err := strconv.ParseFloat(strings.TrimSuffix(key, "v2"), 64); err == nil {
		return false
	}
	noise := []string{"1080", "720", "2160", "web", "baha", "aac", "avc", "hevc", "x264", "x265", "mp4", "mkv", "简", "繁", "chs", "cht", "字幕"}
	for _, item := range noise {
		if strings.Contains(key, item) {
			return true
		}
	}
	return false
}

func removeEpisodeSuffix(title, episodeNumber, episodeType string) string {
	patterns := []string{
		fmt.Sprintf(`第\s*%s\s*[話话集].*$`, regexp.QuoteMeta(episodeNumber)),
		fmt.Sprintf(`(?i)\s*[-_]\s*%s\s*$`, regexp.QuoteMeta(episodeNumber)),
		fmt.Sprintf(`(?i)\s*[-_]\s*%s\s*%s\s*$`, regexp.QuoteMeta(episodeType), regexp.QuoteMeta(episodeNumber)),
		fmt.Sprintf(`(?i)\s*\[\s*%s\s*\]\s*$`, regexp.QuoteMeta(episodeNumber)),
		fmt.Sprintf(`(?i)\s+%s\s*$`, regexp.QuoteMeta(episodeNumber)),
	}
	result := title
	for _, pattern := range patterns {
		result = regexp.MustCompile(pattern).ReplaceAllString(result, "")
	}
	return result
}

func cleanupNamePart(value string) string {
	value = strings.TrimSpace(value)
	value = regexp.MustCompile(`第\s*\d+(?:\.\d+)?\s*[話话集].*$`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`(?i)\s*[-_]\s*(OVA|OAD|SP|SPECIAL)?\s*\d*(?:\.\d+)?\s*$`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`(?i)\s*\b(1080p|720p|2160p|web-dl|mp4|mkv|aac|avc|hevc|baha|gb_cn|gb|big5|av1|opus|flac|ass|chs|cht)\b.*$`).ReplaceAllString(value, "")
	value = strings.Trim(value, " -_[]【】()（）")
	return compactSpaces(value)
}

func stripSeasonSuffix(value string) string {
	if matches := suffixSeasonPattern.FindStringSubmatch(value); len(matches) == 2 {
		return strings.TrimSpace(strings.TrimSuffix(value, matches[1]))
	}
	return value
}

func inferSeasonFromNames(names []string) *int {
	for _, name := range names {
		if matches := seasonPattern.FindStringSubmatch(name); len(matches) == 2 {
			season := parseSeasonNumber(matches[1])
			if season > 0 {
				return &season
			}
		}
		if matches := standaloneSPattern.FindStringSubmatch(name); len(matches) == 2 {
			season := atoi(matches[1])
			if season > 0 {
				return &season
			}
		}
		if matches := suffixSeasonPattern.FindStringSubmatch(name); len(matches) == 2 {
			season := atoi(matches[1])
			return &season
		}
	}
	return nil
}

func parseSeasonNumber(value string) int {
	if parsed, err := strconv.Atoi(value); err == nil {
		return parsed
	}
	numbers := map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	total := 0
	for _, r := range value {
		if r == '十' {
			if total == 0 {
				total = 10
			} else {
				total *= 10
			}
			continue
		}
		total += numbers[r]
	}
	return total
}

func isEpisodeNumber(value string) bool {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return false
	}
	if number == 720 || number == 1080 || number == 2160 {
		return false
	}
	return number >= 0 && number <= 200
}

func atoi(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func compactSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'Ａ' && r <= 'Ｚ':
			r = r - 'Ａ' + 'a'
		case r >= 'ａ' && r <= 'ｚ':
			r = r - 'ａ' + 'a'
		case r >= '０' && r <= '９':
			r = r - '０' + '0'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func similarity(a, b string) scoreResult {
	if a == "" || b == "" {
		return scoreResult{}
	}
	if a == b {
		return scoreResult{Score: 1, Method: "exact"}
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter := math.Min(float64(len([]rune(a))), float64(len([]rune(b))))
		longer := math.Max(float64(len([]rune(a))), float64(len([]rune(b))))
		return scoreResult{Score: 0.86 + 0.14*(shorter/longer), Method: "contains"}
	}
	best := scoreResult{Score: dice(a, b), Method: "dice"}
	if fuzzy := fuzzySimilarity(a, b); fuzzy.Score > best.Score {
		best = fuzzy
	}
	return best
}

func fuzzySimilarity(a, b string) scoreResult {
	aRunes := []rune(a)
	bRunes := []rune(b)
	shorter := min(len(aRunes), len(bRunes))
	if shorter < 5 {
		return scoreResult{}
	}
	coverage := sharedRuneCoverage(aRunes, bRunes)
	lcsCoverage := float64(lcsLength(aRunes, bRunes)) / float64(shorter)
	best := 0.0
	if coverage >= 0.72 {
		best = math.Max(best, 0.62+0.16*coverage)
	}
	if lcsCoverage >= 0.68 {
		best = math.Max(best, 0.61+0.17*lcsCoverage)
	}
	if anchorScore := sharedAnchorScore(aRunes, bRunes); anchorScore > best {
		best = anchorScore
	}
	if best == 0 {
		return scoreResult{}
	}
	if best > 0.78 {
		best = 0.78
	}
	return scoreResult{Score: best, Method: "fuzzy"}
}

func sharedRuneCoverage(a, b []rune) float64 {
	counts := make(map[rune]int, len(a))
	for _, r := range a {
		counts[r]++
	}
	shared := 0
	for _, r := range b {
		if counts[r] > 0 {
			shared++
			counts[r]--
		}
	}
	return float64(shared) / float64(min(len(a), len(b)))
}

func lcsLength(a, b []rune) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				current[j] = previous[j-1] + 1
			} else if previous[j] > current[j-1] {
				current[j] = previous[j]
			} else {
				current[j] = current[j-1]
			}
		}
		previous, current = current, previous
		clear(current)
	}
	return previous[len(b)]
}

func sharedAnchorScore(a, b []rune) float64 {
	aAnchors := anchors(a)
	bAnchors := anchors(b)
	shared := 0
	hasNonSeasonAnchor := false
	for anchor := range aAnchors {
		if _, exists := bAnchors[anchor]; !exists {
			continue
		}
		shared++
		if !isSeasonAnchor(anchor) {
			hasNonSeasonAnchor = true
		}
	}
	if shared < 6 || !hasNonSeasonAnchor {
		return 0
	}
	return 0.68 + math.Min(0.08, float64(shared)*0.01)
}

func anchors(runes []rune) map[string]struct{} {
	result := make(map[string]struct{})
	for _, size := range []int{2, 3} {
		if len(runes) < size {
			continue
		}
		for i := 0; i <= len(runes)-size; i++ {
			if hasDigit(runes[i : i+size]) {
				continue
			}
			result[string(runes[i:i+size])] = struct{}{}
		}
	}
	return result
}

func hasDigit(runes []rune) bool {
	for _, r := range runes {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func isSeasonAnchor(anchor string) bool {
	return strings.Contains(anchor, "第") || strings.Contains(anchor, "季")
}

func dice(a, b string) float64 {
	aRunes := []rune(a)
	bRunes := []rune(b)
	if len(aRunes) < 2 || len(bRunes) < 2 {
		if a == b {
			return 1
		}
		return 0
	}
	aGrams := grams(aRunes)
	bGrams := grams(bRunes)
	intersection := 0
	for gram, aCount := range aGrams {
		if bCount := bGrams[gram]; bCount > 0 {
			intersection += min(aCount, bCount)
		}
	}
	return float64(2*intersection) / float64(len(aRunes)-1+len(bRunes)-1)
}

func grams(runes []rune) map[string]int {
	result := make(map[string]int, len(runes)-1)
	for i := 0; i < len(runes)-1; i++ {
		result[string(runes[i:i+2])]++
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
