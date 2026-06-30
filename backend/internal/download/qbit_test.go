package download

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchTorrentsReadsResponseLargerThanOldLimit(t *testing.T) {
	largeName := strings.Repeat("a", 4<<20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode([]qBitTorrent{{
			Hash:  "large",
			Name:  largeName,
			State: "downloading",
		}}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &qBitClient{client: server.Client()}
	torrents, err := client.fetchTorrents(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetchTorrents() returned error: %v", err)
	}
	if len(torrents) != 1 || torrents[0].Hash != "large" || torrents[0].Name != largeName {
		t.Fatalf("unexpected torrents: count=%d first=%+v", len(torrents), torrents[0])
	}
}

func TestFetchTorrentsDecodeErrorIncludesResponseDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"hash":"abc"`))
	}))
	defer server.Close()

	client := &qBitClient{client: server.Client()}
	_, err := client.fetchTorrents(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected JSON decode error")
	}
	message := err.Error()
	for _, want := range []string{
		"qBittorrent torrents info JSON decode failed",
		"bytes_read=",
		"content_type=",
		"body_start=",
		"body_end=",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error to contain %q, got %q", want, message)
		}
	}
}
