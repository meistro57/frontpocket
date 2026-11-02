package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/meistro57/frontpocket/internal/config"
	logfp "github.com/meistro57/frontpocket/internal/log"
	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/metrics"
)

func TestRequestBodyLimitRejectsOversizedPayload(t *testing.T) {
	server, err := NewServer(config.Config{
		App:    config.AppConfig{Host: "127.0.0.1", Port: 8088},
		Limits: config.LimitsConfig{MaxRequestBodyBytes: 64, RateLimitBurst: 5},
		Qdrant: config.QdrantConfig{URL: "http://127.0.0.1:1", Collection: "frontpocket_test"},
		Redis:  config.RedisConfig{URL: "redis://127.0.0.1:1/0", KeyPrefix: "frontpocket-test"},
		Embedding: config.EmbeddingConfig{
			Provider: "ollama", OllamaModel: "nomic-embed-text", OllamaBaseURL: "http://127.0.0.1:1", Dimensions: 4,
		},
		Chat:        config.ChatConfig{Provider: "none"},
		Chunking:    config.ChunkingConfig{Size: 900, Overlap: 0, MinSize: 1},
		Search:      config.SearchConfig{DefaultLimit: 5, MaxLimit: 20},
		ContextPack: config.ContextPackConfig{DefaultLimit: 5, MaxLimit: 10, MaxCharacters: 4000},
	}, logfp.NewDiscard())
	if err != nil {
		t.Fatalf("failed creating server: %v", err)
	}

	payload := `{"query":"` + strings.Repeat("x", 500) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/memory/search", strings.NewReader(payload))
	req.ContentLength = int64(len(payload))
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for an oversized body, got %d: %s", rr.Code, rr.Body.String())
	}
	var envelope memory.ErrorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if envelope.Error.Code != "BODY_TOO_LARGE" {
		t.Fatalf("expected BODY_TOO_LARGE, got %q", envelope.Error.Code)
	}
}

func TestRateLimitMiddlewareRejectsBurst(t *testing.T) {
	hits := 0
	handler := RateLimitMiddleware(1, 2, metrics.NewRegistry(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))

	statuses := make([]int, 0, 4)
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		statuses = append(statuses, rr.Code)
	}

	if statuses[0] != http.StatusOK || statuses[1] != http.StatusOK {
		t.Fatalf("expected the burst to be allowed, got %v", statuses)
	}
	if statuses[2] != http.StatusTooManyRequests {
		t.Fatalf("expected the third request to be rate limited, got %v", statuses)
	}
	if hits != 2 {
		t.Fatalf("expected only allowed requests to reach the handler, got %d", hits)
	}
}

func TestRateLimitMiddlewareSeparatesClients(t *testing.T) {
	handler := RateLimitMiddleware(1, 1, metrics.NewRegistry(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRequest(http.MethodGet, "/health", nil)
	first.RemoteAddr = "203.0.113.1:1234"
	second := httptest.NewRequest(http.MethodGet, "/health", nil)
	second.RemoteAddr = "203.0.113.2:1234"

	for i, req := range []*http.Request{first, second} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d from a distinct client should be allowed, got %d", i, rr.Code)
		}
	}
}

func TestRequestIDMiddlewareHonoursIncomingHeader(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFrom(r); got != "abc123" {
			t.Errorf("expected request id abc123 in context, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "abc123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") != "abc123" {
		t.Fatalf("expected request id echoed in response, got %q", rr.Header().Get("X-Request-ID"))
	}
}

func TestRequestIDMiddlewareGeneratesID(t *testing.T) {
	var captured string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = RequestIDFrom(r)
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if captured == "" || len(captured) < 8 {
		t.Fatalf("expected a generated request id, got %q", captured)
	}
	if rr.Header().Get("X-Request-ID") != captured {
		t.Fatalf("expected response header to match context id")
	}
}

func TestMetricsMiddlewareRecordsRequestCounts(t *testing.T) {
	registry := metrics.NewRegistry()
	handler := MetricsMiddleware(registry, nil, false, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/memory/search", nil))

	var builder strings.Builder
	registry.Render(&builder)
	rendered := builder.String()
	if !strings.Contains(rendered, `frontpocket_http_requests_total{method="GET",route="/memory/search",status="4xx"} 1`) {
		t.Fatalf("expected request counter with route and status class, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "frontpocket_http_request_duration_seconds_count 1") {
		t.Fatalf("expected duration summary, got:\n%s", rendered)
	}
}

func TestRouteLabelCollapsesPathParameters(t *testing.T) {
	cases := map[string]string{
		"/health":                            "/health",
		"/memory/search":                     "/memory/search",
		"/memory/canon/proposed/abc/approve": "/memory/canon/:arg/:arg/:arg",
		"/":                                  "/",
	}
	for input, expected := range cases {
		if got := routeLabel(input); got != expected {
			t.Fatalf("routeLabel(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestMetricsEndpointServesPrometheusText(t *testing.T) {
	server := &Server{mux: http.NewServeMux()}
	server.registerRoutes()

	rr := httptest.NewRecorder()
	server.mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected prometheus text content type, got %q", ct)
	}
}
