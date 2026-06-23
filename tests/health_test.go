package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/meistro57/frontpocket/internal/api"
	"github.com/meistro57/frontpocket/internal/config"
	logfp "github.com/meistro57/frontpocket/internal/log"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := config.Config{
		App:       config.AppConfig{Host: "127.0.0.1", Port: 8088},
		Qdrant:    config.QdrantConfig{URL: "http://localhost:6333"},
		Redis:     config.RedisConfig{URL: "redis://localhost:6379/0"},
		Embedding: config.EmbeddingConfig{Provider: "ollama", OllamaModel: "nomic-embed-text"},
		Ingestion: config.IngestionConfig{DefaultSourceType: "chat_export", StoreAssistantMessages: true, StoreUserMessages: true},
		Chunking:  config.ChunkingConfig{Size: 900, Overlap: 150, MinSize: 120},
		Search: config.SearchConfig{
			DefaultLimit:       5,
			MaxLimit:           20,
			IncludeSourceQuote: true,
			IncludeFullText:    true,
		},
		ContextPack: config.ContextPackConfig{DefaultLimit: 8, MaxLimit: 20},
	}

	srv, err := api.NewServer(cfg, logfp.NewDiscard())
	if err != nil {
		t.Fatalf("failed creating server: %v", err)
	}

	testServer := httptest.NewServer(srv.Handler())
	defer testServer.Close()

	resp, err := http.Get(testServer.URL + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed decoding health response: %v", err)
	}
	if _, ok := body["status"]; !ok {
		t.Fatal("health response missing status field")
	}
	if _, ok := body["qdrant"]; !ok {
		t.Fatal("health response missing qdrant field")
	}
	if _, ok := body["redis"]; !ok {
		t.Fatal("health response missing redis field")
	}
	if _, ok := body["version"]; !ok {
		t.Fatal("health response missing version field")
	}
}
