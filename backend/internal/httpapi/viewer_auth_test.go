package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/httpapi"
	"bangumipipeline.local/server/internal/viewer"
)

func TestViewerChangePasswordAPI(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "viewer-auth-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	auth := viewer.NewService(db, time.Hour)
	if _, err := auth.UpdateSiteSettings(ctx, viewer.SiteSettingsUpdate{
		SiteName: "Test Viewer", RegistrationEnabled: true, InviteRequired: false,
	}); err != nil {
		t.Fatal(err)
	}
	_, session, err := auth.Register(ctx, "viewer-user", "old-password", "")
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewViewerHandler(auth, nil, nil, logger, false, t.TempDir()))
	defer server.Close()

	requestPasswordChange := func(token, currentPassword, newPassword, confirmPassword string) *http.Response {
		t.Helper()
		body, err := json.Marshal(map[string]string{
			"currentPassword": currentPassword,
			"newPassword":     newPassword,
			"confirmPassword": confirmPassword,
		})
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest(http.MethodPut, server.URL+"/api/auth/password", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}

	unauthorized := requestPasswordChange("", "old-password", "new-password", "new-password")
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized password change status = %d", unauthorized.StatusCode)
	}

	wrongCurrent := requestPasswordChange(session.Token, "wrong-password", "new-password", "new-password")
	defer wrongCurrent.Body.Close()
	if wrongCurrent.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong current password status = %d", wrongCurrent.StatusCode)
	}
	var errorPayload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(wrongCurrent.Body).Decode(&errorPayload); err != nil {
		t.Fatal(err)
	}
	if errorPayload.Error.Code != "invalid_current_password" {
		t.Fatalf("unexpected error code %q", errorPayload.Error.Code)
	}

	success := requestPasswordChange(session.Token, "old-password", "new-password", "new-password")
	success.Body.Close()
	if success.StatusCode != http.StatusNoContent {
		t.Fatalf("successful password change status = %d", success.StatusCode)
	}
	if _, _, err := auth.Login(ctx, "viewer-user", "old-password"); err == nil {
		t.Fatal("expected old password login to fail")
	}
	if _, _, err := auth.Login(ctx, "viewer-user", "new-password"); err != nil {
		t.Fatalf("expected new password login to succeed: %v", err)
	}
}
