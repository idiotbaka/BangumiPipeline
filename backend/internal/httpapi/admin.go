package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bangumipipeline.local/server/internal/applog"
	"bangumipipeline.local/server/internal/auth"
	"bangumipipeline.local/server/internal/bangumi"
	"bangumipipeline.local/server/internal/download"
	"bangumipipeline.local/server/internal/media"
	"bangumipipeline.local/server/internal/subscription"
	"bangumipipeline.local/server/internal/system"
)

const adminSessionCookie = "ab_admin_session"

var hiddenSystemLogSources = []string{"http"}

type AdminAPI struct {
	auth         *auth.Service
	system       *system.Service
	scheduler    *system.Scheduler
	logs         *applog.Service
	catalog      *bangumi.Catalog
	syncer       *bangumi.Syncer
	subscription *subscription.Service
	download     *download.Service
	media        *media.Service
	logger       *slog.Logger
	cookieSecure bool
}

func NewAdminHandler(authService *auth.Service, systemService *system.Service, scheduler *system.Scheduler, logs *applog.Service, catalog *bangumi.Catalog, syncer *bangumi.Syncer, subscriptionService *subscription.Service, downloadService *download.Service, mediaService *media.Service, logger *slog.Logger, cookieSecure bool, webDir string) http.Handler {
	api := &AdminAPI{
		auth: authService, system: systemService, scheduler: scheduler, logs: logs,
		catalog: catalog, syncer: syncer, subscription: subscriptionService, download: downloadService, media: mediaService, logger: logger, cookieSecure: cookieSecure,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", health)
	mux.HandleFunc("GET /api/setup/status", api.setupStatus)
	mux.HandleFunc("POST /api/setup", api.setup)
	mux.HandleFunc("POST /api/auth/login", api.login)
	mux.HandleFunc("GET /api/auth/me", api.me)
	mux.HandleFunc("POST /api/auth/logout", api.logout)
	mux.HandleFunc("GET /api/dashboard/overview", api.dashboardOverview)
	mux.HandleFunc("GET /api/scheduled-tasks", api.listScheduledTasks)
	mux.HandleFunc("PATCH /api/scheduled-tasks/{taskKey}", api.updateScheduledTask)
	mux.HandleFunc("POST /api/scheduled-tasks/{taskKey}/run", api.runScheduledTask)
	mux.HandleFunc("GET /api/settings/network", api.getNetworkSettings)
	mux.HandleFunc("PUT /api/settings/network", api.updateNetworkSettings)
	mux.HandleFunc("GET /api/settings/subscription", api.getSubscriptionSettings)
	mux.HandleFunc("PUT /api/settings/subscription", api.updateSubscriptionSettings)
	mux.HandleFunc("GET /api/settings/download", api.getDownloadSettings)
	mux.HandleFunc("PUT /api/settings/download", api.updateDownloadSettings)
	mux.HandleFunc("POST /api/settings/download/test", api.testDownloadSettings)
	mux.HandleFunc("GET /api/settings/media-storage", api.getMediaStorageSettings)
	mux.HandleFunc("PUT /api/settings/media-storage", api.updateMediaStorageSettings)
	mux.HandleFunc("GET /api/system-logs", api.listSystemLogs)
	mux.HandleFunc("GET /api/system-logs/stream", api.streamSystemLogs)
	mux.HandleFunc("GET /api/anime", api.listAnime)
	mux.HandleFunc("POST /api/anime", api.createAnime)
	mux.HandleFunc("GET /api/anime/search", api.searchAnime)
	mux.HandleFunc("GET /api/anime/{bangumiID}", api.animeDetail)
	mux.HandleFunc("DELETE /api/anime/{bangumiID}", api.deleteAnime)
	mux.HandleFunc("POST /api/anime/{bangumiID}/refresh", api.refreshAnime)
	mux.HandleFunc("POST /api/anime/{bangumiID}/sync-history", api.syncAnimeHistory)
	mux.HandleFunc("POST /api/anime/{bangumiID}/storage", api.moveAnimeStorage)
	mux.HandleFunc("GET /api/anime/{bangumiID}/cover", api.animeCover)
	mux.HandleFunc("GET /api/anime/{bangumiID}/characters/{characterID}/image", api.characterImage)
	mux.HandleFunc("GET /api/actors/{actorID}/image", api.actorImage)
	mux.HandleFunc("GET /api/download/jobs", api.listDownloadJobs)
	mux.HandleFunc("POST /api/download/jobs/{jobID}/retry", api.retryDownloadJob)
	mux.HandleFunc("GET /api/media/jobs", api.listMediaJobs)
	mux.HandleFunc("POST /api/media/jobs/{jobID}/retry", api.retryMediaJob)
	mux.HandleFunc("GET /api/subscription/items", api.listSubscriptionItems)
	mux.HandleFunc("POST /api/subscription/items/{itemID}/confirm", api.confirmSubscriptionBinding)
	mux.HandleFunc("PUT /api/subscription/items/{itemID}/binding", api.bindSubscriptionItem)
	mux.HandleFunc("POST /api/subscription/items/{itemID}/ignore", api.ignoreSubscriptionItem)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "API endpoint not found")
	})
	mux.Handle("/", SPA(webDir))
	return CommonMiddleware(logger, mux)
}

