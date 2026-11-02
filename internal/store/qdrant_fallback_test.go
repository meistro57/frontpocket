package store

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func fourDimEmbed(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}

func failingQdrantServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"status":{"error":"segment unavailable"}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testMemoryPoint(id string) memory.MemoryPoint {
	return memory.MemoryPoint{
		MemoryID:            id,
		ConversationID:      "conv_1",
		SourceType:          "chat_export",
		SourceTitle:         "Test",
		Timestamp:           time.Now().UTC(),
		Speaker:             "user",
		MemoryKind:          memory.KindProjectContext,
		Text:                "hello",
		SourceQuote:         "hello",
		EmbeddingProvider:   "stub",
		EmbeddingModel:      "stub-embed",
		EmbeddingDimensions: 4,
		Vector:              []float32{0.1, 0.2, 0.3, 0.4},
	}
}

func TestUpsertFailsWhenFallbackDisabled(t *testing.T) {
	srv := failingQdrantServer(t)
	memStore := NewQdrantMemoryStore(NewQdrantClient(srv.URL), nil, "frontpocket_test", "", "Cosine", nil)

	err := memStore.Upsert(context.Background(), []memory.MemoryPoint{testMemoryPoint("mem_fail_1")})
	if err == nil {
		t.Fatal("expected an error when points cannot be persisted")
	}
}

func TestUpsertMarksStoreDegradedWhenFallbackUsed(t *testing.T) {
	srv := failingQdrantServer(t)
	fallback := memory.NewInMemoryStore()
	memStore := NewQdrantMemoryStore(NewQdrantClient(srv.URL), nil, "frontpocket_test", "", "Cosine", fallback)

	ctx, status := memory.WithStoreStatus(context.Background())
	if err := memStore.Upsert(ctx, []memory.MemoryPoint{testMemoryPoint("mem_fail_2")}); err != nil {
		t.Fatalf("expected fallback upsert to succeed, got %v", err)
	}
	if !status.Degraded {
		t.Fatal("expected degraded store status after fallback write")
	}
	if status.Reason == "" {
		t.Fatal("expected a degradation reason")
	}
}

func TestSearchMarksStoreDegradedWhenFallbackUsed(t *testing.T) {
	srv := failingQdrantServer(t)
	fallback := memory.NewInMemoryStore()
	memStore := NewQdrantMemoryStore(NewQdrantClient(srv.URL), &testEmbedder{embedText: fourDimEmbed}, "frontpocket_test", "", "Cosine", fallback)

	ctx, status := memory.WithStoreStatus(context.Background())
	if _, err := memStore.Search(ctx, memory.SearchRequest{Query: "hello", Limit: 5}); err != nil {
		t.Fatalf("expected fallback search to succeed, got %v", err)
	}
	if !status.Degraded {
		t.Fatal("expected degraded store status after fallback read")
	}
}

func TestSearchFailsWhenFallbackDisabled(t *testing.T) {
	srv := failingQdrantServer(t)
	memStore := NewQdrantMemoryStore(NewQdrantClient(srv.URL), &testEmbedder{embedText: fourDimEmbed}, "frontpocket_test", "", "Cosine", nil)

	_, err := memStore.Search(context.Background(), memory.SearchRequest{Query: "hello", Limit: 5})
	if err == nil {
		t.Fatal("expected an error when the store cannot search")
	}
}
