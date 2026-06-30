package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/api"
	"github.com/meistro57/frontpocket/internal/config"
	logfp "github.com/meistro57/frontpocket/internal/log"
	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/memoryloop"
)

func TestProposedCanonApproveAndRejectEndpoints(t *testing.T) {
	queuePath := filepath.Join(t.TempDir(), "proposed.json")
	t.Setenv("FRONTPOCKET_PROPOSED_CANON_PATH", queuePath)

	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float64{{0.1, 0.2, 0.3, 0.4}}})
	}))
	defer embeddingServer.Close()

	cfg := config.Config{
		App:         config.AppConfig{Host: "127.0.0.1", Port: 8088},
		Qdrant:      config.QdrantConfig{URL: "http://127.0.0.1:63331", Collection: "frontpocket_test"},
		Redis:       config.RedisConfig{URL: "redis://127.0.0.1:6391/0", KeyPrefix: "frontpocket-test"},
		Embedding:   config.EmbeddingConfig{Provider: "ollama", OllamaModel: "nomic-embed-text", OllamaBaseURL: embeddingServer.URL, Dimensions: 4},
		Chat:        config.ChatConfig{Provider: "none"},
		Ingestion:   config.IngestionConfig{DefaultSourceType: "chat_export", StoreAssistantMessages: true, StoreUserMessages: true},
		Chunking:    config.ChunkingConfig{Size: 900, Overlap: 150, MinSize: 120},
		Search:      config.SearchConfig{DefaultLimit: 5, MaxLimit: 20, IncludeSourceQuote: true, IncludeFullText: true, CacheTTLSeconds: 30},
		ContextPack: config.ContextPackConfig{DefaultLimit: 8, MaxLimit: 20},
	}

	srv, err := api.NewServer(cfg, logfp.NewDiscard())
	if err != nil {
		t.Fatalf("failed creating server: %v", err)
	}
	testServer := httptest.NewServer(srv.Handler())
	defer testServer.Close()

	queue := memoryloop.NewFileReviewQueue(queuePath)
	candidate := memoryloop.Candidate{
		ID:              "cand_api_1",
		Summary:         "Mark prefers concise responses.",
		MemoryKind:      memory.KindPreference,
		Confidence:      memory.ConfidenceHigh,
		Status:          memory.StatusInferredFromSources,
		SourceMemoryIDs: []string{"m1"},
		SourceQuotes:    []string{"keep responses concise"},
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		CreatedByLoop:   true,
	}
	if err := queue.Upsert([]memoryloop.Candidate{candidate}); err != nil {
		t.Fatalf("queue upsert failed: %v", err)
	}

	approveReq, _ := json.Marshal(map[string]any{"reviewed_by": "mark"})
	approveResp, err := http.Post(testServer.URL+"/memory/canon/proposed/cand_api_1/approve", "application/json", bytes.NewReader(approveReq))
	if err != nil {
		t.Fatalf("approve request failed: %v", err)
	}
	defer approveResp.Body.Close()
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("expected approve 200, got %d", approveResp.StatusCode)
	}

	searchReq, _ := json.Marshal(memory.SearchRequest{Query: "concise responses", Limit: 5})
	searchResp, err := http.Post(testServer.URL+"/memory/search", "application/json", bytes.NewReader(searchReq))
	if err != nil {
		t.Fatalf("search request failed: %v", err)
	}
	defer searchResp.Body.Close()
	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected search 200, got %d", searchResp.StatusCode)
	}
	var searchBody memory.SearchResponse
	if err := json.NewDecoder(searchResp.Body).Decode(&searchBody); err != nil {
		t.Fatalf("decode search failed: %v", err)
	}
	if len(searchBody.Results) == 0 || !searchBody.Results[0].Canonical {
		t.Fatalf("expected canonical result after approve, got %#v", searchBody.Results)
	}

	rejectReq, _ := json.Marshal(map[string]any{"reason": "not enough evidence", "reviewed_by": "mark"})
	rejectResp, err := http.Post(testServer.URL+"/memory/canon/proposed/cand_api_1/reject", "application/json", bytes.NewReader(rejectReq))
	if err != nil {
		t.Fatalf("reject request failed: %v", err)
	}
	defer rejectResp.Body.Close()
	if rejectResp.StatusCode != http.StatusOK {
		t.Fatalf("expected reject 200, got %d", rejectResp.StatusCode)
	}
	var rejected memoryloop.Candidate
	if err := json.NewDecoder(rejectResp.Body).Decode(&rejected); err != nil {
		t.Fatalf("decode reject failed: %v", err)
	}
	if rejected.Status != memory.StatusRejected || len(rejected.SourceMemoryIDs) == 0 || len(rejected.SourceQuotes) == 0 {
		t.Fatalf("expected rejected candidate preserving provenance, got %#v", rejected)
	}
}