type dashboardOverview struct {
	Subscription struct {
		PendingBindings int `json:"pendingBindings"`
	} `json:"subscription"`
	Download struct {
		Pending     int `json:"pending"`
		Downloading int `json:"downloading"`
		Failed      int `json:"failed"`
	} `json:"download"`
	Media struct {
		Pending     int `json:"pending"`
		Transcoding int `json:"transcoding"`
		Failed      int `json:"failed"`
	} `json:"media"`
	Storage struct {
		Roots []dashboardStorageRoot `json:"roots"`
	} `json:"storage"`
}

type dashboardStorageRoot struct {
	Label        string  `json:"label"`
	Path         string  `json:"path"`
	IsDefault    bool    `json:"isDefault"`
	Available    bool    `json:"available"`
	FreeBytes    uint64  `json:"freeBytes"`
	TotalBytes   uint64  `json:"totalBytes"`
	UsedBytes    uint64  `json:"usedBytes"`
	UsedPercent  float64 `json:"usedPercent"`
	ErrorMessage string  `json:"errorMessage"`
}

type mediaStorageSettingsResponse struct {
	DefaultRoot string   `json:"defaultRoot"`
	ExtraRoots  []string `json:"extraRoots"`
	UpdatedAt   int64    `json:"updatedAt"`
}

func (a *AdminAPI) dashboardOverview(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	var overview dashboardOverview
	var err error
	overview.Subscription.PendingBindings, err = a.subscription.CountItemsByBindingStatus(r.Context(), subscription.BindingStatusPending)
	if err != nil {
		a.internalError(w, err)
		return
	}
	overview.Download.Pending, err = a.download.CountJobsByStatus(r.Context(), download.StatusPending)
	if err != nil {
		a.internalError(w, err)
		return
	}
	overview.Download.Downloading, err = a.download.CountJobsByStatus(r.Context(), download.StatusDownloading)
	if err != nil {
		a.internalError(w, err)
		return
	}
	overview.Download.Failed, err = a.download.CountJobsByStatus(r.Context(), download.StatusFailed)
	if err != nil {
		a.internalError(w, err)
		return
	}
	mediaCounts, err := a.media.CountJobsByStatuses(
		r.Context(), media.StatusPending, media.StatusTranscoding, media.StatusFailed,
	)
	if err != nil {
		a.internalError(w, err)
		return
	}
	overview.Media.Pending = mediaCounts[media.StatusPending]
	overview.Media.Transcoding = mediaCounts[media.StatusTranscoding]
	overview.Media.Failed = mediaCounts[media.StatusFailed]
	overview.Storage.Roots, err = a.dashboardStorageRoots(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"overview": overview})
}

func (a *AdminAPI) getMediaStorageSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	settings, err := a.system.GetMediaStorageSettings(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": a.mediaStorageSettingsResponse(settings)})
}

