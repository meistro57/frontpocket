package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func APIKeyMiddleware(expectedKey, header string, next http.Handler) http.Handler {
	headerName := strings.TrimSpace(header)
	if headerName == "" {
		headerName = "X-FrontPocket-Key"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get(headerName)) != strings.TrimSpace(expectedKey) {
			writeError(w, http.StatusUnauthorized, memory.ErrorBody{
				Code:    "UNAUTHORIZED",
				Message: "Missing or invalid API key.",
				Detail:  "Provide a valid API key in the configured header.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestLogMiddleware(logger *slog.Logger, enabled bool, next http.Handler) http.Handler {
	if !enabled {
		return next
	}
	if logger == nil {
		logger = slog.Default()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).String())
	})
}
