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

func TestAPIKeyMiddleware(t *testing.T) {
	cfg := config.Config{
		App: config.AppConfig{Host: "127.0.0.1", Port: 8088, PublicURL: "http://localhost:8088"},
		Security: config.SecurityConfig{
			RequireAPIKey: true,
			APIKey:        "test-key",
			APIKeyHeader:  "X-FrontPocket-Key",
		},
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

	unauthorized, err := http.Get(testServer.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("unauthorized request failed: %v", err)
	}
	defer unauthorized.Body.Close()

	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing key, got %d", unauthorized.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(unauthorized.Body).Decode(&body); err != nil {
		t.Fatalf("failed decoding unauthorized response: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatal("error envelope missing")
	}
	if errObj["code"] != "UNAUTHORIZED" {
		t.Fatalf("unexpected error code: %v", errObj["code"])
	}

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/openapi.json", nil)
	req.Header.Set("X-FrontPocket-Key", "test-key")
	authorized, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authorized request failed: %v", err)
	}
	defer authorized.Body.Close()

	if authorized.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for valid key, got %d", authorized.StatusCode)
	}
}