func (a *AdminAPI) updateMediaStorageSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	var input struct {
		ExtraRoots []string `json:"extraRoots"`
	}
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	settings, err := a.system.UpdateMediaStorageSettings(r.Context(), input.ExtraRoots)
	if err != nil {
		if errors.Is(err, system.ErrInvalidMediaStoragePath) {
			writeError(w, http.StatusBadRequest, "invalid_media_storage_path", "额外磁盘存储路径必须是服务器上的绝对路径")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": a.mediaStorageSettingsResponse(settings)})
}

func (a *AdminAPI) mediaStorageSettingsResponse(settings system.MediaStorageSettings) mediaStorageSettingsResponse {
	return mediaStorageSettingsResponse{
		DefaultRoot: a.media.DefaultMediaDir(),
		ExtraRoots:  settings.ExtraRoots,
		UpdatedAt:   settings.UpdatedAt,
	}
}

func (a *AdminAPI) dashboardStorageRoots(ctx context.Context) ([]dashboardStorageRoot, error) {
	settings, err := a.system.GetMediaStorageSettings(ctx)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(settings.ExtraRoots)+1)
	paths = append(paths, a.media.DefaultMediaDir())
	for _, root := range settings.ExtraRoots {
		if storageRootAllowed(root, paths) {
			continue
		}
		paths = append(paths, root)
	}
	roots := make([]dashboardStorageRoot, 0, len(paths))
	for index, path := range paths {
		root := dashboardStorageRoot{
			Label:     "默认媒体目录",
			Path:      path,
			IsDefault: index == 0,
		}
		if index > 0 {
			root.Label = fmt.Sprintf("额外存储 %d", index)
		}
		stats, err := diskSpaceForPath(path)
		if err != nil {
			root.ErrorMessage = err.Error()
			roots = append(roots, root)
			continue
		}
		root.Available = true
		root.FreeBytes = stats.FreeBytes
		root.TotalBytes = stats.TotalBytes
		if stats.TotalBytes >= stats.FreeBytes {
			root.UsedBytes = stats.TotalBytes - stats.FreeBytes
		}
		if stats.TotalBytes > 0 {
			root.UsedPercent = float64(root.UsedBytes) / float64(stats.TotalBytes) * 100
		}
		roots = append(roots, root)
	}
	return roots, nil
}

func (a *AdminAPI) listSystemLogs(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	levels := queryLevels(r)
	if _, err := applog.NormalizeLevels(levels); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_level", "日志等级仅支持 INFO、WARNING、ERROR")
		return
	}
	entries, err := a.logs.ListExcludingSources(r.Context(), levels, hiddenSystemLogSources, applog.MaxInitialEntries)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": entries})
}

func (a *AdminAPI) streamSystemLogs(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	levels, err := applog.NormalizeLevels(queryLevels(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_level", "日志等级仅支持 INFO、WARNING、ERROR")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "stream_unsupported", "服务器不支持实时日志流")
		return
	}
	afterID, _ := strconv.ParseInt(r.URL.Query().Get("afterId"), 10, 64)
	allowed := make(map[string]struct{}, len(levels))
	for _, level := range levels {
		allowed[level] = struct{}{}
	}
	matches := func(entry applog.Entry) bool {
		if isHiddenSystemLogSource(entry.Source) {
			return false
		}
		_, exists := allowed[entry.Level]
		return len(allowed) == 0 || exists
	}

	updates, unsubscribe := a.logs.Subscribe(256)
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	missed, err := a.logs.ListAfterExcludingSources(r.Context(), levels, hiddenSystemLogSources, afterID, applog.MaxInitialEntries)
	if err != nil {
		return
	}
	for _, entry := range missed {
		if err := writeSSE(w, entry); err != nil {
			return
		}
	}
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case entry := <-updates:
			if !matches(entry) || entry.ID <= afterID {
				continue
			}
			if err := writeSSE(w, entry); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (a *AdminAPI) listAnime(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	result, err := a.catalog.List(r.Context(), page, pageSize)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *AdminAPI) createAnime(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	var input struct {
		BangumiID int64 `json:"bangumiId"`
	}
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.BangumiID < 1 {
		writeError(w, http.StatusBadRequest, "invalid_id", "Bangumi Subject ID 必须是正整数")
		return
	}
	if err := a.syncer.AddSubject(r.Context(), input.BangumiID); err != nil {
		a.animeSyncError(w, err)
		return
	}
	detail, err := a.catalog.Detail(r.Context(), input.BangumiID)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"anime": detail})
}

