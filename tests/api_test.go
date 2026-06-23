package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/meistro57/frontpocket/internal/api"
	"github.com/meistro57/frontpocket/internal/config"
	logfp "github.com/meistro57/frontpocket/internal/log"
	"github.com/meistro57/frontpocket/internal/memory"
)

func TestIngestAndSearchFlow(t *testing.T) {
	cfg := config.Config{
		App: config.AppConfig{Host: "127.0.0.1", Port: 8088},
		Security: config.SecurityConfig{
			RequireAPIKey: false,
		},
		Qdrant: config.QdrantConfig{URL: "http://localhost:6333"},
		Redis:  config.RedisConfig{URL: "redis://localhost:6379/0"},
		Embedding: config.EmbeddingConfig{
			Provider:    "ollama",
			OllamaModel: "nomic-embed-text",
		},
		Ingestion: config.IngestionConfig{
			DefaultSourceType:      "chat_export",
			StoreAssistantMessages: true,
			StoreUserMessages:      true,
			StoreSystemMessages:    false,
		},
		Chunking: config.ChunkingConfig{Size: 900, Overlap: 150, MinSize: 120},
		Search: config.SearchConfig{
			DefaultLimit:       5,
			MaxLimit:           20,
			MinScore:           0,
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

	ingestReq := memory.IngestChatRequest{
		SourceTitle: "Test Chat",
		Project:     "FrontPocket",
		Records: []memory.MessageRecord{
			{
				ConversationID: "conv_1",
				Timestamp:      "2026-06-22T11:13:00Z",
				Speaker:        "user",
				Text:           "FrontPocket is a source-backed memory engine in Go.",
			},
		},
	}

	payload, _ := json.Marshal(ingestReq)
	resp, err := http.Post(testServer.URL+"/memory/ingest/chat", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("ingest request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from ingest, got %d", resp.StatusCode)
	}

	searchReq := memory.SearchRequest{Query: "source-backed Go memory", Limit: 3}
	searchPayload, _ := json.Marshal(searchReq)
	searchResp, err := http.Post(testServer.URL+"/memory/search", "application/json", bytes.NewReader(searchPayload))
	if err != nil {
		t.Fatalf("search request failed: %v", err)
	}
	defer searchResp.Body.Close()

	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from search, got %d", searchResp.StatusCode)
	}

	var searchResult memory.SearchResponse
	if err := json.NewDecoder(searchResp.Body).Decode(&searchResult); err != nil {
		t.Fatalf("decoding search response failed: %v", err)
	}

	if len(searchResult.Results) == 0 {
		t.Fatal("expected at least one search result")
	}
	if searchResult.Results[0].SourceTitle != "Test Chat" {
		t.Fatalf("expected source title Test Chat, got %q", searchResult.Results[0].SourceTitle)
	}
}
