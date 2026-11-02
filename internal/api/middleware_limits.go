package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/metrics"
)

type requestIDKey struct{}

func contextWithRequestID(r *http.Request, requestID string) context.Context {
	return context.WithValue(r.Context(), requestIDKey{}, requestID)
}

func jsonDecoder(reader io.Reader) *json.Decoder {
	return json.NewDecoder(reader)
}

// RequestIDFrom returns the request ID attached by RequestIDMiddleware.
func RequestIDFrom(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return value
	}
	return ""
}

func newRequestID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buffer)
}

// RequestIDMiddleware accepts a caller-supplied X-Request-ID or generates one,
// then exposes it to handlers and the request log.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(contextWithRequestID(r, requestID)))
	})
}

// BodyLimitMiddleware rejects requests that advertise (or stream) more than the
// configured maximum body size.
func BodyLimitMiddleware(maxBytes int64, next http.Handler) http.Handler {
	if maxBytes <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxBytes {
			writeError(w, http.StatusRequestEntityTooLarge, memory.ErrorBody{
				Code:    "BODY_TOO_LARGE",
				Message: "Request body is too large.",
				Detail:  "The limit is " + strconv.FormatInt(maxBytes, 10) + " bytes; set MAX_REQUEST_BODY_BYTES to raise it.",
			})
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// decodeJSONBody decodes a JSON request body, translating an oversized body into
// a 413 response instead of a vague parse error.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := jsonDecoder(r.Body)
	err := decoder.Decode(target)
	if err == nil {
		return true
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(w, http.StatusRequestEntityTooLarge, memory.ErrorBody{
			Code:    "BODY_TOO_LARGE",
			Message: "Request body is too large.",
			Detail:  "Reduce the payload size or raise MAX_REQUEST_BODY_BYTES.",
		})
		return false
	}
	writeError(w, http.StatusBadRequest, memory.ErrorBody{
		Code:    "INVALID_REQUEST",
		Message: "Request body must be valid JSON.",
		Detail:  err.Error(),
	})
	return false
}

// rateLimiter is a per-client token bucket. Clients are keyed by API key when
// present, otherwise by remote address.
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64
	burst    float64
	lastGC   time.Time
	registry *metrics.Registry
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

func newRateLimiter(perMinute, burst int, registry *metrics.Registry) *rateLimiter {
	if perMinute <= 0 {
		return nil
	}
	if burst <= 0 {
		burst = perMinute
	}
	return &rateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     float64(perMinute) / 60.0,
		burst:    float64(burst),
		lastGC:   time.Now(),
		registry: registry,
	}
}

func rateLimitKey(r *http.Request) string {
	for _, header := range []string{"X-FrontPocket-Key", "Authorization"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return header + ":" + value
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *rateLimiter) allow(key string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastGC) > time.Minute {
		for bucketKey, entry := range l.buckets {
			if now.Sub(entry.lastSeen) > 5*time.Minute {
				delete(l.buckets, bucketKey)
			}
		}
		l.lastGC = now
	}

	entry, ok := l.buckets[key]
	if !ok {
		entry = &bucket{tokens: l.burst, lastSeen: now}
		l.buckets[key] = entry
	}
	elapsed := now.Sub(entry.lastSeen).Seconds()
	entry.lastSeen = now
	entry.tokens = minFloat(l.burst, entry.tokens+elapsed*l.rate)

	if entry.tokens < 1 {
		if l.registry != nil {
			l.registry.Inc("frontpocket_rate_limited_total", nil)
		}
		return false
	}
	entry.tokens--
	return true
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// RateLimitMiddleware rejects clients that exceed the configured request rate.
func RateLimitMiddleware(perMinute, burst int, registry *metrics.Registry, next http.Handler) http.Handler {
	limiter := newRateLimiter(perMinute, burst, registry)
	if limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(rateLimitKey(r)) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, memory.ErrorBody{
				Code:    "RATE_LIMITED",
				Message: "Too many requests.",
				Detail:  "Slow down or set RATE_LIMIT_PER_MINUTE to change the limit.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	if s.status == 0 {
		s.status = status
	}
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(data []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(data)
}

// MetricsMiddleware records request counts and durations per route and status
// class, plus the request ID in the log line for correlation.
func MetricsMiddleware(registry *metrics.Registry, logger *slog.Logger, enabled bool, next http.Handler) http.Handler {
	if registry == nil {
		registry = metrics.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		elapsed := time.Since(start)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		route := routeLabel(r.URL.Path)
		registry.Inc("frontpocket_http_requests_total", map[string]string{
			"route":  route,
			"method": r.Method,
			"status": statusClass(status),
		})
		registry.Observe("frontpocket_http_request_duration_seconds", elapsed.Seconds())

		if enabled && logger != nil {
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration", elapsed.String(),
				"request_id", RequestIDFrom(r),
			)
		}
	})
}

func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

// routeLabel keeps metric cardinality bounded by collapsing path parameters.
func routeLabel(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		return "/"
	}
	labelled := make([]string, 0, len(segments))
	for i, segment := range segments {
		if i >= 2 {
			labelled = append(labelled, ":arg")
			continue
		}
		labelled = append(labelled, segment)
	}
	return "/" + strings.Join(labelled, "/")
}
