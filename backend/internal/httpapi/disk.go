package httpapi

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type diskSpaceStats struct {
	FreeBytes  uint64
	TotalBytes uint64
}

func diskSpaceForPath(path string) (diskSpaceStats, error) {
	probePath, err := existingDiskProbePath(path)
	if err != nil {
		return diskSpaceStats{}, err
	}
	return diskSpace(probePath)
}

func existingDiskProbePath(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return "", errors.New("empty path")
	}
	for {
		info, err := os.Stat(path)
		if err == nil {
			if info.IsDir() {
				return path, nil
			}
			path = filepath.Dir(path)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		path = parent
	}
}
