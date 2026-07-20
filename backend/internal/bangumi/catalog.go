package bangumi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"bangumipipeline.local/server/internal/database"
)

var (
	ErrAnimeNotFound      = errors.New("anime not found")
	ErrAnimeAlreadyExists = errors.New("anime already exists")
	ErrInvalidSubjectType = errors.New("invalid subject type")
	ErrInvalidSearchTags  = errors.New("invalid bangumi search tags")
)

const (
	opSkipStatusNoMedia     = "no_media"
	opSkipStatusPending     = "pending"
	opSkipStatusUnsupported = "unsupported"
)

type Catalog struct {
	db               database.Executor
	mediaDir         string
	commentAvatarDir string
}

func NewCatalog(db database.Executor, mediaDir ...string) *Catalog {
	root := "./data/bangumi"
	if len(mediaDir) > 0 && strings.TrimSpace(mediaDir[0]) != "" {
		root = strings.TrimSpace(mediaDir[0])
	}
	return NewCatalogWithConfig(db, CatalogConfig{MediaDir: root})
}

type CatalogConfig struct {
	MediaDir         string
	CommentAvatarDir string
}

func NewCatalogWithConfig(db database.Executor, config CatalogConfig) *Catalog {
	mediaDir := cleanAbsoluteDirectory(config.MediaDir, "./data/bangumi")
	avatarDir := cleanAbsoluteDirectory(config.CommentAvatarDir, "./data/images/bangumi/avatar")
	return &Catalog{db: db, mediaDir: mediaDir, commentAvatarDir: avatarDir}
}

func cleanAbsoluteDirectory(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if abs, err := filepath.Abs(value); err == nil {
		value = abs
	}
	return filepath.Clean(value)
}

type AnimeListItem struct {
	BangumiID                 int64                 `json:"bangumiId"`
	Name                      string                `json:"name"`
	NameCN                    string                `json:"nameCN"`
	AirDate                   string                `json:"airDate"`
	AirWeekday                int                   `json:"airWeekday"`
	Episodes                  int                   `json:"episodes"`
	Platform                  string                `json:"platform"`
	ImageStatus               string                `json:"imageStatus"`
	HasCover                  bool                  `json:"hasCover"`
	DetailStatus              string                `json:"detailStatus"`
	StorageRoot               string                `json:"storageRoot"`
	StoragePath               string                `json:"storagePath"`
	SubscriptionEpisodeOffset int                   `json:"subscriptionEpisodeOffset"`
	MatchedEpisodes           []AnimeMatchedEpisode `json:"matchedEpisodes"`
	CreatedAt                 int64                 `json:"createdAt"`
}

