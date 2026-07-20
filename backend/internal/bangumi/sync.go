package bangumi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bangumipipeline.local/server/internal/system"
)

const (
	maxCharactersPerAnime = 10
	subjectSearchPageSize = 20
)

type SettingsProvider interface {
	GetNetworkSettings(context.Context) (system.NetworkSettings, error)
}

type SyncerConfig struct {
	APIBaseURL     string
	UserAgent      string
	CoverDir       string
	FFmpegPath     string
	APIInterval    time.Duration
	RequestTimeout time.Duration
	RequestLimiter *RequestLimiter
}

type Syncer struct {
	db       *sql.DB
	settings SettingsProvider
	logger   *slog.Logger
	config   SyncerConfig
	limiter  *RequestLimiter
	now      func() time.Time
}

func NewSyncer(db *sql.DB, settings SettingsProvider, logger *slog.Logger, config SyncerConfig) *Syncer {
	if config.APIInterval <= 0 {
		config.APIInterval = 2 * time.Second
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 20 * time.Second
	}
	if strings.TrimSpace(config.FFmpegPath) == "" {
		config.FFmpegPath = "ffmpeg"
	}
	limiter := config.RequestLimiter
	if limiter == nil {
		limiter = NewRequestLimiter(config.APIInterval)
	}
	return &Syncer{
		db: db, settings: settings, logger: logger, config: config,
		limiter: limiter, now: time.Now,
	}
}

func (s *Syncer) Execute(ctx context.Context) error {
	client, err := s.newAPIClient(ctx)
	if err != nil {
		return err
	}
	defer client.close()

	var calendar []calendarDay
	if err := client.getJSON(ctx, "/calendar", &calendar); err != nil {
		return fmt.Errorf("请求 Bangumi Calendar API: %w", err)
	}

	failures := make([]string, 0)
	if imageFailures, err := s.retryAnimeImages(ctx, client); err != nil {
		failures = append(failures, fmt.Sprintf("查询待重试番剧封面失败: %v", err))
	} else {
		failures = append(failures, imageFailures...)
	}
	inserted, skipped, discoveryFailures := s.discoverCalendarAnime(ctx, client, calendar)
	failures = append(failures, discoveryFailures...)
	customInserted, customSkipped, customFetched := 0, 0, 0
	customSettings, err := s.CustomSearchSettings(ctx)
	if err != nil {
		return fmt.Errorf("读取 Bangumi 自定义抓取设置: %w", err)
	}
	if len(customSettings.Tags) > 0 {
		subjects, err := s.fetchCustomSearchSubjects(ctx, client, customSettings.Tags)
		if err != nil {
			failures = append(failures, fmt.Sprintf("自定义标签搜索失败: %v", err))
		} else {
			customFetched = len(subjects)
			customInserted, customSkipped, discoveryFailures = s.discoverSearchAnime(ctx, client, subjects)
			failures = append(failures, discoveryFailures...)
		}
	}
	incomplete, err := listIncompleteSubjects(ctx, s.db)
	if err != nil {
		return fmt.Errorf("查询待补全番剧: %w", err)
	}
	detailCompleted, characterCompleted, episodeCompleted := 0, 0, 0
	for _, subject := range incomplete {
		if subject.DetailStatus != stageStatusCompleted {
			if err := s.syncDetail(ctx, client, subject.BangumiID); err != nil {
				failures = append(failures, err.Error())
			} else {
				detailCompleted++
			}
		}
		if subject.CharacterStatus != stageStatusCompleted {
			if err := s.syncCharacters(ctx, client, subject.BangumiID); err != nil {
				failures = append(failures, err.Error())
			} else {
				characterCompleted++
			}
		}
		if subject.EpisodesStatus != stageStatusCompleted || subject.EpisodesMissing {
			if err := s.syncEpisodes(ctx, client, subject.BangumiID); err != nil {
				failures = append(failures, err.Error())
			} else {
				episodeCompleted++
			}
		}
	}

	s.logger.Info("Bangumi metadata synchronized",
		"base_inserted", inserted, "base_skipped", skipped,
		"custom_tags", len(customSettings.Tags), "custom_fetched", customFetched,
		"custom_inserted", customInserted, "custom_skipped", customSkipped,
		"details_completed", detailCompleted, "characters_completed", characterCompleted,
		"episodes_completed", episodeCompleted,
		"failed", len(failures),
	)
	if len(failures) > 0 {
		shown := failures
		if len(shown) > 3 {
			shown = shown[:3]
		}
		return fmt.Errorf("同步存在 %d 个错误：%s", len(failures), strings.Join(shown, "；"))
	}
	return nil
}

