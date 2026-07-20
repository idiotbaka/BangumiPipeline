package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AdminAddr           string
	ViewerAddr          string
	DatabasePath        string
	AdminWebDir         string
	ViewerWebDir        string
	CoverDir            string
	DownloadDir         string
	MediaDir            string
	FFmpegPath          string
	FFprobePath         string
	BangumiAPIURL       string
	BangumiNextAPIURL   string
	BangumiUserAgent    string
	WebPushContactEmail string
	CookieSecure        bool
	SessionTTL          time.Duration
	SchedulerPoll       time.Duration
	ShutdownPeriod      time.Duration
}

func Load() Config {
	return Config{
		AdminAddr:           getenvAny([]string{"BP_ADMIN_ADDR", "AB_ADMIN_ADDR"}, ":8080"),
		ViewerAddr:          getenvAny([]string{"BP_VIEWER_ADDR", "AB_VIEWER_ADDR"}, ":8090"),
		DatabasePath:        getenvAny([]string{"BP_DATABASE_PATH", "AB_DATABASE_PATH"}, defaultDatabasePath()),
		AdminWebDir:         getenvAny([]string{"BP_ADMIN_WEB_DIR", "AB_ADMIN_WEB_DIR"}, "./frontend/apps/admin/dist"),
		ViewerWebDir:        getenvAny([]string{"BP_VIEWER_WEB_DIR", "AB_VIEWER_WEB_DIR"}, "./frontend/apps/viewer/dist"),
		CoverDir:            getenvAny([]string{"BP_COVER_DIR", "AB_COVER_DIR"}, "./data/images/bangumi"),
		DownloadDir:         getenvAny([]string{"BP_DOWNLOAD_DIR", "AB_DOWNLOAD_DIR"}, "./data/downloads"),
		MediaDir:            getenvAny([]string{"BP_MEDIA_DIR", "AB_MEDIA_DIR"}, "./data/bangumi"),
		FFmpegPath:          getenvAny([]string{"BP_FFMPEG_PATH", "AB_FFMPEG_PATH"}, "ffmpeg"),
		FFprobePath:         getenvAny([]string{"BP_FFPROBE_PATH", "AB_FFPROBE_PATH"}, "ffprobe"),
		BangumiAPIURL:       getenvAny([]string{"BP_BANGUMI_API_URL", "AB_BANGUMI_API_URL"}, "https://api.bgm.tv"),
		BangumiNextAPIURL:   getenvAny([]string{"BP_BANGUMI_NEXT_API_URL"}, "https://next.bgm.tv"),
		BangumiUserAgent:    getenvAny([]string{"BP_BANGUMI_USER_AGENT", "AB_BANGUMI_USER_AGENT"}, "private-user/BangumiPipeline/0.1 (private deployment)"),
		WebPushContactEmail: getenvAny([]string{"BP_WEB_PUSH_CONTACT_EMAIL", "AB_WEB_PUSH_CONTACT_EMAIL"}, "noreply@localhost"),
		CookieSecure:        getenvBoolAny([]string{"BP_COOKIE_SECURE", "AB_COOKIE_SECURE"}, false),
		SessionTTL:          30 * 24 * time.Hour,
		SchedulerPoll:       10 * time.Second,
		ShutdownPeriod:      10 * time.Second,
	}
}

func defaultDatabasePath() string {
	legacy := "./data/autobangumi.db"
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return "./data/bangumi-pipeline.db"
}

func getenvAny(keys []string, fallback string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return fallback
}

func getenvBoolAny(keys []string, fallback bool) bool {
	for _, key := range keys {
		value := os.Getenv(key)
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fallback
		}
		return parsed
	}
	return fallback
}
