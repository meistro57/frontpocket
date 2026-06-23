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

func TestMemorySessionEndpoint(t *testing.T) {
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

	invalidPayload := []byte(`{"project":"FrontPocket"}`)
	invalidResp, err := http.Post(testServer.URL+"/memory/session", "application/json", bytes.NewReader(invalidPayload))
	if err != nil {
		t.Fatalf("invalid session request failed: %v", err)
	}
	defer invalidResp.Body.Close()
	if invalidResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing session_id, got %d", invalidResp.StatusCode)
	}

	invalidDeleteReq, err := http.NewRequest(http.MethodDelete, testServer.URL+"/memory/session", nil)
	if err != nil {
		t.Fatalf("invalid delete request creation failed: %v", err)
	}
	invalidDeleteResp, err := http.DefaultClient.Do(invalidDeleteReq)
	if err != nil {
		t.Fatalf("invalid delete request failed: %v", err)
	}
	defer invalidDeleteResp.Body.Close()
	if invalidDeleteResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for delete without session_id query parameter, got %d", invalidDeleteResp.StatusCode)
	}

	setReq := memory.SessionRequest{
		SessionID:       "session_1",
		Project:         "FrontPocket",
		ActiveSummary:   "Working on memory stats and session endpoints.",
		RecentMemoryIDs: []string{"conv_1_turn_001_chunk_001", "conv_1_turn_002_chunk_001"},
		Metadata: map[string]string{
			"active_project": "FrontPocket",
			"mode":           "development",
		},
	}

	setPayload, _ := json.Marshal(setReq)
	setResp, err := http.Post(testServer.URL+"/memory/session", "application/json", bytes.NewReader(setPayload))
	if err != nil {
		t.Fatalf("set session request failed: %v", err)
	}
	defer setResp.Body.Close()
	if setResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for set session, got %d", setResp.StatusCode)
	}

	var setBody memory.SessionResponse
	if err := json.NewDecoder(setResp.Body).Decode(&setBody); err != nil {
		t.Fatalf("decoding set session response failed: %v", err)
	}
	if !setBody.Found || setBody.State == nil {
		t.Fatal("expected found=true and populated state after set")
	}
	if setBody.State.SessionID != "session_1" {
		t.Fatalf("expected session_id session_1, got %q", setBody.State.SessionID)
	}
	if setBody.State.Project != "FrontPocket" {
		t.Fatalf("expected project FrontPocket, got %q", setBody.State.Project)
	}

	loadReq := memory.SessionRequest{SessionID: "session_1", LoadOnly: true}
	loadPayload, _ := json.Marshal(loadReq)
	loadResp, err := http.Post(testServer.URL+"/memory/session", "application/json", bytes.NewReader(loadPayload))
	if err != nil {
		t.Fatalf("load session request failed: %v", err)
	}
	defer loadResp.Body.Close()
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for load session, got %d", loadResp.StatusCode)
	}

	var loadBody memory.SessionResponse
	if err := json.NewDecoder(loadResp.Body).Decode(&loadBody); err != nil {
		t.Fatalf("decoding load session response failed: %v", err)
	}
	if !loadBody.Found || loadBody.State == nil {
		t.Fatal("expected found=true and state on load")
	}
	if loadBody.State.ActiveSummary != "Working on memory stats and session endpoints." {
		t.Fatalf("unexpected active_summary: %q", loadBody.State.ActiveSummary)
	}
	if len(loadBody.State.RecentMemoryIDs) != 2 {
		t.Fatalf("expected 2 recent_memory_ids, got %d", len(loadBody.State.RecentMemoryIDs))
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, testServer.URL+"/memory/session?session_id=session_1", nil)
	if err != nil {
		t.Fatalf("delete session request creation failed: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete session request failed: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for delete session, got %d", deleteResp.StatusCode)
	}

	var deleteBody memory.SessionResponse
	if err := json.NewDecoder(deleteResp.Body).Decode(&deleteBody); err != nil {
		t.Fatalf("decoding delete session response failed: %v", err)
	}
	if deleteBody.Found {
		t.Fatal("expected found=false after delete")
	}
	if deleteBody.State != nil {
		t.Fatal("expected state=nil after delete")
	}

	loadAfterDeleteReq := memory.SessionRequest{SessionID: "session_1", LoadOnly: true}
	loadAfterDeletePayload, _ := json.Marshal(loadAfterDeleteReq)
	loadAfterDeleteResp, err := http.Post(testServer.URL+"/memory/session", "application/json", bytes.NewReader(loadAfterDeletePayload))
	if err != nil {
		t.Fatalf("load after delete request failed: %v", err)
	}
	defer loadAfterDeleteResp.Body.Close()
	if loadAfterDeleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for load after delete, got %d", loadAfterDeleteResp.StatusCode)
	}

	var loadAfterDeleteBody memory.SessionResponse
	if err := json.NewDecoder(loadAfterDeleteResp.Body).Decode(&loadAfterDeleteBody); err != nil {
		t.Fatalf("decoding load after delete response failed: %v", err)
	}
	if loadAfterDeleteBody.Found {
		t.Fatal("expected found=false after session delete")
	}
	if loadAfterDeleteBody.State != nil {
		t.Fatal("expected empty state after session delete")
	}
}
