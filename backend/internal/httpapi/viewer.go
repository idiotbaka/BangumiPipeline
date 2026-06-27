package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"bangumipipeline.local/server/internal/viewer"
)

const viewerSessionCookie = "bp_viewer_session"

type ViewerAPI struct {
	auth         *viewer.Service
	logger       *slog.Logger
	cookieSecure bool
}

func NewViewerHandler(authService *viewer.Service, logger *slog.Logger, cookieSecure bool, webDir string) http.Handler {
	api := &ViewerAPI{auth: authService, logger: logger, cookieSecure: cookieSecure}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", health)
	mux.HandleFunc("GET /api/site-settings", api.siteSettings)
	mux.HandleFunc("POST /api/auth/register", api.register)
	mux.HandleFunc("POST /api/auth/login", api.login)
	mux.HandleFunc("GET /api/auth/me", api.me)
	mux.HandleFunc("POST /api/auth/logout", api.logout)
	mux.HandleFunc("GET /favicon.png", api.favicon)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "API endpoint not found")
	})
	mux.Handle("/", SPA(webDir))
	return CommonMiddleware(logger, mux)
}

func (a *ViewerAPI) register(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := decodeJSON(w, r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	user, session, err := a.auth.Register(r.Context(), input.Username, input.Password)
	if err != nil {
		switch {
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
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
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
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
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
	cookie, err := r.Cookie(viewerSessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}
