package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/meistro57/frontpocket/internal/chat"
	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/embed"
	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/memoryloop"
	"github.com/meistro57/frontpocket/internal/store"
)

type Server struct {
	cfg               config.Config
	logger            *slog.Logger
	mux               *http.ServeMux
	qdrant            *store.QdrantClient
	redis             *store.RedisClient
	memoryStore       memory.MemoryStore
	mindDrillStore    memory.MemoryStore
	ingestor          memory.Ingestor
	contextPacker     memory.ContextPacker
	chatClient        chat.Client
	defaultSearch     int
	maxSearch         int
	minSearchScore    float64
	searchCacheTTL    time.Duration
	searchCacheKey    string
	statsCacheTTL     time.Duration
	statsQueryTimeout time.Duration
	statsCacheMu      sync.RWMutex
	statsCache        map[string]memoryStatsCacheEntry
	sessionFallbackMu sync.RWMutex
	sessionFallback   map[string]memory.SessionState
	defaultSessionTTL time.Duration
	reviewQueue       *memoryloop.FileReviewQueue
}

type memoryStatsCacheEntry struct {
	stats     memory.MemoryStats
	expiresAt time.Time
}

func NewServer(cfg config.Config, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}

	qdrant := store.NewQdrantClient(cfg.Qdrant.URL)
	redis, err := store.NewRedisClient(cfg.Redis.URL)
	if err != nil {
		return nil, err
	}

	embedder, err := selectEmbedder(cfg)
	if err != nil {
		return nil, err
	}

	chatClient, err := selectChatClient(cfg)
	if err != nil {
		return nil, err
	}

	fallbackStore := memory.NewInMemoryStore()
	memStore := store.NewQdrantMemoryStore(
		qdrant,
		embedder,
		cfg.Qdrant.Collection,
		cfg.Qdrant.VectorName,
		cfg.Qdrant.Distance,
		fallbackStore,
	)
	mindDrillStore := memory.MemoryStore(memory.NewInMemoryStore())
	if cfg.MindDrillMemory.Enabled {
		mindDrillStore = store.NewQdrantMemoryStore(
			qdrant,
			embedder,
			cfg.MindDrillMemory.Collection,
			cfg.Qdrant.VectorName,
			cfg.Qdrant.Distance,
			memory.NewInMemoryStore(),
		)
	}
	ingestor := memory.Ingestor{
		Chunker: memory.Chunker{
			Size:    cfg.Chunking.Size,
			Overlap: cfg.Chunking.Overlap,
			MinSize: cfg.Chunking.MinSize,
		},
		Embedder:   embedder,
		Store:      memStore,
		SourceType: cfg.Ingestion.DefaultSourceType,
		MemoryKind: memory.KindProjectContext,
		SpeakerRules: memory.SpeakerRules{
			StoreAssistant: cfg.Ingestion.StoreAssistantMessages,
			StoreUser:      cfg.Ingestion.StoreUserMessages,
			StoreSystem:    cfg.Ingestion.StoreSystemMessages,
		},
	}

	queuePath := strings.TrimSpace(os.Getenv("FRONTPOCKET_PROPOSED_CANON_PATH"))
	if queuePath == "" {
		queuePath = "data/proposed_canon.json"
	}

	s := &Server{
		cfg:               cfg,
		logger:            logger,
		mux:               http.NewServeMux(),
		qdrant:            qdrant,
		redis:             redis,
		memoryStore:       memStore,
		mindDrillStore:    mindDrillStore,
		ingestor:          ingestor,
		contextPacker:     memory.ContextPacker{Store: memStore},
		chatClient:        chatClient,
		defaultSearch:     cfg.Search.DefaultLimit,
		maxSearch:         cfg.Search.MaxLimit,
		minSearchScore:    cfg.Search.MinScore,
		searchCacheTTL:    time.Duration(cfg.Search.CacheTTLSeconds) * time.Second,
		searchCacheKey:    strings.TrimSpace(cfg.Redis.KeyPrefix),
		statsCacheTTL:     45 * time.Second,
		statsQueryTimeout: 4 * time.Second,
		statsCache:        make(map[string]memoryStatsCacheEntry),
		sessionFallback:   make(map[string]memory.SessionState),
		defaultSessionTTL: time.Hour,
		reviewQueue:       memoryloop.NewFileReviewQueue(queuePath),
	}

	s.registerRoutes()

	return s, nil
}

