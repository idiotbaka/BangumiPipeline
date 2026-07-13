package httpapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bangumipipeline.local/server/internal/viewer"
)

const (
	appReleaseMultipartMemory = 16 << 20
	appReleaseFormOverhead    = 1 << 20
)

func (a *AdminAPI) listAppReleases(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	items, err := a.viewer.ListAppReleases(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *AdminAPI) publishAppRelease(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	input, ok := readAppReleaseForm(w, r, true)
	if !ok {
		return
	}

	release, err := a.viewer.PublishAppRelease(r.Context(), input)
	if err != nil {
		a.writeAppReleaseError(w, err)
		return
	}
	a.logger.Info("app release published",
		"source", "viewer",
		"version", release.Version,
		"apk_bytes", release.APKSize,
	)
	writeJSON(w, http.StatusCreated, map[string]any{"release": release})
}

func (a *AdminAPI) updateAppRelease(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("releaseID"))
	if !ok {
		return
	}
	input, ok := readAppReleaseForm(w, r, false)
	if !ok {
		return
	}

	release, err := a.viewer.UpdateAppRelease(r.Context(), id, input)
	if err != nil {
		a.writeAppReleaseError(w, err)
		return
	}
	a.logger.Info("app release updated",
		"source", "viewer",
		"release_id", release.ID,
		"version", release.Version,
		"apk_replaced", input.APKData != nil,
	)
	writeJSON(w, http.StatusOK, map[string]any{"release": release})
}

func (a *AdminAPI) deleteAppRelease(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	id, ok := parsePathID(w, r.PathValue("releaseID"))
	if !ok {
		return
	}
	if err := a.viewer.DeleteAppRelease(r.Context(), id); err != nil {
		a.writeAppReleaseError(w, err)
		return
	}
	a.logger.Info("app release deleted", "source", "viewer", "release_id", id)
	w.WriteHeader(http.StatusNoContent)
}

func readAppReleaseForm(w http.ResponseWriter, r *http.Request, apkRequired bool) (viewer.AppReleaseInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, viewer.MaxAppAPKBytes+appReleaseFormOverhead)
	if err := r.ParseMultipartForm(appReleaseMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "版本表单无效或 APK 文件超过 256MiB")
		return viewer.AppReleaseInput{}, false
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	input := viewer.AppReleaseInput{
		Version:      r.FormValue("version"),
		ReleaseNotes: r.FormValue("releaseNotes"),
	}
	file, header, err := r.FormFile("file")
	if errors.Is(err, http.ErrMissingFile) && !apkRequired {
		return input, true
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_apk", "请上传 arm64-v8a 的 APK 文件")
		return viewer.AppReleaseInput{}, false
	}
	defer file.Close()
	if !strings.EqualFold(filepath.Ext(strings.TrimSpace(header.Filename)), ".apk") {
		writeError(w, http.StatusBadRequest, "invalid_apk", "仅支持上传 .apk 文件")
		return viewer.AppReleaseInput{}, false
	}
	apkData, err := io.ReadAll(io.LimitReader(file, viewer.MaxAppAPKBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_apk", "读取 APK 文件失败")
		return viewer.AppReleaseInput{}, false
	}
	input.APKData = apkData
	return input, true
}

func (a *AdminAPI) writeAppReleaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, viewer.ErrInvalidAppVersion):
		writeError(w, http.StatusBadRequest, "invalid_app_version", "版本号必须使用 major.minor.patch 格式，例如 1.1.0")
	case errors.Is(err, viewer.ErrInvalidAppReleaseNotes):
		writeError(w, http.StatusBadRequest, "invalid_release_notes", "更新日志需要填写 1 到 10000 个字符")
	case errors.Is(err, viewer.ErrInvalidAppAPK):
		writeError(w, http.StatusBadRequest, "invalid_apk", "APK 文件无效或超过 256MiB")
	case errors.Is(err, viewer.ErrAppReleaseVersionExists):
		writeError(w, http.StatusConflict, "app_version_exists", "该 APP 版本已经发布")
	case errors.Is(err, viewer.ErrAppReleaseNotFound):
		writeError(w, http.StatusNotFound, "app_release_not_found", "APP 版本不存在")
	default:
		a.internalError(w, err)
	}
}

func (a *ViewerAPI) latestAppRelease(w http.ResponseWriter, r *http.Request) {
	release, err := a.auth.LatestAppRelease(r.Context())
	if errors.Is(err, viewer.ErrAppReleaseNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"release": nil})
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, map[string]any{"release": release})
}

func (a *ViewerAPI) downloadAppRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r.PathValue("releaseID"))
	if !ok {
		return
	}
	file, err := a.auth.AppReleaseAPK(r.Context(), id)
	if errors.Is(err, viewer.ErrAppReleaseNotFound) {
		writeError(w, http.StatusNotFound, "app_release_not_found", "APP 版本不存在")
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}

	filename := "BakaVip2-" + file.Version + ".apk"
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+file.SHA256+`"`)
	http.ServeContent(w, r, filename, time.Unix(file.PublishedAt, 0), bytes.NewReader(file.Data))
}
