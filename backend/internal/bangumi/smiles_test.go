package bangumi

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/system"
)

func TestBangumiSmileDefinitionsAndCommentCodeMatching(t *testing.T) {
	definitions := bangumiSmileDefinitions(defaultBangumiWebBaseURL, defaultBangumiLainBaseURL)
	if len(definitions) != BangumiSmileAssetCount {
		t.Fatalf("expected %d smile definitions, got %d", BangumiSmileAssetCount, len(definitions))
	}
	byCode := make(map[string]bangumiSmileDefinition, len(definitions))
	for _, definition := range definitions {
		if _, duplicate := byCode[definition.Code]; duplicate {
			t.Fatalf("duplicate smile code %s", definition.Code)
		}
		byCode[definition.Code] = definition
	}
	assertSmileSource(t, byCode, "(bgm24)", "/img/smiles/tv/01.gif")
	assertSmileSource(t, byCode, "(bgm125)", "/img/smiles/tv/102.gif")
	assertSmileSource(t, byCode, "(bgm200)", "/img/smiles/tv_vs/bgm_200.png")
	assertSmileSource(t, byCode, "(bgm238)", "/img/smiles/tv_vs/bgm_238.png")
	assertSmileCandidates(t, byCode, "(bgm500)", ".png", ".gif")
	assertSmileCandidates(t, byCode, "(bgm01)", ".png", ".gif")
	assertSmileSource(t, byCode, "(musume_118)", "/img/smiles/musume/musume_118.gif")
	assertSmileSource(t, byCode, "(blake_01)", "/img/smiles/blake/blake_01.gif")
	if _, exists := byCode["(musume_97)"]; exists {
		t.Fatal("officially unimplemented musume_97 must not be downloaded")
	}
	if _, exists := byCode["(musume_98)"]; exists {
		t.Fatal("officially unimplemented musume_98 must not be downloaded")
	}
	if !IsBangumiSmileCode("(bgm24)") || IsBangumiSmileCode("(musume_97)") || IsBangumiSmileCode("(bgm999)") {
		t.Fatal("strict smile code lookup returned an unexpected result")
	}

	content := "前(bgm24)中(musume_06)(bgm126)(musume_97)(blake_118)后(bgm999)"
	matches := MatchBangumiSmileCodes(content)
	if len(matches) != 3 {
		t.Fatalf("expected three supported codes, got %+v", matches)
	}
	want := []string{"(bgm24)", "(musume_06)", "(blake_118)"}
	for index, match := range matches {
		if match.Code != want[index] || content[match.Start:match.End] != want[index] {
			t.Fatalf("unexpected smile match %+v at %d", match, index)
		}
	}
	unsafeManifest := BangumiSmileManifest{
		Version: BangumiSmileManifestVersion,
		Assets: map[string]BangumiSmileAsset{
			"(bgm24)": {Code: "(bgm24)", File: "..", ContentType: "image/gif"},
		},
	}
	if _, _, ok := unsafeManifest.Resolve(t.TempDir(), "(bgm24)"); ok {
		t.Fatal("manifest resolver accepted an unsafe filename")
	}
}

