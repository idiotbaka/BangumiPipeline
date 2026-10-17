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

func (a *AdminAPI) translationContent(w http.ResponseWriter, r *http.Request) {
	a.handleTranslationContent(w, r, false)
}

func (a *AdminAPI) updateTranslation(w http.ResponseWriter, r *http.Request) {
	a.handleTranslationContent(w, r, true)
}

func (a *AdminAPI) handleTranslationContent(w http.ResponseWriter, r *http.Request, update bool) {
	if !a.requireAdministrator(w, r) {
		return
	}
	bangumiID, ok := parsePathID(w, r.PathValue("bangumiID"))
	if !ok {
		return
	}
	id, ok := parsePathID(w, r.PathValue("targetID"))
	if !ok {
		return
	}
	var item translation.TranslationContent
	var err error
	if update {
		var input translation.UpdateTranslationInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "请求内容无效")
			return
		}
		item, err = a.translation.UpdateTranslation(r.Context(), bangumiID, r.PathValue("kind"), id, input)
	} else {
		item, err = a.translation.TranslationContent(r.Context(), bangumiID, r.PathValue("kind"), id)
	}
	if err != nil {
		a.progressError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
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
	case errors.Is(err, translation.ErrTranslationNotFound):
		writeError(w, http.StatusNotFound, "translation_not_found", "翻译项目不存在或已不属于该番剧")
	case errors.Is(err, translation.ErrTranslationConflict):
		writeError(w, http.StatusConflict, "translation_conflict", "原文或译文已发生变化，请重新加载后编辑")
	case errors.Is(err, translation.ErrTranslationNoSource):
		writeError(w, http.StatusBadRequest, "translation_no_source", "该项目没有原文，无法编辑翻译")
	case errors.Is(err, translation.ErrInvalidTranslation):
		writeError(w, http.StatusBadRequest, "invalid_translation", "中文译文不能为空，且必须提供编辑时的原文和译文")
	case errors.Is(err, translation.ErrInvalidTranslationKind):
		writeError(w, http.StatusBadRequest, "invalid_translation_kind", "翻译项目类别无效")
	case errors.Is(err, translation.ErrInvalidProgressStatus), errors.Is(err, opskip.ErrInvalidProgressStatus):
		writeError(w, http.StatusBadRequest, "invalid_progress_status", "进度筛选状态无效")
	default:
		a.internalError(w, err)
	}
}
