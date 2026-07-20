package bangumi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const viewerOPSkipReserveSeconds = 2

type ViewerAnimeDetail struct {
	BangumiID     int64                 `json:"bangumiId"`
	Title         string                `json:"title"`
	OriginalTitle string                `json:"originalTitle"`
	AirDate       string                `json:"airDate"`
	AirWeekday    int                   `json:"airWeekday"`
	Platform      string                `json:"platform"`
	Summary       string                `json:"summary"`
	TotalEpisodes int                   `json:"totalEpisodes"`
	HasCover      bool                  `json:"hasCover"`
	RatingScore   *float64              `json:"ratingScore"`
	Infobox       []map[string]any      `json:"infobox"`
	MetaTags      []string              `json:"metaTags"`
	Tags          []AnimeTag            `json:"tags"`
	Characters    []AnimeCharacter      `json:"characters"`
	Episodes      []ViewerDetailEpisode `json:"episodes"`
}

type ViewerDetailEpisode struct {
	Key           string               `json:"key"`
	EpisodeID     int64                `json:"episodeId"`
	MediaID       int64                `json:"mediaId"`
	Label         string               `json:"label"`
	Title         string               `json:"title"`
	OriginalTitle string               `json:"originalTitle"`
	Summary       string               `json:"summary"`
	AirDate       string               `json:"airDate"`
	Duration      string               `json:"duration"`
	CommentCount  int                  `json:"commentCount"`
	SortNumber    float64              `json:"sortNumber"`
	Type          int                  `json:"type"`
	HasMedia      bool                 `json:"hasMedia"`
	HasCover      bool                 `json:"hasCover"`
	MediaInfo     *ViewerMediaInfo     `json:"mediaInfo"`
	OPSkip        *ViewerOPSkipSegment `json:"opSkip"`
}

// ViewerMediaInfo contains completed-output metadata safe to expose to viewer clients.
// It deliberately excludes source and output paths so local server paths stay private.
type ViewerMediaInfo struct {
	Format               string `json:"format"`
	VideoCodec           string `json:"videoCodec"`
	AudioCodec           string `json:"audioCodec"`
	HasInternalSubtitles bool   `json:"hasInternalSubtitles"`
	HasExternalSubtitles bool   `json:"hasExternalSubtitles"`
	Action               string `json:"action"`
}

type ViewerOPSkipSegment struct {
	StartSeconds       float64 `json:"startSeconds"`
	EndSeconds         float64 `json:"endSeconds"`
	PromptStartSeconds float64 `json:"promptStartSeconds"`
	PromptEndSeconds   float64 `json:"promptEndSeconds"`
	SeekToSeconds      float64 `json:"seekToSeconds"`
}

type viewerDetailMedia struct {
	id                   int64
	season               int
	episodeType          string
	episodeNumber        string
	coverPath            string
	coverStatus          string
	format               string
	videoCodec           string
	audioCodec           string
	hasInternalSubtitles bool
	hasExternalSubtitles bool
	action               string
	updatedAt            int64
	opSkip               *ViewerOPSkipSegment
}

