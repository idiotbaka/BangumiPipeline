package imageutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	ViewerCoverMaxWidth = 560
	ViewerCoverMaxBytes = 80_000
)

// Serialize image encoding, including concurrent manual metadata refreshes.
var viewerCoverSlot = make(chan struct{}, 1)

// ViewerCoverPath preserves the original extension to avoid collisions with
// legacy WebP originals. No database path or schema change is required.
func ViewerCoverPath(original string) string { return original + ".viewer.webp" }

func ViewerCoverCurrent(original string) bool {
	source, err := os.Stat(original)
	if err != nil || !source.Mode().IsRegular() {
		return false
	}
	path := ViewerCoverPath(original)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 12 || info.Size() > ViewerCoverMaxBytes || info.ModTime().Before(source.ModTime()) {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	var header [12]byte
	_, err = io.ReadFull(file, header[:])
	return err == nil && string(header[:4]) == "RIFF" && string(header[8:]) == "WEBP"
}

// OptimizeViewerCover atomically publishes a static lossy WebP, retaining the
// original on every failure. force is reserved for the offline repair command.
func OptimizeViewerCover(ctx context.Context, ffmpegPath, original string, force bool) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	select {
	case viewerCoverSlot <- struct{}{}:
		defer func() { <-viewerCoverSlot }()
	case <-ctx.Done():
		return false, ctx.Err()
	}
	if !force && ViewerCoverCurrent(original) {
		return false, nil
	}
	source, err := os.Stat(original)
	if err != nil {
		return false, err
	}
	if !source.Mode().IsRegular() || source.Size() == 0 {
		return false, fmt.Errorf("封面源文件无效: %s", original)
	}
	if strings.TrimSpace(ffmpegPath) == "" {
		ffmpegPath = "ffmpeg"
	}
	temporary, err := os.CreateTemp(filepath.Dir(original), ".viewer-cover-*.webp")
	if err != nil {
		return false, err
	}
	path := temporary.Name()
	defer os.Remove(path)
	if err := temporary.Close(); err != nil {
		return false, err
	}
	// Keep the highest quality that fits. Do not upscale small originals or
	// truncate files with -fs, which could publish an undecodable image.
	for _, quality := range []int{75, 65, 55, 45, 35, 25, 15, 5} {
		command := exec.CommandContext(ctx, ffmpegPath,
			"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-filter_threads", "1",
			"-threads", "1", "-i", original, "-map", "0:v:0", "-frames:v", "1", "-an", "-sn",
			"-vf", fmt.Sprintf("scale='min(%d,iw)':-1:flags=lanczos", ViewerCoverMaxWidth),
			"-c:v", "libwebp", "-lossless", "0", "-preset", "picture",
			"-quality", fmt.Sprint(quality), "-compression_level", "6",
			"-threads", "1", "-map_metadata", "-1", "-f", "webp", path,
		)
		output, err := command.CombinedOutput()
		if err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			message := strings.TrimSpace(string(output))
			if len(message) > 2000 {
				message = message[len(message)-2000:]
			}
			return false, fmt.Errorf("ffmpeg 优化封面失败: %w: %s", err, message)
		}
		info, err := os.Stat(path)
		if err != nil {
			return false, err
		}
		if info.Size() < 12 {
			return false, fmt.Errorf("ffmpeg 输出封面为空或无效")
		}
		if info.Size() > ViewerCoverMaxBytes {
			continue
		}
		// Decode the complete candidate before exposing it to either client.
		validate := exec.CommandContext(ctx, ffmpegPath,
			"-hide_banner", "-loglevel", "error", "-nostdin", "-xerror",
			"-err_detect", "explode", "-threads", "1", "-i", path, "-frames:v", "1",
			"-threads", "1", "-f", "null", "-",
		)
		if output, err := validate.CombinedOutput(); err != nil {
			return false, fmt.Errorf("校验 WebP 封面失败: %w: %.2000s", err, output)
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if err := os.Chmod(path, 0o644); err != nil {
			return false, err
		}
		// Also handle imported originals with a future mtime.
		modified := time.Now()
		if source.ModTime().After(modified) {
			modified = source.ModTime()
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			return false, err
		}
		if err := os.Rename(path, ViewerCoverPath(original)); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("封面在宽度 %dpx、最低质量下仍超过 %d 字节", ViewerCoverMaxWidth, ViewerCoverMaxBytes)
}
