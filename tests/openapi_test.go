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
	if info["version"] != "0.2.0" {
		t.Fatalf("unexpected info.version: %v", info["version"])
	}

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("openapi paths missing")
	}
	if _, ok := paths["/openapi.json"]; !ok {
		t.Fatal("/openapi.json missing from openapi paths")
	}
	if _, ok := paths["/memory/search"]; !ok {
		t.Fatal("/memory/search missing from openapi paths")
	}
	if _, ok := paths["/memory/stats"]; !ok {
		t.Fatal("/memory/stats missing from openapi paths")
	}
	sessionPath, ok := paths["/memory/session"].(map[string]any)
	if !ok {
		t.Fatal("/memory/session missing from openapi paths")
	}
	if _, ok := sessionPath["delete"]; !ok {
		t.Fatal("DELETE /memory/session missing from openapi paths")
	}
	if _, ok := paths["/memory/chat"]; !ok {
		t.Fatal("/memory/chat missing from openapi paths")
	}
	if _, ok := paths["/memory/ingest/chat"]; !ok {
		t.Fatal("/memory/ingest/chat missing from openapi paths")
	}

	components, ok := spec["components"].(map[string]any)
	if !ok {
		t.Fatal("openapi components missing")
	}

	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("openapi component schemas missing")
	}
	if _, ok := schemas["SearchRequest"]; !ok {
		t.Fatal("SearchRequest schema missing from openapi components")
	}
	if _, ok := schemas["SessionResponse"]; !ok {
		t.Fatal("SessionResponse schema missing from openapi components")
	}
	if _, ok := schemas["ChatMessageRequest"]; !ok {
		t.Fatal("ChatMessageRequest schema missing from openapi components")
	}
	if _, ok := schemas["ChatMessageResponse"]; !ok {
		t.Fatal("ChatMessageResponse schema missing from openapi components")
	}

	parameters, ok := components["parameters"].(map[string]any)
	if !ok {
		t.Fatal("openapi component parameters missing")
	}
	if _, ok := parameters["sessionIDQuery"]; !ok {
		t.Fatal("sessionIDQuery parameter missing from openapi components")
	}

	responses, ok := components["responses"].(map[string]any)
	if !ok {
		t.Fatal("openapi component responses missing")
	}
	if _, ok := responses["ValidationError"]; !ok {
		t.Fatal("ValidationError response missing from openapi components")
	}
	if _, ok := responses["InternalServerError"]; !ok {
		t.Fatal("InternalServerError response missing from openapi components")
	}
}

func TestOpenAPISecurityMetadataWhenAPIKeyRequired(t *testing.T) {
	cfg := config.Config{
		App:       config.AppConfig{Host: "127.0.0.1", Port: 8088, PublicURL: "http://localhost:8088"},
		Security:  config.SecurityConfig{RequireAPIKey: true, APIKey: "test-key", APIKeyHeader: "X-Test-Key"},
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

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 without API key, got %d", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/openapi.json", nil)
	if err != nil {
		t.Fatalf("request creation failed: %v", err)
	}
	req.Header.Set("X-Test-Key", "test-key")

	respWithKey, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("openapi request with key failed: %v", err)
	}
	defer respWithKey.Body.Close()

	if respWithKey.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 with API key, got %d", respWithKey.StatusCode)
	}

	var spec map[string]any
	if err := json.NewDecoder(respWithKey.Body).Decode(&spec); err != nil {
		t.Fatalf("failed decoding openapi response: %v", err)
	}

	security, ok := spec["security"].([]any)
	if !ok || len(security) == 0 {
		t.Fatal("expected top-level security requirement when API key is enabled")
	}

	components, ok := spec["components"].(map[string]any)
	if !ok {
		t.Fatal("openapi components missing")
	}
	securitySchemes, ok := components["securitySchemes"].(map[string]any)
	if !ok {
		t.Fatal("openapi securitySchemes missing")
	}
	apiKeyAuth, ok := securitySchemes["ApiKeyAuth"].(map[string]any)
	if !ok {
		t.Fatal("ApiKeyAuth security scheme missing")
	}
	if apiKeyAuth["name"] != "X-Test-Key" {
		t.Fatalf("expected ApiKeyAuth header X-Test-Key, got %v", apiKeyAuth["name"])
	}
}