func (s *Syncer) CustomSearchSettings(ctx context.Context) (CustomSearchSettings, error) {
	return customSearchSettings(ctx, s.db)
}

func (s *Syncer) UpdateCustomSearchSettings(ctx context.Context, tags []string) (CustomSearchSettings, error) {
	return updateCustomSearchSettings(ctx, s.db, tags, s.now())
}

func (s *Syncer) AddSubject(ctx context.Context, bangumiID int64) error {
	return s.syncSubjectByID(ctx, bangumiID, false)
}

func (s *Syncer) RefreshSubject(ctx context.Context, bangumiID int64) error {
	return s.syncSubjectByID(ctx, bangumiID, true)
}

func (s *Syncer) newAPIClient(ctx context.Context) (*apiClient, error) {
	settings, err := s.settings.GetNetworkSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取代理设置: %w", err)
	}
	return newAPIClient(settings, s.config, s.logger, s.limiter)
}

func (s *Syncer) syncSubjectByID(ctx context.Context, bangumiID int64, refresh bool) error {
	active, err := activeSubjectExists(ctx, s.db, bangumiID)
	if err != nil {
		return fmt.Errorf("Bangumi #%d 状态查询失败: %w", bangumiID, err)
	}
	if refresh && !active {
		return ErrAnimeNotFound
	}
	if !refresh && active {
		return ErrAnimeAlreadyExists
	}
	client, err := s.newAPIClient(ctx)
	if err != nil {
		return err
	}
	defer client.close()

	s.logger.Info("手动同步 Bangumi 元数据开始", "source", "bangumi", "bangumi_id", bangumiID, "refresh", refresh)
	if err := s.syncSubjectMetadata(ctx, client, bangumiID, active); err != nil {
		s.logger.Error("手动同步 Bangumi 元数据失败", "source", "bangumi", "bangumi_id", bangumiID, "refresh", refresh, "error", err)
		return err
	}
	s.logger.Info("手动同步 Bangumi 元数据成功", "source", "bangumi", "bangumi_id", bangumiID, "refresh", refresh)
	return nil
}

func (s *Syncer) syncSubjectMetadata(ctx context.Context, client *apiClient, bangumiID int64, markExistingFailures bool) error {
	detail, err := s.fetchSubjectDetail(ctx, client, bangumiID)
	if err != nil {
		if markExistingFailures {
			return combineStageError("详情抓取", err, markDetailFailed(ctx, s.db, bangumiID, err))
		}
		return err
	}
	if detail.Type != 0 && detail.Type != 2 {
		return fmt.Errorf("%w: Bangumi #%d type=%d", ErrInvalidSubjectType, bangumiID, detail.Type)
	}
	cover, downloadErr := client.downloadImage(ctx, detail.Images.Large, s.config.CoverDir, strconv.FormatInt(bangumiID, 10))
	if err := upsertBaseMetadataFromDetail(ctx, s.db, bangumiID, detail, cover, downloadErr, s.now()); err != nil {
		return fmt.Errorf("Bangumi #%d 基础数据入库失败: %w", bangumiID, err)
	}

	failures := make([]string, 0)
	if downloadErr != nil {
		failures = append(failures, fmt.Sprintf("Bangumi #%d 封面下载失败: %v", bangumiID, downloadErr))
	}
	if err := saveSubjectDetail(ctx, s.db, bangumiID, detail, s.now()); err != nil {
		runErr := fmt.Errorf("Bangumi #%d 详情入库失败: %w", bangumiID, err)
		failures = append(failures, combineStageError("详情抓取", runErr, markDetailFailed(ctx, s.db, bangumiID, runErr)).Error())
	}
	if err := s.syncCharacters(ctx, client, bangumiID); err != nil {
		failures = append(failures, err.Error())
	}
	if err := s.syncEpisodes(ctx, client, bangumiID); err != nil {
		failures = append(failures, err.Error())
	}
	if len(failures) == 0 {
		return nil
	}
	shown := failures
	if len(shown) > 3 {
		shown = shown[:3]
	}
	return fmt.Errorf("Bangumi #%d 元数据同步存在 %d 个错误：%s", bangumiID, len(failures), strings.Join(shown, "；"))
}

