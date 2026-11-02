package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaEmbedderDimensionValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float64{{0.1, 0.2, 0.3}},
		})
	}))
	defer srv.Close()

	embedder := NewOllamaEmbedder(srv.URL, "nomic-embed-text", 4)
	_, err := embedder.EmbedText(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected dimension mismatch error")
	}
	if !strings.Contains(err.Error(), "returned 3 dimensions, expected 4") {
		t.Fatalf("expected dimension mismatch to surface directly, got %q", err.Error())
	}
}

func TestOllamaEmbedderFallsBackToLegacyEndpoint(t *testing.T) {
	legacyHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embed":
			http.NotFound(w, r)
		case "/api/embeddings":
			legacyHits++
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2, 0.3, 0.4}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	embedder := NewOllamaEmbedder(srv.URL, "nomic-embed-text", 4)
	vector, err := embedder.EmbedText(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected legacy fallback to succeed, got %v", err)
	}
	if len(vector) != 4 {
		t.Fatalf("expected 4 dimensions, got %d", len(vector))
	}
	if legacyHits != 1 {
		t.Fatalf("expected 1 legacy request, got %d", legacyHits)
	}
}

func TestOllamaEmbedderReportsBothErrorsWhenFallbackFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	embedder := NewOllamaEmbedder(srv.URL, "nomic-embed-text", 4)
	_, err := embedder.EmbedText(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected an error when both endpoints fail")
	}
	message := err.Error()
	if !strings.Contains(message, "ollama embedding failed") {
		t.Fatalf("expected modern failure in message, got %q", message)
	}
	if !strings.Contains(message, "legacy fallback") {
		t.Fatalf("expected legacy failure in message, got %q", message)
	}
}

func TestOllamaEmbedderDoesNotMaskProviderErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("model not loaded"))
	}))
	defer srv.Close()

	embedder := NewOllamaEmbedder(srv.URL, "nomic-embed-text", 4)
	_, err := embedder.EmbedText(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected provider error")
	}
	if !strings.Contains(err.Error(), "model not loaded") {
		t.Fatalf("expected original provider error, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "legacy fallback") {
		t.Fatalf("expected no legacy fallback for a provider error, got %q", err.Error())
	}
}
