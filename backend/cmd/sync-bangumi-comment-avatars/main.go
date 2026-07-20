package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"bangumipipeline.local/server/internal/bangumi"
	"bangumipipeline.local/server/internal/config"
	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/system"
)

func main() {
	cfg := config.Load()
	databasePath := flag.String("database", cfg.DatabasePath, "BangumiPipeline SQLite 数据库路径")
	output := flag.String("output", filepath.Join(cfg.CoverDir, "avatar"), "Bangumi 评论用户头像输出目录")
	userAgent := flag.String("user-agent", cfg.BangumiUserAgent, "可识别的请求 User-Agent")
	timeout := flag.Duration("timeout", 10*time.Second, "单个头像请求超时")
	batchSize := flag.Int("batch", 50, "每批串行下载的头像数量")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if *batchSize < 1 || *batchSize > 500 {
		logger.Error("batch 必须在 1 到 500 之间", "source", "bangumi")
		os.Exit(2)
	}
	if info, err := os.Stat(*databasePath); err != nil || info.IsDir() {
		logger.Error("数据库文件不存在，请通过 --database 指定生产数据库", "source", "bangumi", "path", *databasePath, "error", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := database.Open(ctx, *databasePath)
	if err != nil {
		logger.Error("打开数据库失败", "source", "database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	settingsService := system.NewService(db)
	network, err := settingsService.GetNetworkSettings(ctx)
	if err != nil {
		logger.Error("读取系统代理设置失败", "source", "bangumi", "error", err)
		os.Exit(1)
	}
	store := bangumi.NewBangumiCommentAvatarStore(db, logger, bangumi.BangumiCommentAvatarSyncConfig{
		Directory: *output, UserAgent: *userAgent, RequestTimeout: *timeout,
	})
	discovered, err := store.EnqueueHistorical(ctx)
	if err != nil {
		logger.Error("扫描历史评论头像失败", "source", "bangumi", "error", err)
		os.Exit(1)
	}
	missing, err := store.RequeueMissingFiles(ctx)
	if err != nil {
		logger.Error("校验本地评论头像失败", "source", "bangumi", "error", err)
		os.Exit(1)
	}
	retried, err := store.RetryFailuresNow(ctx)
	if err != nil {
		logger.Error("重置头像重试状态失败", "source", "bangumi", "error", err)
		os.Exit(1)
	}

	total := bangumi.BangumiCommentAvatarSyncResult{}
	hadFailures := false
	for batches := 0; batches < discovered+int(retried)+100; batches++ {
		result, runErr := store.SyncPending(ctx, network, *batchSize)
		total.Due += result.Due
		total.Downloaded += result.Downloaded
		total.Cached += result.Cached
		total.NotFound += result.NotFound
		total.Failed += result.Failed
		if runErr != nil {
			hadFailures = true
			logger.Warn("当前批次存在头像下载失败，已记录退避重试", "source", "bangumi", "error", runErr)
		}
		if result.Due == 0 {
			break
		}
		if err := ctx.Err(); err != nil {
			logger.Error("头像补全已中止", "source", "bangumi", "error", err)
			os.Exit(1)
		}
	}

	fmt.Printf("历史评论头像同步完成：发现 %d 位用户，修复缺失文件 %d，重试 %d；下载 %d，复用 %d，404 %d，失败 %d\n",
		discovered, missing, retried, total.Downloaded, total.Cached, total.NotFound, total.Failed)
	if hadFailures {
		os.Exit(1)
	}
}