func (s *Syncer) discoverCalendarAnime(ctx context.Context, client *apiClient, calendar []calendarDay) (int, int, []string) {
	inserted, skipped := 0, 0
	failures := make([]string, 0)
	for _, day := range calendar {
		for _, item := range day.Items {
			if item.Type != 2 {
				continue
			}
			processed, err := isProcessed(ctx, s.db, item.ID)
			if err != nil {
				failures = append(failures, fmt.Sprintf("Bangumi #%d 基础数据查询失败: %v", item.ID, err))
				continue
			}
			if processed {
				skipped++
				continue
			}
			cover, downloadErr := client.downloadImage(ctx, item.Images.Large, s.config.CoverDir, strconv.FormatInt(item.ID, 10))
			if err := insertBaseMetadata(ctx, s.db, item, cover, downloadErr, s.now()); err != nil {
				failures = append(failures, fmt.Sprintf("Bangumi #%d 基础数据入库失败: %v", item.ID, err))
				continue
			}
			inserted++
			if downloadErr != nil {
				failures = append(failures, fmt.Sprintf("Bangumi #%d 封面下载失败: %v", item.ID, downloadErr))
			}
		}
	}
	return inserted, skipped, failures
}

func (s *Syncer) fetchCustomSearchSubjects(ctx context.Context, client *apiClient, tags []string) ([]subjectDetail, error) {
	subjects := make([]subjectDetail, 0)
	seen := make(map[int64]struct{})
	offset := 0
	total := -1
	for {
		var response subjectSearchResponse
		payload := subjectSearchRequest{
			Filter: subjectSearchFilter{Type: []int{2}, Tag: tags},
		}
		path := fmt.Sprintf("/v0/search/subjects?limit=%d&offset=%d", subjectSearchPageSize, offset)
		if err := client.postJSON(ctx, path, payload, &response); err != nil {
			return nil, err
		}
		if response.Total >= 0 {
			total = response.Total
		}
		for _, subject := range response.Data {
			if subject.ID == 0 {
				continue
			}
			if _, exists := seen[subject.ID]; exists {
				continue
			}
			seen[subject.ID] = struct{}{}
			subjects = append(subjects, subject)
		}
		if len(response.Data) == 0 {
			break
		}
		nextOffset := offset + subjectSearchPageSize
		if nextOffset <= offset {
			break
		}
		offset = nextOffset
		if total >= 0 && offset >= total {
			break
		}
	}
	return subjects, nil
}

func (s *Syncer) discoverSearchAnime(ctx context.Context, client *apiClient, subjects []subjectDetail) (int, int, []string) {
	inserted, skipped := 0, 0
	failures := make([]string, 0)
	for _, subject := range subjects {
		if subject.Type != 2 {
			continue
		}
		processed, err := isProcessed(ctx, s.db, subject.ID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("Bangumi #%d 基础数据查询失败: %v", subject.ID, err))
			continue
		}
		if processed {
			skipped++
			continue
		}
		cover, downloadErr := client.downloadImage(ctx, subject.Images.Large, s.config.CoverDir, strconv.FormatInt(subject.ID, 10))
		if err := upsertBaseMetadataFromDetail(ctx, s.db, subject.ID, subject, cover, downloadErr, s.now()); err != nil {
			failures = append(failures, fmt.Sprintf("Bangumi #%d 自定义搜索基础数据入库失败: %v", subject.ID, err))
			continue
		}
		inserted++
		if downloadErr != nil {
			failures = append(failures, fmt.Sprintf("Bangumi #%d 封面下载失败: %v", subject.ID, downloadErr))
		}
	}
	return inserted, skipped, failures
}

