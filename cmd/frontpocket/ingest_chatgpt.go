package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/embed"
	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/store"
)

func runIngestCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("ingest subcommand is required")
	}
	if args[0] != "chatgpt" {
		return fmt.Errorf("unsupported ingest source %q", args[0])
	}
	return runIngestChatGPT(args[1:])
}

func runIngestChatGPT(args []string) error {
	flags := flag.NewFlagSet("frontpocket ingest chatgpt", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	dryRun := flags.Bool("dry-run", false, "parse and report stats without writing to storage")
	project := flags.String("project", "", "project label to attach to imported records")
	since := flags.String("since", "", "only include messages at or after this date (YYYY-MM-DD or RFC3339)")
	conversation := flags.String("conversation", "", "only include conversations whose title or id contains this value")
	out := flags.String("out", "", "write normalized JSONL output to this path")

	normalizedArgs, sourcePath := normalizeIngestChatGPTArgs(args)
	if err := flags.Parse(normalizedArgs); err != nil {
		return err
	}

	positionals := flags.Args()
	if sourcePath != "" {
		positionals = append([]string{sourcePath}, positionals...)
	}
	if len(positionals) != 1 {
		return fmt.Errorf("usage: frontpocket ingest chatgpt <zip-or-folder> [--dry-run] [--project <name>] [--since <date>] [--conversation <match>] [--out <path>]")
	}

	sinceTime, err := parseSince(*since)
	if err != nil {
		return err
	}

	_ = config.LoadDotEnv(".env")
	cfg, cfgErr := config.Load()

	rules := speakerRulesFromEnv()
	if cfgErr == nil {
		rules = memory.SpeakerRules{
			StoreAssistant: cfg.Ingestion.StoreAssistantMessages,
			StoreUser:      cfg.Ingestion.StoreUserMessages,
			StoreSystem:    cfg.Ingestion.StoreSystemMessages,
		}
	}

	result, err := memory.ParseChatGPTExport(positionals[0], memory.ChatGPTImportOptions{
		Project:            strings.TrimSpace(*project),
		ConversationFilter: strings.TrimSpace(*conversation),
		Since:              sinceTime,
		SpeakerRules:       rules,
	})
	if err != nil {
		return err
	}

	if strings.TrimSpace(*out) != "" {
		if err := memory.WriteNormalizedJSONL(*out, result.Records); err != nil {
			return err
		}
	}

	printImportSummary(result)
	if strings.TrimSpace(*out) != "" {
		fmt.Printf("normalized output: %s\n", strings.TrimSpace(*out))
	}

	if *dryRun {
		fmt.Println("storage write: skipped (--dry-run)")
		return nil
	}

	if len(result.Records) == 0 {
		fmt.Println("storage write: skipped (no accepted messages)")
		return nil
	}

	if cfgErr != nil {
		fmt.Printf("storage write: not wired (%v)\n", cfgErr)
		return nil
	}

	ingestor, err := buildCLIIngestor(cfg)
	if err != nil {
		fmt.Printf("storage write: not wired (%v)\n", err)
		return nil
	}

	points, err := ingestor.Ingest(context.Background(), memory.ToMessageRecords(result.Records))
	if err != nil {
		return fmt.Errorf("parsed %d records but storage ingest failed: %w", len(result.Records), err)
	}
	fmt.Printf("storage write: inserted %d memory points\n", len(points))
	return nil
}

func printImportSummary(result memory.ChatGPTImportResult) {
	fmt.Printf("source path: %s\n", result.SourcePath)
	fmt.Printf("import_id: %s\n", result.ImportID)
	fmt.Printf("conversations found: %d\n", result.ConversationsFound)
	fmt.Printf("messages found: %d\n", result.MessagesFound)
	fmt.Printf("messages accepted: %d\n", result.MessagesAccepted)
	fmt.Printf("messages skipped: %d\n", result.MessagesSkipped)
	if len(result.RolesFound) == 0 {
		fmt.Println("roles found: none")
	} else {
		fmt.Printf("roles found: %s\n", strings.Join(result.RolesFound, ", "))
	}
	if len(result.UnsupportedContentTypes) == 0 {
		fmt.Println("unsupported content types: none")
	} else {
		names := make([]string, 0, len(result.UnsupportedContentTypes))
		for name := range result.UnsupportedContentTypes {
			names = append(names, name)
		}
		sort.Strings(names)
		pairs := make([]string, 0, len(names))
		for _, name := range names {
			pairs = append(pairs, fmt.Sprintf("%s=%d", name, result.UnsupportedContentTypes[name]))
		}
		fmt.Printf("unsupported content types: %s\n", strings.Join(pairs, ", "))
	}
	fmt.Printf("attachments/assets detected: %d\n", result.AttachmentsAssetsDetected)
	attachmentsIngested := "no"
	if result.AttachmentsIngested {
		attachmentsIngested = "yes"
	}
	fmt.Printf("attachments ingested: %s\n", attachmentsIngested)
}

func normalizeIngestChatGPTArgs(args []string) ([]string, string) {
	if len(args) == 0 {
		return nil, ""
	}

	normalized := make([]string, 0, len(args))
	sourcePath := ""
	expectsValue := false

	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" {
			continue
		}

		if expectsValue {
			normalized = append(normalized, trimmed)
			expectsValue = false
			continue
		}

		if strings.HasPrefix(trimmed, "--") {
			normalized = append(normalized, trimmed)
			if trimmed == "--project" || trimmed == "--since" || trimmed == "--conversation" || trimmed == "--out" {
				expectsValue = true
			}
			continue
		}

		if sourcePath == "" {
			sourcePath = trimmed
			continue
		}
		normalized = append(normalized, trimmed)
	}

	return normalized, sourcePath
}

func parseSince(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid --since value %q: expected YYYY-MM-DD or RFC3339", raw)
}

func speakerRulesFromEnv() memory.SpeakerRules {
	return memory.SpeakerRules{
		StoreAssistant: envBool("STORE_ASSISTANT_MESSAGES", true),
		StoreUser:      envBool("STORE_USER_MESSAGES", true),
		StoreSystem:    envBool("STORE_SYSTEM_MESSAGES", false),
	}
}

func envBool(key string, fallback bool) bool {
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

func buildCLIIngestor(cfg config.Config) (memory.Ingestor, error) {
	embedder, err := selectCLIEmbedder(cfg)
	if err != nil {
		return memory.Ingestor{}, err
	}

	qdrant := store.NewQdrantClient(cfg.Qdrant.URL)
	fallbackStore := memory.NewInMemoryStore()
	memStore := store.NewQdrantMemoryStore(
		qdrant,
		embedder,
		cfg.Qdrant.Collection,
		cfg.Qdrant.VectorName,
		cfg.Qdrant.Distance,
		fallbackStore,
	)

	return memory.Ingestor{
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
	}, nil
}

func selectCLIEmbedder(cfg config.Config) (embed.Embedder, error) {
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
