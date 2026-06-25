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

// CORSMiddleware sets Access-Control-Allow-Origin headers based on the allowed
// origins list from config. If the request Origin matches an allowed origin it
// is reflected back. A wildcard "*" in the list allows all origins.
// OPTIONS preflight requests are answered immediately with 204.
func CORSMiddleware(allowedOrigins []string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	wildcard := false
	for _, o := range allowedOrigins {
		trimmed := strings.TrimSpace(o)
		if trimmed == "*" {
			wildcard = true
		}
		allowed[trimmed] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if wildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-FrontPocket-Key")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