func (s *Syncer) retryAnimeImages(ctx context.Context, client *apiClient) ([]string, error) {
	images, err := listRetryableAnimeImages(ctx, s.db)
	if err != nil {
		return nil, err
	}
	failures := make([]string, 0)
	for _, image := range images {
		download, downloadErr := client.downloadImage(
			ctx, image.SourceURL, s.config.CoverDir, strconv.FormatInt(image.BangumiID, 10),
		)
		if err := updateAnimeImage(ctx, s.db, image.BangumiID, download, downloadErr); err != nil {
			failures = append(failures, fmt.Sprintf("Bangumi #%d 封面状态保存失败: %v", image.BangumiID, err))
			continue
		}
		if downloadErr != nil {
			failures = append(failures, fmt.Sprintf("Bangumi #%d 封面重试失败: %v", image.BangumiID, downloadErr))
		}
	}
	return failures, nil
}

func (s *Syncer) syncDetail(ctx context.Context, client *apiClient, bangumiID int64) error {
	detail, err := s.fetchSubjectDetail(ctx, client, bangumiID)
	if err != nil {
		return combineStageError("详情抓取", err, markDetailFailed(ctx, s.db, bangumiID, err))
	}
	if err := saveSubjectDetail(ctx, s.db, bangumiID, detail, s.now()); err != nil {
		runErr := fmt.Errorf("Bangumi #%d 详情入库失败: %w", bangumiID, err)
		return combineStageError("详情抓取", runErr, markDetailFailed(ctx, s.db, bangumiID, runErr))
	}
	return nil
}

func (s *Syncer) fetchSubjectDetail(ctx context.Context, client *apiClient, bangumiID int64) (subjectDetail, error) {
	var detail subjectDetail
	path := fmt.Sprintf("/v0/subjects/%d", bangumiID)
	if err := client.getJSON(ctx, path, &detail); err != nil {
		return subjectDetail{}, fmt.Errorf("Bangumi #%d 详情请求失败: %w", bangumiID, err)
	}
	if detail.ID != 0 && detail.ID != bangumiID {
		return subjectDetail{}, fmt.Errorf("Bangumi #%d 详情响应 ID 不匹配: %d", bangumiID, detail.ID)
	}
	return detail, nil
}

func (s *Syncer) syncCharacters(ctx context.Context, client *apiClient, bangumiID int64) error {
	var response []relatedCharacter
	path := fmt.Sprintf("/v0/subjects/%d/characters", bangumiID)
	if err := client.getJSON(ctx, path, &response); err != nil {
		runErr := fmt.Errorf("Bangumi #%d 角色请求失败: %w", bangumiID, err)
		return combineStageError("角色抓取", runErr, markCharactersFailed(ctx, s.db, bangumiID, runErr))
	}
	if len(response) > maxCharactersPerAnime {
		s.logger.Info("角色列表已按上限截断", "source", "bangumi", "bangumi_id", bangumiID,
			"received", len(response), "kept", maxCharactersPerAnime)
		response = response[:maxCharactersPerAnime]
	}
	characters := make([]storedCharacter, 0, len(response))
	characterDir := filepath.Join(s.config.CoverDir, "characters", strconv.FormatInt(bangumiID, 10))
	for _, character := range response {
		image, err := client.downloadImage(ctx, character.Images.Large, characterDir, strconv.FormatInt(character.ID, 10))
		if err != nil {
			runErr := fmt.Errorf("Bangumi #%d 角色 #%d 图片下载失败: %w", bangumiID, character.ID, err)
			return combineStageError("角色抓取", runErr, markCharactersFailed(ctx, s.db, bangumiID, runErr))
		}
		actorIDs := make([]int64, 0, len(character.Actors))
		for _, actor := range character.Actors {
			if actor.ID == 0 {
				continue
			}
			if err := s.syncActor(ctx, client, actor); err != nil {
				runErr := fmt.Errorf("Bangumi #%d 角色 #%d 声优 #%d 处理失败: %w", bangumiID, character.ID, actor.ID, err)
				return combineStageError("角色抓取", runErr, markCharactersFailed(ctx, s.db, bangumiID, runErr))
			}
			actorIDs = append(actorIDs, actor.ID)
		}
		characters = append(characters, storedCharacter{
			CharacterID: character.ID, Name: character.Name, Summary: character.Summary,
			Relation: character.Relation, Type: character.Type,
			ImageLargeURL: character.Images.Large, ImagePath: image.Path,
			ImageStatus: image.Status, ActorIDs: actorIDs,
		})
	}
	if err := saveCharacters(ctx, s.db, bangumiID, characters, s.now()); err != nil {
		runErr := fmt.Errorf("Bangumi #%d 角色入库失败: %w", bangumiID, err)
		return combineStageError("角色抓取", runErr, markCharactersFailed(ctx, s.db, bangumiID, runErr))
	}
	return nil
}

