package httpapi

import (
	"net/http"
	"time"
)

func (a *AdminAPI) dashboardPipeline(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	overview, err := a.system.DashboardPipeline(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"overview": overview})
}

func (a *AdminAPI) dashboardStorage(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	roots, err := a.dashboardStorageRoots(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"storage": map[string]any{"roots": roots}})
}

func (a *AdminAPI) dashboardActivities(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdministrator(w, r) {
		return
	}
	items, err := a.viewer.RecentActivities(r.Context())
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "generatedAt": time.Now().Unix()})
}