func (c *Catalog) ViewerAnimeDetail(ctx context.Context, bangumiID int64) (ViewerAnimeDetail, error) {
	detail, err := c.Detail(ctx, bangumiID)
	if err != nil {
		return ViewerAnimeDetail{}, err
	}
	result := ViewerAnimeDetail{
		BangumiID:     detail.BangumiID,
		Title:         displayAnimeTitle(detail.NameCN, detail.Name),
		OriginalTitle: detail.Name,
		AirDate:       detail.AirDate,
		AirWeekday:    detail.AirWeekday,
		Platform:      detail.Platform,
		Summary:       detail.Summary,
		TotalEpisodes: detail.TotalEpisodes,
		HasCover:      detail.HasCover,
		RatingScore:   mapRatingScore(detail.Rating),
		Infobox:       detail.Infobox,
		MetaTags:      detail.MetaTags,
		Tags:          detail.Tags,
		Characters:    detail.Characters,
		Episodes:      make([]ViewerDetailEpisode, 0, len(detail.Episodes)),
	}
	if result.TotalEpisodes <= 0 {
		result.TotalEpisodes = detail.Eps
	}
	sort.SliceStable(result.Characters, func(i, j int) bool {
		return viewerCharacterRelationRank(result.Characters[i].Relation) < viewerCharacterRelationRank(result.Characters[j].Relation)
	})

	mediaItems, err := c.viewerDetailMedia(ctx, bangumiID)
	if err != nil {
		return ViewerAnimeDetail{}, err
	}
	commentCounts, err := c.viewerEpisodeCommentCounts(ctx, bangumiID)
	if err != nil {
		return ViewerAnimeDetail{}, err
	}
	mediaByKey := make(map[string]viewerDetailMedia, len(mediaItems))
	canonicalMedia := make(map[int64]struct{}, len(mediaItems))
	for _, media := range mediaItems {
		key := viewerDetailEpisodeKey(media.episodeType, media.episodeNumber)
		if _, exists := mediaByKey[key]; !exists {
			mediaByKey[key] = media
			canonicalMedia[media.id] = struct{}{}
		}
	}
	usedMedia := make(map[int64]struct{}, len(mediaItems))
	for _, episode := range detail.Episodes {
		number := episode.SortNumber
		if episode.Type == 0 && episode.EpNumber > 0 {
			number = float64(episode.EpNumber)
		}
		category := "special"
		if episode.Type == 0 {
			category = "episode"
		}
		media, hasMedia := mediaByKey[viewerDetailEpisodeKey(category, strconv.FormatFloat(number, 'f', -1, 64))]
		label := viewerMetadataEpisodeLabel(episode)
		if hasMedia {
			label = viewerEpisodeLabel(viewerEpisodeRef{
				season: media.season, episodeType: media.episodeType, episodeNumber: media.episodeNumber,
			})
			usedMedia[media.id] = struct{}{}
		}
		result.Episodes = append(result.Episodes, ViewerDetailEpisode{
			Key:           fmt.Sprintf("episode-%d", episode.EpisodeID),
			EpisodeID:     episode.EpisodeID,
			MediaID:       media.id,
			Label:         label,
			Title:         displayAnimeTitle(episode.NameCN, episode.Name),
			OriginalTitle: episode.Name,
			Summary:       episode.Description,
			AirDate:       episode.Airdate,
			Duration:      episode.Duration,
			CommentCount:  commentCounts[episode.EpisodeID],
			SortNumber:    episode.SortNumber,
			Type:          episode.Type,
			HasMedia:      hasMedia,
			HasCover:      hasMedia && media.coverStatus == "completed" && strings.TrimSpace(media.coverPath) != "",
			MediaInfo:     viewerMediaInfo(media, hasMedia),
			OPSkip:        media.opSkip,
		})
	}

	for _, media := range mediaItems {
		if _, canonical := canonicalMedia[media.id]; !canonical {
			continue
		}
		if _, used := usedMedia[media.id]; used {
			continue
		}
		sortNumber, _ := strconv.ParseFloat(strings.TrimSpace(media.episodeNumber), 64)
		episodeType := 1
		if viewerEpisodeTypeRank(media.episodeType) == viewerEpisodeTypeRank("episode") {
			episodeType = 0
		}
		label := viewerEpisodeLabel(viewerEpisodeRef{
			season: media.season, episodeType: media.episodeType, episodeNumber: media.episodeNumber,
		})
		result.Episodes = append(result.Episodes, ViewerDetailEpisode{
			Key:        fmt.Sprintf("media-%d", media.id),
			MediaID:    media.id,
			Label:      label,
			Title:      label,
			SortNumber: sortNumber,
			Type:       episodeType,
			HasMedia:   true,
			HasCover:   media.coverStatus == "completed" && strings.TrimSpace(media.coverPath) != "",
			MediaInfo:  viewerMediaInfo(media, true),
			OPSkip:     media.opSkip,
		})
	}
	sort.SliceStable(result.Episodes, func(i, j int) bool {
		left, right := result.Episodes[i], result.Episodes[j]
		if (left.Type == 0) != (right.Type == 0) {
			return left.Type == 0
		}
		if left.SortNumber != right.SortNumber {
			return left.SortNumber < right.SortNumber
		}
		return left.Key < right.Key
	})
	return result, nil
}