func (s *Syncer) syncEpisodes(ctx context.Context, client *apiClient, bangumiID int64) error {
	episodes, err := s.fetchEpisodes(ctx, client, bangumiID)
	if err != nil {
		return combineStageError("分集抓取", err, markEpisodesFailed(ctx, s.db, bangumiID, err))
	}
	if err := saveEpisodes(ctx, s.db, bangumiID, episodes, s.now()); err != nil {
		runErr := fmt.Errorf("Bangumi #%d 分集入库失败: %w", bangumiID, err)
		return combineStageError("分集抓取", runErr, markEpisodesFailed(ctx, s.db, bangumiID, runErr))
	}
	return nil
}

func (s *Syncer) fetchEpisodes(ctx context.Context, client *apiClient, bangumiID int64) ([]episodeDetail, error) {
	firstPath := fmt.Sprintf("/v0/episodes?subject_id=%d", bangumiID)
	var response episodesResponse
	if err := client.getJSON(ctx, firstPath, &response); err != nil {
		return nil, fmt.Errorf("Bangumi #%d 分集请求失败: %w", bangumiID, err)
	}
	episodes := append([]episodeDetail(nil), response.Data...)
	total := response.Total
	limit := response.Limit
	offset := response.Offset + len(response.Data)
	if limit <= 0 {
		limit = len(response.Data)
	}
	for total > len(episodes) && limit > 0 && offset < total {
		var page episodesResponse
		path := fmt.Sprintf("/v0/episodes?subject_id=%d&limit=%d&offset=%d", bangumiID, limit, offset)
		if err := client.getJSON(ctx, path, &page); err != nil {
			return nil, fmt.Errorf("Bangumi #%d 分集分页请求失败: %w", bangumiID, err)
		}
		if page.Total > total {
			total = page.Total
		}
		if len(page.Data) == 0 {
			break
		}
		episodes = append(episodes, page.Data...)
		offset = page.Offset + len(page.Data)
	}
	for _, episode := range episodes {
		if episode.ID == 0 {
			return nil, fmt.Errorf("Bangumi #%d 分集响应缺少 episode id", bangumiID)
		}
		if episode.SubjectID != 0 && episode.SubjectID != bangumiID {
			return nil, fmt.Errorf("Bangumi #%d 分集响应 subject_id 不匹配: episode #%d subject_id=%d",
				bangumiID, episode.ID, episode.SubjectID)
		}
	}
	return episodes, nil
}

func (s *Syncer) syncActor(ctx context.Context, client *apiClient, actor relatedActor) error {
	state, err := getActorImageState(ctx, s.db, actor.ID)
	if err != nil {
		return err
	}
	download := imageDownload{Status: imageStatusPending}
	var downloadErr error
	if state.Exists && state.ImageStatus == imageStatusDownloaded {
		if info, statErr := os.Stat(state.ImagePath); statErr == nil && info.Size() > 0 {
			download = imageDownload{Path: state.ImagePath, Status: imageStatusDownloaded}
		}
	}
	if download.Status == imageStatusPending && state.Exists && state.ImageStatus == imageStatusNotFound && state.ImageLargeURL == actor.Images.Large {
		download = imageDownload{Status: imageStatusNotFound}
	}
	if download.Status == imageStatusPending {
		download, downloadErr = client.downloadImage(
			ctx, actor.Images.Large, filepath.Join(s.config.CoverDir, "actors"), strconv.FormatInt(actor.ID, 10),
		)
	}
	careerJSON, err := json.Marshal(actor.Career)
	if err != nil {
		return err
	}
	stored := storedActor{
		ActorID: actor.ID, Name: actor.Name, ShortSummary: actor.ShortSummary,
		CareerJSON: string(careerJSON), Type: actor.Type, Locked: actor.Locked,
		ImageLargeURL: actor.Images.Large, ImagePath: download.Path, ImageStatus: download.Status,
		ImageError: truncateError(downloadErr),
	}
	if err := upsertActor(ctx, s.db, stored, s.now()); err != nil {
		return err
	}
	return downloadErr
}
