package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bangumipipeline.local/server/internal/viewer"
)

const (
	maxCarouselImageBytes = 10 << 20
	maxCarouselFormBytes  = 12 << 20
)

func (a *AdminAPI) listViewerCarousels(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	items, err := a.viewer.ListCarouselItems(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *AdminAPI) createViewerCarousel(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	input, ok := parseCarouselMultipart(w, r, true)
	if !ok {
		return
	}
	item, err := a.viewer.CreateCarouselItem(r.Context(), input)
	if err != nil {
		a.writeCarouselError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"item": item})
}

func (a *AdminAPI) updateViewerCarousel(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("carouselID"))
	if !ok {
		return
	}
	input, ok := parseCarouselMultipart(w, r, false)
	if !ok {
		return
	}
	item, err := a.viewer.UpdateCarouselItem(r.Context(), id, input)
	if err != nil {
		a.writeCarouselError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *AdminAPI) deleteViewerCarousel(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("carouselID"))
	if !ok {
		return
	}
	if err := a.viewer.DeleteCarouselItem(r.Context(), id); err != nil {
		a.writeCarouselError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *AdminAPI) viewerCarouselImage(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("carouselID"))
	if !ok {
		return
	}
	image, err := a.viewer.CarouselImage(r.Context(), id)
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

func parseCarouselMultipart(w http.ResponseWriter, r *http.Request, imageRequired bool) (viewer.CarouselItemInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCarouselFormBytes)
	if err := r.ParseMultipartForm(maxCarouselFormBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "轮播图表单无效或文件过大")
		return viewer.CarouselItemInput{}, false
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	bangumiID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("bangumiId")), 10, 64)
	if err != nil || bangumiID < 1 {
		writeError(w, http.StatusBadRequest, "invalid_bangumi_id", "请选择要绑定的番剧")
		return viewer.CarouselItemInput{}, false
	}
	sortOrder := 0
	if raw := strings.TrimSpace(r.FormValue("sortOrder")); raw != "" {
		sortOrder, err = strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_sort_order", "排序必须是整数")
			return viewer.CarouselItemInput{}, false
		}
	}
	input := viewer.CarouselItemInput{BangumiID: bangumiID, SortOrder: sortOrder}
	file, _, err := r.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) && !imageRequired {
			return input, true
		}
		writeError(w, http.StatusBadRequest, "missing_image", "请上传轮播宽图")
		return viewer.CarouselItemInput{}, false
	}
	defer file.Close()
	input.ImageData, err = io.ReadAll(io.LimitReader(file, maxCarouselImageBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_image", "读取轮播图失败")
		return viewer.CarouselItemInput{}, false
	}
	return input, true
}

func (a *AdminAPI) writeCarouselError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, viewer.ErrCarouselNotFound):
		writeError(w, http.StatusNotFound, "carousel_not_found", "轮播图配置不存在")
	case errors.Is(err, viewer.ErrCarouselAnimeMissing):
		writeError(w, http.StatusBadRequest, "anime_not_found", "绑定的番剧不存在")
	case errors.Is(err, viewer.ErrCarouselAnimeExists):
		writeError(w, http.StatusConflict, "carousel_anime_exists", "该番剧已经配置了轮播图")
	case errors.Is(err, viewer.ErrInvalidCarouselImage):
		writeError(w, http.StatusBadRequest, "invalid_carousel_image", "轮播图必须是 10MiB 以内的横向 JPG 或 PNG 图片")
	default:
		a.internalError(w, err)
	}
}

func writeCarouselImage(w http.ResponseWriter, image viewer.CarouselImage, cacheControl string) {
	w.Header().Set("Content-Type", image.ContentType)
	w.Header().Set("Cache-Control", cacheControl)
	if image.UpdatedAt > 0 {
		w.Header().Set("Last-Modified", time.Unix(image.UpdatedAt, 0).UTC().Format(http.TimeFormat))
	}
	_, _ = w.Write(image.Data)
}