func (c *Catalog) viewerEpisodeCommentCounts(ctx context.Context, bangumiID int64) (map[int64]int, error) {
	rows, err := c.db.QueryContext(ctx, `
SELECT comments.episode_id, COUNT(*)
FROM anime_episodes episodes
JOIN bangumi_episode_comments comments ON comments.episode_id = episodes.episode_id
WHERE episodes.bangumi_id = ? AND comments.parent_comment_id = 0
GROUP BY comments.episode_id`, bangumiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[int64]int)
	for rows.Next() {
		var episodeID int64
		var count int
		if err := rows.Scan(&episodeID, &count); err != nil {
			return nil, err
		}
		counts[episodeID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

func (c *Catalog) ViewerMediaPath(ctx context.Context, bangumiID, mediaID int64) (string, error) {
	return c.viewerMediaFilePath(ctx, `
SELECT output_path
FROM media_jobs
WHERE id = ? AND bangumi_id = ? AND status = 'completed' AND output_path != ''`, mediaID, bangumiID)
}

func (c *Catalog) ViewerMediaCoverPath(ctx context.Context, bangumiID, mediaID int64) (string, error) {
	return c.viewerMediaFilePath(ctx, `
SELECT cover_path
FROM media_jobs
WHERE id = ? AND bangumi_id = ? AND status = 'completed'
  AND cover_status = 'completed' AND cover_path != ''`, mediaID, bangumiID)
}

func (c *Catalog) viewerMediaFilePath(ctx context.Context, query string, args ...any) (string, error) {
	var path string
	err := c.db.QueryRowContext(ctx, query, args...).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAnimeNotFound
	}
	return path, err
}

func (c *Catalog) viewerDetailMedia(ctx context.Context, bangumiID int64) ([]viewerDetailMedia, error) {
	rows, err := c.db.QueryContext(ctx, `
	SELECT mj.id, mj.season_number, COALESCE(NULLIF(mj.episode_type, ''), 'episode'),
	       mj.episode_number, mj.cover_path, mj.cover_status, mj.output_path,
	       mj.video_codec, mj.audio_codec, mj.has_internal_subtitles, mj.has_external_subtitles, mj.action,
	       COALESCE(mj.completed_at, mj.updated_at, mj.created_at, 0),
       mos.start_seconds, mos.end_seconds
FROM media_jobs mj
LEFT JOIN media_op_segments mos ON mos.media_job_id = mj.id
  AND mos.status = 'detected'
  AND mos.end_seconds > mos.start_seconds
WHERE mj.bangumi_id = ? AND mj.status = 'completed' AND mj.output_path != ''
ORDER BY COALESCE(mj.completed_at, mj.updated_at, mj.created_at, 0) DESC, mj.id DESC`, bangumiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]viewerDetailMedia, 0)
	for rows.Next() {
		var item viewerDetailMedia
		var outputPath string
		var opStartSeconds, opEndSeconds sql.NullFloat64
		if err := rows.Scan(
			&item.id, &item.season, &item.episodeType, &item.episodeNumber,
			&item.coverPath, &item.coverStatus, &outputPath,
			&item.videoCodec, &item.audioCodec, &item.hasInternalSubtitles, &item.hasExternalSubtitles, &item.action,
			&item.updatedAt,
			&opStartSeconds, &opEndSeconds,
		); err != nil {
			return nil, err
		}
		if opStartSeconds.Valid && opEndSeconds.Valid {
			item.opSkip = viewerOPSkipSegment(opStartSeconds.Float64, opEndSeconds.Float64)
		}
		item.format = viewerMediaFormat(outputPath)
		items = append(items, item)
	}
	return items, rows.Err()
}

func viewerMediaInfo(media viewerDetailMedia, available bool) *ViewerMediaInfo {
	if !available {
		return nil
	}
	videoCodec := media.videoCodec
	audioCodec := media.audioCodec
	if media.action == "transcode" || media.action == "burn_subtitles" {
		videoCodec = "h264"
		audioCodec = "aac"
	}
	return &ViewerMediaInfo{
		Format:               media.format,
		VideoCodec:           videoCodec,
		AudioCodec:           audioCodec,
		HasInternalSubtitles: media.hasInternalSubtitles,
		HasExternalSubtitles: media.hasExternalSubtitles,
		Action:               media.action,
	}
}

func viewerMediaFormat(outputPath string) string {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return ""
	}
	filename := outputPath[strings.LastIndexAny(outputPath, `/\\`)+1:]
	lastDot := strings.LastIndex(filename, ".")
	if lastDot < 0 || lastDot == len(filename)-1 {
		return ""
	}
	return strings.ToLower(filename[lastDot+1:])
}

