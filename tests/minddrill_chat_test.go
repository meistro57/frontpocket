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

func TestMemoryChatResponseShapeAndMindDrillWriteMetadata(t *testing.T) {
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}

		var req struct {
			Input any `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		count := 1
		switch v := req.Input.(type) {
		case []any:
			if len(v) > 0 {
				count = len(v)
			}
		}

		embeddings := make([][]float64, 0, count)
		for i := 0; i < count; i++ {
			embeddings = append(embeddings, []float64{0.1, 0.2, 0.3, 0.4})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": embeddings})
	}))
	defer embeddingServer.Close()

	cfg := config.Config{
		App:    config.AppConfig{Host: "127.0.0.1", Port: 8088, PublicURL: "http://localhost:8088"},
		Qdrant: config.QdrantConfig{URL: "http://127.0.0.1:63331", Collection: "frontpocket_test"},
		Redis:  config.RedisConfig{URL: "redis://127.0.0.1:6391/0", KeyPrefix: "frontpocket-test"},
		Embedding: config.EmbeddingConfig{
			Provider:      "ollama",
			OllamaModel:   "nomic-embed-text",
			OllamaBaseURL: embeddingServer.URL,
			Dimensions:    4,
		},
		Chat: config.ChatConfig{Provider: "none"},
		MindDrillMemory: config.MindDrillMemoryConfig{
			Collection:          "minddrill_chat_memory",
			Enabled:             true,
			WriteMode:           "summary",
			TopK:                6,
			SessionSummaryEvery: 8,
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
			CacheTTLSeconds:    30,
		},
		ContextPack: config.ContextPackConfig{DefaultLimit: 8, MaxLimit: 20},
		Dev:         config.DevConfig{DebugEndpoints: true},
	}

	srv, err := api.NewServer(cfg, logfp.NewDiscard())
	if err != nil {
		t.Fatalf("failed creating server: %v", err)
	}

	testServer := httptest.NewServer(srv.Handler())
	defer testServer.Close()

	ingestReq := memory.IngestChatRequest{
		SourceTitle: "FrontPocket Source",
		Records: []memory.MessageRecord{{
			ConversationID: "conv_fp_1",
			Timestamp:      "2026-06-22T11:13:00Z",
			Speaker:        "user",
			Text:           "FrontPocket should keep source-backed retrieval as primary evidence.",
		}},
	}
	ingestPayload, _ := json.Marshal(ingestReq)
	ingestResp, err := http.Post(testServer.URL+"/memory/ingest/chat", "application/json", bytes.NewReader(ingestPayload))
	if err != nil {
		t.Fatalf("ingest request failed: %v", err)
	}
	defer ingestResp.Body.Close()
	if ingestResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from ingest, got %d", ingestResp.StatusCode)
	}

	chatReq := memory.ChatMessageRequest{
		SessionID:    "mind_sess_1",
		Message:      "remember this: keep replies concise",
		RememberThis: true,
	}
	chatPayload, _ := json.Marshal(chatReq)
	chatResp, err := http.Post(testServer.URL+"/memory/chat", "application/json", bytes.NewReader(chatPayload))
	if err != nil {
		t.Fatalf("chat request failed: %v", err)
	}
	defer chatResp.Body.Close()
	if chatResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from chat endpoint, got %d", chatResp.StatusCode)
	}

	var chatBody map[string]any
	if err := json.NewDecoder(chatResp.Body).Decode(&chatBody); err != nil {
		t.Fatalf("decoding chat response failed: %v", err)
	}
	if stringsValue(chatBody["answer"]) == "" {
		t.Fatal("expected non-empty answer")
	}
	if _, ok := chatBody["used_frontpocket_memories"].([]any); !ok {
		t.Fatalf("expected used_frontpocket_memories array, got %#v", chatBody["used_frontpocket_memories"])
	}
	if _, ok := chatBody["used_minddrill_memories"].([]any); !ok {
		t.Fatalf("expected used_minddrill_memories array, got %#v", chatBody["used_minddrill_memories"])
	}
	if stringsValue(chatBody["provider"]) == "" {
		t.Fatal("expected provider in chat response")
	}
	if stringsValue(chatBody["model"]) == "" {
		t.Fatal("expected model in chat response")
	}

	searchPayload, _ := json.Marshal(memory.SearchRequest{Query: "keep replies concise", Limit: 5})
	searchResp, err := http.Post(testServer.URL+"/minddrill/memory/search", "application/json", bytes.NewReader(searchPayload))
	if err != nil {
		t.Fatalf("minddrill search request failed: %v", err)
	}
	defer searchResp.Body.Close()
	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from minddrill search endpoint, got %d", searchResp.StatusCode)
	}

	var searchBody memory.SearchResponse
	if err := json.NewDecoder(searchResp.Body).Decode(&searchBody); err != nil {
		t.Fatalf("decoding minddrill search response failed: %v", err)
	}
	if len(searchBody.Results) == 0 {
		t.Fatal("expected at least one minddrill memory result")
	}
	if searchBody.Results[0].SourceType != "minddrill_chat" {
		t.Fatalf("expected source_type minddrill_chat, got %q", searchBody.Results[0].SourceType)
	}
	if searchBody.Results[0].SessionID != "mind_sess_1" {
		t.Fatalf("expected session_id mind_sess_1, got %q", searchBody.Results[0].SessionID)
	}
}

func TestMindDrillChatSessionDeleteRouteAvailableWithoutDebugEndpoints(t *testing.T) {
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float64{{0.1, 0.2, 0.3, 0.4}}})
	}))
	defer embeddingServer.Close()

	cfg := config.Config{
		App:    config.AppConfig{Host: "127.0.0.1", Port: 8088, PublicURL: "http://localhost:8088"},
		Qdrant: config.QdrantConfig{URL: "http://127.0.0.1:63331", Collection: "frontpocket_test"},
		Redis:  config.RedisConfig{URL: "redis://127.0.0.1:6391/0", KeyPrefix: "frontpocket-test"},
		Embedding: config.EmbeddingConfig{
			Provider:      "ollama",
			OllamaModel:   "nomic-embed-text",
			OllamaBaseURL: embeddingServer.URL,
			Dimensions:    4,
		},
		Chat: config.ChatConfig{Provider: "none"},
		MindDrillMemory: config.MindDrillMemoryConfig{
			Collection:          "minddrill_chat_memory",
			Enabled:             false,
			WriteMode:           "summary",
			TopK:                6,
			SessionSummaryEvery: 8,
		},
		Ingestion:   config.IngestionConfig{DefaultSourceType: "chat_export", StoreAssistantMessages: true, StoreUserMessages: true},
		Chunking:    config.ChunkingConfig{Size: 900, Overlap: 150, MinSize: 120},
		Search:      config.SearchConfig{DefaultLimit: 5, MaxLimit: 20, IncludeSourceQuote: true, IncludeFullText: true},
		ContextPack: config.ContextPackConfig{DefaultLimit: 8, MaxLimit: 20},
		Dev:         config.DevConfig{DebugEndpoints: false},
	}

	srv, err := api.NewServer(cfg, logfp.NewDiscard())
	if err != nil {
		t.Fatalf("failed creating server: %v", err)
	}
	testServer := httptest.NewServer(srv.Handler())
	defer testServer.Close()

	deleteReq, err := http.NewRequest(http.MethodDelete, testServer.URL+"/memory/chat/session?session_id=mind_sess_no_debug", nil)
	if err != nil {
		t.Fatalf("building delete request failed: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected non-debug chat session delete route to return 200, got %d", deleteResp.StatusCode)
	}

	var body memory.ChatSessionDeleteResponse
	if err := json.NewDecoder(deleteResp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding delete response failed: %v", err)
	}
	if body.SessionID != "mind_sess_no_debug" || !body.SessionCleared || !body.MemoryCleared {
		t.Fatalf("unexpected delete response: %#v", body)
	}

	debugReq, err := http.NewRequest(http.MethodDelete, testServer.URL+"/minddrill/memory/session?session_id=mind_sess_no_debug", nil)
	if err != nil {
		t.Fatalf("building debug delete request failed: %v", err)
	}
	debugResp, err := http.DefaultClient.Do(debugReq)
	if err != nil {
		t.Fatalf("debug delete request failed: %v", err)
	}
	defer debugResp.Body.Close()
	if debugResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected debug-only delete route to stay hidden, got %d", debugResp.StatusCode)
	}
}

func stringsValue(v any) string {
	s, _ := v.(string)
	return s
}
