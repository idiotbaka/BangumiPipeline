package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminDatabaseReadWorkloadBoundsAPIReads(t *testing.T) {
	t.Parallel()
	var deadline time.Time
	handler := adminDatabaseReadWorkload(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		deadline, ok = r.Context().Deadline()
		if !ok {
			t.Error("admin GET API request has no deadline")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	started := time.Now()
	request := httptest.NewRequest(http.MethodGet, "/api/anime", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	remaining := deadline.Sub(started)
	if remaining < adminReadRequestTimeout-time.Second || remaining > adminReadRequestTimeout+time.Second {
		t.Fatalf("admin deadline remaining = %v, want about %v", remaining, adminReadRequestTimeout)
	}
}

func TestAdminDatabaseReadWorkloadDoesNotExpireLogStream(t *testing.T) {
	t.Parallel()
	handler := adminDatabaseReadWorkload(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Deadline(); ok {
			t.Error("system log stream must not receive the normal admin read deadline")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/system-logs/stream", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestAdminDatabaseReadWorkloadDoesNotBoundMutations(t *testing.T) {
	t.Parallel()
	handler := adminDatabaseReadWorkload(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Deadline(); ok {
			t.Error("admin mutation must keep the caller deadline because it may perform bounded external work")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/anime/1/refresh", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}
