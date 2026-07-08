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
				ConversationID:         "conv_1",
				Timestamp:              "2026-06-22T11:13:00Z",
				Speaker:                "user",
				Project:                "FrontPocket",
				MemoryKind:             memory.KindProjectContext,
				AIProvider:             "chatgpt",
				AIModel:                "gpt-4o",
				UserStarred:            true,
				FeedbackRating:         "thumbs_down",
				AttachmentSourceSystem: "chatgpt_export",
				Text:                   "FrontPocket is source-backed and local-first.",
			},
			{
				ConversationID: "conv_1",
				Timestamp:      "2026-06-22T11:14:00Z",
				Speaker:        "assistant",
				Project:        "FrontPocket",
				MemoryKind:     memory.KindTechnicalSolution,
				AIProvider:     "chatgpt",
				AIModel:        "gpt-4o-mini",
				UserShared:     true,
				Text:           "Redis is used for fast recall caching.",
			},
			{
				ConversationID: "conv_2",
				Timestamp:      "2026-06-22T11:15:00Z",
				Speaker:        "user",
				Project:        "Notebook",
				MemoryKind:     memory.KindResearchNote,
				AIProvider:     "claude",
				AIModel:        "claude-3-5-sonnet",
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

	providerResp, err := http.Get(testServer.URL + "/memory/stats?ai_provider=chatgpt")
	if err != nil {
		t.Fatalf("provider-filtered stats request failed: %v", err)
	}
	defer providerResp.Body.Close()
	if providerResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from provider-filtered stats, got %d", providerResp.StatusCode)
	}
	var providerFiltered memory.MemoryStats
	if err := json.NewDecoder(providerResp.Body).Decode(&providerFiltered); err != nil {
		t.Fatalf("decoding provider-filtered stats response failed: %v", err)
	}
	if providerFiltered.Total != 2 {
		t.Fatalf("expected chatgpt total 2, got %d", providerFiltered.Total)
	}

	starredResp, err := http.Get(testServer.URL + "/memory/stats?starred=true")
	if err != nil {
		t.Fatalf("starred-filtered stats request failed: %v", err)
	}
	defer starredResp.Body.Close()
	if starredResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from starred-filtered stats, got %d", starredResp.StatusCode)
	}
	var starredFiltered memory.MemoryStats
	if err := json.NewDecoder(starredResp.Body).Decode(&starredFiltered); err != nil {
		t.Fatalf("decoding starred-filtered stats response failed: %v", err)
	}
	if starredFiltered.Total != 1 {
		t.Fatalf("expected starred total 1, got %d", starredFiltered.Total)
	}

	feedbackResp, err := http.Get(testServer.URL + "/memory/stats?feedback_rating=thumbs_down")
	if err != nil {
		t.Fatalf("feedback-filtered stats request failed: %v", err)
	}
	defer feedbackResp.Body.Close()
	if feedbackResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from feedback-filtered stats, got %d", feedbackResp.StatusCode)
	}
	var feedbackFiltered memory.MemoryStats
	if err := json.NewDecoder(feedbackResp.Body).Decode(&feedbackFiltered); err != nil {
		t.Fatalf("decoding feedback-filtered stats response failed: %v", err)
	}
	if feedbackFiltered.Total != 1 {
		t.Fatalf("expected feedback total 1, got %d", feedbackFiltered.Total)
	}

	attachmentResp, err := http.Get(testServer.URL + "/memory/stats?has_attachment=true")
	if err != nil {
		t.Fatalf("attachment-filtered stats request failed: %v", err)
	}
	defer attachmentResp.Body.Close()
	if attachmentResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from attachment-filtered stats, got %d", attachmentResp.StatusCode)
	}
	var attachmentFiltered memory.MemoryStats
	if err := json.NewDecoder(attachmentResp.Body).Decode(&attachmentFiltered); err != nil {
		t.Fatalf("decoding attachment-filtered stats response failed: %v", err)
	}
	if attachmentFiltered.Total != 1 {
		t.Fatalf("expected attachment total 1, got %d", attachmentFiltered.Total)
	}
}
