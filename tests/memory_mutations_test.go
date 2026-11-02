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

func TestMemoryPinCorrectAndForgetFlow(t *testing.T) {
	qdrantURL, redisURL := requireLiveStores(t)

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
		App:         config.AppConfig{Host: "127.0.0.1", Port: 8088},
		Limits:      config.LimitsConfig{MaxRequestBodyBytes: 1 << 20, RateLimitBurst: 20},
		Qdrant:      config.QdrantConfig{URL: qdrantURL, Collection: testCollectionName(t, qdrantURL, "frontpocket_test")},
		Redis:       config.RedisConfig{URL: redisURL, KeyPrefix: "frontpocket-test"},
		Embedding:   config.EmbeddingConfig{Provider: "ollama", OllamaModel: "nomic-embed-text", OllamaBaseURL: embeddingServer.URL, Dimensions: 4},
		Chat:        config.ChatConfig{Provider: "none"},
		Ingestion:   config.IngestionConfig{DefaultSourceType: "chat_export", StoreAssistantMessages: true, StoreUserMessages: true},
		Chunking:    config.ChunkingConfig{Size: 900, Overlap: 0, MinSize: 1},
		Search:      config.SearchConfig{DefaultLimit: 5, MaxLimit: 20, IncludeSourceQuote: true, IncludeFullText: true, CacheTTLSeconds: 0},
		ContextPack: config.ContextPackConfig{DefaultLimit: 5, MaxLimit: 10, MaxCharacters: 4000},
		Security:    config.SecurityConfig{EnableAdminEndpoints: true},
	}

	srv, err := api.NewServer(cfg, logfp.NewDiscard())
	if err != nil {
		t.Fatalf("failed creating server: %v", err)
	}
	testServer := httptest.NewServer(srv.Handler())
	defer testServer.Close()

	ingestReq := memory.IngestChatRequest{
		SourceTitle: "Mutation Flow",
		Project:     "FrontPocket",
		Records: []memory.MessageRecord{{
			ConversationID: "conv_mutation",
			Timestamp:      "2026-06-22T11:13:00Z",
			Speaker:        "user",
			Project:        "FrontPocket",
			Text:           "FrontPocket keeps source-backed memory for corrections.",
		}},
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
	var ingestBody memory.IngestChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ingestBody); err != nil {
		t.Fatalf("decoding ingest response failed: %v", err)
	}
	if len(ingestBody.MemoryIDs) == 0 {
		t.Fatal("expected at least one ingested memory id")
	}
	memoryID := ingestBody.MemoryIDs[0]

	pinBody, _ := json.Marshal(memory.PinRequest{MemoryIDs: []string{memoryID}, Reason: "keep for audit"})
	pinResp, err := http.Post(testServer.URL+"/memory/pin", "application/json", bytes.NewReader(pinBody))
	if err != nil {
		t.Fatalf("pin request failed: %v", err)
	}
	defer pinResp.Body.Close()
	if pinResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from pin, got %d", pinResp.StatusCode)
	}

	correctText := "FrontPocket keeps source-backed memory that users can correct."
	correctBody, _ := json.Marshal(memory.CorrectRequest{MemoryID: memoryID, Text: &correctText, Reason: "clarify wording"})
	correctResp, err := http.Post(testServer.URL+"/memory/correct", "application/json", bytes.NewReader(correctBody))
	if err != nil {
		t.Fatalf("correct request failed: %v", err)
	}
	defer correctResp.Body.Close()
	if correctResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from correct, got %d", correctResp.StatusCode)
	}

	searchReq := memory.SearchRequest{Query: "source-backed memory corrections", Limit: 5}
	searchPayload, _ := json.Marshal(searchReq)
	searchResp, err := http.Post(testServer.URL+"/memory/search", "application/json", bytes.NewReader(searchPayload))
	if err != nil {
		t.Fatalf("search request failed: %v", err)
	}
	defer searchResp.Body.Close()
	var searchBody memory.SearchResponse
	if err := json.NewDecoder(searchResp.Body).Decode(&searchBody); err != nil {
		t.Fatalf("decoding search response failed: %v", err)
	}
	found := false
	for _, result := range searchBody.Results {
		if result.MemoryID != memoryID {
			continue
		}
		found = true
		if !result.Pinned {
			t.Fatalf("expected pinned flag on corrected memory, got %#v", result)
		}
		if result.Text != correctText {
			t.Fatalf("expected corrected text, got %q", result.Text)
		}
	}
	if !found {
		t.Fatalf("expected the pinned memory in search results, got %#v", searchBody.Results)
	}

	forgetBody, _ := json.Marshal(memory.ForgetRequest{MemoryIDs: []string{memoryID}, Hard: true, Reason: "test cleanup"})
	forgetReq, _ := http.NewRequest(http.MethodDelete, testServer.URL+"/memory/forget", bytes.NewReader(forgetBody))
	forgetReq.Header.Set("Content-Type", "application/json")
	forgetResp, err := http.DefaultClient.Do(forgetReq)
	if err != nil {
		t.Fatalf("forget request failed: %v", err)
	}
	defer forgetResp.Body.Close()
	if forgetResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from forget, got %d", forgetResp.StatusCode)
	}

	afterResp, err := http.Post(testServer.URL+"/memory/search", "application/json", bytes.NewReader(searchPayload))
	if err != nil {
		t.Fatalf("post-forget search failed: %v", err)
	}
	defer afterResp.Body.Close()
	var afterBody memory.SearchResponse
	if err := json.NewDecoder(afterResp.Body).Decode(&afterBody); err != nil {
		t.Fatalf("decoding post-forget search failed: %v", err)
	}
	for _, result := range afterBody.Results {
		if result.MemoryID == memoryID {
			t.Fatalf("expected the forgotten memory to be gone, got %#v", result)
		}
	}
}
