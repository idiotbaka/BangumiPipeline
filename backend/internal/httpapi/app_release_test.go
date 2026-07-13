package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/httpapi"
	"bangumipipeline.local/server/internal/viewer"
)

func TestPublicAppReleaseMetadataAndDownload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "app-release-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := viewer.NewService(db, time.Hour)
	apk := []byte{'P', 'K', 0x03, 0x04, 'a', 'p', 'k'}
	release, err := service.PublishAppRelease(ctx, viewer.AppReleaseInput{
		Version: "1.1.0", ReleaseNotes: "1. 新版本", APKData: apk,
	})
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewViewerHandler(service, nil, nil, logger, false, t.TempDir()))
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/api/app/releases/latest")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("latest release status = %d", response.StatusCode)
	}
	var payload struct {
		Release *viewer.AppRelease `json:"release"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Release == nil || payload.Release.ID != release.ID || payload.Release.Version != "1.1.0" || payload.Release.PublishedAt <= 0 {
		t.Fatalf("unexpected latest release: %#v", payload.Release)
	}

	download, err := server.Client().Get(server.URL + "/api/app/releases/" + stringID(release.ID) + "/download")
	if err != nil {
		t.Fatal(err)
	}
	defer download.Body.Close()
	if download.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d", download.StatusCode)
	}
	if got := download.Header.Get("Content-Disposition"); got != `attachment; filename="BakaVip2-1.1.0.apk"` {
		t.Fatalf("unexpected content disposition %q", got)
	}
	if got := download.Header.Get("Content-Type"); got != "application/vnd.android.package-archive" {
		t.Fatalf("unexpected content type %q", got)
	}
	data, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(apk) {
		t.Fatalf("unexpected APK response %q", data)
	}
}

func stringID(value int64) string {
	return strconv.FormatInt(value, 10)
}