type AnimePage struct {
	Items    []AnimeListItem `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

type AnimeSettings struct {
	BangumiID                 int64 `json:"bangumiId"`
	SubscriptionEpisodeOffset int   `json:"subscriptionEpisodeOffset"`
}

type AnimeListSort string

const (
	AnimeListSortCreated AnimeListSort = "created"
	AnimeListSortAirDate AnimeListSort = "airDate"
)

type ListOptions struct {
	Query string
	Sort  string
}

type AnimeMatchedEpisode struct {
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeType   string `json:"episodeType"`
	EpisodeNumber string `json:"episodeNumber"`
	Status        string `json:"status"`
}

type AnimeSearchItem struct {
	BangumiID int64  `json:"bangumiId"`
	Name      string `json:"name"`
	NameCN    string `json:"nameCN"`
}

type ViewerHome struct {
	HotRecommendations []ViewerAnimeCard `json:"hotRecommendations"`
	RecentUpdates      []ViewerAnimeCard `json:"recentUpdates"`
}

type ViewerAnimeCard struct {
	BangumiID          int64    `json:"bangumiId"`
	Name               string   `json:"name"`
	NameCN             string   `json:"nameCN"`
	Title              string   `json:"title"`
	AirDate            string   `json:"airDate"`
	HasCover           bool     `json:"hasCover"`
	ImageStatus        string   `json:"imageStatus"`
	RatingScore        *float64 `json:"ratingScore"`
	LatestEpisode      string   `json:"latestEpisode"`
	LatestEpisodeLabel string   `json:"latestEpisodeLabel"`
	LatestEpisodeTitle string   `json:"latestEpisodeTitle"`
	UpdatedAt          *int64   `json:"updatedAt"`
}

type viewerAnimeAggregate struct {
	card            ViewerAnimeCard
	progressEpisode viewerEpisodeRef
	recentEpisode   viewerEpisodeRef
	hasProgress     bool
	hasRecent       bool
}

type viewerEpisodeRef struct {
	season        int
	episodeType   string
	episodeNumber string
	title         string
	updatedAt     int64
}

type AnimeTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type AnimeActor struct {
	ActorID     int64    `json:"actorId"`
	Name        string   `json:"name"`
	Summary     string   `json:"summary"`
	Career      []string `json:"career"`
	HasImage    bool     `json:"hasImage"`
	ImageStatus string   `json:"imageStatus"`
}

type AnimeCharacter struct {
	CharacterID int64        `json:"characterId"`
	Name        string       `json:"name"`
	Summary     string       `json:"summary"`
	Relation    string       `json:"relation"`
	HasImage    bool         `json:"hasImage"`
	ImageStatus string       `json:"imageStatus"`
	Actors      []AnimeActor `json:"actors"`
}

type AnimeEpisode struct {
	EpisodeID       int64   `json:"episodeId"`
	EpNumber        int     `json:"epNumber"`
	SortNumber      float64 `json:"sortNumber"`
	Type            int     `json:"type"`
	Disc            int     `json:"disc"`
	Airdate         string  `json:"airdate"`
	Name            string  `json:"name"`
	NameCN          string  `json:"nameCN"`
	Duration        string  `json:"duration"`
	DurationSeconds int     `json:"durationSeconds"`
	Description     string  `json:"description"`
	CommentCount    int     `json:"commentCount"`
	OPSkip          OPSkip  `json:"opSkip"`
}

type OPSkip struct {
	MediaID      int64   `json:"mediaId"`
	Status       string  `json:"status"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
	ErrorMessage string  `json:"errorMessage"`
	UpdatedAt    *int64  `json:"updatedAt"`
}

type AnimeDetail struct {
	BangumiID      int64            `json:"bangumiId"`
	URL            string           `json:"url"`
	Name           string           `json:"name"`
	NameCN         string           `json:"nameCN"`
	AirDate        string           `json:"airDate"`
	AirWeekday     int              `json:"airWeekday"`
	DetailDate     string           `json:"detailDate"`
	Platform       string           `json:"platform"`
	Summary        string           `json:"summary"`
	Eps            int              `json:"eps"`
	TotalEpisodes  int              `json:"totalEpisodes"`
	Volumes        int              `json:"volumes"`
	Series         bool             `json:"series"`
	Locked         bool             `json:"locked"`
	NSFW           bool             `json:"nsfw"`
	HasCover       bool             `json:"hasCover"`
	ImageStatus    string           `json:"imageStatus"`
	DetailStatus   string           `json:"detailStatus"`
	CharacterState string           `json:"characterStatus"`
	EpisodesStatus string           `json:"episodesStatus"`
	StorageRoot    string           `json:"storageRoot"`
	StoragePath    string           `json:"storagePath"`
	Infobox        []map[string]any `json:"infobox"`
	Rating         map[string]any   `json:"rating"`
	Collection     map[string]any   `json:"collection"`
	MetaTags       []string         `json:"metaTags"`
	Tags           []AnimeTag       `json:"tags"`
	Aliases        []string         `json:"aliases"`
	Characters     []AnimeCharacter `json:"characters"`
	Episodes       []AnimeEpisode   `json:"episodes"`
	CreatedAt      int64            `json:"createdAt"`
}

func (c *Catalog) List(ctx context.Context, page, pageSize int, options ...ListOptions) (AnimePage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 24
	}
	option := ListOptions{Sort: string(AnimeListSortCreated)}
	if len(options) > 0 {
		option = options[0]
	}
	where, args := animeListWhere(option.Query)
	result := AnimePage{Items: make([]AnimeListItem, 0), Page: page, PageSize: pageSize}
	if err := c.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM anime_metadata am
WHERE %s`, where), args...).Scan(&result.Total); err != nil {
		return result, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := c.db.QueryContext(ctx, `
SELECT bangumi_id, name, name_cn, air_date, air_weekday,
       CASE WHEN total_episodes > 0 THEN total_episodes ELSE eps END,
       platform, image_status, image_local_path != '', detail_status, media_storage_root,
       subscription_episode_offset, created_at
