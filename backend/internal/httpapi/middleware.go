package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"bangumipipeline.local/server/internal/database"
)

const adminReadRequestTimeout = 15 * time.Second

func databaseReadWorkload(workload database.ReadWorkload, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := database.WithReadWorkload(r.Context(), workload)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func adminDatabaseReadWorkload(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := database.WithReadWorkload(r.Context(), database.ReadAdmin)
		cancel := func() {}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/system-logs/stream" {
			ctx, cancel = context.WithTimeout(ctx, adminReadRequestTimeout)
		}
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CommonMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic while serving request", "source", "http", "error", recovered, "method", r.Method, "path", r.URL.Path)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			logger.Info("request", "source", "http", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
		}()
		next.ServeHTTP(w, r)
	})
}