func (a *AdminAPI) searchAnime(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := a.catalog.Search(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *AdminAPI) animeDetail(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	detail, err := a.catalog.Detail(r.Context(), id)
	if err != nil {
		if errors.Is(err, bangumi.ErrAnimeNotFound) {
			writeError(w, http.StatusNotFound, "anime_not_found", "番剧不存在")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"anime": detail})
}

func (a *AdminAPI) refreshAnime(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	if err := a.syncer.RefreshSubject(r.Context(), id); err != nil {
		a.animeSyncError(w, err)
		return
	}
	detail, err := a.catalog.Detail(r.Context(), id)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"anime": detail})
}

func (a *AdminAPI) syncAnimeHistory(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	var input struct {
		RSSURL       string `json:"rssUrl"`
		ExcludeTitle string `json:"excludeTitle"`
		IncludeTitle string `json:"includeTitle"`
	}
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := a.subscription.SyncHistory(r.Context(), id, subscription.HistorySyncOptions{
		RSSURL: input.RSSURL, ExcludeTitle: input.ExcludeTitle, IncludeTitle: input.IncludeTitle,
	})
	if err != nil {
		a.subscriptionHistorySyncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (a *AdminAPI) moveAnimeStorage(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	var input struct {
		StorageRoot string `json:"storageRoot"`
	}
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	root, err := normalizeStorageRootInput(input.StorageRoot)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_media_storage_path", "目标存储路径必须是服务器上的绝对路径")
		return
	}
	allowed, err := a.allowedMediaStorageRoots(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	if !storageRootAllowed(root, allowed) {
		writeError(w, http.StatusBadRequest, "media_storage_path_not_configured", "目标存储路径需要先在系统设置中配置")
		return
	}
	result, err := a.media.MoveAnimeStorage(r.Context(), id, root)
	if err != nil {
		a.mediaStorageMoveError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (a *AdminAPI) deleteAnime(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	if err := a.catalog.Delete(r.Context(), id); err != nil {
		if errors.Is(err, bangumi.ErrAnimeNotFound) {
			writeError(w, http.StatusNotFound, "anime_not_found", "番剧不存在")
			return
		}
		a.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *AdminAPI) animeCover(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	a.serveCatalogImage(w, r, func() (string, error) { return a.catalog.AnimeImagePath(r.Context(), id) })
}

func (a *AdminAPI) characterImage(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	bangumiID, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	characterID, ok := parsePathID(w, r.PathValue("characterID"))
	if !ok {
		return
	}
	a.serveCatalogImage(w, r, func() (string, error) {
		return a.catalog.CharacterImagePath(r.Context(), bangumiID, characterID)
	})
}

func (a *AdminAPI) actorImage(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	actorID, ok := parsePathID(w, r.PathValue("actorID"))
	if !ok {
		return
	}
	a.serveCatalogImage(w, r, func() (string, error) { return a.catalog.ActorImagePath(r.Context(), actorID) })
}

func (a *AdminAPI) listSubscriptionItems(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	result, err := a.subscription.ListItems(r.Context(), page, pageSize, r.URL.Query().Get("bindingStatus"))
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *AdminAPI) confirmSubscriptionBinding(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("itemID"))
	if !ok {
		return
	}
	item, err := a.subscription.ConfirmBinding(r.Context(), id)
	if err != nil {
		a.subscriptionBindingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *AdminAPI) bindSubscriptionItem(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("itemID"))
	if !ok {
		return
	}
	var input struct {
		BangumiID     int64  `json:"bangumiId"`
		SeasonNumber  int    `json:"seasonNumber"`
		EpisodeType   string `json:"episodeType"`
		EpisodeNumber string `json:"episodeNumber"`
	}
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := a.subscription.BindManually(r.Context(), id, subscription.BindingInput{
		BangumiID: input.BangumiID, SeasonNumber: input.SeasonNumber,
		EpisodeType: input.EpisodeType, EpisodeNumber: input.EpisodeNumber,
	})
	if err != nil {
		a.subscriptionBindingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *AdminAPI) ignoreSubscriptionItem(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("itemID"))
	if !ok {
		return
	}
	item, err := a.subscription.IgnoreItem(r.Context(), id)
	if err != nil {
		a.subscriptionBindingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *AdminAPI) subscriptionHistorySyncError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, subscription.ErrHistorySourceNotFound):
		writeError(w, http.StatusConflict, "history_source_not_found", "该番剧没有可用于同步历史的已绑定话数")
	case errors.Is(err, subscription.ErrHistoryRSSURLRequired):
		writeError(w, http.StatusBadRequest, "history_rss_url_required", "没有已绑定话数时，番剧 RSS 链接必填")
	case errors.Is(err, subscription.ErrInvalidHistorySearch):
		writeError(w, http.StatusBadRequest, "invalid_history_search", "无法从最新绑定标题中生成历史话数搜索条件")
	case errors.Is(err, subscription.ErrInvalidHistoryRSSURL):
		writeError(w, http.StatusBadRequest, "invalid_history_rss_url", "番剧 RSS 链接必须是 HTTP/HTTPS 完整地址")
	case errors.Is(err, subscription.ErrInvalidBinding):
		writeError(w, http.StatusBadRequest, "invalid_subscription_binding", "绑定信息不完整或番剧不存在")
	default:
		a.internalError(w, err)
	}
}
func (a *AdminAPI) subscriptionBindingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, subscription.ErrItemNotFound):
		writeError(w, http.StatusNotFound, "subscription_item_not_found", "订阅条目不存在")
	case errors.Is(err, subscription.ErrInvalidBinding):
		writeError(w, http.StatusBadRequest, "invalid_subscription_binding", "绑定信息不完整或番剧不存在")
	default:
		a.internalError(w, err)
	}
}

