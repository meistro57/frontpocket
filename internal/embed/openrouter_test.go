package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestOpenRouterEmbedderRetriesEmptyVectorResponse(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&calls, 1)
		if attempt == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": []float64{0.1, 0.2, 0.3}}},
		})
	}))
	defer srv.Close()

	embedder := NewOpenRouterEmbedder(srv.URL, "google/gemini-embedding-2-preview", "test-key", "", "", 0)
	vectors, err := embedder.EmbedBatch(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("expected retry to recover, got error: %v", err)
	}
	if len(vectors) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vectors))
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestOpenRouterEmbedderFallsBackToSerialForBatchMismatch(t *testing.T) {
	var batchCalls int32
	var singleCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(payload.Input) > 1 {
			atomic.AddInt32(&batchCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"index": 0, "embedding": []float64{0.4, 0.5, 0.6}}},
			})
			return
		}
		atomic.AddInt32(&singleCalls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": []float64{0.7, 0.8, 0.9}}},
		})
	}))
	defer srv.Close()

	embedder := NewOpenRouterEmbedder(srv.URL, "google/gemini-embedding-2-preview", "test-key", "", "", 0)
	vectors, err := embedder.EmbedBatch(context.Background(), []string{"one", "two"})
	if err != nil {
		t.Fatalf("expected serial fallback to succeed, got error: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}
	if got := atomic.LoadInt32(&batchCalls); got != 1 {
		t.Fatalf("expected 1 batch call before fallback, got %d", got)
	}
	if got := atomic.LoadInt32(&singleCalls); got != 2 {
		t.Fatalf("expected 2 single fallback calls, got %d", got)
	}
}
