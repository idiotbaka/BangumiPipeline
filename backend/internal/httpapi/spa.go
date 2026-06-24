package httpapi

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func SPA(webDir string) http.Handler {
	absDir, err := filepath.Abs(webDir)
	if err != nil {
		absDir = webDir
	}
	fileSystem := os.DirFS(absDir)
	fileServer := http.FileServer(http.FS(fileSystem))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if relPath != "." && relPath != "" {
			if info, statErr := fs.Stat(fileSystem, relPath); statErr == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		index, readErr := fs.ReadFile(fileSystem, "index.html")
		if readErr != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "Frontend is not built. Run `npm run build:ui`.\nExpected: %s\n", absDir)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
