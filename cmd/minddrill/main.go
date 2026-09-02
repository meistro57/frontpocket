package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/meistro57/frontpocket/internal/config"
	embedpkg "github.com/meistro57/frontpocket/internal/embed"
	"github.com/meistro57/frontpocket/internal/store"
	"github.com/meistro57/frontpocket/internal/tools"
)

//go:embed index.html logo.png
var assets embed.FS

func newHandler(apiBaseURL string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		data, err := assets.ReadFile("index.html")
		if err != nil {
			http.Error(w, "could not read index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /logo.png", func(w http.ResponseWriter, r *http.Request) {
		data, err := assets.ReadFile("logo.png")
		if err != nil {
			http.Error(w, "could not read logo.png", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /config.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"api_base_url":%q}`, apiBaseURL)))
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","app":"minddrill"}`)
	})

	return mux
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("minddrill command failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && strings.EqualFold(strings.TrimSpace(args[0]), "inspect") {
		return runInspect(args[1:])
	}
	return runUI(args)
}

func runUI(args []string) error {
	flags := flag.NewFlagSet("minddrill", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	port := flags.Int("port", 8089, "port to serve MindDrill UI on")
	api := flags.String("api", "http://localhost:8088", "FrontPocket API base URL")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: minddrill [options]")
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "  Serves the MindDrill memory explorer UI in your browser.")
		fmt.Fprintln(flags.Output(), "  Requires FrontPocket API to be running.")
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "Commands:")
		fmt.Fprintln(flags.Output(), "  inspect <collection>    Inspect a vector collection for research readiness.")
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "Options:")
		flags.PrintDefaults()
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "Examples:")
		fmt.Fprintln(flags.Output(), "  minddrill")
		fmt.Fprintln(flags.Output(), "  minddrill --port 9000")
		fmt.Fprintln(flags.Output(), "  minddrill --api http://localhost:8088")
		fmt.Fprintln(flags.Output(), "  minddrill inspect frontpocket_memory")
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	mux := newHandler(*api)
	srv := &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", *port), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	addr := fmt.Sprintf("http://localhost:%d", *port)
	slog.Info("MindDrill running", "url", addr, "api", *api)
	fmt.Printf("\n  MindDrill is ready\n  Open: %s\n  API:  %s\n\n", addr, *api)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runInspect(args []string) error {
	flags := flag.NewFlagSet("minddrill inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	sampleLimit := flags.Int("sample-limit", 1200, "max vectors to sample while profiling metadata")
	listOnly := flags.Bool("list", false, "list available vector collections")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: minddrill inspect [--list] [--sample-limit N] <collection>")
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "Examples:")
		fmt.Fprintln(flags.Output(), "  minddrill inspect --list")
		fmt.Fprintln(flags.Output(), "  minddrill inspect frontpocket_memory")
		fmt.Fprintln(flags.Output(), "  minddrill inspect --sample-limit 4000 centerstone")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *listOnly {
		if flags.NArg() > 0 {
			return fmt.Errorf("inspect --list does not accept a collection name")
		}
		if err := config.LoadDotEnv(".env"); err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		client := store.NewQdrantClient(cfg.Qdrant.URL)
		collections, err := client.ListCollections(context.Background())
		if err != nil {
			return err
		}
		fmt.Println(toJSON(map[string]any{"collections": collections}))
		return nil
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("inspect requires exactly one collection name")
	}
	collection := strings.TrimSpace(flags.Arg(0))
	if collection == "" {
		return fmt.Errorf("collection name cannot be empty")
	}

	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	embedder, err := selectEmbedder(cfg)
	if err != nil {
		return err
	}
	runtime := tools.NewMindDrillResearchRuntime(store.NewQdrantClient(cfg.Qdrant.URL), embedder, cfg.Qdrant.VectorName, slog.Default())
	report, err := tools.InspectCollection(context.Background(), runtime, collection, *sampleLimit)
	if err != nil {
		return err
	}
	fmt.Println(toJSON(report))
	return nil
}

func selectEmbedder(cfg config.Config) (embedpkg.Embedder, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Embedding.Provider)) {
	case "ollama":
		return embedpkg.NewOllamaEmbedder(cfg.Embedding.OllamaBaseURL, cfg.Embedding.OllamaModel, cfg.Embedding.Dimensions), nil
	case "openai":
		return embedpkg.NewOpenAIEmbedder(cfg.Embedding.OpenAIBaseURL, cfg.Embedding.OpenAIModel, cfg.Embedding.OpenAIKey, cfg.Embedding.Dimensions), nil
	case "openrouter":
		return embedpkg.NewOpenRouterEmbedder(
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

func toJSON(value any) string {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
