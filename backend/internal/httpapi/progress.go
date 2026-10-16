package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"bangumipipeline.local/server/internal/bangumi"
	"bangumipipeline.local/server/internal/opskip"
	"bangumipipeline.local/server/internal/translation"
)

func (a *AdminAPI) listTranslationProgress(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	result, err := a.translation.ListProgress(r.Context(), page, pageSize, r.URL.Query().Get("q"), r.URL.Query().Get("status"))
	if err != nil {
		a.progressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *AdminAPI) translationProgressDetails(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	items, err := a.translation.ProgressDetails(r.Context(), id)
	if err != nil {
		a.progressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *AdminAPI) listOpeningProgress(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	result, err := a.opskip.ListProgress(r.Context(), page, pageSize, r.URL.Query().Get("q"), r.URL.Query().Get("status"))
	if err != nil {
		a.progressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *AdminAPI) openingProgressDetails(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	items, err := a.opskip.ProgressDetails(r.Context(), id)
	if err != nil {
		a.progressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *AdminAPI) progressError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bangumi.ErrAnimeNotFound):
		writeError(w, http.StatusNotFound, "anime_not_found", "番剧不存在")
	case errors.Is(err, translation.ErrInvalidProgressStatus), errors.Is(err, opskip.ErrInvalidProgressStatus):
		writeError(w, http.StatusBadRequest, "invalid_progress_status", "进度筛选状态无效")
	default:
		a.internalError(w, err)
	}
}