func TestBangumiSmileStoreDownloadsFallbackRetriesAndCaches(t *testing.T) {
	pngData := mustDecodeBase64(t, "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	gifData := mustDecodeBase64(t, "R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==")
	var requests atomic.Int32
	var transientRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/img/smiles/bgm/01.png" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/img/smiles/tv_500/bgm_529.png" && transientRequests.Add(1) == 1 {
			http.Error(w, "temporary", http.StatusInternalServerError)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".gif") {
			w.Header().Set("Content-Type", "image/gif")
			_, _ = w.Write(gifData)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".png") {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngData)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	directory := filepath.Join(t.TempDir(), "smiles")
	store := NewBangumiSmileStore(discardLogger(), BangumiSmileSyncConfig{
		Directory: directory, BangumiBaseURL: server.URL, LainBaseURL: server.URL,
		UserAgent: "test/BangumiPipeline-smiles", RequestTimeout: 2 * time.Second,
	})
	result, err := store.Ensure(context.Background(), system.NetworkSettings{})
	if err == nil {
		t.Fatal("expected the transient smile failure to make the first sync incomplete")
	}
	if result.Available != BangumiSmileAssetCount-1 || result.Complete {
		t.Fatalf("unexpected partial result: %+v", result)
	}
	partial, err := LoadBangumiSmileManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Complete || len(partial.Assets) != BangumiSmileAssetCount-1 {
		t.Fatalf("partial manifest was not preserved: complete=%v assets=%d", partial.Complete, len(partial.Assets))
	}
	firstRequestCount := requests.Load()

	result, err = store.Ensure(context.Background(), system.NetworkSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Available != BangumiSmileAssetCount || result.Downloaded != 1 || result.Cached != BangumiSmileAssetCount-1 {
		t.Fatalf("unexpected retry result: %+v", result)
	}
	if requests.Load() != firstRequestCount+1 {
		t.Fatalf("retry should request only the missing asset: before=%d after=%d", firstRequestCount, requests.Load())
	}

	manifest, err := LoadBangumiSmileManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedSmile(t, manifest, directory, "(bgm01)", "bgm01.gif", "image/gif")
	assertResolvedSmile(t, manifest, directory, "(bgm24)", "bgm24.gif", "image/gif")
	assertResolvedSmile(t, manifest, directory, "(bgm500)", "bgm500.png", "image/png")
	assertResolvedSmile(t, manifest, directory, "(musume_06)", "musume_06.gif", "image/gif")

	completeRequestCount := requests.Load()
	result, err = store.Ensure(context.Background(), system.NetworkSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Downloaded != 0 || result.Cached != BangumiSmileAssetCount || requests.Load() != completeRequestCount {
		t.Fatalf("complete cache unexpectedly used the network: result=%+v requests=%d", result, requests.Load())
	}

	if err := os.Remove(filepath.Join(directory, "bgm500.png")); err != nil {
		t.Fatal(err)
	}
	result, err = store.Ensure(context.Background(), system.NetworkSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Downloaded != 1 || requests.Load() != completeRequestCount+1 {
		t.Fatalf("missing local file was not repaired selectively: result=%+v requests=%d", result, requests.Load())
	}
}

func assertSmileSource(t *testing.T, definitions map[string]bangumiSmileDefinition, code, suffix string) {
	t.Helper()
	definition, ok := definitions[code]
	if !ok || len(definition.SourceURLs) == 0 || !strings.HasSuffix(definition.SourceURLs[0], suffix) {
		t.Fatalf("unexpected source mapping for %s: %+v", code, definition)
	}
}

func assertSmileCandidates(t *testing.T, definitions map[string]bangumiSmileDefinition, code string, extensions ...string) {
	t.Helper()
	definition, ok := definitions[code]
	if !ok || len(definition.SourceURLs) != len(extensions) {
		t.Fatalf("unexpected candidates for %s: %+v", code, definition)
	}
	for index, extension := range extensions {
		if !strings.HasSuffix(definition.SourceURLs[index], extension) {
			t.Fatalf("candidate %d for %s should end in %s: %s", index, code, extension, definition.SourceURLs[index])
		}
	}
}

func assertResolvedSmile(t *testing.T, manifest BangumiSmileManifest, directory, code, filename, contentType string) {
	t.Helper()
	asset, path, ok := manifest.Resolve(directory, code)
	if !ok || asset.File != filename || asset.ContentType != contentType {
		t.Fatalf("unexpected resolved asset for %s: asset=%+v ok=%v", code, asset, ok)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := io.Copy(io.Discard, file); err != nil {
		t.Fatal(err)
	}
}

func mustDecodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