func viewerOPSkipSegment(startSeconds, endSeconds float64) *ViewerOPSkipSegment {
	if !isFiniteNonNegative(startSeconds) || !isFiniteNonNegative(endSeconds) || endSeconds <= startSeconds {
		return nil
	}
	seekToSeconds := math.Max(0, endSeconds-viewerOPSkipReserveSeconds)
	promptStartSeconds := math.Max(0, startSeconds-viewerOPSkipReserveSeconds)
	promptEndSeconds := seekToSeconds
	if promptEndSeconds < promptStartSeconds {
		promptEndSeconds = promptStartSeconds
	}
	return &ViewerOPSkipSegment{
		StartSeconds:       startSeconds,
		EndSeconds:         endSeconds,
		PromptStartSeconds: promptStartSeconds,
		PromptEndSeconds:   promptEndSeconds,
		SeekToSeconds:      seekToSeconds,
	}
}

func isFiniteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func viewerDetailEpisodeKey(episodeType, number string) string {
	category := "special"
	if viewerEpisodeTypeRank(episodeType) == viewerEpisodeTypeRank("episode") {
		category = "episode"
	}
	number = strings.TrimSpace(number)
	if parsed, err := strconv.ParseFloat(number, 64); err == nil {
		number = strconv.FormatFloat(parsed, 'f', -1, 64)
	}
	return category + ":" + number
}

func viewerMetadataEpisodeLabel(episode AnimeEpisode) string {
	number := episode.SortNumber
	if episode.Type == 0 && episode.EpNumber > 0 {
		number = float64(episode.EpNumber)
	}
	value := strconv.FormatFloat(number, 'f', -1, 64)
	if episode.Type == 0 {
		return fmt.Sprintf("第 %s 话", value)
	}
	return fmt.Sprintf("特别篇 %s", value)
}

func viewerCharacterRelationRank(relation string) int {
	value := strings.ToLower(strings.TrimSpace(relation))
	switch {
	case strings.Contains(value, "主角"), strings.Contains(value, "main"):
		return 0
	case strings.Contains(value, "配角"), strings.Contains(value, "support"):
		return 1
	default:
		return 2
	}
}

func mapRatingScore(rating map[string]any) *float64 {
	value, ok := rating["score"]
	if !ok {
		return nil
	}
	var score float64
	switch typed := value.(type) {
	case float64:
		score = typed
	}
	if score <= 0 {
		return nil
	}
	return &score
}
