package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/memoryloop"
	"github.com/meistro57/frontpocket/internal/store"
)

func runMemoryLoopCommand(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "approve":
			return runMemoryLoopApprove(args[1:])
		case "reject":
			return runMemoryLoopReject(args[1:])
		case "merge":
			return runMemoryLoopMerge(args[1:])
		case "list":
			return runMemoryLoopList(args[1:])
		case "run":
			return runMemoryLoop(args[1:])
		}
	}
	return runMemoryLoop(args)
}

func runMemoryLoop(args []string) error {
	flags := flag.NewFlagSet("frontpocket memory-loop", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	batchSize := flags.Int("batch-size", 200, "batch size for memory harvesting")
	project := flags.String("project", "", "project filter")
	speaker := flags.String("speaker", "", "speaker filter")
	kind := flags.String("kind", "", "memory kind filter")
	since := flags.String("since", "", "start date (YYYY-MM-DD or RFC3339)")
	until := flags.String("until", "", "end date (YYYY-MM-DD or RFC3339)")
	dryRun := flags.Bool("dry-run", false, "print candidates without writing")
	writeCandidates := flags.Bool("write-candidates", false, "write candidates to proposed canon queue")
	limit := flags.Int("limit", 0, "max memories to process")
	verbose := flags.Bool("verbose", false, "print full candidate payload")
	clusterSize := flags.Int("cluster-size", 25, "max points per cluster")
	canonicalOnly := flags.Bool("canonical-only", false, "harvest only canonical records")
	includeExistingCanon := flags.Bool("include-existing-canon", false, "include canonical records in harvesting")
	queuePath := flags.String("queue-path", memoryLoopQueuePath(), "proposed canon queue file path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if !*dryRun && !*writeCandidates {
		return fmt.Errorf("choose --dry-run or --write-candidates")
	}

	sinceTime, err := parseOptionalDate(*since)
	if err != nil {
		return err
	}
	untilTime, err := parseOptionalDate(*until)
	if err != nil {
		return err
	}

	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	qdrant := store.NewQdrantMemoryStore(store.NewQdrantClient(cfg.Qdrant.URL), nil, cfg.Qdrant.Collection, cfg.Qdrant.VectorName, cfg.Qdrant.Distance, nil)
	harvester := memoryloop.NewMemoryHarvester(qdrant)
	runner := memoryloop.Runner{
		Harvester:   harvester,
		Clusterer:   memoryloop.MemoryClusterer{MaxClusterSize: *clusterSize},
		Extractor:   memoryloop.FactExtractor{},
		Scout:       memoryloop.ContradictionScout{},
		Timeline:    memoryloop.TimelineBuilder{},
		Curator:     memoryloop.CanonCurator{},
		ReviewQueue: memoryloop.NewFileReviewQueue(*queuePath),
	}
	result, err := runner.Run(context.Background(), memoryloop.HarvestFilter{
		Project:              strings.TrimSpace(*project),
		Speaker:              strings.TrimSpace(*speaker),
		MemoryKind:           strings.TrimSpace(*kind),
		Since:                sinceTime,
		Until:                untilTime,
		BatchSize:            *batchSize,
		Limit:                *limit,
		CanonicalOnly:        *canonicalOnly,
		IncludeExistingCanon: *includeExistingCanon,
	}, *writeCandidates)
	if err != nil {
		return err
	}

	if *dryRun {
		for _, candidate := range result.Candidates {
			printCandidate(candidate, *verbose)
		}
	}
	fmt.Printf("candidates_created=%d\n", len(result.Candidates))
	fmt.Printf("duplicates_skipped=%d\n", result.SkippedDuplicates)
	fmt.Printf("contradictions_marked=%d\n", result.ContradictionMarked)
	if *writeCandidates {
		fmt.Printf("queue_path=%s\n", *queuePath)
	}
	return nil
}

func runMemoryLoopList(args []string) error {
	flags := flag.NewFlagSet("frontpocket memory-loop list", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	status := flags.String("status", "", "status filter")
	kind := flags.String("kind", "", "memory kind filter")
	project := flags.String("project", "", "project filter")
	speaker := flags.String("speaker", "", "speaker filter")
	confidence := flags.String("confidence", "", "confidence filter")
	queuePath := flags.String("queue-path", memoryLoopQueuePath(), "proposed canon queue file path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	queue := memoryloop.NewFileReviewQueue(*queuePath)
	records, err := queue.List(memoryloop.CandidateFilter{
		Status:     strings.TrimSpace(*status),
		MemoryKind: strings.TrimSpace(*kind),
		Project:    strings.TrimSpace(*project),
		Speaker:    strings.TrimSpace(*speaker),
		Confidence: strings.TrimSpace(*confidence),
	})
	if err != nil {
		return err
	}
	for _, record := range records {
		printCandidate(record, false)
	}
	fmt.Printf("total=%d\n", len(records))
	return nil
}

func runMemoryLoopApprove(args []string) error {
	flags := flag.NewFlagSet("frontpocket memory-loop approve", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	id := flags.String("id", "", "candidate id")
	reviewedBy := flags.String("reviewed-by", "user", "reviewer id")
	queuePath := flags.String("queue-path", memoryLoopQueuePath(), "proposed canon queue file path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*id) == "" {
		return fmt.Errorf("--id is required")
	}
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	embedder, err := selectCLIEmbedder(cfg)
	if err != nil {
		return err
	}
	canonicalStore := store.NewQdrantMemoryStore(store.NewQdrantClient(cfg.Qdrant.URL), embedder, cfg.Qdrant.Collection, cfg.Qdrant.VectorName, cfg.Qdrant.Distance, memory.NewInMemoryStore())
	queue := memoryloop.NewFileReviewQueue(*queuePath)
	candidate, err := queue.Approve(context.Background(), strings.TrimSpace(*id), strings.TrimSpace(*reviewedBy), canonicalStore, nil)
	if err != nil {
		return err
	}
	printCandidate(candidate, false)
	return nil
}

func runMemoryLoopReject(args []string) error {
	flags := flag.NewFlagSet("frontpocket memory-loop reject", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	id := flags.String("id", "", "candidate id")
	reason := flags.String("reason", "", "rejection reason")
	reviewedBy := flags.String("reviewed-by", "user", "reviewer id")
	queuePath := flags.String("queue-path", memoryLoopQueuePath(), "proposed canon queue file path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*id) == "" {
		return fmt.Errorf("--id is required")
	}
	queue := memoryloop.NewFileReviewQueue(*queuePath)
	candidate, err := queue.Reject(strings.TrimSpace(*id), strings.TrimSpace(*reason), strings.TrimSpace(*reviewedBy))
	if err != nil {
		return err
	}
	printCandidate(candidate, false)
	return nil
}

func runMemoryLoopMerge(args []string) error {
	flags := flag.NewFlagSet("frontpocket memory-loop merge", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	id := flags.String("id", "", "candidate id")
	target := flags.String("target", "", "merge target candidate or canonical id")
	reviewedBy := flags.String("reviewed-by", "user", "reviewer id")
	queuePath := flags.String("queue-path", memoryLoopQueuePath(), "proposed canon queue file path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*id) == "" || strings.TrimSpace(*target) == "" {
		return fmt.Errorf("--id and --target are required")
	}
	queue := memoryloop.NewFileReviewQueue(*queuePath)
	candidate, err := queue.Merge(strings.TrimSpace(*id), strings.TrimSpace(*target), strings.TrimSpace(*reviewedBy))
	if err != nil {
		return err
	}
	printCandidate(candidate, false)
	return nil
}

func parseOptionalDate(raw string) (time.Time, error) {
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
	return time.Time{}, fmt.Errorf("invalid date %q (use YYYY-MM-DD or RFC3339)", raw)
}

func memoryLoopQueuePath() string {
	if env := strings.TrimSpace(os.Getenv("FRONTPOCKET_PROPOSED_CANON_PATH")); env != "" {
		return env
	}
	return "data/proposed_canon.json"
}

func printCandidate(candidate memoryloop.Candidate, verbose bool) {
	fmt.Printf("- id=%s kind=%s confidence=%s status=%s project=%s\n", candidate.ID, candidate.MemoryKind, candidate.Confidence, candidate.Status, candidate.Project)
	fmt.Printf("  summary=%s\n", candidate.Summary)
	fmt.Printf("  source_memory_ids=%s\n", strings.Join(candidate.SourceMemoryIDs, ","))
	fmt.Printf("  source_quotes=%s\n", strings.Join(candidate.SourceQuotes, " | "))
	if verbose {
		payload, _ := json.MarshalIndent(candidate, "", "  ")
		fmt.Println(string(payload))
	}
}