func (a *AdminAPI) animeSyncError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bangumi.ErrAnimeNotFound):
		writeError(w, http.StatusNotFound, "anime_not_found", "番剧不存在")
	case errors.Is(err, bangumi.ErrAnimeAlreadyExists):
		writeError(w, http.StatusConflict, "anime_exists", "番剧已存在")
	case errors.Is(err, bangumi.ErrInvalidSubjectType):
		writeError(w, http.StatusBadRequest, "invalid_subject_type", "该 Bangumi Subject 不是动画条目")
	default:
		a.internalError(w, err)
	}
}

func (a *AdminAPI) mediaStorageMoveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, media.ErrAnimeNotFound):
		writeError(w, http.StatusNotFound, "anime_not_found", "番剧不存在")
	case errors.Is(err, media.ErrInvalidStorageRoot):
		writeError(w, http.StatusBadRequest, "invalid_media_storage_path", "目标存储路径必须是服务器上的绝对路径")
	case errors.Is(err, media.ErrAnimeTranscoding):
		writeError(w, http.StatusConflict, "anime_transcoding", "该番剧有转码中的任务，暂不能移动存储路径")
	case errors.Is(err, media.ErrStorageTargetConflict):
		writeError(w, http.StatusConflict, "media_storage_target_conflict", "目标路径已存在同名文件，请先处理后再移动")
	default:
		a.internalError(w, err)
	}
}

func (a *AdminAPI) allowedMediaStorageRoots(ctx context.Context) ([]string, error) {
	settings, err := a.system.GetMediaStorageSettings(ctx)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(settings.ExtraRoots)+1)
	roots = append(roots, a.media.DefaultMediaDir())
	roots = append(roots, settings.ExtraRoots...)
	return roots, nil
}

func normalizeStorageRootInput(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("empty storage root")
	}
	value = filepath.Clean(value)
	if !filepath.IsAbs(value) {
		return "", errors.New("relative storage root")
	}
	return value, nil
}

func storageRootAllowed(root string, allowed []string) bool {
	for _, candidate := range allowed {
		if sameStorageRoot(root, candidate) {
			return true
		}
	}
	return false
}

