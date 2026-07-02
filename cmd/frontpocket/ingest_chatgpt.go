package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
		return fmt.Errorf("ingest subcommand is required (see: frontpocket ingest --help)")
	}
	if args[0] == "--help" || args[0] == "-help" || args[0] == "-h" {
		printIngestHelp(os.Stdout)
		return nil
	}
	switch args[0] {
	case "chatgpt":
		return runIngestChatGPT(args[1:])
	case "claude":
		return runIngestClaude(args[1:])
	default:
		return fmt.Errorf("unsupported ingest source %q", args[0])
	}
}

func runIngestChatGPT(args []string) error {
	flags := flag.NewFlagSet("frontpocket ingest chatgpt", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	flags.Usage = func() {
		printIngestChatGPTHelp(flags)
	}

	dryRun := flags.Bool("dry-run", false, "parse and report stats without writing to storage")
	project := flags.String("project", "", "project label to attach to imported records")
	since := flags.String("since", "", "only include messages at or after this date (YYYY-MM-DD or RFC3339)")
	conversation := flags.String("conversation", "", "only include conversations whose title or id contains this value")
	conversationID := flags.String("conversation-id", "", "only include the conversation with exactly this id (exact match, useful for targeted debugging)")
	out := flags.String("out", "", "write normalized JSONL output to this path")
	resume := flags.String("resume", "", "path to a JSON progress journal; resumes from it if present and updates it as batches are stored")
	captionCachePath := flags.String("caption-cache", "", "path to persistent attachment caption cache (defaults alongside --resume when set)")
	parseCheckpointPath := flags.String("parse-checkpoint", "", "path to parse-phase conversation checkpoint file (defaults alongside --resume when set)")
	limit := flags.Int("limit", 0, "cap the number of conversations processed (0 = no limit)")
	noCaption := flags.Bool("no-caption", false, "resolve attachment metadata but skip vision captioning API calls")
	aiProvider := flags.String("ai-provider", "", "AI provider label stored in memory payloads (required, e.g. chatgpt)")

	normalizedArgs, sourcePath := normalizeIngestChatGPTArgs(args)
	if err := flags.Parse(normalizedArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	positionals := flags.Args()
	if sourcePath != "" {
		positionals = append([]string{sourcePath}, positionals...)
	}
	if len(positionals) != 1 {
		return fmt.Errorf("usage: frontpocket ingest chatgpt <zip-or-folder> [--dry-run] [--project <name>] [--since <date>] [--conversation <match>] [--conversation-id <id>] [--out <path>] [--resume <path>] [--caption-cache <path>] [--parse-checkpoint <path>] [--limit <n>] [--no-caption] [--ai-provider <name>]")
	}
	if strings.TrimSpace(*aiProvider) == "" {
		return fmt.Errorf("--ai-provider is required (example: --ai-provider chatgpt)")
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

	resolvedSourcePath, err := filepath.Abs(positionals[0])
	if err != nil {
		return err
	}

	var captioner memory.Captioner
	if !*noCaption && cfgErr == nil {
		captioner = buildCLICaptioner(cfg)
		cachePath := defaultCaptionCachePath(strings.TrimSpace(*captionCachePath), strings.TrimSpace(*resume))
		if cachePath != "" {
			cache, cacheErr := memory.OpenCaptionCache(cachePath)
			if cacheErr != nil {
				return cacheErr
			}
			captioner = memory.NewCachingCaptioner(captioner, cache)
			fmt.Printf("caption cache: %s (%d entries)\n", cachePath, cache.Count())
		}
	}

	checkpointPath := defaultParseCheckpointPath(strings.TrimSpace(*parseCheckpointPath), strings.TrimSpace(*resume))
	var parseCheckpoint *memory.ChatGPTParseCheckpoint
	if checkpointPath != "" {
		cp, resumed, cpErr := memory.OpenChatGPTParseCheckpoint(checkpointPath, memory.ChatGPTParseCheckpointMeta{
			Source:            resolvedSourcePath,
			ConversationID:    strings.TrimSpace(*conversationID),
			ConversationMatch: strings.TrimSpace(*conversation),
			Since:             memory.ChatGPTParseCheckpointSinceValue(sinceTime),
			Limit:             *limit,
			CaptioningEnabled: captioner != nil && !*dryRun,
		})
		if cpErr != nil {
			return cpErr
		}
		parseCheckpoint = cp
		if resumed {
			fmt.Printf("parse checkpoint: continuing with %d completed conversations (%s)\n", cp.DoneCount(), checkpointPath)
		} else {
			fmt.Printf("parse checkpoint: tracking conversation progress in %s\n", checkpointPath)
		}
	}

	result, err := memory.ParseChatGPTExport(positionals[0], memory.ChatGPTImportOptions{
		Project:            strings.TrimSpace(*project),
		ConversationFilter: strings.TrimSpace(*conversation),
		ConversationID:     strings.TrimSpace(*conversationID),
		Since:              sinceTime,
		SpeakerRules:       rules,
		Limit:              *limit,
		Captioner:          captioner,
		DryRun:             *dryRun,
		AIProvider:         strings.TrimSpace(*aiProvider),
		ParseCheckpoint:    parseCheckpoint,
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
	if parseCheckpoint != nil {
		fmt.Printf("parse checkpoint: skipped=%d newly_checkpointed=%d total_completed=%d\n", result.ConversationsSkippedByCheckpoint, result.ConversationsCheckpointed, parseCheckpoint.DoneCount())
	}

	if *dryRun {
		printDryRunSignalSummary(result)
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

	ingestor, qdrantClient, err := buildCLIIngestor(cfg)
	if err != nil {
		fmt.Printf("storage write: not wired (%v)\n", err)
		return nil
	}

	chunksEmbedded := 0
	ingestor.ProgressFn = func(ev memory.ProgressEvent) {
		chunksEmbedded = ev.ChunksEmbedded
		elapsed := ev.Elapsed.Truncate(time.Second)
		rate := 0.0
		if ev.Elapsed.Seconds() > 0 {
			rate = float64(ev.RecordsProcessed) / ev.Elapsed.Seconds()
		}
		eta := time.Duration(0)
		if rate > 0 && ev.RecordsTotal > ev.RecordsProcessed {
			eta = time.Duration(float64(ev.RecordsTotal-ev.RecordsProcessed)/rate) * time.Second
		}
		fmt.Printf("embedding progress: %d/%d records (%d chunks) elapsed=%s eta=%s\n",
			ev.RecordsProcessed, ev.RecordsTotal, ev.ChunksEmbedded, elapsed, eta)
	}

	var journal *memory.FileJournal
	if path := strings.TrimSpace(*resume); path != "" {
		logMissingResumeJournal(path)
		j, resumed, err := memory.OpenFileJournal(path, memory.JournalMeta{
			Source:         resolvedSourcePath,
			Collection:     cfg.Qdrant.Collection,
			EmbeddingModel: ingestor.Embedder.ModelName(),
		})
		if err != nil {
			return err
		}
		journal = j
		ingestor.Journal = j
		if resumed {
			fmt.Printf("resume: continuing from record %d (journal %s)\n", j.LastRecordIndex()+1, path)
		} else {
			fmt.Printf("resume: tracking progress in %s\n", path)
		}
	}

	integritySourceType := strings.TrimSpace(result.SourceType)
	if integritySourceType == "" {
		integritySourceType = cfg.Ingestion.DefaultSourceType
	}
	integrityFilter := buildCLIIngestIntegrityFilter(integritySourceType, strings.TrimSpace(*project), strings.TrimSpace(*aiProvider))
	baselineCount, err := qdrantClient.CountPoints(context.Background(), cfg.Qdrant.Collection, integrityFilter)
	if err != nil {
		return fmt.Errorf("integrity check baseline failed: %w", err)
	}

	points, err := ingestor.Ingest(context.Background(), memory.ToMessageRecords(result.Records))
	if err != nil {
		return fmt.Errorf("parsed %d records but storage ingest failed: %w", len(result.Records), err)
	}
	fmt.Printf("storage write: inserted %d memory points\n", len(points))
	if chunksEmbedded == 0 {
		chunksEmbedded = len(points)
	}
	finalCount, err := qdrantClient.CountPoints(context.Background(), cfg.Qdrant.Collection, integrityFilter)
	if err != nil {
		return fmt.Errorf("integrity check final count failed: %w", err)
	}
	actualAdded := finalCount - baselineCount
	fmt.Printf("integrity check: baseline=%d final=%d expected_new=%d actual_new=%d\n", baselineCount, finalCount, chunksEmbedded, actualAdded)
	if actualAdded != chunksEmbedded {
		return fmt.Errorf("integrity check mismatch: expected %d new points but qdrant count changed by %d for ai_provider=%q source_type=%q project=%q", chunksEmbedded, actualAdded, strings.TrimSpace(*aiProvider), integritySourceType, strings.TrimSpace(*project))
	}
	fmt.Println("integrity check: match")

	if journal != nil {
		if err := journal.Remove(); err != nil {
			fmt.Printf("resume: could not remove completed journal: %v\n", err)
		}
	}
	if parseCheckpoint != nil {
		if err := parseCheckpoint.Remove(); err != nil {
			fmt.Printf("parse checkpoint: could not remove completed checkpoint: %v\n", err)
		}
	}
	return nil
}

func printIngestHelp(output *os.File) {
	fmt.Fprintln(output, "Usage:")
	fmt.Fprintln(output, "  frontpocket ingest <subcommand> [options]")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Subcommands:")
	fmt.Fprintln(output, "  chatgpt      Import from a ChatGPT export zip or folder.")
	fmt.Fprintln(output, "  claude       Import from a Claude export folder.")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Help:")
	fmt.Fprintln(output, "  frontpocket ingest --help")
	fmt.Fprintln(output, "  frontpocket ingest chatgpt --help")
	fmt.Fprintln(output, "  frontpocket ingest claude --help")
}

func printIngestChatGPTHelp(flags *flag.FlagSet) {
	fmt.Fprintln(flags.Output(), "Usage:")
	fmt.Fprintln(flags.Output(), "  frontpocket ingest chatgpt <zip-or-folder> [--dry-run] [--project <name>] [--since <date>] [--conversation <match>] [--conversation-id <id>] [--out <path>] [--resume <path>] [--caption-cache <path>] [--parse-checkpoint <path>] [--limit <n>] [--no-caption] [--ai-provider <name>]")
	fmt.Fprintln(flags.Output())
	fmt.Fprintln(flags.Output(), "Command Reference:")
	fmt.Fprintln(flags.Output(), "  frontpocket ingest --help")
	fmt.Fprintln(flags.Output(), "  frontpocket ingest chatgpt --help")
	fmt.Fprintln(flags.Output())
	fmt.Fprintln(flags.Output(), "Options:")
	flags.PrintDefaults()
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
	fmt.Printf("attachments captioned: %d (failed: %d)\n", result.AttachmentsCaptioned, result.AttachmentsCaptionFailed)
	if result.CaptionCacheHits > 0 || result.CaptionCacheMisses > 0 || result.CaptionCacheWrites > 0 {
		fmt.Printf("caption cache: hits=%d misses=%d writes=%d\n", result.CaptionCacheHits, result.CaptionCacheMisses, result.CaptionCacheWrites)
	}
}

func printDryRunSignalSummary(result memory.ChatGPTImportResult) {
	fmt.Printf("dry-run signals: starred=%d shared=%d feedback=%d (thumbs_up=%d, thumbs_down=%d)\n",
		result.StarredConversations,
		result.SharedConversations,
		result.FeedbackConversations,
		result.FeedbackThumbsUp,
		result.FeedbackThumbsDown,
	)
	fmt.Printf("dry-run attachments: total=%d resolved_asset_files=%d resolved_library_files=%d unresolved=%d would_caption=%d\n",
		result.AttachmentsTotal,
		result.AttachmentsResolvedAssetFiles,
		result.AttachmentsResolvedLibraryFiles,
		result.AttachmentsUnresolved,
		result.AttachmentsWouldCaption,
	)
}

func buildCLICaptioner(cfg config.Config) memory.Captioner {
	return memory.NewVisionCaptioner(
		cfg.Embedding.OpenRouterURL,
		cfg.Embedding.OpenRouterKey,
		cfg.Vision.Model,
		cfg.Embedding.OpenRouterSite,
		cfg.Embedding.OpenRouterApp,
	)
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
			if trimmed == "--project" || trimmed == "--since" || trimmed == "--conversation" || trimmed == "--conversation-id" || trimmed == "--out" || trimmed == "--resume" || trimmed == "--caption-cache" || trimmed == "--parse-checkpoint" || trimmed == "--limit" || trimmed == "--ai-provider" {
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

func logMissingResumeJournal(path string) {
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		fmt.Printf("resume: journal not found at %s; starting fresh\n", path)
	}
}

func defaultCaptionCachePath(explicitPath, resumePath string) string {
	if strings.TrimSpace(explicitPath) != "" {
		return strings.TrimSpace(explicitPath)
	}
	if strings.TrimSpace(resumePath) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(strings.TrimSpace(resumePath)), "caption_cache.json")
}

func defaultParseCheckpointPath(explicitPath, resumePath string) string {
	if strings.TrimSpace(explicitPath) != "" {
		return strings.TrimSpace(explicitPath)
	}
	if strings.TrimSpace(resumePath) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(strings.TrimSpace(resumePath)), "parse_checkpoint.json")
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

func buildCLIIngestor(cfg config.Config) (memory.Ingestor, *store.QdrantClient, error) {
	embedder, err := selectCLIEmbedder(cfg)
	if err != nil {
		return memory.Ingestor{}, nil, err
	}

	qdrant := store.NewQdrantClient(cfg.Qdrant.URL)
	memStore := store.NewQdrantMemoryStore(
		qdrant,
		embedder,
		cfg.Qdrant.Collection,
		cfg.Qdrant.VectorName,
		cfg.Qdrant.Distance,
		nil,
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
	}, qdrant, nil
}

func buildCLIIngestIntegrityFilter(sourceType, project, aiProvider string) map[string]string {
	filter := map[string]string{}
	if strings.TrimSpace(aiProvider) != "" {
		filter["ai_provider"] = strings.TrimSpace(aiProvider)
	}
	if strings.TrimSpace(sourceType) != "" {
		filter["source_type"] = strings.TrimSpace(sourceType)
	}
	if strings.TrimSpace(project) != "" {
		filter["project"] = strings.TrimSpace(project)
	}
	return filter
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