FROM anime_metadata am
WHERE `+where+`
ORDER BY `+animeListOrderBy(option.Sort)+`
LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item AnimeListItem
		if err := rows.Scan(
			&item.BangumiID, &item.Name, &item.NameCN, &item.AirDate, &item.AirWeekday,
			&item.Episodes, &item.Platform, &item.ImageStatus, &item.HasCover,
			&item.DetailStatus, &item.StorageRoot, &item.SubscriptionEpisodeOffset, &item.CreatedAt,
		); err != nil {
			rows.Close()
			return result, err
		}
		item.StorageRoot = c.storageRoot(item.StorageRoot)
		item.StoragePath = animeStoragePath(item.StorageRoot, item.NameCN, item.Name)
		item.MatchedEpisodes = make([]AnimeMatchedEpisode, 0)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	if err := c.attachMatchedEpisodes(ctx, result.Items); err != nil {
		return result, err
	}
	return result, nil
}

func animeListWhere(query string) (string, []any) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "am.deleted_at IS NULL", nil
	}
	like := "%" + query + "%"
	return `am.deleted_at IS NULL
  AND (
    am.name LIKE ?
    OR am.name_cn LIKE ?
    OR EXISTS (
      SELECT 1 FROM anime_aliases aa
      WHERE aa.bangumi_id = am.bangumi_id AND aa.alias LIKE ?
    )
  )`, []any{like, like, like}
}

func animeListOrderBy(sort string) string {
	switch AnimeListSort(sort) {
	case AnimeListSortAirDate:
		return "CASE WHEN am.air_date = '' THEN 1 ELSE 0 END, am.air_date DESC, am.created_at DESC, am.id DESC"
	default:
		return "am.created_at DESC, am.id DESC"
	}
}

