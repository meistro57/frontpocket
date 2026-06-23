package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
}
