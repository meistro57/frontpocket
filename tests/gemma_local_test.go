package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/api"
	"github.com/meistro57/frontpocket/internal/chat"
	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/embed"
	logfp "github.com/meistro57/frontpocket/internal/log"
	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/loadout"
)

func TestLocalOllamaGemma4Acceptance(t *testing.T) {
	if err := config.LoadDotEnv("../.env"); err != nil {
		t.Fatalf("LoadDotEnv failed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	if cfg.Chat.Provider != "ollama" || cfg.Embedding.Provider != "ollama" || cfg.Chat.OllamaModel != "gemma4:12b" {
		t.Skipf("Skipping local test, chat provider is %s / %s, embed provider is %s", cfg.Chat.Provider, cfg.Chat.OllamaModel, cfg.Embedding.Provider)
	}

	srv, err := api.NewServer(cfg, logfp.NewDiscard())
	if err != nil {
		t.Fatalf("api.NewServer failed: %v", err)
	}

	testServer := httptest.NewServer(srv.Handler())
	defer testServer.Close()

	// 1. Basic local chat & model reporting
	t.Run("Basic Local Chat", func(t *testing.T) {
		reqPayload, _ := json.Marshal(memory.ChatMessageRequest{
			SessionID: "sess_basic",
			Message:   "Hi Eli.",
		})
		resp, err := http.Post(testServer.URL+"/memory/chat", "application/json", bytes.NewReader(reqPayload))
		if err != nil {
			t.Fatalf("Post failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errBody memory.ErrorBody
			_ = json.NewDecoder(resp.Body).Decode(&errBody)
			t.Fatalf("Expected 200, got %d: code=%s msg=%s detail=%s", resp.StatusCode, errBody.Code, errBody.Message, errBody.Detail)
		}

		var body memory.ChatMessageResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode failed: %v", err)
		}

		if strings.TrimSpace(body.Answer) == "" {
			t.Fatal("Expected non-empty answer")
		}
		if body.Provider != "ollama" {
			t.Fatalf("Expected provider 'ollama', got %q", body.Provider)
		}
		if body.Model != "gemma4:12b" {
			t.Fatalf("Expected model 'gemma4:12b', got %q", body.Model)
		}
		t.Logf("Basic answer: %s", body.Answer)
	})

	// 2. Persona / system prompt
	t.Run("Persona System Prompt", func(t *testing.T) {
		reqPayload, _ := json.Marshal(memory.ChatMessageRequest{
			SessionID:    "sess_persona",
			Message:      "State your role in one sentence.",
			SystemPrompt: "You are Eli, a calm and concise memory assistant.",
		})
		resp, err := http.Post(testServer.URL+"/memory/chat", "application/json", bytes.NewReader(reqPayload))
		if err != nil {
			t.Fatalf("Post failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}
		var body memory.ChatMessageResponse
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if strings.TrimSpace(body.Answer) == "" {
			t.Fatal("Expected non-empty answer")
		}
		t.Logf("Persona answer: %s", body.Answer)
	})

	// 3. Empty memory corpus & Memory-style question
	t.Run("Memory Style Question Empty Corpus", func(t *testing.T) {
		q := "Search my FrontPocket memory and reconstruct the run-in I had with r/ADHD. Separate directly retrieved information from inference. If the memory is not available, say so rather than guessing."
		reqPayload, _ := json.Marshal(memory.ChatMessageRequest{
			SessionID: "sess_adhd",
			Message:   q,
		})
		resp, err := http.Post(testServer.URL+"/memory/chat", "application/json", bytes.NewReader(reqPayload))
		if err != nil {
			t.Fatalf("Post failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errBody memory.ErrorBody
			_ = json.NewDecoder(resp.Body).Decode(&errBody)
			t.Fatalf("Expected 200, got %d: detail=%s", resp.StatusCode, errBody.Detail)
		}
		var body memory.ChatMessageResponse
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if strings.TrimSpace(body.Answer) == "" {
			t.Fatal("Expected non-empty answer")
		}
		t.Logf("Memory-style answer: %s", body.Answer)
	})

	// 4. EmbeddingGemma test
	t.Run("EmbeddingGemma Integration", func(t *testing.T) {
		embedder := embed.NewOllamaEmbedder(cfg.Embedding.OllamaBaseURL, cfg.Embedding.OllamaModel, cfg.Embedding.Dimensions)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		vec, err := embedder.EmbedText(ctx, "Test embedding string for EmbeddingGemma")
		if err != nil {
			t.Fatalf("EmbedText failed: %v", err)
		}
		if len(vec) != 768 {
			t.Fatalf("Expected 768 dimensions for EmbeddingGemma, got %d", len(vec))
		}
		t.Logf("Embedding vector length: %d", len(vec))
	})
}

func TestGemma4MultiTurnToolLoopDirect(t *testing.T) {
	if err := config.LoadDotEnv("../.env"); err != nil {
		t.Fatalf("LoadDotEnv failed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	client := chat.NewOllamaChatClient(cfg.Embedding.OllamaBaseURL, cfg.Chat.OllamaModel)

	messages := []chat.Message{
		{Role: "system", Content: "You are MindDrill's chat assistant."},
		{Role: "user", Content: "Please search memory for ADHD."},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []loadout.ToolCall{
				{ID: "call_wl52c5si", Name: "search_source_memory", Arguments: `{"query":"ADHD"}`},
			},
		},
		{
			Role:       "tool",
			ToolCallID: "call_wl52c5si",
			Content:    "no results found",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := client.CompleteWithTools(ctx, messages, nil)
	if err != nil {
		t.Fatalf("Multi-turn tool loop call failed: %v", err)
	}
	if strings.TrimSpace(res.Content) == "" {
		t.Fatal("Expected non-empty content response")
	}
	t.Logf("Multi-turn response: %s", res.Content)
}
