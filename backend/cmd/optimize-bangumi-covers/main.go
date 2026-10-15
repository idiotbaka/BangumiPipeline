package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"bangumipipeline.local/server/internal/config"
	"bangumipipeline.local/server/internal/imageutil"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg := config.Load()
	root := flag.String("root", cfg.CoverDir, "番剧封面目录（BP_COVER_DIR）")
	ffmpeg := flag.String("ffmpeg", cfg.FFmpegPath, "ffmpeg 路径（需要 libwebp）")
	apply := flag.Bool("apply", false, "生成优化副本；默认仅预览")
	force := flag.Bool("force", false, "重新生成已有优化副本")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "不支持位置参数，请使用 --root 指定封面目录")
		return 2
	}
	directory, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *apply {
		if _, err := exec.LookPath(*ffmpeg); err != nil {
			fmt.Fprintln(os.Stderr, "ffmpeg 不可用:", err)
			return 2
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Printf("封面目录: %s；apply=%t force=%t；最大宽度=%dpx 上限=%d 字节\n",
		directory, *apply, *force, imageutil.ViewerCoverMaxWidth, imageutil.ViewerCoverMaxBytes)
	candidates, created, skipped, failed := 0, 0, 0, 0
	var originalBytes, optimizedBytes int64
	for _, entry := range entries {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "转换已中止:", ctx.Err())
			return 1
		}
		// Metadata covers are top-level <Bangumi ID>.<image extension>.
		// Never recurse into characters, actors, smiles or avatar directories;
		// ignore symlinks, temporary files and generated sidecars.
		if !entry.Type().IsRegular() || !isCover(entry.Name()) {
			continue
		}
		original := filepath.Join(directory, entry.Name())
		if !*force && imageutil.ViewerCoverCurrent(original) {
			skipped++
			continue
		}
		candidates++
		if !*apply {
			fmt.Printf("待转换: %s -> %s\n", original, imageutil.ViewerCoverPath(original))
			continue
		}
		changed, err := imageutil.OptimizeViewerCover(ctx, *ffmpeg, original, *force)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "转换失败: %s: %v\n", original, err)
			continue
		}
		if !changed {
			skipped++
			continue
		}
		created++
		sourceInfo, sourceErr := os.Stat(original)
		targetInfo, targetErr := os.Stat(imageutil.ViewerCoverPath(original))
		if sourceErr == nil && targetErr == nil {
			originalBytes += sourceInfo.Size()
			optimizedBytes += targetInfo.Size()
			fmt.Printf("已转换: %s (%d -> %d 字节)\n", original, sourceInfo.Size(), targetInfo.Size())
		}
	}
	fmt.Printf("完成: 候选=%d 新增=%d 跳过=%d 失败=%d；新增副本对应原图=%d 字节，优化后=%d 字节\n",
		candidates, created, skipped, failed, originalBytes, optimizedBytes)
	if !*apply {
		fmt.Println("当前仅预览；添加 --apply 才会写入副本。")
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func isCover(name string) bool {
	ext := filepath.Ext(name)
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		return false
	}
	id, err := strconv.ParseInt(strings.TrimSuffix(name, ext), 10, 64)
	return err == nil && id > 0
}
