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
