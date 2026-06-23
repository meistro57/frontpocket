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

func TestOpenAPIEndpoint(t *testing.T) {
	cfg := config.Config{
		App:       config.AppConfig{Host: "127.0.0.1", Port: 8088, PublicURL: "http://localhost:8088"},
		Qdrant:    config.QdrantConfig{URL: "http://localhost:6333", Collection: "frontpocket_test"},
		Redis:     config.RedisConfig{URL: "redis://localhost:6379/0", KeyPrefix: "frontpocket-test"},
		Embedding: config.EmbeddingConfig{Provider: "ollama", OllamaModel: "nomic-embed-text"},
		Ingestion: config.IngestionConfig{DefaultSourceType: "chat_export", StoreAssistantMessages: true, StoreUserMessages: true},
		Chunking:  config.ChunkingConfig{Size: 900, Overlap: 150, MinSize: 120},
		Search: config.SearchConfig{
			DefaultLimit:       5,
			MaxLimit:           20,
			IncludeSourceQuote: true,
			IncludeFullText:    true,
			CacheTTLSeconds:    30,
		},
		ContextPack: config.ContextPackConfig{DefaultLimit: 8, MaxLimit: 20},
	}

	srv, err := api.NewServer(cfg, logfp.NewDiscard())
	if err != nil {
		t.Fatalf("failed creating server: %v", err)
	}

	testServer := httptest.NewServer(srv.Handler())
	defer testServer.Close()

	resp, err := http.Get(testServer.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("openapi request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var spec map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("failed decoding openapi response: %v", err)
	}

	if spec["openapi"] != "3.1.0" {
		t.Fatalf("unexpected openapi version: %v", spec["openapi"])
	}

	info, ok := spec["info"].(map[string]any)
	if !ok {
		t.Fatal("openapi info missing")
	}
	if info["version"] != "0.1.0" {
		t.Fatalf("unexpected info.version: %v", info["version"])
	}

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("openapi paths missing")
	}
	if _, ok := paths["/memory/search"]; !ok {
		t.Fatal("/memory/search missing from openapi paths")
	}
	if _, ok := paths["/memory/stats"]; !ok {
		t.Fatal("/memory/stats missing from openapi paths")
	}
	if _, ok := paths["/memory/session"]; !ok {
		t.Fatal("/memory/session missing from openapi paths")
	}
	if _, ok := paths["/memory/ingest/chat"]; !ok {
		t.Fatal("/memory/ingest/chat missing from openapi paths")
	}
}