func (s *Server) Handler() http.Handler {
	var handler http.Handler = s.mux

	// CORS must wrap everything including OPTIONS preflight — apply first (outermost).
	handler = CORSMiddleware(s.cfg.Dev.CORSOrigins, handler)

	if s.cfg.Security.RequireAPIKey {
		handler = APIKeyMiddleware(
			s.cfg.Security.APIKey,
			s.cfg.Security.APIKeyHeader,
			handler,
		)
	}

	return requestLogMiddleware(s.logger, s.cfg.Logging.LogRequests, handler)
}

func (s *Server) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", s.cfg.App.Host, s.cfg.App.Port),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	s.logger.Info("starting server", "addr", server.Addr)
	err := server.ListenAndServe()
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)
	s.mux.HandleFunc("POST /memory/ingest/chat", s.handleMemoryIngest)
	s.mux.HandleFunc("POST /memory/search", s.handleMemorySearch)
	s.mux.HandleFunc("POST /memory/context-pack", s.handleContextPack)
	s.mux.HandleFunc("GET /memory/browse", s.handleMemoryBrowse)
	s.mux.HandleFunc("GET /memory/stats", s.handleMemoryStats)
	s.mux.HandleFunc("POST /memory/session", s.handleMemorySession)
	s.mux.HandleFunc("DELETE /memory/session", s.handleMemorySessionDelete)
	s.mux.HandleFunc("POST /memory/chat", s.handleMemoryChat)
	s.mux.HandleFunc("DELETE /memory/chat/session", s.handleMemoryChatSessionDelete)
	s.mux.HandleFunc("GET /memory/canon/proposed", s.handleProposedCanonList)
	s.mux.HandleFunc("GET /memory/canon/proposed/{id}", s.handleProposedCanonGet)
	s.mux.HandleFunc("POST /memory/canon/proposed/{id}/approve", s.handleProposedCanonApprove)
	s.mux.HandleFunc("POST /memory/canon/proposed/{id}/reject", s.handleProposedCanonReject)
	s.mux.HandleFunc("POST /memory/canon/proposed/{id}/merge", s.handleProposedCanonMerge)
	if s.cfg.Dev.DebugEndpoints {
		s.mux.HandleFunc("GET /minddrill/memory/stats", s.handleMindDrillMemoryStats)
		s.mux.HandleFunc("POST /minddrill/memory/search", s.handleMindDrillMemorySearch)
		s.mux.HandleFunc("DELETE /minddrill/memory/session", s.handleMindDrillMemorySessionDelete)
	}
}

func selectEmbedder(cfg config.Config) (embed.Embedder, error) {
	switch cfg.Embedding.Provider {
	case "ollama":
		return embed.NewOllamaEmbedder(cfg.Embedding.OllamaBaseURL, cfg.Embedding.OllamaModel, cfg.Embedding.Dimensions), nil
	case "openai":
		return embed.NewOpenAIEmbedder(cfg.Embedding.OpenAIBaseURL, cfg.Embedding.OpenAIModel, cfg.Embedding.OpenAIKey, cfg.Embedding.Dimensions), nil
	case "openrouter":
		return embed.NewOpenRouterEmbedder(
			cfg.Embedding.OpenRouterURL,
			cfg.Embedding.OpenRouterMod,
			cfg.Embedding.OpenRouterKey,
			cfg.Embedding.OpenRouterSite,
			cfg.Embedding.OpenRouterApp,
			cfg.Embedding.Dimensions,
		), nil
	default:
		return nil, fmt.Errorf("unsupported EMBEDDING_PROVIDER: %s", cfg.Embedding.Provider)
	}
}

func selectChatClient(cfg config.Config) (chat.Client, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Chat.Provider)) {
	case "none", "":
		return nil, nil
	case "openrouter":
		return chat.NewOpenRouterClient(
			cfg.Embedding.OpenRouterURL,
			cfg.Chat.OpenRouterModel,
			cfg.Embedding.OpenRouterKey,
			cfg.Embedding.OpenRouterSite,
			cfg.Embedding.OpenRouterApp,
		), nil
	default:
		return nil, fmt.Errorf("unsupported CHAT_PROVIDER for /memory/chat: %s", cfg.Chat.Provider)
	}
}
