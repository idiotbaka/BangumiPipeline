package mediafile

import (
	"path/filepath"
	"strings"
)

// IsVideoPath reports whether path uses a media extension supported by the
// download-product discovery and local-history import flows.
func IsVideoPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".mkv", ".mov", ".avi", ".wmv", ".flv", ".ts", ".m2ts", ".webm":
		return true
	default:
		return false
	}
}
