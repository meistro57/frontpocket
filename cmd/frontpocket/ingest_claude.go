package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/memory"
)

func runIngestClaude(args []string) error {
	flags := flag.NewFlagSet("frontpocket ingest claude", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	flags.Usage = func() {
		printIngestClaudeHelp(flags)
	}

	dryRun := flags.Bool("dry-run", false, "parse and report stats without writing to storage")
	project := flags.String("project", "", "project label to attach to imported records")
	conversation := flags.String("conversation", "", "only include conversations whose title or id contains this value")
	conversationID := flags.String("conversation-id", "", "only include the conversation with exactly this id")
	limit := flags.Int("limit", 0, "cap the number of conversations processed (0 = no limit)")
	resume := flags.String("resume", "", "path to a JSON progress journal; resumes from it if present and updates it as batches are stored")
	aiProvider := flags.String("ai-provider", "claude", "AI provider label stored in memory payloads")

	normalizedArgs, sourcePath := normalizeIngestClaudeArgs(args)
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
		return fmt.Errorf("usage: frontpocket ingest claude <export-folder> [--dry-run] [--project <name>] [--conversation <match>] [--conversation-id <id>] [--limit <n>] [--resume <path>] [--ai-provider <name>]")
	}
	if strings.TrimSpace(*aiProvider) == "" {
		return fmt.Errorf("--ai-provider is required (example: --ai-provider claude)")
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

	result, err := memory.ParseClaudeExport(positionals[0], memory.ClaudeImportOptions{
		Project:            strings.TrimSpace(*project),
		ConversationFilter: strings.TrimSpace(*conversation),
		ConversationID:     strings.TrimSpace(*conversationID),
		SpeakerRules:       rules,
		Limit:              *limit,
		AIProvider:         strings.TrimSpace(*aiProvider),
	})
	if err != nil {
		return err
	}

	printClaudeImportSummary(result)
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

	ingestor, qdrantClient, err := buildCLIIngestor(cfg)
	if err != nil {
		fmt.Printf("storage write: not wired (%v)\n", err)
		return nil
	}
	ingestor.SourceType = result.SourceType
	ingestor.AIProvider = strings.TrimSpace(*aiProvider)
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

	integrityFilter := buildCLIIngestIntegrityFilter(strings.TrimSpace(result.SourceType), strings.TrimSpace(*project), strings.TrimSpace(*aiProvider))
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
		return fmt.Errorf("integrity check mismatch: expected %d new points but qdrant count changed by %d for ai_provider=%q source_type=%q project=%q", chunksEmbedded, actualAdded, strings.TrimSpace(*aiProvider), strings.TrimSpace(result.SourceType), strings.TrimSpace(*project))
	}
	fmt.Println("integrity check: match")
	if journal != nil {
		if err := journal.Remove(); err != nil {
			fmt.Printf("resume: could not remove completed journal: %v\n", err)
		}
	}
	return nil
}

func printIngestClaudeHelp(flags *flag.FlagSet) {
	fmt.Fprintln(flags.Output(), "Usage:")
	fmt.Fprintln(flags.Output(), "  frontpocket ingest claude <export-folder> [--dry-run] [--project <name>] [--conversation <match>] [--conversation-id <id>] [--limit <n>] [--resume <path>] [--ai-provider <name>]")
	fmt.Fprintln(flags.Output())
	fmt.Fprintln(flags.Output(), "Command Reference:")
	fmt.Fprintln(flags.Output(), "  frontpocket ingest --help")
	fmt.Fprintln(flags.Output(), "  frontpocket ingest claude --help")
	fmt.Fprintln(flags.Output())
	fmt.Fprintln(flags.Output(), "Options:")
	flags.PrintDefaults()
}

func normalizeIngestClaudeArgs(args []string) ([]string, string) {
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
			if trimmed == "--project" || trimmed == "--conversation" || trimmed == "--conversation-id" || trimmed == "--limit" || trimmed == "--resume" || trimmed == "--ai-provider" {
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

func printClaudeImportSummary(result memory.ClaudeImportResult) {
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

	if len(result.ContentTypesEncountered) == 0 {
		fmt.Println("content types encountered: none")
	} else {
		names := make([]string, 0, len(result.ContentTypesEncountered))
		for name := range result.ContentTypesEncountered {
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, name := range names {
			parts = append(parts, fmt.Sprintf("%s=%d", name, result.ContentTypesEncountered[name]))
		}
		fmt.Printf("content types encountered: %s\n", strings.Join(parts, ", "))
	}

	if len(result.UnsupportedContentTypes) == 0 {
		fmt.Println("unsupported content types: none")
	} else {
		names := make([]string, 0, len(result.UnsupportedContentTypes))
		for name := range result.UnsupportedContentTypes {
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, name := range names {
			parts = append(parts, fmt.Sprintf("%s=%d", name, result.UnsupportedContentTypes[name]))
		}
		fmt.Printf("unsupported content types: %s\n", strings.Join(parts, ", "))
	}
	fmt.Printf("branching conversations: %d\n", result.BranchingConversations)
	fmt.Printf("branch parent occurrences: %d\n", result.BranchParentOccurrences)
	fmt.Printf("max children for one parent: %d\n", result.MaxChildrenForParent)
	fmt.Printf("memories.json points: %d\n", result.MemoriesPoints)
	fmt.Printf("projects points: %d\n", result.ProjectsPoints)
}
