package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App             AppConfig
	Security        SecurityConfig
	Qdrant          QdrantConfig
	Redis           RedisConfig
	Embedding       EmbeddingConfig
	Chat            ChatConfig
	MindDrillMemory MindDrillMemoryConfig
	Chunking        ChunkingConfig
	Ingestion       IngestionConfig
	Search          SearchConfig
	ContextPack     ContextPackConfig
	Logging         LoggingConfig
	Dev             DevConfig
}

type AppConfig struct {
	Mode      string
	Host      string
	Port      int
	PublicURL string
}

type SecurityConfig struct {
	RequireAPIKey bool
	APIKey        string
	APIKeyHeader  string
}

type QdrantConfig struct {
	URL        string
	Collection string
	VectorName string
	Distance   string
}

type RedisConfig struct {
	URL       string
	KeyPrefix string
}

type EmbeddingConfig struct {
	Provider       string
	Dimensions     int
	OllamaBaseURL  string
	OllamaModel    string
	OpenAIKey      string
	OpenAIBaseURL  string
	OpenAIModel    string
	OpenRouterKey  string
	OpenRouterURL  string
	OpenRouterMod  string
	OpenRouterSite string
	OpenRouterApp  string
}

type ChatConfig struct {
	Provider        string
	OllamaModel     string
	OpenAIModel     string
	OpenRouterModel string
}

type MindDrillMemoryConfig struct {
	Collection          string
	Enabled             bool
	WriteMode           string
	TopK                int
	SessionSummaryEvery int
}

type ChunkingConfig struct {
	Size    int
	Overlap int
	MinSize int
}

type IngestionConfig struct {
	DefaultSourceType      string
	StoreAssistantMessages bool
	StoreUserMessages      bool
	StoreSystemMessages    bool
}

type SearchConfig struct {
	DefaultLimit       int
	MaxLimit           int
	MinScore           float64
	IncludeSourceQuote bool
	IncludeFullText    bool
	CacheTTLSeconds    int
}

type ContextPackConfig struct {
	DefaultLimit  int
	MaxLimit      int
	UseSummarizer bool
}

type LoggingConfig struct {
	Level        string
	Format       string
	LogRequests  bool
	LogResponses bool
}

type DevConfig struct {
	Reload         bool
	DebugEndpoints bool
	CORSOrigins    []string
}

