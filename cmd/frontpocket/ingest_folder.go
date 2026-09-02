package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/memory"
)

func runIngestFolder(args []string) error {
	flags := flag.NewFlagSet("frontpocket ingest folder", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	flags.Usage = func() {
		printIngestFolderHelp(flags)
	}

	dryRun := flags.Bool("dry-run", false, "scan, parse, and report stats without writing to storage")
	project := flags.String("project", "", "project label to attach to imported records (e.g. UTMGR)")
	cacheDir := flags.String("cache-dir", ".frontpocket_cache", "directory for persistent cache (slides, audio transcripts, caption cache)")
	resume := flags.String("resume", "", "path to a JSON progress journal; resumes from it if present and updates it as batches are stored")
	noAudio := flags.Bool("no-audio", false, "skip transcribing audio and video files (.m4a, .mp4, etc.)")
	noVision := flags.Bool("no-vision", false, "skip vision model calls for presentation slides (.pdf) and images (.png)")
	limit := flags.Int("limit", 0, "cap the number of files processed (0 = no limit)")
	out := flags.String("out", "", "write normalized JSONL output of extracted records to this path")
	aiProvider := flags.String("ai-provider", "frontpocket", "AI provider label stored in memory payloads")

	normalizedArgs, folderPath := normalizeIngestFolderArgs(args)
	if err := flags.Parse(normalizedArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	positionals := flags.Args()
	if folderPath != "" {
		positionals = append([]string{folderPath}, positionals...)
	}

	targetPath := ""
	if len(positionals) >= 1 {
		targetPath = positionals[0]
	} else {
		// Default to incoming/ if present, otherwise current directory
		if fi, err := os.Stat("incoming"); err == nil && fi.IsDir() {
			targetPath = "incoming"
		} else {
			targetPath = "."
		}
	}

	resolvedPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolve folder path %s: %w", targetPath, err)
	}

	resolvedCacheDir, err := filepath.Abs(strings.TrimSpace(*cacheDir))
	if err != nil {
		return fmt.Errorf("resolve cache dir: %w", err)
	}

	_ = config.LoadDotEnv(".env")
	cfg, cfgErr := config.Load()

	var captioner memory.Captioner
	var captionCache *memory.CaptionCache

	if !*noVision && cfgErr == nil {
		captioner = buildCLICaptioner(cfg)
		cachePath := filepath.Join(resolvedCacheDir, "caption_cache.json")
		cache, cErr := memory.OpenCaptionCache(cachePath)
		if cErr == nil {
			captionCache = cache
			captioner = memory.NewCachingCaptioner(captioner, cache)
			fmt.Printf("caption cache: %s (%d entries)\n", cachePath, cache.Count())
		}
	}

	proj := strings.TrimSpace(*project)
	if proj == "" {
		proj = filepath.Base(resolvedPath)
	}

	fmt.Printf("scanning folder: %s (project: %s, cache: %s)\n", resolvedPath, proj, resolvedCacheDir)

	result, err := memory.ParseFolder(context.Background(), resolvedPath, memory.FolderImportOptions{
		Path:            resolvedPath,
		Project:         proj,
		CacheDir:        resolvedCacheDir,
		NoAudio:         *noAudio,
		NoVision:        *noVision,
		Limit:           *limit,
		AIProvider:      strings.TrimSpace(*aiProvider),
		VisionCaptioner: captioner,
		CaptionCache:    captionCache,
		ProgressFn: func(ev memory.FolderProgressEvent) {
			if ev.CurrentFile != "" {
				fmt.Printf("[%d/%d] %s...\n", ev.FilesDone+1, ev.FilesTotal, ev.Message)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("folder import failed: %w", err)
	}

	printFolderImportSummary(result)

	if strings.TrimSpace(*out) != "" {
		outPath := strings.TrimSpace(*out)
		if err := writeRecordsJSONL(outPath, result.Records); err != nil {
			return fmt.Errorf("write JSONL out %s: %w", outPath, err)
		}
		fmt.Printf("wrote %d records to %s\n", len(result.Records), outPath)
	}

	if *dryRun {
		fmt.Println("storage write: skipped (--dry-run)")
		return nil
	}

	if len(result.Records) == 0 {
		fmt.Println("storage write: skipped (no records produced)")
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
	ingestor.SourceType = "document"
	ingestor.AIProvider = strings.TrimSpace(*aiProvider)
	ingestor.Project = proj

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
		fmt.Printf("embedding progress: %d/%d records (%d chunks embedded) elapsed=%s eta=%s\n",
			ev.RecordsProcessed, ev.RecordsTotal, ev.ChunksEmbedded, elapsed, eta)
	}

	var journal *memory.FileJournal
	resumePath := strings.TrimSpace(*resume)
	if resumePath == "" {
		resumePath = filepath.Join(resolvedCacheDir, "resume_journal.json")
	}
	if resumePath != "" {
		j, resumed, jErr := memory.OpenFileJournal(resumePath, memory.JournalMeta{
			Source:         resolvedPath,
			Collection:     cfg.Qdrant.Collection,
			EmbeddingModel: ingestor.Embedder.ModelName(),
		})
		if jErr == nil {
			journal = j
			ingestor.Journal = j
			if resumed {
				fmt.Printf("resume: continuing from record %d (journal %s)\n", j.LastRecordIndex()+1, resumePath)
			} else {
				fmt.Printf("resume: tracking progress in %s\n", resumePath)
			}
		}
	}

	integrityFilter := buildCLIIngestIntegrityFilter(strings.TrimSpace(ingestor.SourceType), strings.TrimSpace(proj), strings.TrimSpace(*aiProvider))
	baselineCount, _ := qdrantClient.CountPoints(context.Background(), cfg.Qdrant.Collection, integrityFilter)

	points, err := ingestor.Ingest(context.Background(), result.Records)
	if err != nil {
		return fmt.Errorf("ingest records to storage: %w", err)
	}

	finalCount, _ := qdrantClient.CountPoints(context.Background(), cfg.Qdrant.Collection, integrityFilter)
	actualAdded := finalCount - baselineCount
	fmt.Printf("storage write: inserted %d memory points into %s (embedded chunks: %d, collection total for project: %d, newly added: %d)\n",
		len(points), cfg.Qdrant.Collection, chunksEmbedded, finalCount, actualAdded)
	if journal != nil {
		_ = journal.Remove()
	}

	return nil
}

func printFolderImportSummary(r *memory.FolderImportResult) {
	fmt.Println("----------------------------------------")
	fmt.Printf("folder import summary:\n")
	fmt.Printf("  path:                %s\n", r.SourcePath)
	fmt.Printf("  project:             %s\n", r.Project)
	fmt.Printf("  files scanned:       %d\n", r.FilesScanned)
	fmt.Printf("  files processed:     %d\n", r.FilesProcessed)
	fmt.Printf("  files skipped:       %d\n", r.FilesSkipped)
	fmt.Printf("  documents parsed:    %d\n", r.DocumentsParsed)
	fmt.Printf("  slides parsed:       %d\n", r.SlidesParsed)
	fmt.Printf("  images parsed:       %d\n", r.ImagesParsed)
	fmt.Printf("  audios transcribed:  %d\n", r.AudiosTranscribed)
	fmt.Printf("  total memory records:%d\n", r.TotalRecords)
	fmt.Println("----------------------------------------")
}

func writeRecordsJSONL(path string, records []memory.MessageRecord) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, rec := range records {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func printIngestFolderHelp(flags *flag.FlagSet) {
	fmt.Fprintln(flags.Output(), "Usage:")
	fmt.Fprintln(flags.Output(), "  frontpocket ingest folder [path] [options]")
	fmt.Fprintln(flags.Output())
	fmt.Fprintln(flags.Output(), "Description:")
	fmt.Fprintln(flags.Output(), "  Auto-detects and ingests mixed media (.docx, .pdf, .png, .m4a, .mp4, .md, .txt, .jsonl)")
	fmt.Fprintln(flags.Output(), "  from incoming/ or any specified directory.")
	fmt.Fprintln(flags.Output())
	fmt.Fprintln(flags.Output(), "Options:")
	flags.PrintDefaults()
}

func normalizeIngestFolderArgs(args []string) ([]string, string) {
	if len(args) == 0 {
		return nil, ""
	}

	normalized := make([]string, 0, len(args))
	folderPath := ""
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
		if strings.HasPrefix(trimmed, "-") {
			normalized = append(normalized, trimmed)
			if trimmed == "--project" || trimmed == "--cache-dir" || trimmed == "--resume" || trimmed == "--limit" || trimmed == "--out" || trimmed == "--ai-provider" {
				expectsValue = true
			}
			continue
		}
		if folderPath == "" {
			folderPath = trimmed
			continue
		}
		normalized = append(normalized, trimmed)
	}

	return normalized, folderPath
}
