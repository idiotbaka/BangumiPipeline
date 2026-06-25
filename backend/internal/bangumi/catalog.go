package bangumi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrAnimeNotFound      = errors.New("anime not found")
	ErrAnimeAlreadyExists = errors.New("anime already exists")
	ErrInvalidSubjectType = errors.New("invalid subject type")
)

type Catalog struct {
	db *sql.DB
}

func NewCatalog(db *sql.DB) *Catalog {
	return &Catalog{db: db}
}

type AnimeListItem struct {
	BangumiID       int64                 `json:"bangumiId"`
	Name            string                `json:"name"`
	NameCN          string                `json:"nameCN"`
	AirDate         string                `json:"airDate"`
	AirWeekday      int                   `json:"airWeekday"`
	Episodes        int                   `json:"episodes"`
	Platform        string                `json:"platform"`
	ImageStatus     string                `json:"imageStatus"`
	HasCover        bool                  `json:"hasCover"`
	DetailStatus    string                `json:"detailStatus"`
	MatchedEpisodes []AnimeMatchedEpisode `json:"matchedEpisodes"`
	CreatedAt       int64                 `json:"createdAt"`
}

type AnimePage struct {
	Items    []AnimeListItem `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
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
	Infobox        []map[string]any `json:"infobox"`
	Rating         map[string]any   `json:"rating"`
	Collection     map[string]any   `json:"collection"`
	MetaTags       []string         `json:"metaTags"`
	Tags           []AnimeTag       `json:"tags"`
	Aliases        []string         `json:"aliases"`
	Characters     []AnimeCharacter `json:"characters"`
	CreatedAt      int64            `json:"createdAt"`
}

func (c *Catalog) List(ctx context.Context, page, pageSize int) (AnimePage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 24
	}
	result := AnimePage{Items: make([]AnimeListItem, 0), Page: page, PageSize: pageSize}
	if err := c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM anime_metadata WHERE deleted_at IS NULL").Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := c.db.QueryContext(ctx, `
SELECT bangumi_id, name, name_cn, air_date, air_weekday,
       CASE WHEN total_episodes > 0 THEN total_episodes ELSE eps END,
       platform, image_status, image_local_path != '', detail_status, created_at
FROM anime_metadata
WHERE deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?`, pageSize, (page-1)*pageSize)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item AnimeListItem
		if err := rows.Scan(
			&item.BangumiID, &item.Name, &item.NameCN, &item.AirDate, &item.AirWeekday,
			&item.Episodes, &item.Platform, &item.ImageStatus, &item.HasCover,
			&item.DetailStatus, &item.CreatedAt,
		); err != nil {
			rows.Close()
			return result, err
		}
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
	args := []any{limit}
	where := "am.deleted_at IS NULL"
	if query != "" {
		like := "%" + query + "%"
		where += " AND (am.name LIKE ? OR am.name_cn LIKE ? OR aa.alias LIKE ?)"
		args = []any{like, like, like, limit}
	}
	rows, err := c.db.QueryContext(ctx, fmt.Sprintf(`
SELECT am.bangumi_id, am.name, am.name_cn
FROM anime_metadata am
LEFT JOIN anime_aliases aa ON aa.bangumi_id = am.bangumi_id
WHERE %s
GROUP BY am.bangumi_id, am.name, am.name_cn
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

func (c *Catalog) Detail(ctx context.Context, bangumiID int64) (AnimeDetail, error) {
	var detail AnimeDetail
	var infoboxJSON, ratingJSON, collectionJSON, metaTagsJSON string
	err := c.db.QueryRowContext(ctx, `
SELECT bangumi_id, url, name, name_cn, air_date, air_weekday, detail_date,
       platform, summary, eps, total_episodes, volumes, series, locked, nsfw,
       image_local_path != '', image_status, detail_status, characters_status,
       infobox_json, rating_json, collection_json, meta_tags_json, created_at
FROM anime_metadata WHERE bangumi_id = ? AND deleted_at IS NULL`, bangumiID).Scan(
		&detail.BangumiID, &detail.URL, &detail.Name, &detail.NameCN, &detail.AirDate,
		&detail.AirWeekday, &detail.DetailDate, &detail.Platform, &detail.Summary,
		&detail.Eps, &detail.TotalEpisodes, &detail.Volumes, &detail.Series,
		&detail.Locked, &detail.NSFW, &detail.HasCover, &detail.ImageStatus,
		&detail.DetailStatus, &detail.CharacterState, &infoboxJSON, &ratingJSON,
		&collectionJSON, &metaTagsJSON, &detail.CreatedAt,
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

	if detail.Tags, err = c.tags(ctx, bangumiID); err != nil {
		return AnimeDetail{}, err
	}
	if detail.Aliases, err = c.aliases(ctx, bangumiID); err != nil {
		return AnimeDetail{}, err
	}
	if detail.Characters, err = c.characters(ctx, bangumiID); err != nil {
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
FROM anime_characters WHERE bangumi_id = ? ORDER BY id LIMIT 10`, bangumiID)
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
SELECT a.actor_id, a.name, a.short_summary, a.career_json,
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
