package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadAppReleaseFormAllowsOmittedAPKWhenUpdating(t *testing.T) {
	request := newAppReleaseFormRequest(t, false)
	recorder := httptest.NewRecorder()

	input, ok := readAppReleaseForm(recorder, request, false)
	if !ok {
		t.Fatalf("expected update form to be accepted, status = %d", recorder.Code)
	}
	if input.Version != "1.2.0" || input.ReleaseNotes != "更新日志" || input.APKData != nil {
		t.Fatalf("unexpected update form: %#v", input)
	}
}

func TestReadAppReleaseFormRequiresAPKWhenPublishing(t *testing.T) {
	request := newAppReleaseFormRequest(t, false)
	recorder := httptest.NewRecorder()

	if _, ok := readAppReleaseForm(recorder, request, true); ok {
		t.Fatal("expected publish form without APK to be rejected")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestReadAppReleaseFormReadsReplacementAPK(t *testing.T) {
	request := newAppReleaseFormRequest(t, true)
	recorder := httptest.NewRecorder()

	input, ok := readAppReleaseForm(recorder, request, false)
	if !ok {
		t.Fatalf("expected replacement APK form to be accepted, status = %d", recorder.Code)
	}
	if string(input.APKData) != string([]byte{'P', 'K', 0x03, 0x04, 'a', 'p', 'k'}) {
		t.Fatalf("unexpected replacement APK: %q", input.APKData)
	}
}

func newAppReleaseFormRequest(t *testing.T, withAPK bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("version", "1.2.0"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("releaseNotes", "更新日志"); err != nil {
		t.Fatal(err)
	}
	if withAPK {
		file, err := writer.CreateFormFile("file", "BakaVip2-1.2.0.apk")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte{'P', 'K', 0x03, 0x04, 'a', 'p', 'k'}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPut, "/api/viewer/app-releases/1", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