func sameStorageRoot(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (a *AdminAPI) serveCatalogImage(w http.ResponseWriter, r *http.Request, resolve func() (string, error)) {
	path, err := resolve()
	if err != nil {
		if errors.Is(err, bangumi.ErrAnimeNotFound) {
			http.NotFound(w, r)
			return
		}
		a.internalError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, path)
}

func queryLevels(r *http.Request) []string {
	values := r.URL.Query()["level"]
	if len(values) == 1 && strings.Contains(values[0], ",") {
		return strings.Split(values[0], ",")
	}
	return values
}

func isHiddenSystemLogSource(source string) bool {
	for _, hidden := range hiddenSystemLogSources {
		if strings.EqualFold(strings.TrimSpace(source), hidden) {
			return true
		}
	}
	return false
}

func parsePathID(w http.ResponseWriter, value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid_id", "ID 必须是正整数")
		return 0, false
	}
	return id, true
}

func writeSSE(w io.Writer, entry applog.Entry) error {
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", entry.ID, payload)
	return err
}

func NewViewerHandler(logger *slog.Logger, webDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", health)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "API endpoint not found")
	})
	mux.Handle("/", SPA(webDir))
	return CommonMiddleware(logger, mux)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *AdminAPI) setupStatus(w http.ResponseWriter, r *http.Request) {
	initialized, err := a.auth.IsInitialized(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"initialized": initialized})
}

