package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bangumipipeline.local/server/internal/applog"
	"bangumipipeline.local/server/internal/auth"
	"bangumipipeline.local/server/internal/bangumi"
	"bangumipipeline.local/server/internal/config"
	"bangumipipeline.local/server/internal/database"
	"bangumipipeline.local/server/internal/download"
	"bangumipipeline.local/server/internal/httpapi"
	"bangumipipeline.local/server/internal/media"
	"bangumipipeline.local/server/internal/opskip"
	"bangumipipeline.local/server/internal/subscription"
	"bangumipipeline.local/server/internal/system"
	"bangumipipeline.local/server/internal/translation"
	"bangumipipeline.local/server/internal/viewer"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	logService := applog.NewService(db)
	logger = slog.New(applog.NewHandler(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		logService,
	))

	authService := auth.NewService(db, cfg.SessionTTL)
	if err := authService.DeleteExpiredSessions(ctx); err != nil {
		return err
	}
	viewerAuthService := viewer.NewService(db, cfg.SessionTTL)
	if err := viewerAuthService.DeleteExpiredSessions(ctx); err != nil {
		return err
	}
	viewerPushService := viewer.NewPushService(db, logger, cfg.WebPushContactEmail)
	systemService := system.NewService(db)
	metadataSyncer := bangumi.NewSyncer(db, systemService, logger, bangumi.SyncerConfig{
		APIBaseURL: cfg.BangumiAPIURL, UserAgent: cfg.BangumiUserAgent, CoverDir: cfg.CoverDir,
		FFmpegPath: cfg.FFmpegPath, APIInterval: 2 * time.Second, RequestTimeout: 20 * time.Second,
	})
	subscriptionService := subscription.NewService(db, systemService, logger)
	downloadService := download.NewService(db, systemService, logger, download.Config{DownloadDir: cfg.DownloadDir})
	mediaService := media.NewService(db, logger, media.Config{
		MediaDir: cfg.MediaDir, FFmpegPath: cfg.FFmpegPath, FFprobePath: cfg.FFprobePath,
		DownloadCleaner: downloadService, MetadataRefresher: metadataSyncer, CompletionNotifier: viewerPushService,
	})
	opSkipService := opskip.NewService(db, logger, opskip.Config{
		FFmpegPath: cfg.FFmpegPath, FFprobePath: cfg.FFprobePath,
	})
	catalog := bangumi.NewCatalog(db, mediaService.DefaultMediaDir())
	translationService := translation.NewService(db, systemService, logger)
	scheduler := system.NewScheduler(systemService, logger, cfg.SchedulerPoll)
	scheduler.Register("bangumi-season-metadata", metadataSyncer)
	scheduler.Register(subscription.TaskKey, subscriptionService)
	scheduler.Register(download.TaskKey, downloadService)
	scheduler.Register(media.TaskKey, mediaService)
	scheduler.Register(opskip.TaskKey, opSkipService)
	scheduler.Register(translation.TaskKey, translationService)
	scheduler.Register(viewer.PushDeliveryTaskKey, viewerPushService)
	if err := scheduler.Start(ctx); err != nil {
		return err
	}

	adminServer := &http.Server{
		Addr: cfg.AdminAddr,
		Handler: httpapi.NewAdminHandler(
			authService, systemService, scheduler, logService, catalog,
			metadataSyncer, subscriptionService, downloadService, mediaService, translationService, viewerAuthService, logger, cfg.CookieSecure, cfg.AdminWebDir,
		),
		ReadHeaderTimeout: 5 * time.Second,
	}
	viewerServer := &http.Server{
		Addr:              cfg.ViewerAddr,
		Handler:           httpapi.NewViewerHandler(viewerAuthService, viewerPushService, catalog, logger, cfg.CookieSecure, cfg.ViewerWebDir),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 2)
	go serve(logger, "admin", adminServer, errCh)
	go serve(logger, "viewer", viewerServer, errCh)

	select {
	case <-ctx.Done():
		err = nil
	case err = <-errCh:
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownPeriod)
	defer cancel()
	if shutdownErr := adminServer.Shutdown(shutdownCtx); shutdownErr != nil && err == nil {
		err = shutdownErr
	}
	if shutdownErr := viewerServer.Shutdown(shutdownCtx); shutdownErr != nil && err == nil {
		err = shutdownErr
	}
	return err
}

func serve(logger *slog.Logger, name string, server *http.Server, errCh chan<- error) {
	logger.Info("server listening", "source", "server", "name", name, "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errCh <- err
	}
}
