package store

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/metrics"
)

func TestSearchReusesCachedQueryEmbedding(t *testing.T) {
	var embedCalls int
	embedder := &testEmbedder{embedText: func(_ context.Context, _ string) ([]float32, error) {
		embedCalls++
		return []float32{0.1, 0.2, 0.3, 0.4}, nil
	}}

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/points/search"):
			_, _ = io.WriteString(w, `{"result":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	memStore := NewQdrantMemoryStore(NewQdrantClient(qdrant.URL), embedder, "frontpocket_test", "", "Cosine", nil)
	request := memory.SearchRequest{Query: "What did we decide?", Limit: 5}

	for i := 0; i < 3; i++ {
		if _, err := memStore.Search(context.Background(), request); err != nil {
			t.Fatalf("search %d failed: %v", i, err)
		}
	}
	if embedCalls != 1 {
		t.Fatalf("expected one embedding call for repeated queries, got %d", embedCalls)
	}

	if _, err := memStore.Search(context.Background(), memory.SearchRequest{Query: "different question", Limit: 5}); err != nil {
		t.Fatalf("search with a new query failed: %v", err)
	}
	if embedCalls != 2 {
		t.Fatalf("expected a new embedding for a new query, got %d calls", embedCalls)
	}
}

func TestEmbeddingCacheNormalisesQueryCasingAndSpacing(t *testing.T) {
	var embedCalls int
	embedder := &testEmbedder{embedText: func(_ context.Context, _ string) ([]float32, error) {
		embedCalls++
		return []float32{0.1, 0.2, 0.3, 0.4}, nil
	}}

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/points/search"):
			_, _ = io.WriteString(w, `{"result":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	memStore := NewQdrantMemoryStore(NewQdrantClient(qdrant.URL), embedder, "frontpocket_test", "", "Cosine", nil)
	queries := []string{"FrontPocket memory", "frontpocket   memory", "  frontpocket memory  "}
	for _, query := range queries {
		if _, err := memStore.Search(context.Background(), memory.SearchRequest{Query: query, Limit: 5}); err != nil {
			t.Fatalf("search %q failed: %v", query, err)
		}
	}
	if embedCalls != 1 {
		t.Fatalf("expected normalised queries to share a cache entry, got %d calls", embedCalls)
	}
}

func TestMetricsRegistryRendersCountersAndDurations(t *testing.T) {
	registry := metrics.NewRegistry()
	registry.Inc("frontpocket_http_requests_total", map[string]string{"route": "/memory/search", "status": "2xx"})
	registry.Inc("frontpocket_http_requests_total", map[string]string{"route": "/memory/search", "status": "2xx"})
	registry.Observe("frontpocket_http_request_duration_seconds", 0.25)

	var builder strings.Builder
	registry.Render(&builder)
	rendered := builder.String()

	if !strings.Contains(rendered, `frontpocket_http_requests_total{route="/memory/search",status="2xx"} 2`) {
		t.Fatalf("expected labelled counter in output, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "frontpocket_http_request_duration_seconds_count 1") {
		t.Fatalf("expected duration count in output, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "frontpocket_http_request_duration_seconds_sum 0.25") {
		t.Fatalf("expected duration sum in output, got:\n%s", rendered)
	}
}

func TestEmbeddingCacheIsBounded(t *testing.T) {
	embedder := &testEmbedder{embedText: func(_ context.Context, _ string) ([]float32, error) {
		return []float32{0.1, 0.2, 0.3, 0.4}, nil
	}}
	store := NewQdrantMemoryStore(NewQdrantClient("http://127.0.0.1:1"), embedder, "frontpocket_test", "", "Cosine", nil)
	store.embedCacheMax = 4

	for _, query := range []string{"a", "b", "c", "d", "e", "f"} {
		store.storeEmbedding(store.embeddingCacheKey(query), []float32{0.1})
	}

	store.embedCacheMu.Lock()
	size := len(store.embedCache)
	store.embedCacheMu.Unlock()
	if size > 4 {
		t.Fatalf("expected cache to stay bounded at 4 entries, got %d", size)
	}
}
