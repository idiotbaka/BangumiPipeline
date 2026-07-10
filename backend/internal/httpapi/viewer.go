package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bangumipipeline.local/server/internal/bangumi"
	"bangumipipeline.local/server/internal/viewer"
)

const viewerSessionCookie = "bp_viewer_session"

type ViewerAPI struct {
	auth         *viewer.Service
	push         *viewer.PushService
	catalog      *bangumi.Catalog
	logger       *slog.Logger
	cookieSecure bool
}

func NewViewerHandler(authService *viewer.Service, pushService *viewer.PushService, catalog *bangumi.Catalog, logger *slog.Logger, cookieSecure bool, webDir string) http.Handler {
	api := &ViewerAPI{auth: authService, push: pushService, catalog: catalog, logger: logger, cookieSecure: cookieSecure}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", health)
	mux.HandleFunc("GET /api/site-settings", api.siteSettings)
	mux.HandleFunc("POST /api/auth/register", api.register)
	mux.HandleFunc("POST /api/auth/login", api.login)
	mux.HandleFunc("GET /api/auth/me", api.me)
	mux.HandleFunc("POST /api/auth/logout", api.logout)
	mux.HandleFunc("GET /api/push/config", api.pushConfig)
	mux.HandleFunc("POST /api/push/subscriptions", api.upsertPushSubscription)
	mux.HandleFunc("DELETE /api/push/subscriptions", api.removePushSubscription)
	mux.HandleFunc("GET /api/home", api.home)
	mux.HandleFunc("GET /api/anime-schedule", api.animeSchedule)
	mux.HandleFunc("GET /api/library/filters", api.libraryFilters)
	mux.HandleFunc("GET /api/library", api.animeLibrary)
	mux.HandleFunc("GET /api/watch-history", api.watchHistory)
	mux.HandleFunc("GET /api/follows", api.followedAnime)
	mux.HandleFunc("GET /api/carousels/{carouselID}/image", api.carouselImage)
	mux.HandleFunc("GET /api/anime/{bangumiID}/detail", api.animeDetail)
	mux.HandleFunc("GET /api/anime/{bangumiID}/cover", api.animeCover)
	mux.HandleFunc("PUT /api/anime/{bangumiID}/follow", api.updateAnimeFollow)
	mux.HandleFunc("GET /api/anime/{bangumiID}/media/{mediaID}/stream", api.animeMediaStream)
	mux.HandleFunc("GET /api/anime/{bangumiID}/media/{mediaID}/cover", api.animeMediaCover)
	mux.HandleFunc("PUT /api/anime/{bangumiID}/media/{mediaID}/progress", api.updateWatchProgress)
	mux.HandleFunc("GET /api/anime/{bangumiID}/characters/{characterID}/image", api.animeCharacterImage)
	mux.HandleFunc("GET /api/actors/{actorID}/image", api.animeActorImage)
	mux.HandleFunc("GET /favicon.png", api.favicon)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "API endpoint not found")
	})
	mux.Handle("/", SPA(webDir))
	return CommonMiddleware(logger, viewerCORSMiddleware(mux))
}

func (a *ViewerAPI) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		InviteCode string `json:"inviteCode"`
	}
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	user, session, err := a.auth.Register(r.Context(), input.Username, input.Password, input.InviteCode)
	if err != nil {
		switch {
		case errors.Is(err, viewer.ErrRegistrationClosed):
			writeError(w, http.StatusForbidden, "registration_closed", "当前暂未开放注册")
		case errors.Is(err, viewer.ErrInviteRequired):
			writeError(w, http.StatusBadRequest, "invite_required", "请填写邀请码")
		case errors.Is(err, viewer.ErrInvalidInviteCode):
			writeError(w, http.StatusBadRequest, "invalid_invite_code", "邀请码无效")
		case errors.Is(err, viewer.ErrInviteUsed):
			writeError(w, http.StatusConflict, "invite_used", "邀请码已被使用")
		case errors.Is(err, viewer.ErrUsernameTaken):
			writeError(w, http.StatusConflict, "username_taken", "用户名已被使用")
		case errors.Is(err, viewer.ErrInvalidUsername):
			writeError(w, http.StatusBadRequest, "invalid_username", "用户名需要 3 到 32 个可显示字符")
		case errors.Is(err, viewer.ErrInvalidPassword):
			writeError(w, http.StatusBadRequest, "invalid_password", "密码需要 10 到 128 个字符")
		default:
			a.internalError(w, err)
		}
		return
	}
	a.setSessionCookie(w, session)
	writeJSON(w, http.StatusCreated, map[string]any{
		"user": user, "token": session.Token, "expiresAt": session.ExpiresAt.Unix(),
	})
}