func Load() (Config, error) {
	cfg := Config{
		App: AppConfig{
			Mode:      getEnv("FRONTPOCKET_MODE", "local"),
			Host:      getEnv("FRONTPOCKET_HOST", "0.0.0.0"),
			Port:      getEnvInt("FRONTPOCKET_PORT", 8088),
			PublicURL: getEnv("FRONTPOCKET_PUBLIC_URL", "http://localhost:8088"),
		},
		Security: SecurityConfig{
			RequireAPIKey: getEnvBool("FRONTPOCKET_REQUIRE_API_KEY", false),
			APIKey:        getEnv("FRONTPOCKET_API_KEY", "change-me"),
			APIKeyHeader:  getEnv("FRONTPOCKET_API_KEY_HEADER", "X-FrontPocket-Key"),
		},
		Qdrant: QdrantConfig{
			URL:        getEnv("QDRANT_URL", "http://localhost:6333"),
			Collection: getEnv("QDRANT_COLLECTION", "frontpocket_memory"),
			VectorName: getEnv("QDRANT_VECTOR_NAME", ""),
			Distance:   getEnv("QDRANT_DISTANCE", "Cosine"),
		},
		Redis: RedisConfig{
			URL:       getEnv("REDIS_URL", "redis://localhost:6379/0"),
			KeyPrefix: getEnv("REDIS_KEY_PREFIX", "frontpocket"),
		},
		Embedding: EmbeddingConfig{
			Provider:       strings.ToLower(getEnv("EMBEDDING_PROVIDER", "ollama")),
			Dimensions:     getEnvInt("EMBEDDING_DIMENSIONS", 0),
			OllamaBaseURL:  getEnv("OLLAMA_BASE_URL", "http://localhost:11434"),
			OllamaModel:    getEnv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text"),
			OpenAIKey:      getEnv("OPENAI_API_KEY", ""),
			OpenAIBaseURL:  getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			OpenAIModel:    getEnv("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
			OpenRouterKey:  getEnv("OPENROUTER_API_KEY", ""),
			OpenRouterURL:  getEnv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
			OpenRouterMod:  getEnv("OPENROUTER_EMBEDDING_MODEL", "openai/text-embedding-3-small"),
			OpenRouterSite: getEnv("OPENROUTER_SITE_URL", ""),
			OpenRouterApp:  getEnv("OPENROUTER_APP_NAME", "FrontPocket"),
		},
		Chat: ChatConfig{
			Provider:        strings.ToLower(getEnv("CHAT_PROVIDER", "none")),
			OllamaModel:     getEnv("OLLAMA_CHAT_MODEL", "llama3.1"),
			OpenAIModel:     getEnv("OPENAI_CHAT_MODEL", "gpt-4o-mini"),
			OpenRouterModel: getEnv("OPENROUTER_CHAT_MODEL", "openai/gpt-4o-mini"),
		},
		MindDrillMemory: MindDrillMemoryConfig{
			Collection:          getEnv("MINDDRILL_MEMORY_COLLECTION", "minddrill_chat_memory"),
			Enabled:             getEnvBool("MINDDRILL_MEMORY_ENABLED", true),
			WriteMode:           strings.ToLower(getEnv("MINDDRILL_MEMORY_WRITE_MODE", "summary")),
			TopK:                getEnvInt("MINDDRILL_MEMORY_TOP_K", 6),
			SessionSummaryEvery: getEnvInt("MINDDRILL_MEMORY_SESSION_SUMMARY_EVERY", 8),
		},
		Chunking: ChunkingConfig{
			Size:    getEnvInt("CHUNK_SIZE", 900),
			Overlap: getEnvInt("CHUNK_OVERLAP", 150),
			MinSize: getEnvInt("MIN_CHUNK_SIZE", 120),
		},
		Ingestion: IngestionConfig{
			DefaultSourceType:      getEnv("DEFAULT_SOURCE_TYPE", "chat_export"),
			StoreAssistantMessages: getEnvBool("STORE_ASSISTANT_MESSAGES", true),
			StoreUserMessages:      getEnvBool("STORE_USER_MESSAGES", true),
			StoreSystemMessages:    getEnvBool("STORE_SYSTEM_MESSAGES", false),
		},
		Search: SearchConfig{
			DefaultLimit:       getEnvInt("SEARCH_DEFAULT_LIMIT", 5),
			MaxLimit:           getEnvInt("SEARCH_MAX_LIMIT", 20),
			MinScore:           getEnvFloat("SEARCH_MIN_SCORE", 0),
			IncludeSourceQuote: getEnvBool("INCLUDE_SOURCE_QUOTES", true),
			IncludeFullText:    getEnvBool("INCLUDE_FULL_TEXT", true),
			CacheTTLSeconds:    getEnvInt("SEARCH_CACHE_TTL_SECONDS", 45),
		},
		ContextPack: ContextPackConfig{
			DefaultLimit:  getEnvInt("CONTEXT_PACK_DEFAULT_LIMIT", 8),
			MaxLimit:      getEnvInt("CONTEXT_PACK_MAX_LIMIT", 20),
			UseSummarizer: getEnvBool("CONTEXT_PACK_USE_SUMMARIZER", false),
		},
		Logging: LoggingConfig{
			Level:        strings.ToLower(getEnv("LOG_LEVEL", "info")),
			Format:       strings.ToLower(getEnv("LOG_FORMAT", "text")),
			LogRequests:  getEnvBool("LOG_REQUESTS", true),
			LogResponses: getEnvBool("LOG_RESPONSES", false),
		},
		Dev: DevConfig{
			Reload:         getEnvBool("DEV_RELOAD", false),
			DebugEndpoints: getEnvBool("DEV_DEBUG_ENDPOINTS", false),
			CORSOrigins:    splitCSV(getEnv("CORS_ALLOW_ORIGINS", "http://localhost:3000,http://localhost:5173,http://localhost:8080")),
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if c.App.Port <= 0 || c.App.Port > 65535 {
		return fmt.Errorf("FRONTPOCKET_PORT must be between 1 and 65535")
	}
	if c.Search.DefaultLimit <= 0 {
		return errors.New("SEARCH_DEFAULT_LIMIT must be greater than 0")
	}
	if c.Search.MaxLimit <= 0 {
		return errors.New("SEARCH_MAX_LIMIT must be greater than 0")
	}
	if c.Search.DefaultLimit > c.Search.MaxLimit {
		return errors.New("SEARCH_DEFAULT_LIMIT cannot exceed SEARCH_MAX_LIMIT")
	}
	if c.Search.CacheTTLSeconds < 0 {
		return errors.New("SEARCH_CACHE_TTL_SECONDS cannot be negative")
	}
	if c.ContextPack.DefaultLimit <= 0 {
		return errors.New("CONTEXT_PACK_DEFAULT_LIMIT must be greater than 0")
	}
	if c.ContextPack.MaxLimit <= 0 {
		return errors.New("CONTEXT_PACK_MAX_LIMIT must be greater than 0")
	}
	if c.ContextPack.DefaultLimit > c.ContextPack.MaxLimit {
		return errors.New("CONTEXT_PACK_DEFAULT_LIMIT cannot exceed CONTEXT_PACK_MAX_LIMIT")
	}
	if c.Chunking.Size <= 0 {
		return errors.New("CHUNK_SIZE must be greater than 0")
	}
	if c.Chunking.Overlap < 0 {
		return errors.New("CHUNK_OVERLAP cannot be negative")
	}
	if c.Chunking.MinSize <= 0 {
		return errors.New("MIN_CHUNK_SIZE must be greater than 0")
	}
	if c.Security.RequireAPIKey && strings.TrimSpace(c.Security.APIKey) == "" {
		return errors.New("FRONTPOCKET_API_KEY is required when FRONTPOCKET_REQUIRE_API_KEY=true")
	}
	if c.ContextPack.UseSummarizer && c.Chat.Provider == "none" {
		return errors.New("CHAT_PROVIDER cannot be none when CONTEXT_PACK_USE_SUMMARIZER=true")
	}
	if c.MindDrillMemory.Enabled && strings.TrimSpace(c.MindDrillMemory.Collection) == "" {
		return errors.New("MINDDRILL_MEMORY_COLLECTION is required when MINDDRILL_MEMORY_ENABLED=true")
	}
	switch c.MindDrillMemory.WriteMode {
	case "raw", "summary":
	default:
		return errors.New("MINDDRILL_MEMORY_WRITE_MODE must be raw or summary")
	}
	if c.MindDrillMemory.TopK <= 0 {
		return errors.New("MINDDRILL_MEMORY_TOP_K must be greater than 0")
	}
	if c.MindDrillMemory.SessionSummaryEvery <= 0 {
		return errors.New("MINDDRILL_MEMORY_SESSION_SUMMARY_EVERY must be greater than 0")
	}

	switch c.Embedding.Provider {
	case "ollama":
		if strings.TrimSpace(c.Embedding.OllamaModel) == "" {
			return errors.New("OLLAMA_EMBEDDING_MODEL is required when EMBEDDING_PROVIDER=ollama")
		}
	case "openai":
		if strings.TrimSpace(c.Embedding.OpenAIKey) == "" {
			return errors.New("OPENAI_API_KEY is required when EMBEDDING_PROVIDER=openai")
		}
		if strings.TrimSpace(c.Embedding.OpenAIModel) == "" {
			return errors.New("OPENAI_EMBEDDING_MODEL is required when EMBEDDING_PROVIDER=openai")
		}
	case "openrouter":
		if strings.TrimSpace(c.Embedding.OpenRouterKey) == "" {
			return errors.New("OPENROUTER_API_KEY is required when EMBEDDING_PROVIDER=openrouter")
		}
		if strings.TrimSpace(c.Embedding.OpenRouterMod) == "" {
			return errors.New("OPENROUTER_EMBEDDING_MODEL is required when EMBEDDING_PROVIDER=openrouter")
		}
	default:
		return fmt.Errorf("unsupported EMBEDDING_PROVIDER: %s", c.Embedding.Provider)
	}

	return nil
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getEnvFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}

func getEnvBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	return clean
}
