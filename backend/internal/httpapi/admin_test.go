package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/applog"
	"bangumipipeline.local/server/internal/auth"
	"bangumipipeline.local/server/internal/bangumi"
	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/download"
	"bangumipipeline.local/server/internal/httpapi"
	"bangumipipeline.local/server/internal/media"
	"bangumipipeline.local/server/internal/subscription"
	"bangumipipeline.local/server/internal/system"
	"bangumipipeline.local/server/internal/translation"
	"bangumipipeline.local/server/internal/viewer"
)

func TestAdministratorSetupAndLoginLifecycle(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	systemService := system.NewService(db)
	scheduler := system.NewScheduler(systemService, logger, time.Hour)
	scheduler.Register("bangumi-season-metadata", system.ExecutorFunc(func(context.Context) error { return nil }))
	subscriptionService := subscription.NewService(db, systemService, logger)
	scheduler.Register(subscription.TaskKey, subscriptionService)
	downloadService := download.NewService(db, systemService, logger, download.Config{DownloadDir: t.TempDir()})
	mediaService := media.NewService(db, logger, media.Config{MediaDir: t.TempDir(), FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"})
	translationService := translation.NewService(db, systemService, logger)
	scheduler.Register(download.TaskKey, downloadService)
	scheduler.Register(media.TaskKey, mediaService)
	scheduler.Register(translation.TaskKey, translationService)
	if err := scheduler.Start(ctx); err != nil {
		t.Fatal(err)
	}
	metadataSyncer := bangumi.NewSyncer(db, systemService, logger, bangumi.SyncerConfig{
		APIBaseURL: "http://127.0.0.1", UserAgent: "test/httpapi", CoverDir: t.TempDir(),
		APIInterval: time.Millisecond, RequestTimeout: time.Second,
	})
	handler := httpapi.NewAdminHandler(
		auth.NewService(db, time.Hour), systemService, scheduler,
		applog.NewService(db), bangumi.NewCatalog(db), metadataSyncer, subscriptionService, downloadService, mediaService, translationService, viewer.NewService(db, time.Hour), logger, false, t.TempDir(),
	)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	status := doJSON(t, client, http.MethodGet, server.URL+"/api/setup/status", nil)
	assertStatus(t, status, http.StatusOK)
	var setupStatus struct {
		Initialized bool `json:"initialized"`
	}
	decodeResponse(t, status, &setupStatus)
	if setupStatus.Initialized {
		t.Fatal("new database unexpectedly initialized")
	}

	setup := doJSON(t, client, http.MethodPost, server.URL+"/api/setup", map[string]string{
		"username": "administrator",
		"password": "a-secure-password",
	})
	assertStatus(t, setup, http.StatusCreated)
	setup.Body.Close()

	me := doJSON(t, client, http.MethodGet, server.URL+"/api/auth/me", nil)
	assertStatus(t, me, http.StatusOK)
	var mePayload struct {
		User auth.User `json:"user"`
	}
	decodeResponse(t, me, &mePayload)
	if mePayload.User.Username != "administrator" || !mePayload.User.IsAdmin {
		t.Fatalf("unexpected current user: %+v", mePayload.User)
	}

	tasks := doJSON(t, client, http.MethodGet, server.URL+"/api/scheduled-tasks", nil)
	assertStatus(t, tasks, http.StatusOK)
	var tasksPayload struct {
		Tasks []system.ScheduledTask `json:"tasks"`
	}
	decodeResponse(t, tasks, &tasksPayload)
	var metadataTask *system.ScheduledTask
	for index := range tasksPayload.Tasks {
		if tasksPayload.Tasks[index].Key == "bangumi-season-metadata" {
			metadataTask = &tasksPayload.Tasks[index]
		}
	}
	if metadataTask == nil || metadataTask.Enabled || metadataTask.IntervalMinutes != 15 {
		t.Fatalf("unexpected seeded tasks: %+v", tasksPayload.Tasks)
	}

	enableTask := doJSON(t, client, http.MethodPatch, server.URL+"/api/scheduled-tasks/bangumi-season-metadata", map[string]any{
		"enabled": true, "intervalMinutes": 30,
	})
	assertStatus(t, enableTask, http.StatusOK)
	var taskPayload struct {
		Task system.ScheduledTask `json:"task"`
	}
	decodeResponse(t, enableTask, &taskPayload)
	if !taskPayload.Task.Enabled || taskPayload.Task.IntervalMinutes != 30 || taskPayload.Task.NextRunAt == nil {
		t.Fatalf("scheduled task was not updated: %+v", taskPayload.Task)
	}

	runTask := doJSON(t, client, http.MethodPost, server.URL+"/api/scheduled-tasks/bangumi-season-metadata/run", map[string]string{})
	assertStatus(t, runTask, http.StatusAccepted)
	runTask.Body.Close()

	invalidProxy := doJSON(t, client, http.MethodPut, server.URL+"/api/settings/network", map[string]string{
		"httpProxy": "127.0.0.1:10808",
	})
	assertStatus(t, invalidProxy, http.StatusBadRequest)
	invalidProxy.Body.Close()

	updateSettings := doJSON(t, client, http.MethodPut, server.URL+"/api/settings/network", map[string]string{
		"httpProxy":  "http://127.0.0.1:10808",
		"httpsProxy": "http://127.0.0.1:10808",
	})
	assertStatus(t, updateSettings, http.StatusOK)
	var settingsPayload struct {
		Settings system.NetworkSettings `json:"settings"`
	}
	decodeResponse(t, updateSettings, &settingsPayload)
	if settingsPayload.Settings.HTTPProxy != "http://127.0.0.1:10808" || settingsPayload.Settings.HTTPSProxy != "http://127.0.0.1:10808" {
		t.Fatalf("unexpected network settings: %+v", settingsPayload.Settings)
	}

	duplicate := doJSON(t, client, http.MethodPost, server.URL+"/api/setup", map[string]string{
		"username": "other-admin",
		"password": "another-secure-password",
	})
	assertStatus(t, duplicate, http.StatusConflict)
	duplicate.Body.Close()

	logout := doJSON(t, client, http.MethodPost, server.URL+"/api/auth/logout", map[string]string{})
	assertStatus(t, logout, http.StatusNoContent)
	logout.Body.Close()

	unauthorized := doJSON(t, client, http.MethodGet, server.URL+"/api/auth/me", nil)
	assertStatus(t, unauthorized, http.StatusUnauthorized)
	unauthorized.Body.Close()
	unauthorizedTasks := doJSON(t, client, http.MethodGet, server.URL+"/api/scheduled-tasks", nil)
	assertStatus(t, unauthorizedTasks, http.StatusUnauthorized)
	unauthorizedTasks.Body.Close()

	login := doJSON(t, client, http.MethodPost, server.URL+"/api/auth/login", map[string]string{
		"username": "administrator",
		"password": "a-secure-password",
	})
	assertStatus(t, login, http.StatusOK)
	login.Body.Close()
}

func doJSON(t *testing.T, client *http.Client, method, url string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertStatus(t *testing.T, response *http.Response, expected int) {
	t.Helper()
	if response.StatusCode != expected {
		payload, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("expected status %d, got %d: %s", expected, response.StatusCode, payload)
	}
}

func decodeResponse(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
