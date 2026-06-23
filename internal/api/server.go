package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/embed"
	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/store"
)

type Server struct {
	cfg            config.Config
	logger         *slog.Logger
	mux            *http.ServeMux
	qdrant         *store.QdrantClient
	redis          *store.RedisClient
	memoryStore    memory.MemoryStore
	ingestor       memory.Ingestor
	contextPacker  memory.ContextPacker
	defaultSearch  int
	maxSearch      int
	minSearchScore float64
	searchCacheTTL time.Duration
	searchCacheKey string
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

	fallbackStore := memory.NewInMemoryStore()
	memStore := store.NewQdrantMemoryStore(
		qdrant,
		embedder,
		cfg.Qdrant.Collection,
		cfg.Qdrant.VectorName,
		cfg.Qdrant.Distance,
		fallbackStore,
	)
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

	s := &Server{
		cfg:            cfg,
		logger:         logger,
		mux:            http.NewServeMux(),
		qdrant:         qdrant,
		redis:          redis,
		memoryStore:    memStore,
		ingestor:       ingestor,
		contextPacker:  memory.ContextPacker{Store: memStore},
		defaultSearch:  cfg.Search.DefaultLimit,
		maxSearch:      cfg.Search.MaxLimit,
		minSearchScore: cfg.Search.MinScore,
		searchCacheTTL: time.Duration(cfg.Search.CacheTTLSeconds) * time.Second,
		searchCacheKey: strings.TrimSpace(cfg.Redis.KeyPrefix),
	}

	s.registerRoutes()

	return s, nil
}

func (s *Server) Handler() http.Handler {
	var handler http.Handler = s.mux
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
	s.mux.HandleFunc("POST /memory/ingest/chat", s.handleMemoryIngest)
	s.mux.HandleFunc("POST /memory/search", s.handleMemorySearch)
	s.mux.HandleFunc("POST /memory/context-pack", s.handleContextPack)
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
