package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bangumipipeline.local/server/internal/bangumi"
	"bangumipipeline.local/server/internal/system"
)

func main() {
	output := flag.String("output", "../data/images/bangumi/smiles", "Bangumi 表情资源输出目录")
	httpProxy := flag.String("http-proxy", "", "可选 HTTP 代理 URL")
	httpsProxy := flag.String("https-proxy", "", "可选 HTTPS 代理 URL")
	userAgent := flag.String("user-agent", "private-user/BangumiPipeline-smiles/0.1", "可识别的请求 User-Agent")
	timeout := flag.Duration("timeout", 20*time.Second, "单个资源请求超时")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store := bangumi.NewBangumiSmileStore(logger, bangumi.BangumiSmileSyncConfig{
		Directory: *output, UserAgent: *userAgent, RequestTimeout: *timeout,
	})
	result, err := store.Ensure(ctx, system.NetworkSettings{HTTPProxy: *httpProxy, HTTPSProxy: *httpsProxy})
	if err != nil {
		logger.Error("Bangumi 表情资源同步失败", "source", "bangumi", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Bangumi 表情资源已就绪：%d/%d（新下载 %d，复用 %d）\n", result.Available, result.Expected, result.Downloaded, result.Cached)
}