func (c *Catalog) attachMatchedEpisodes(ctx context.Context, items []AnimeListItem) error {
	if len(items) == 0 {
		return nil
	}
	indexByID := make(map[int64]int, len(items))
	args := make([]any, 0, len(items))
	placeholders := make([]string, 0, len(items))
	for index, item := range items {
		indexByID[item.BangumiID] = index
		args = append(args, item.BangumiID)
		placeholders = append(placeholders, "?")
	}

	rows, err := c.db.QueryContext(ctx, fmt.Sprintf(`
SELECT si.bound_bangumi_id, si.bound_season_number,
       COALESCE(NULLIF(si.bound_episode_type, ''), 'episode') AS bound_episode_type,
       si.bound_episode_number,
       MAX(CASE WHEN mj.status = 'completed' THEN 1 ELSE 0 END) AS media_completed
FROM subscription_items si
LEFT JOIN download_jobs dj ON dj.subscription_item_id = si.id
LEFT JOIN media_jobs mj ON mj.download_job_id = dj.id
WHERE si.binding_status = 'bound'
  AND si.bound_bangumi_id IN (%s)
  AND si.bound_season_number IS NOT NULL
  AND si.bound_episode_number != ''
GROUP BY si.bound_bangumi_id, si.bound_season_number,
         COALESCE(NULLIF(si.bound_episode_type, ''), 'episode'),
         si.bound_episode_number
ORDER BY bound_bangumi_id, bound_season_number,
         CASE WHEN bound_episode_type = 'episode' THEN 0 ELSE 1 END,
         CASE WHEN bound_episode_number GLOB '[0-9]*' THEN 0 ELSE 1 END,
         CAST(bound_episode_number AS REAL),
         bound_episode_number`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var bangumiID int64
		var episode AnimeMatchedEpisode
		var mediaCompleted int
		if err := rows.Scan(&bangumiID, &episode.SeasonNumber, &episode.EpisodeType, &episode.EpisodeNumber, &mediaCompleted); err != nil {
			return err
		}
		episode.Status = "matched"
		if mediaCompleted > 0 {
			episode.Status = "completed"
		}
		if index, ok := indexByID[bangumiID]; ok {
			items[index].MatchedEpisodes = append(items[index].MatchedEpisodes, episode)
		}
	}
	return rows.Err()
}

func (c *Catalog) Search(ctx context.Context, query string, limit int) ([]AnimeSearchItem, error) {
	query = strings.TrimSpace(query)
	if limit < 1 || limit > 1000 {
		limit = 500
	}
	where, args := animeListWhere(query)
	args = append(args, limit)
	rows, err := c.db.QueryContext(ctx, fmt.Sprintf(`
SELECT am.bangumi_id, am.name, am.name_cn
FROM anime_metadata am
WHERE %s
ORDER BY am.created_at DESC, am.id DESC
LIMIT ?`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AnimeSearchItem, 0)
	for rows.Next() {
		var item AnimeSearchItem
		if err := rows.Scan(&item.BangumiID, &item.Name, &item.NameCN); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (c *Catalog) ViewerHome(ctx context.Context) (ViewerHome, error) {
	const hotLimit = 32
	const recentLimit = 24

	aggregates, err := c.viewerAnimeAggregates(ctx)
	if err != nil {
		return ViewerHome{}, err
	}

	now := time.Now().UTC()
	cutoff := now.AddDate(0, -6, 0)
	hot := make([]ViewerAnimeCard, 0, len(aggregates))
	recent := make([]ViewerAnimeCard, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if aggregate.hasProgress && airDateWithinRange(aggregate.card.AirDate, cutoff, now) {
			card := aggregate.card
			card.LatestEpisode = aggregate.progressEpisode.episodeNumber
			card.LatestEpisodeLabel = viewerEpisodeLabel(aggregate.progressEpisode)
			card.LatestEpisodeTitle = aggregate.progressEpisode.title
			if aggregate.hasRecent {
				card.UpdatedAt = ptrInt64(aggregate.recentEpisode.updatedAt)
			}
			hot = append(hot, card)
		}
		if aggregate.hasRecent {
			card := aggregate.card
			card.LatestEpisode = aggregate.recentEpisode.episodeNumber
			card.LatestEpisodeLabel = viewerEpisodeLabel(aggregate.recentEpisode)
			card.LatestEpisodeTitle = aggregate.recentEpisode.title
			card.UpdatedAt = ptrInt64(aggregate.recentEpisode.updatedAt)
			recent = append(recent, card)
		}
	}

	sort.SliceStable(hot, func(i, j int) bool {
		left := hot[i]
		right := hot[j]
		switch {
		case left.RatingScore != nil && right.RatingScore != nil && *left.RatingScore != *right.RatingScore:
			return *left.RatingScore > *right.RatingScore
		case left.RatingScore != nil && right.RatingScore == nil:
			return true
		case left.RatingScore == nil && right.RatingScore != nil:
			return false
		case nullableInt64Value(left.UpdatedAt) != nullableInt64Value(right.UpdatedAt):
			return nullableInt64Value(left.UpdatedAt) > nullableInt64Value(right.UpdatedAt)
		case left.AirDate != right.AirDate:
			return left.AirDate > right.AirDate
		default:
			return left.BangumiID > right.BangumiID
		}
	})
	if len(hot) > hotLimit {
		hot = hot[:hotLimit]
	}

	sort.SliceStable(recent, func(i, j int) bool {
		left := recent[i]
		right := recent[j]
		if nullableInt64Value(left.UpdatedAt) != nullableInt64Value(right.UpdatedAt) {
			return nullableInt64Value(left.UpdatedAt) > nullableInt64Value(right.UpdatedAt)
		}
		return left.BangumiID > right.BangumiID
	})
	if len(recent) > recentLimit {
		recent = recent[:recentLimit]
	}

	return ViewerHome{HotRecommendations: hot, RecentUpdates: recent}, nil
}

func (c *Catalog) viewerAnimeAggregates(ctx context.Context) ([]viewerAnimeAggregate, error) {
	rows, err := c.db.QueryContext(ctx, `
SELECT am.bangumi_id, am.name, am.name_cn, am.air_date,
       am.image_local_path != '', am.image_status, am.rating_json,
       mj.season_number,
       COALESCE(NULLIF(mj.episode_type, ''), 'episode') AS episode_type,
       mj.episode_number,
       COALESCE((
           SELECT COALESCE(NULLIF(ae.name_cn, ''), ae.name)
           FROM anime_episodes ae
           WHERE ae.bangumi_id = mj.bangumi_id
             AND ae.sort_number = CAST(mj.episode_number AS REAL)
             AND (
                 (LOWER(COALESCE(NULLIF(mj.episode_type, ''), 'episode')) = 'episode' AND ae.type = 0)
                 OR
                 (LOWER(COALESCE(NULLIF(mj.episode_type, ''), 'episode')) != 'episode' AND ae.type != 0)
             )
           ORDER BY ae.type, ae.episode_id
           LIMIT 1
       ), '') AS episode_title,
       COALESCE(mj.completed_at, mj.updated_at, mj.created_at, 0) AS media_updated_at
FROM media_jobs mj
JOIN anime_metadata am ON am.bangumi_id = mj.bangumi_id
WHERE am.deleted_at IS NULL
  AND mj.status = 'completed'
  AND mj.output_path != ''
ORDER BY am.bangumi_id, media_updated_at DESC, mj.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexByID := make(map[int64]int)
	aggregates := make([]viewerAnimeAggregate, 0)
	for rows.Next() {
		var card ViewerAnimeCard
		var ratingJSON string
		var episode viewerEpisodeRef
		if err := rows.Scan(
			&card.BangumiID, &card.Name, &card.NameCN, &card.AirDate,
			&card.HasCover, &card.ImageStatus, &ratingJSON,
			&episode.season, &episode.episodeType, &episode.episodeNumber, &episode.title, &episode.updatedAt,
		); err != nil {
			return nil, err
		}
		index, ok := indexByID[card.BangumiID]
		if !ok {
			card.Title = displayAnimeTitle(card.NameCN, card.Name)
			card.RatingScore = ratingScore(ratingJSON)
			aggregates = append(aggregates, viewerAnimeAggregate{card: card})
			index = len(aggregates) - 1
			indexByID[card.BangumiID] = index
		}
		aggregate := &aggregates[index]
		if !aggregate.hasProgress || viewerEpisodeProgressLess(aggregate.progressEpisode, episode) {
			aggregate.progressEpisode = episode
			aggregate.hasProgress = true
		}
		if !aggregate.hasRecent || aggregate.recentEpisode.updatedAt < episode.updatedAt {
			aggregate.recentEpisode = episode
			aggregate.hasRecent = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return aggregates, nil
}

func (c *Catalog) Detail(ctx context.Context, bangumiID int64) (AnimeDetail, error) {
	var detail AnimeDetail
	var infoboxJSON, ratingJSON, collectionJSON, metaTagsJSON string
	err := c.db.QueryRowContext(ctx, `
SELECT bangumi_id, url, name, name_cn, air_date, air_weekday, detail_date,
       platform, COALESCE(NULLIF(summary_cn, ''), summary), eps, total_episodes, volumes, series, locked, nsfw,
       image_local_path != '', image_status, detail_status, characters_status, episodes_status,
       media_storage_root, infobox_json, rating_json, collection_json, meta_tags_json, created_at
FROM anime_metadata WHERE bangumi_id = ? AND deleted_at IS NULL`, bangumiID).Scan(
		&detail.BangumiID, &detail.URL, &detail.Name, &detail.NameCN, &detail.AirDate,
		&detail.AirWeekday, &detail.DetailDate, &detail.Platform, &detail.Summary,
		&detail.Eps, &detail.TotalEpisodes, &detail.Volumes, &detail.Series,
		&detail.Locked, &detail.NSFW, &detail.HasCover, &detail.ImageStatus,
		&detail.DetailStatus, &detail.CharacterState, &detail.EpisodesStatus,
		&detail.StorageRoot, &infoboxJSON, &ratingJSON, &collectionJSON, &metaTagsJSON, &detail.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AnimeDetail{}, ErrAnimeNotFound
	}
	if err != nil {
		return AnimeDetail{}, err
	}
	detail.Infobox = make([]map[string]any, 0)
	detail.Rating = make(map[string]any)
	detail.Collection = make(map[string]any)
	detail.MetaTags = make([]string, 0)
	_ = json.Unmarshal([]byte(infoboxJSON), &detail.Infobox)
	_ = json.Unmarshal([]byte(ratingJSON), &detail.Rating)
	_ = json.Unmarshal([]byte(collectionJSON), &detail.Collection)
	_ = json.Unmarshal([]byte(metaTagsJSON), &detail.MetaTags)
	detail.StorageRoot = c.storageRoot(detail.StorageRoot)
	detail.StoragePath = animeStoragePath(detail.StorageRoot, detail.NameCN, detail.Name)

	if detail.Tags, err = c.tags(ctx, bangumiID); err != nil {
		return AnimeDetail{}, err
	}
	if detail.Aliases, err = c.aliases(ctx, bangumiID); err != nil {
		return AnimeDetail{}, err
	}
	if detail.Characters, err = c.characters(ctx, bangumiID); err != nil {
		return AnimeDetail{}, err
	}
	if detail.Episodes, err = c.episodes(ctx, bangumiID); err != nil {
		return AnimeDetail{}, err
	}
	if err := c.attachEpisodeOPSkips(ctx, bangumiID, detail.Episodes); err != nil {
		return AnimeDetail{}, err
	}
	return detail, nil
}

func (c *Catalog) Delete(ctx context.Context, bangumiID int64) error {
	result, err := c.db.ExecContext(ctx, `
UPDATE anime_metadata
SET deleted_at = unixepoch()
WHERE bangumi_id = ? AND deleted_at IS NULL`, bangumiID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrAnimeNotFound
	}
	return nil
}

func (c *Catalog) UpdateSettings(ctx context.Context, bangumiID int64, settings AnimeSettings) (AnimeSettings, error) {
	if bangumiID < 1 {
		return AnimeSettings{}, ErrAnimeNotFound
	}
	result, err := c.db.ExecContext(ctx, `
UPDATE anime_metadata
SET subscription_episode_offset = ?
WHERE bangumi_id = ? AND deleted_at IS NULL`, settings.SubscriptionEpisodeOffset, bangumiID)
	if err != nil {
		return AnimeSettings{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return AnimeSettings{}, err
	}
	if affected == 0 {
		return AnimeSettings{}, ErrAnimeNotFound
	}
	return AnimeSettings{
		BangumiID:                 bangumiID,
		SubscriptionEpisodeOffset: settings.SubscriptionEpisodeOffset,
	}, nil
}

func (c *Catalog) tags(ctx context.Context, bangumiID int64) ([]AnimeTag, error) {
	rows, err := c.db.QueryContext(ctx, `
SELECT name, count FROM anime_tags WHERE bangumi_id = ? ORDER BY count DESC, name LIMIT 30`, bangumiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AnimeTag, 0)
	for rows.Next() {
		var tag AnimeTag
		if err := rows.Scan(&tag.Name, &tag.Count); err != nil {
			return nil, err
		}
		result = append(result, tag)
	}
	return result, rows.Err()
}

func (c *Catalog) aliases(ctx context.Context, bangumiID int64) ([]string, error) {
	rows, err := c.db.QueryContext(ctx, `
SELECT alias FROM anime_aliases WHERE bangumi_id = ? ORDER BY sort_order, alias`, bangumiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		result = append(result, alias)
	}
	return result, rows.Err()
}

func (c *Catalog) characters(ctx context.Context, bangumiID int64) ([]AnimeCharacter, error) {
	rows, err := c.db.QueryContext(ctx, `
SELECT character_id, name, summary, relation, image_local_path != '', image_status
FROM (
    SELECT character_id, name, COALESCE(NULLIF(summary_cn, ''), summary) AS summary,
           relation, image_local_path, image_status, id
    FROM anime_characters
    WHERE bangumi_id = ?
) ORDER BY id LIMIT 10`, bangumiID)
	if err != nil {
		return nil, err
	}
	characters := make([]AnimeCharacter, 0, 10)
	for rows.Next() {
		var character AnimeCharacter
		if err := rows.Scan(&character.CharacterID, &character.Name, &character.Summary,
			&character.Relation, &character.HasImage, &character.ImageStatus); err != nil {
			rows.Close()
			return nil, err
		}
		character.Actors = make([]AnimeActor, 0)
		characters = append(characters, character)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range characters {
		actorRows, err := c.db.QueryContext(ctx, `
SELECT a.actor_id, a.name, COALESCE(NULLIF(a.short_summary_cn, ''), a.short_summary), a.career_json,
       a.image_local_path != '', a.image_status
FROM character_actors ca
JOIN actors a ON a.actor_id = ca.actor_id
WHERE ca.bangumi_id = ? AND ca.character_id = ?
ORDER BY ca.sort_order, a.actor_id`, bangumiID, characters[index].CharacterID)
		if err != nil {
			return nil, err
		}
		for actorRows.Next() {
			var actor AnimeActor
			var careerJSON string
			if err := actorRows.Scan(&actor.ActorID, &actor.Name, &actor.Summary, &careerJSON, &actor.HasImage, &actor.ImageStatus); err != nil {
				actorRows.Close()
				return nil, err
			}
			_ = json.Unmarshal([]byte(careerJSON), &actor.Career)
			characters[index].Actors = append(characters[index].Actors, actor)
		}
		if err := actorRows.Close(); err != nil {
			return nil, err
		}
	}
	return characters, nil
}

func (c *Catalog) episodes(ctx context.Context, bangumiID int64) ([]AnimeEpisode, error) {
	rows, err := c.db.QueryContext(ctx, `
SELECT episode_id, ep_number, sort_number, type, disc, airdate, name, name_cn,
       duration, duration_seconds, COALESCE(NULLIF(description_cn, ''), description), comment_count
FROM anime_episodes
WHERE bangumi_id = ?
ORDER BY sort_number,
         CASE WHEN type = 0 THEN 0 ELSE 1 END,
         episode_id`, bangumiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AnimeEpisode, 0)
	for rows.Next() {
		var episode AnimeEpisode
		if err := rows.Scan(
			&episode.EpisodeID, &episode.EpNumber, &episode.SortNumber, &episode.Type,
			&episode.Disc, &episode.Airdate, &episode.Name, &episode.NameCN,
			&episode.Duration, &episode.DurationSeconds, &episode.Description,
			&episode.CommentCount,
		); err != nil {
			return nil, err
		}
		result = append(result, episode)
	}
	return result, rows.Err()
}

func (c *Catalog) attachEpisodeOPSkips(ctx context.Context, bangumiID int64, episodes []AnimeEpisode) error {
	if len(episodes) == 0 {
		return nil
	}
	indexByNumber := make(map[string]int, len(episodes))
	for index := range episodes {
		if episodes[index].Type != 0 {
			episodes[index].OPSkip = OPSkip{Status: opSkipStatusUnsupported}
			continue
		}
		episodes[index].OPSkip = OPSkip{Status: opSkipStatusNoMedia}
		indexByNumber[animeEpisodeMediaNumber(episodes[index])] = index
	}
	if len(indexByNumber) == 0 {
		return nil
	}

	rows, err := c.db.QueryContext(ctx, `
SELECT mj.id, mj.episode_number,
       mos.status, mos.start_seconds, mos.end_seconds, mos.error_message, mos.updated_at
FROM media_jobs mj
LEFT JOIN media_op_segments mos ON mos.media_job_id = mj.id
WHERE mj.bangumi_id = ?
  AND mj.status = 'completed'
  AND mj.output_path != ''
  AND LOWER(COALESCE(NULLIF(mj.episode_type, ''), 'episode')) = 'episode'
ORDER BY COALESCE(mj.completed_at, mj.updated_at, mj.created_at, 0) DESC, mj.id DESC`, bangumiID)
	if err != nil {
		return err
	}
	defer rows.Close()

	attached := make(map[string]struct{}, len(indexByNumber))
	for rows.Next() {
		var mediaID int64
		var episodeNumber string
		var status, errorMessage sql.NullString
		var startSeconds, endSeconds sql.NullFloat64
		var updatedAt sql.NullInt64
		if err := rows.Scan(&mediaID, &episodeNumber, &status, &startSeconds, &endSeconds, &errorMessage, &updatedAt); err != nil {
			return err
		}
		key := normalizeEpisodeNumber(episodeNumber)
		index, ok := indexByNumber[key]
		if !ok {
			continue
		}
		if _, exists := attached[key]; exists {
			continue
		}
		opSkip := OPSkip{
			MediaID: mediaID,
			Status:  opSkipStatusPending,
		}
		if status.Valid && strings.TrimSpace(status.String) != "" {
			opSkip.Status = status.String
		}
		if startSeconds.Valid {
			opSkip.StartSeconds = startSeconds.Float64
		}
		if endSeconds.Valid {
			opSkip.EndSeconds = endSeconds.Float64
		}
		if errorMessage.Valid {
			opSkip.ErrorMessage = errorMessage.String
		}
		if updatedAt.Valid {
			opSkip.UpdatedAt = ptrInt64(updatedAt.Int64)
		}
		episodes[index].OPSkip = opSkip
		attached[key] = struct{}{}
	}
	return rows.Err()
}

func animeEpisodeMediaNumber(episode AnimeEpisode) string {
	number := episode.SortNumber
	if episode.Type == 0 && episode.EpNumber > 0 {
		number = float64(episode.EpNumber)
	}
	return normalizeEpisodeNumber(strconv.FormatFloat(number, 'f', -1, 64))
}

func normalizeEpisodeNumber(value string) string {
	value = strings.TrimSpace(value)
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return strconv.FormatFloat(parsed, 'f', -1, 64)
	}
	return value
}

func (c *Catalog) AnimeImagePath(ctx context.Context, bangumiID int64) (string, error) {
	return c.imagePath(ctx, "SELECT image_local_path FROM anime_metadata WHERE bangumi_id = ? AND deleted_at IS NULL", bangumiID)
}

func (c *Catalog) CharacterImagePath(ctx context.Context, bangumiID, characterID int64) (string, error) {
	return c.imagePath(ctx, `SELECT ac.image_local_path FROM anime_characters ac
JOIN anime_metadata am ON am.bangumi_id = ac.bangumi_id
WHERE ac.bangumi_id = ? AND ac.character_id = ? AND am.deleted_at IS NULL`, bangumiID, characterID)
}

func (c *Catalog) ActorImagePath(ctx context.Context, actorID int64) (string, error) {
	return c.imagePath(ctx, "SELECT image_local_path FROM actors WHERE actor_id = ?", actorID)
}

func (c *Catalog) imagePath(ctx context.Context, query string, args ...any) (string, error) {
	var path string
	if err := c.db.QueryRowContext(ctx, query, args...).Scan(&path); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrAnimeNotFound
		}
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("%w: image unavailable", ErrAnimeNotFound)
	}
	return path, nil
}

func (c *Catalog) storageRoot(stored string) string {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return c.mediaDir
	}
	return stored
}

func animeStoragePath(root, nameCN, name string) string {
	displayName := nameCN
	if strings.TrimSpace(displayName) == "" {
		displayName = name
	}
	return filepath.Join(root, safePathSegment(displayName))
}

func safePathSegment(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	value = strings.Trim(value, " ._")
	if value == "" {
		value = "media"
	}
	runes := []rune(value)
	if len(runes) > 120 {
		value = string(runes[:120])
	}
	return value
}

func displayAnimeTitle(nameCN, name string) string {
	if strings.TrimSpace(nameCN) != "" {
		return nameCN
	}
	return name
}

func ratingScore(raw string) *float64 {
	var payload struct {
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload.Score <= 0 {
		return nil
	}
	return &payload.Score
}

func airDateWithinRange(value string, start, end time.Time) bool {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return false
	}
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	end = time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, time.UTC)
	return !date.Before(start) && !date.After(end)
}

func viewerEpisodeLabel(episode viewerEpisodeRef) string {
	number := strings.TrimSpace(episode.episodeNumber)
	if number == "" {
		number = "?"
	}
	episodeType := strings.ToLower(strings.TrimSpace(episode.episodeType))
	if episodeType == "" || episodeType == "episode" {
		label := fmt.Sprintf("第 %s 话", number)
		if episode.season > 1 {
			return fmt.Sprintf("S%02d %s", episode.season, label)
		}
		return label
	}
	typeLabel := strings.ToUpper(episodeType)
	switch episodeType {
	case "ova":
		typeLabel = "OVA"
	case "oad":
		typeLabel = "OAD"
	case "sp":
		typeLabel = "SP"
	}
	label := fmt.Sprintf("%s %s", typeLabel, number)
	if episode.season > 1 {
		return fmt.Sprintf("S%02d %s", episode.season, label)
	}
	return label
}

func viewerEpisodeProgressLess(left, right viewerEpisodeRef) bool {
	if left.season != right.season {
		return left.season < right.season
	}
	if leftRank, rightRank := viewerEpisodeTypeRank(left.episodeType), viewerEpisodeTypeRank(right.episodeType); leftRank != rightRank {
		return leftRank < rightRank
	}
	leftNumber, leftOK := viewerEpisodeNumber(left.episodeNumber)
	rightNumber, rightOK := viewerEpisodeNumber(right.episodeNumber)
	if leftOK && rightOK && leftNumber != rightNumber {
		return leftNumber < rightNumber
	}
	if leftOK != rightOK {
		return !leftOK
	}
	if left.episodeNumber != right.episodeNumber {
		return left.episodeNumber < right.episodeNumber
	}
	return left.updatedAt < right.updatedAt
}

func viewerEpisodeTypeRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "episode":
		return 3
	case "ova", "oad", "sp":
		return 2
	default:
		return 1
	}
}

func viewerEpisodeNumber(value string) (float64, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return number, err == nil
}

func nullableInt64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func ptrInt64(value int64) *int64 {
	return &value
}
