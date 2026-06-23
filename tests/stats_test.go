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

func TestMemoryStatsEndpoint(t *testing.T) {
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
		App:    config.AppConfig{Host: "127.0.0.1", Port: 8088},
		Qdrant: config.QdrantConfig{URL: "http://127.0.0.1:63331", Collection: "frontpocket_test"},
		Redis:  config.RedisConfig{URL: "redis://127.0.0.1:6391/0", KeyPrefix: "frontpocket-test"},
		Embedding: config.EmbeddingConfig{
			Provider:      "ollama",
			OllamaModel:   "nomic-embed-text",
			OllamaBaseURL: embeddingServer.URL,
			Dimensions:    4,
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
	}

	srv, err := api.NewServer(cfg, logfp.NewDiscard())
	if err != nil {
		t.Fatalf("failed creating server: %v", err)
	}

	testServer := httptest.NewServer(srv.Handler())
	defer testServer.Close()

	ingestReq := memory.IngestChatRequest{
		SourceTitle: "Stats Test Chat",
		Records: []memory.MessageRecord{
			{
				ConversationID: "conv_1",
				Timestamp:      "2026-06-22T11:13:00Z",
				Speaker:        "user",
				Project:        "FrontPocket",
				MemoryKind:     memory.KindProjectContext,
				Text:           "FrontPocket is source-backed and local-first.",
			},
			{
				ConversationID: "conv_1",
				Timestamp:      "2026-06-22T11:14:00Z",
				Speaker:        "assistant",
				Project:        "FrontPocket",
				MemoryKind:     memory.KindTechnicalSolution,
				Text:           "Redis is used for fast recall caching.",
			},
			{
				ConversationID: "conv_2",
				Timestamp:      "2026-06-22T11:15:00Z",
				Speaker:        "user",
				Project:        "Notebook",
				MemoryKind:     memory.KindResearchNote,
				Text:           "Keep retrieval source-backed.",
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

	statsResp, err := http.Get(testServer.URL + "/memory/stats")
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	defer statsResp.Body.Close()

	if statsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from stats, got %d", statsResp.StatusCode)
	}

	var stats memory.MemoryStats
	if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
		t.Fatalf("decoding stats response failed: %v", err)
	}

	if stats.Total != 3 {
		t.Fatalf("expected total 3, got %d", stats.Total)
	}
	if stats.BySpeaker["user"] != 2 || stats.BySpeaker["assistant"] != 1 {
		t.Fatalf("unexpected by_speaker counts: %#v", stats.BySpeaker)
	}
	if stats.ByKind[memory.KindProjectContext] != 1 || stats.ByKind[memory.KindTechnicalSolution] != 1 || stats.ByKind[memory.KindResearchNote] != 1 {
		t.Fatalf("unexpected by_kind counts: %#v", stats.ByKind)
	}
	if stats.ByProject["FrontPocket"] != 2 || stats.ByProject["Notebook"] != 1 {
		t.Fatalf("unexpected by_project counts: %#v", stats.ByProject)
	}

	filteredResp, err := http.Get(testServer.URL + "/memory/stats?project=FrontPocket")
	if err != nil {
		t.Fatalf("filtered stats request failed: %v", err)
	}
	defer filteredResp.Body.Close()

	if filteredResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from filtered stats, got %d", filteredResp.StatusCode)
	}

	var filtered memory.MemoryStats
	if err := json.NewDecoder(filteredResp.Body).Decode(&filtered); err != nil {
		t.Fatalf("decoding filtered stats response failed: %v", err)
	}
	if filtered.Total != 2 {
		t.Fatalf("expected filtered total 2, got %d", filtered.Total)
	}
	if filtered.ByProject["FrontPocket"] != 2 {
		t.Fatalf("expected FrontPocket project count 2, got %#v", filtered.ByProject)
	}
}
