package tests

import (
	"testing"

	"github.com/meistro57/frontpocket/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.App.Port != 8088 {
		t.Fatalf("expected default port 8088, got %d", cfg.App.Port)
	}
	if cfg.Embedding.Provider != "ollama" {
		t.Fatalf("expected provider ollama, got %s", cfg.Embedding.Provider)
	}
}

func TestOpenAIProviderRequiresAPIKey(t *testing.T) {
	t.Setenv("EMBEDDING_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing OPENAI_API_KEY")
	}
}

func TestSearchCacheTTLCannotBeNegative(t *testing.T) {
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")
	t.Setenv("SEARCH_CACHE_TTL_SECONDS", "-1")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for negative SEARCH_CACHE_TTL_SECONDS")
	}
}

func TestMindDrillMemoryCollectionConfig(t *testing.T) {
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")
	t.Setenv("MINDDRILL_MEMORY_COLLECTION", "minddrill_chat_memory")
	t.Setenv("MINDDRILL_MEMORY_ENABLED", "true")
	t.Setenv("MINDDRILL_MEMORY_WRITE_MODE", "summary")
	t.Setenv("MINDDRILL_MEMORY_TOP_K", "6")
	t.Setenv("MINDDRILL_MEMORY_SESSION_SUMMARY_EVERY", "8")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}
	if cfg.MindDrillMemory.Collection != "minddrill_chat_memory" {
		t.Fatalf("expected minddrill collection minddrill_chat_memory, got %q", cfg.MindDrillMemory.Collection)
	}
	if cfg.MindDrillMemory.WriteMode != "summary" {
		t.Fatalf("expected write mode summary, got %q", cfg.MindDrillMemory.WriteMode)
	}
}

func TestDeepDrillConfigDefaults(t *testing.T) {
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}
	if cfg.DeepDrill.ThoughtCollection != "minddrill_research_thoughts" {
		t.Fatalf("expected default thought collection, got %q", cfg.DeepDrill.ThoughtCollection)
	}
	if cfg.DeepDrill.FreezeLowGainAfter != 2 {
		t.Fatalf("expected freeze threshold 2, got %d", cfg.DeepDrill.FreezeLowGainAfter)
	}
}

func TestDeepDrillFreezeThresholdMustBePositive(t *testing.T) {
	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")
	t.Setenv("DEEPDRILL_FREEZE_LOW_GAIN_AFTER", "0")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected DEEPDRILL_FREEZE_LOW_GAIN_AFTER validation error")
	}
}
