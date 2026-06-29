package httpapi

import (
	"errors"
	"io"
	"net/http"

	"bangumipipeline.local/server/internal/viewer"
)

func (a *AdminAPI) listViewerFilterDimensions(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	items, err := a.viewer.ListFilterDimensions(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *AdminAPI) createViewerFilterDimension(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	input, ok := decodeFilterDimensionInput(w, r)
	if !ok {
		return
	}
	item, err := a.viewer.CreateFilterDimension(r.Context(), input)
	if err != nil {
		a.writeFilterDimensionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"item": item})
}

func (a *AdminAPI) updateViewerFilterDimension(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("dimensionID"))
	if !ok {
		return
	}
	input, ok := decodeFilterDimensionInput(w, r)
	if !ok {
		return
	}
	item, err := a.viewer.UpdateFilterDimension(r.Context(), id, input)
	if err != nil {
		a.writeFilterDimensionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *AdminAPI) deleteViewerFilterDimension(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("dimensionID"))
	if !ok {
		return
	}
	if err := a.viewer.DeleteFilterDimension(r.Context(), id); err != nil {
		a.writeFilterDimensionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeFilterDimensionInput(w http.ResponseWriter, r *http.Request) (viewer.FilterDimensionInput, bool) {
	var payload struct {
		Name      string   `json:"name"`
		SortOrder int      `json:"sortOrder"`
		Tags      []string `json:"tags"`
	}
	if err := decodeJSON(w, r, &payload); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return viewer.FilterDimensionInput{}, false
	}
	return viewer.FilterDimensionInput{
		Name: payload.Name, SortOrder: payload.SortOrder, Tags: payload.Tags,
	}, true
}

func (a *AdminAPI) writeFilterDimensionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, viewer.ErrFilterDimensionNotFound):
		writeError(w, http.StatusNotFound, "filter_dimension_not_found", "筛选维度不存在")
	case errors.Is(err, viewer.ErrFilterDimensionExists):
		writeError(w, http.StatusConflict, "filter_dimension_exists", "筛选维度名称已存在")
	case errors.Is(err, viewer.ErrTooManyFilterDimensions):
		writeError(w, http.StatusBadRequest, "too_many_filter_dimensions", "筛选维度最多配置 20 个")
	case errors.Is(err, viewer.ErrInvalidFilterDimension):
		writeError(w, http.StatusBadRequest, "invalid_filter_dimension", "维度名称和标签不能为空；每个维度最多配置 50 个标签")
	default:
		a.internalError(w, err)
	}
}