func (a *AdminAPI) setup(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	user, session, err := a.auth.Setup(r.Context(), input.Username, input.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrAlreadyInitialized):
			writeError(w, http.StatusConflict, "already_initialized", "管理员账号已经创建")
		case errors.Is(err, auth.ErrInvalidUsername), errors.Is(err, auth.ErrInvalidPassword):
			writeError(w, http.StatusBadRequest, "invalid_credentials", err.Error())
		default:
			a.internalError(w, err)
		}
		return
	}
	a.setSessionCookie(w, session)
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (a *AdminAPI) login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	user, session, err := a.auth.Login(r.Context(), input.Username, input.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
			return
		}
		a.internalError(w, err)
		return
	}
	a.setSessionCookie(w, session)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (a *AdminAPI) me(w http.ResponseWriter, r *http.Request) {
	token := readSessionToken(r)
	user, err := a.auth.Authenticate(r.Context(), token)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (a *AdminAPI) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.auth.Logout(r.Context(), readSessionToken(r)); err != nil {
		a.internalError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: adminSessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *AdminAPI) listScheduledTasks(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	tasks, err := a.system.ListScheduledTasks(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (a *AdminAPI) updateScheduledTask(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	var input struct {
		Enabled         *bool `json:"enabled"`
		IntervalMinutes *int  `json:"intervalMinutes"`
	}
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.Enabled == nil && input.IntervalMinutes == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "至少需要提供 enabled 或 intervalMinutes")
		return
	}
	task, err := a.system.UpdateScheduledTask(r.Context(), r.PathValue("taskKey"), system.TaskUpdate{
		Enabled: input.Enabled, IntervalMinutes: input.IntervalMinutes,
	})
	if err != nil {
		switch {
		case errors.Is(err, system.ErrTaskNotFound):
			writeError(w, http.StatusNotFound, "task_not_found", "计划任务不存在")
		case errors.Is(err, system.ErrInvalidInterval):
			writeError(w, http.StatusBadRequest, "invalid_interval", "执行间隔必须在 1 到 43200 分钟之间")
		default:
			a.internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": task})
}

func (a *AdminAPI) runScheduledTask(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	task, err := a.scheduler.Trigger(r.PathValue("taskKey"))
	if err != nil {
		switch {
		case errors.Is(err, system.ErrTaskNotFound):
			writeError(w, http.StatusNotFound, "task_not_found", "计划任务不存在")
		case errors.Is(err, system.ErrTaskAlreadyRunning):
			writeError(w, http.StatusConflict, "task_running", "计划任务正在执行中")
		case errors.Is(err, system.ErrExecutorNotFound):
			writeError(w, http.StatusServiceUnavailable, "executor_not_found", "计划任务执行器未注册")
		default:
			a.internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"task": task})
}

func (a *AdminAPI) getNetworkSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	settings, err := a.system.GetNetworkSettings(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *AdminAPI) updateNetworkSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	var input struct {
		HTTPProxy  string `json:"httpProxy"`
		HTTPSProxy string `json:"httpsProxy"`
	}
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	settings, err := a.system.UpdateNetworkSettings(r.Context(), input.HTTPProxy, input.HTTPSProxy)
	if err != nil {
		if errors.Is(err, system.ErrInvalidProxy) {
			writeError(w, http.StatusBadRequest, "invalid_proxy", "代理地址必须是完整的 HTTP 或 HTTPS URL")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *AdminAPI) getSubscriptionSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	settings, err := a.system.GetSubscriptionSettings(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *AdminAPI) updateSubscriptionSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	var input struct {
		RSSURL string `json:"rssUrl"`
	}
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	settings, err := a.system.UpdateSubscriptionSettings(r.Context(), input.RSSURL)
	if err != nil {
		if errors.Is(err, system.ErrInvalidRSSURL) {
			writeError(w, http.StatusBadRequest, "invalid_rss_url", "RSS 订阅地址必须是完整的 HTTP 或 HTTPS URL")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *AdminAPI) getDownloadSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	settings, err := a.system.GetDownloadSettings(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *AdminAPI) updateDownloadSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	var input system.DownloadSettings
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	settings, err := a.system.UpdateDownloadSettings(r.Context(), input)
	if err != nil {
		if errors.Is(err, system.ErrInvalidDownloadSettings) {
			writeError(w, http.StatusBadRequest, "invalid_download_settings", "下载设置不完整或超出允许范围")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *AdminAPI) testDownloadSettings(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	var input system.DownloadSettings
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := a.download.TestConnection(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadGateway, "download_connection_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (a *AdminAPI) listDownloadJobs(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	result, err := a.download.ListJobs(r.Context(), page, pageSize, r.URL.Query().Get("status"))
	if err != nil {
		if errors.Is(err, download.ErrInvalidStatus) {
			writeError(w, http.StatusBadRequest, "invalid_download_status", "下载状态筛选条件无效")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *AdminAPI) retryDownloadJob(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("jobID"))
	if !ok {
		return
	}
	result, err := a.download.RetryFailedJob(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, download.ErrDownloadJobNotFound):
			writeError(w, http.StatusNotFound, "download_job_not_found", "下载任务不存在")
		case errors.Is(err, download.ErrRetryNotAllowed):
			writeError(w, http.StatusConflict, "download_retry_not_allowed", "只有下载失败的任务可以重试")
		case errors.Is(err, download.ErrQBitUnavailable):
			writeError(w, http.StatusBadGateway, "download_qbit_failed", "qBittorrent 操作失败："+err.Error())
		default:
			a.internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (a *AdminAPI) listMediaJobs(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	result, err := a.media.ListJobs(r.Context(), page, pageSize, r.URL.Query().Get("status"))
	if err != nil {
		if errors.Is(err, media.ErrInvalidStatus) {
			writeError(w, http.StatusBadRequest, "invalid_media_status", "媒体处理状态筛选条件无效")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *AdminAPI) retryMediaJob(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("jobID"))
	if !ok {
		return
	}
	job, err := a.media.RetryFailedJob(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, media.ErrMediaJobNotFound):
			writeError(w, http.StatusNotFound, "media_job_not_found", "转码任务不存在")
		case errors.Is(err, media.ErrRetryNotAllowed):
			writeError(w, http.StatusConflict, "media_retry_not_allowed", "只有处理失败的转码任务可以重试")
		default:
			a.internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

func (a *AdminAPI) requireAdministrator(w http.ResponseWriter, r *http.Request) bool {
	user, err := a.auth.Authenticate(r.Context(), readSessionToken(r))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
			return false
		}
		a.internalError(w, err)
		return false
	}
	if !user.IsAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "需要管理员权限")
		return false
	}
	return true
}

func (a *AdminAPI) setSessionCookie(w http.ResponseWriter, session auth.Session) {
	http.SetCookie(w, &http.Cookie{
		Name: adminSessionCookie, Value: session.Token, Path: "/",
		Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds()),
		HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteStrictMode,
	})
}

func (a *AdminAPI) internalError(w http.ResponseWriter, err error) {
	a.logger.Error("admin API error", "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "服务器内部错误")
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func readSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(adminSessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
