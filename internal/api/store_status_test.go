package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/store"
)

func newStoreStatusTestServer(memStore memory.MemoryStore) *Server {
	return &Server{
		cfg: config.Config{
			Search:      config.SearchConfig{IncludeSourceQuote: true, IncludeFullText: true},
			ContextPack: config.ContextPackConfig{DefaultLimit: 5, MaxLimit: 10},
		},
		memoryStore:       memStore,
		searchCacheKey:    "frontpocket-test",
		defaultSearch:     5,
		maxSearch:         20,
		statsCacheTTL:     time.Minute,
		statsQueryTimeout: time.Second,
		statsCache:        make(map[string]memoryStatsCacheEntry),
	}
}

func searchRequest(t *testing.T) *http.Request {
	t.Helper()
	body := strings.NewReader(`{"query":"what did we decide"}`)
	return httptest.NewRequest(http.MethodPost, "/memory/search", body)
}

func TestSearchReturnsServiceUnavailableWhenStoreFails(t *testing.T) {
	s := newStoreStatusTestServer(&stubMemoryStore{searchErr: errors.New("connection refused")})

	rr := httptest.NewRecorder()
	s.handleMemorySearch(rr, searchRequest(t))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when the store cannot answer, got %d", rr.Code)
	}

	var body memory.ErrorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if body.Error.Code != "SEARCH_FAILED" {
		t.Fatalf("expected SEARCH_FAILED code, got %q", body.Error.Code)
	}
	if !strings.Contains(body.Error.Detail, "connection refused") {
		t.Fatalf("expected store error detail, got %q", body.Error.Detail)
	}
}

func TestSearchKeepsInternalServerErrorForDimensionMismatch(t *testing.T) {
	mismatch := &store.DimensionMismatchError{CollectionSize: 768, EmbeddingSize: 3072}
	s := newStoreStatusTestServer(&stubMemoryStore{searchErr: mismatch})

	rr := httptest.NewRecorder()
	s.handleMemorySearch(rr, searchRequest(t))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for a configuration mismatch, got %d", rr.Code)
	}
}

func TestSearchReportsDegradedStoreInResponse(t *testing.T) {
	s := newStoreStatusTestServer(&stubMemoryStore{
		degradedResults: []memory.SearchResult{{MemoryID: "m1", Score: 0.5}},
	})

	rr := httptest.NewRecorder()
	s.handleMemorySearch(rr, searchRequest(t))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for a degraded read, got %d", rr.Code)
	}

	var body memory.SearchResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode search response: %v", err)
	}
	if body.Store == nil || !body.Store.Degraded {
		t.Fatalf("expected degraded store status, got %#v", body.Store)
	}
	if body.Store.Reason == "" {
		t.Fatal("expected a degradation reason")
	}
}

func TestSearchOmitsStoreStatusWhenHealthy(t *testing.T) {
	s := newStoreStatusTestServer(&stubMemoryStore{})

	rr := httptest.NewRecorder()
	s.handleMemorySearch(rr, searchRequest(t))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), `"store"`) {
		t.Fatalf("expected no store status when healthy, got %s", rr.Body.String())
	}
}

func TestStatsReturnsServiceUnavailableWhenStoreFails(t *testing.T) {
	s := newStoreStatusTestServer(&stubMemoryStore{statsErr: errors.New("qdrant unavailable")})

	rr := httptest.NewRecorder()
	s.handleMemoryStats(rr, httptest.NewRequest(http.MethodGet, "/memory/stats", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when stats cannot be read, got %d", rr.Code)
	}
}

func TestIngestReturnsServiceUnavailableWhenStoreFails(t *testing.T) {
	s := newStoreStatusTestServer(&stubMemoryStore{})
	s.ingestor = memory.Ingestor{
		Chunker:      memory.Chunker{Size: 900, Overlap: 0, MinSize: 1},
		Store:        failingUpsertStore{},
		Embedder:     stubEmbedder{},
		SourceType:   "chat_export",
		MemoryKind:   memory.KindProjectContext,
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
	}

	body := strings.NewReader(`{"records":[{"conversation_id":"c1","timestamp":"2026-01-01T00:00:00Z","speaker":"user","text":"hello there"}]}`)
	rr := httptest.NewRecorder()
	s.handleMemoryIngest(rr, httptest.NewRequest(http.MethodPost, "/memory/ingest/chat", body))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when points cannot be persisted, got %d: %s", rr.Code, rr.Body.String())
	}

	var envelope memory.ErrorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode ingest error body: %v", err)
	}
	if envelope.Error.Code != "INGEST_FAILED" {
		t.Fatalf("expected INGEST_FAILED code, got %q", envelope.Error.Code)
	}
}

type failingUpsertStore struct{}

func (failingUpsertStore) Upsert(context.Context, []memory.MemoryPoint) error {
	return errors.New("qdrant write failed")
}

func (failingUpsertStore) Search(context.Context, memory.SearchRequest) ([]memory.SearchResult, error) {
	return nil, nil
}

func (failingUpsertStore) Stats(context.Context, memory.SearchFilters) (memory.MemoryStats, error) {
	return memory.MemoryStats{}, nil
}

type stubEmbedder struct{}

func (stubEmbedder) EmbedText(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}

func (stubEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for range texts {
		out = append(out, []float32{0.1, 0.2, 0.3, 0.4})
	}
	return out, nil
}

func (stubEmbedder) ProviderName() string { return "stub" }

func (stubEmbedder) ModelName() string { return "stub-embed" }