func (a *ViewerAPI) login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	user, session, err := a.auth.Login(r.Context(), input.Username, input.Password)
	if err != nil {
		switch {
		case errors.Is(err, viewer.ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		case errors.Is(err, viewer.ErrUserDisabled):
			writeError(w, http.StatusForbidden, "user_disabled", "账号已被禁用")
		default:
			a.internalError(w, err)
		}
		return
	}
	a.setSessionCookie(w, session)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": user, "token": session.Token, "expiresAt": session.ExpiresAt.Unix(),
	})
}

func (a *ViewerAPI) me(w http.ResponseWriter, r *http.Request) {
	user, err := a.auth.Authenticate(r.Context(), readViewerSessionToken(r))
	if err != nil {
		if errors.Is(err, viewer.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (a *ViewerAPI) home(w http.ResponseWriter, r *http.Request) {
	user, ok := a.authenticatedViewer(w, r)
	if !ok {
		return
	}
	home, err := a.catalog.ViewerHome(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	slides, err := a.auth.CarouselSlides(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	follows, err := a.auth.FollowedAnime(r.Context(), user.ID)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"home": map[string]any{
		"hotRecommendations": home.HotRecommendations,
		"recentUpdates":      home.RecentUpdates,
		"carouselSlides":     slides,
		"myFollows":          follows,
	}})
}

func (a *ViewerAPI) animeSchedule(w http.ResponseWriter, r *http.Request) {
	if !a.requireViewer(w, r) {
		return
	}
	year, month, ok := parseAnimeSeason(r.URL.Query().Get("season"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_season", "季度参数必须为 YYYY-MM，月份仅支持 01、04、07、10")
		return
	}
	schedule, err := a.catalog.ViewerSchedule(r.Context(), year, month)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedule": schedule})
}

func (a *ViewerAPI) libraryFilters(w http.ResponseWriter, r *http.Request) {
	if !a.requireViewer(w, r) {
		return
	}
	items, err := a.auth.ListFilterDimensions(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *ViewerAPI) animeLibrary(w http.ResponseWriter, r *http.Request) {
	if !a.requireViewer(w, r) {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_query", "搜索关键词不能超过 100 个字符")
		return
	}
	page, pageSize, err := parseViewerLibraryPage(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pagination", "分页参数无效")
		return
	}
	selections := make(map[int64][]string)
	rawFilters := r.URL.Query()["filter"]
	if len(rawFilters) > 1000 {
		writeError(w, http.StatusBadRequest, "invalid_filter", "筛选标签数量过多")
		return
	}
	for _, value := range rawFilters {
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 {
			writeError(w, http.StatusBadRequest, "invalid_filter", "筛选标签参数无效")
			return
		}
		dimensionID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || dimensionID < 1 || parts[1] == "" {
			writeError(w, http.StatusBadRequest, "invalid_filter", "筛选标签参数无效")
			return
		}
		selections[dimensionID] = append(selections[dimensionID], parts[1])
	}
	groups, err := a.auth.ResolveLibraryFilters(r.Context(), selections)
	if err != nil {
		if errors.Is(err, viewer.ErrInvalidLibraryFilter) {
			writeError(w, http.StatusBadRequest, "invalid_filter", "筛选标签不存在或已被管理员移除")
			return
		}
		a.internalError(w, err)
		return
	}
	library, err := a.catalog.ViewerLibraryPage(r.Context(), query, groups, page, pageSize)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"library": library})
}

func parseViewerLibraryPage(r *http.Request) (int, int, error) {
	pageValue := strings.TrimSpace(r.URL.Query().Get("page"))
	pageSizeValue := strings.TrimSpace(r.URL.Query().Get("page_size"))
	if pageValue == "" && pageSizeValue == "" {
		return 0, 0, nil
	}
	page := 1
	pageSize := 16
	var err error
	if pageValue != "" {
		page, err = strconv.Atoi(pageValue)
		if err != nil || page < 1 {
			return 0, 0, errors.New("invalid page")
		}
	}
	if pageSizeValue != "" {
		pageSize, err = strconv.Atoi(pageSizeValue)
		if err != nil || pageSize < 1 || pageSize > 100 {
			return 0, 0, errors.New("invalid page size")
		}
	}
	return page, pageSize, nil
}

func parseAnimeSeason(value string) (int, int, bool) {
	if value == "" {
		now := time.Now()
		return now.Year(), ((int(now.Month())-1)/3)*3 + 1, true
	}
	if len(value) != 7 || value[4] != '-' {
		return 0, 0, false
	}
	year, err := strconv.Atoi(value[:4])
	if err != nil || year < 1 || year > 9999 {
		return 0, 0, false
	}
	month, err := strconv.Atoi(value[5:])
	if err != nil || (month != 1 && month != 4 && month != 7 && month != 10) {
		return 0, 0, false
	}
	return year, month, true
}

func (a *ViewerAPI) carouselImage(w http.ResponseWriter, r *http.Request) {
	if !a.requireViewer(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("carouselID"))
	if !ok {
		return
	}
	image, err := a.auth.CarouselImage(r.Context(), id)
	if err != nil {
		if errors.Is(err, viewer.ErrCarouselNotFound) {
			http.NotFound(w, r)
			return
		}
		a.internalError(w, err)
		return
	}
	writeCarouselImage(w, image, "private, max-age=86400")
}

func (a *ViewerAPI) animeCover(w http.ResponseWriter, r *http.Request) {
	if !a.requireViewer(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	a.serveCatalogImage(w, r, func() (string, error) { return a.catalog.AnimeImagePath(r.Context(), id) })
}

func (a *ViewerAPI) animeDetail(w http.ResponseWriter, r *http.Request) {
	user, ok := a.authenticatedViewer(w, r)
	if !ok {
		return
	}
	id, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	detail, err := a.catalog.ViewerAnimeDetail(r.Context(), id)
	if err != nil {
		if errors.Is(err, bangumi.ErrAnimeNotFound) {
			writeError(w, http.StatusNotFound, "anime_not_found", "番剧不存在")
			return
		}
		a.internalError(w, err)
		return
	}
	progress, err := a.auth.LastWatchProgress(r.Context(), user.ID, id)
	if err != nil {
		a.internalError(w, err)
		return
	}
	followed, err := a.auth.IsAnimeFollowed(r.Context(), user.ID, id)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"anime": detail, "watchProgress": progress, "followed": followed,
	})
}

func (a *ViewerAPI) followedAnime(w http.ResponseWriter, r *http.Request) {
	user, ok := a.authenticatedViewer(w, r)
	if !ok {
		return
	}
	items, err := a.auth.FollowedAnime(r.Context(), user.ID)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *ViewerAPI) updateAnimeFollow(w http.ResponseWriter, r *http.Request) {
	user, ok := a.authenticatedViewer(w, r)
	if !ok {
		return
	}
	bangumiID, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	var input struct {
		Followed bool `json:"followed"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	followed, err := a.auth.SetAnimeFollow(r.Context(), user.ID, bangumiID, input.Followed)
	if err != nil {
		if errors.Is(err, viewer.ErrFollowAnimeNotFound) {
			writeError(w, http.StatusNotFound, "anime_not_found", "番剧不存在")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"followed": followed})
}

func (a *ViewerAPI) pushConfig(w http.ResponseWriter, r *http.Request) {
	if !a.requireViewer(w, r) {
		return
	}
	config, err := a.push.Config(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": config})
}

func (a *ViewerAPI) upsertPushSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := a.authenticatedViewer(w, r)
	if !ok {
		return
	}
	var input viewer.PushSubscriptionInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_push_subscription", "浏览器推送订阅信息无效")
		return
	}
	if err := a.push.UpsertSubscription(r.Context(), user.ID, input); err != nil {
		if errors.Is(err, viewer.ErrInvalidPushSubscription) {
			writeError(w, http.StatusBadRequest, "invalid_push_subscription", "浏览器推送订阅信息无效")
			return
		}
		a.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *ViewerAPI) removePushSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := a.authenticatedViewer(w, r)
	if !ok {
		return
	}
	var input struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decodeJSON(w, r, &input); err != nil || strings.TrimSpace(input.Endpoint) == "" {
		writeError(w, http.StatusBadRequest, "invalid_push_subscription", "浏览器推送订阅信息无效")
		return
	}
	if err := a.push.RemoveSubscription(r.Context(), user.ID, input.Endpoint); err != nil {
		a.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *ViewerAPI) watchHistory(w http.ResponseWriter, r *http.Request) {
	user, ok := a.authenticatedViewer(w, r)
	if !ok {
		return
	}
	items, err := a.auth.WatchHistory(r.Context(), user.ID, 100)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *ViewerAPI) updateWatchProgress(w http.ResponseWriter, r *http.Request) {
	user, ok := a.authenticatedViewer(w, r)
	if !ok {
		return
	}
	bangumiID, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	mediaID, ok := parsePathID(w, r.PathValue("mediaID"))
	if !ok {
		return
	}
	var input struct {
		PositionSeconds float64 `json:"positionSeconds"`
		DurationSeconds float64 `json:"durationSeconds"`
	}
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	progress, err := a.auth.RecordWatchProgress(r.Context(), user.ID, viewer.WatchProgressInput{
		BangumiID: bangumiID, MediaID: mediaID,
		PositionSeconds: input.PositionSeconds, DurationSeconds: input.DurationSeconds,
	})
	if err != nil {
		if errors.Is(err, viewer.ErrWatchMediaNotFound) {
			writeError(w, http.StatusBadRequest, "invalid_watch_progress", "播放进度或媒体信息无效")
			return
		}
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"progress": progress})
}

func (a *ViewerAPI) animeMediaStream(w http.ResponseWriter, r *http.Request) {
	if !a.requireViewer(w, r) {
		return
	}
	bangumiID, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	mediaID, ok := parsePathID(w, r.PathValue("mediaID"))
	if !ok {
		return
	}
	path, err := a.catalog.ViewerMediaPath(r.Context(), bangumiID, mediaID)
	if err != nil {
		if errors.Is(err, bangumi.ErrAnimeNotFound) {
			http.NotFound(w, r)
			return
		}
		a.internalError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, path)
}

func (a *ViewerAPI) animeMediaCover(w http.ResponseWriter, r *http.Request) {
	if !a.requireViewer(w, r) {
		return
	}
	bangumiID, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	mediaID, ok := parsePathID(w, r.PathValue("mediaID"))
	if !ok {
		return
	}
	a.serveCatalogImage(w, r, func() (string, error) {
		return a.catalog.ViewerMediaCoverPath(r.Context(), bangumiID, mediaID)
	})
}

func (a *ViewerAPI) animeCharacterImage(w http.ResponseWriter, r *http.Request) {
	if !a.requireViewer(w, r) {
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

func (a *ViewerAPI) animeActorImage(w http.ResponseWriter, r *http.Request) {
	if !a.requireViewer(w, r) {
		return
	}
	actorID, ok := parsePathID(w, r.PathValue("actorID"))
	if !ok {
		return
	}
	a.serveCatalogImage(w, r, func() (string, error) {
		return a.catalog.ActorImagePath(r.Context(), actorID)
	})
}

func (a *ViewerAPI) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.auth.Logout(r.Context(), readViewerSessionToken(r)); err != nil {
		a.internalError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: viewerSessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *ViewerAPI) siteSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.auth.SiteSettings(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *ViewerAPI) favicon(w http.ResponseWriter, r *http.Request) {
	data, updatedAt, ok, err := a.auth.Favicon(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if updatedAt > 0 {
		w.Header().Set("Last-Modified", time.Unix(updatedAt, 0).UTC().Format(http.TimeFormat))
	}
	_, _ = w.Write(data)
}

func (a *ViewerAPI) requireViewer(w http.ResponseWriter, r *http.Request) bool {
	_, ok := a.authenticatedViewer(w, r)
	return ok
}

func (a *ViewerAPI) authenticatedViewer(w http.ResponseWriter, r *http.Request) (viewer.User, bool) {
	user, err := a.auth.Authenticate(r.Context(), readViewerSessionToken(r))
	if err != nil {
		if errors.Is(err, viewer.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
			return viewer.User{}, false
		}
		a.internalError(w, err)
		return viewer.User{}, false
	}
	return user, true
}

func (a *ViewerAPI) serveCatalogImage(w http.ResponseWriter, r *http.Request, resolve func() (string, error)) {
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

func (a *ViewerAPI) setSessionCookie(w http.ResponseWriter, session viewer.Session) {
	http.SetCookie(w, &http.Cookie{
		Name: viewerSessionCookie, Value: session.Token, Path: "/",
		Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds()),
		HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteStrictMode,
	})
}

func (a *ViewerAPI) internalError(w http.ResponseWriter, err error) {
	a.logger.Error("viewer API error", "source", "viewer", "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "服务器内部错误")
}

func readViewerSessionToken(r *http.Request) string {
	if authorization := strings.TrimSpace(r.Header.Get("Authorization")); authorization != "" {
		scheme, token, ok := strings.Cut(authorization, " ")
		if ok && strings.EqualFold(strings.TrimSpace(scheme), "Bearer") {
			return strings.TrimSpace(token)
		}
	}
	cookie, err := r.Cookie(viewerSessionCookie)
	if err == nil {
		return cookie.Value
	}
	if token := strings.TrimSpace(r.URL.Query().Get("viewer_token")); token != "" {
		return token
	}
	return ""
}

func viewerCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions && strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
