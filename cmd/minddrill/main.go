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
	if len(args) > 0 {
		switch strings.ToLower(strings.TrimSpace(args[0])) {
		case "inspect":
			return runInspect(args[1:])
		case "deepdrill":
			return runDeepDrill(args[1:])
		}
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
		fmt.Fprintln(flags.Output(), "  deepdrill <command>     Run bounded autonomous research planning/iterations.")
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "Options:")
		flags.PrintDefaults()
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "Examples:")
		fmt.Fprintln(flags.Output(), "  minddrill")
		fmt.Fprintln(flags.Output(), "  minddrill --port 9000")
		fmt.Fprintln(flags.Output(), "  minddrill --api http://localhost:8088")
		fmt.Fprintln(flags.Output(), "  minddrill inspect frontpocket_memory")
		fmt.Fprintln(flags.Output(), "  minddrill deepdrill plan --session centerstone --collection frontpocket_memory")
		fmt.Fprintln(flags.Output(), "  minddrill deepdrill run --session centerstone --steps 3")
		fmt.Fprintln(flags.Output(), "  minddrill deepdrill thoughts --session centerstone --summary")
		fmt.Fprintln(flags.Output(), "  minddrill deepdrill show deepdrill_centerstone_12345")
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
	runtime.ConfigureDeepDrill(cfg.DeepDrill.ThoughtCollection, cfg.DeepDrill.FreezeLowGainAfter)
	report, err := tools.InspectCollection(context.Background(), runtime, collection, *sampleLimit)
	if err != nil {
		return err
	}
	fmt.Println(toJSON(report))
	return nil
}

func runDeepDrill(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("deepdrill requires a subcommand: plan, run, thoughts, show, or provenance")
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
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
	runtime.ConfigureDeepDrill(cfg.DeepDrill.ThoughtCollection, cfg.DeepDrill.FreezeLowGainAfter)
	switch action {
	case "plan":
		flags := flag.NewFlagSet("minddrill deepdrill plan", flag.ContinueOnError)
		flags.SetOutput(os.Stdout)
		sessionID := flags.String("session", "", "research session id")
		collection := flags.String("collection", "", "source collection (optional if already bound)")
		researchQuestion := flags.String("research-question", "", "research question for this session")
		hypothesesCSV := flags.String("hypotheses", "", "semicolon list of hypotheses as H1|statement;H2|statement")
		blockersCSV := flags.String("blockers", "", "comma list of uncertainty blockers")
		highestValueQuestion := flags.String("highest-value-question", "", "current highest-value discriminating question")
		forceReopen := flags.Bool("force-reopen", false, "force reopening a frozen branch")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if strings.TrimSpace(*sessionID) == "" {
			return fmt.Errorf("--session is required")
		}
		plan, err := runtime.DeepDrillPlan(context.Background(), tools.DeepDrillPlanRequest{
			SessionID:            *sessionID,
			Collection:           *collection,
			ResearchQuestion:     *researchQuestion,
			Hypotheses:           tools.ParseDeepDrillHypothesesCSV(*hypothesesCSV),
			KnownBlockers:        tools.ParseDeepDrillUncertaintiesCSV(*blockersCSV),
			HighestValueQuestion: *highestValueQuestion,
			ForceReopen:          *forceReopen,
		})
		if err != nil {
			return err
		}
		fmt.Println(toJSON(plan))
		return nil
	case "run":
		flags := flag.NewFlagSet("minddrill deepdrill run", flag.ContinueOnError)
		flags.SetOutput(os.Stdout)
		sessionID := flags.String("session", "", "research session id")
		collection := flags.String("collection", "", "source collection (optional if already bound)")
		researchQuestion := flags.String("research-question", "", "research question for this session")
		hypothesesCSV := flags.String("hypotheses", "", "semicolon list of hypotheses as H1|statement;H2|statement")
		blockersCSV := flags.String("blockers", "", "comma list of uncertainty blockers")
		highestValueQuestion := flags.String("highest-value-question", "", "current highest-value discriminating question")
		steps := flags.Int("steps", 1, "bounded DeepDrill iterations")
		untilStable := flags.Bool("until-stable", false, "run until branch freezes or limit is reached")
		forceReopen := flags.Bool("force-reopen", false, "force reopening a frozen branch")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if strings.TrimSpace(*sessionID) == "" {
			return fmt.Errorf("--session is required")
		}
		runResult, err := runtime.DeepDrillRun(context.Background(), tools.DeepDrillRunRequest{
			DeepDrillPlanRequest: tools.DeepDrillPlanRequest{
				SessionID:            *sessionID,
				Collection:           *collection,
				ResearchQuestion:     *researchQuestion,
				Hypotheses:           tools.ParseDeepDrillHypothesesCSV(*hypothesesCSV),
				KnownBlockers:        tools.ParseDeepDrillUncertaintiesCSV(*blockersCSV),
				HighestValueQuestion: *highestValueQuestion,
				ForceReopen:          *forceReopen,
			},
			Steps:       *steps,
			UntilStable: *untilStable,
		})
		if err != nil {
			return err
		}
		fmt.Println(toJSON(runResult))
		return nil
	case "thoughts":
		flags := flag.NewFlagSet("minddrill deepdrill thoughts", flag.ContinueOnError)
		flags.SetOutput(os.Stdout)
		sessionID := flags.String("session", "", "filter by session id")
		status := flags.String("status", "", "filter by status (OPEN/FROZEN/SUPERSEDED/RESOLVED or stored status)")
		typeName := flags.String("type", "", "filter by thought type")
		uncertainty := flags.String("uncertainty", "", "filter by uncertainty class")
		hypothesis := flags.String("hypothesis", "", "filter by hypothesis id")
		infoGain := flags.String("info-gain", "", "filter by info gain (LOW/MEDIUM/HIGH)")
		limit := flags.Int("limit", 20, "max thoughts returned")
		jsonOut := flags.Bool("json", false, "output JSON")
		summaryMode := flags.Bool("summary", false, "show session-level thought summary")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		query := tools.DeepDrillThoughtQuery{
			SessionID:   strings.TrimSpace(*sessionID),
			Status:      strings.TrimSpace(*status),
			Type:        tools.ParseDeepDrillThoughtType(*typeName),
			Uncertainty: tools.DeepDrillUncertainty(strings.ToUpper(strings.TrimSpace(*uncertainty))),
			Hypothesis:  strings.TrimSpace(*hypothesis),
			InfoGain:    tools.ParseDeepDrillInfoGain(*infoGain),
			Limit:       *limit,
		}
		if *summaryMode {
			summary, err := runtime.DeepDrillThoughtSummary(context.Background(), query)
			if err != nil {
				return err
			}
			if *jsonOut {
				fmt.Println(toJSON(summary))
				return nil
			}
			fmt.Println(formatDeepDrillThoughtSummary(summary))
			return nil
		}
		list, err := runtime.DeepDrillThoughts(context.Background(), query)
		if err != nil {
			return err
		}
		if *jsonOut {
			fmt.Println(toJSON(list))
			return nil
		}
		fmt.Println(formatDeepDrillThoughtList(list))
		return nil
	case "show":
		flags := flag.NewFlagSet("minddrill deepdrill show", flag.ContinueOnError)
		flags.SetOutput(os.Stdout)
		jsonOut := flags.Bool("json", false, "output JSON")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if flags.NArg() != 1 {
			return fmt.Errorf("show requires exactly one thought_id")
		}
		thought, err := runtime.DeepDrillShowThought(context.Background(), strings.TrimSpace(flags.Arg(0)))
		if err != nil {
			return err
		}
		if *jsonOut {
			fmt.Println(toJSON(thought))
			return nil
		}
		fmt.Println(formatDeepDrillThoughtShow(thought))
		return nil
	case "provenance":
		flags := flag.NewFlagSet("minddrill deepdrill provenance", flag.ContinueOnError)
		flags.SetOutput(os.Stdout)
		sessionID := flags.String("session", "", "research session id used to resolve collection")
		collection := flags.String("collection", "", "source collection")
		sourceID := flags.String("source", "", "source id to trace")
		limit := flags.Int("limit", 1200, "max records scanned while building provenance trace")
		jsonOut := flags.Bool("json", false, "output JSON")
		rawArgs := args[1:]
		thoughtID := ""
		if len(rawArgs) > 0 && !strings.HasPrefix(strings.TrimSpace(rawArgs[0]), "-") {
			thoughtID = strings.TrimSpace(rawArgs[0])
			rawArgs = rawArgs[1:]
		}
		if err := flags.Parse(rawArgs); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if thoughtID != "" && flags.NArg() > 0 {
			return fmt.Errorf("provenance accepts at most one thought_id argument")
		}
		if thoughtID == "" && flags.NArg() == 1 {
			thoughtID = strings.TrimSpace(flags.Arg(0))
		}
		if flags.NArg() > 1 {
			return fmt.Errorf("provenance accepts at most one thought_id argument")
		}
		if thoughtID == "" && strings.TrimSpace(*sourceID) == "" {
			return fmt.Errorf("provenance requires a thought_id argument or --source")
		}
		trace, err := runtime.DeepDrillProvenanceTrace(context.Background(), tools.DeepDrillProvenanceTraceRequest{
			SessionID:  strings.TrimSpace(*sessionID),
			Collection: strings.TrimSpace(*collection),
			ThoughtID:  thoughtID,
			SourceID:   strings.TrimSpace(*sourceID),
			Limit:      *limit,
		})
		if err != nil {
			return err
		}
		if *jsonOut {
			fmt.Println(toJSON(trace))
			return nil
		}
		fmt.Println(formatDeepDrillProvenanceTrace(trace))
		return nil
	default:
		return fmt.Errorf("unknown deepdrill subcommand %q (use plan, run, thoughts, show, or provenance)", action)
	}
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

func formatDeepDrillThoughtList(list tools.DeepDrillThoughtList) string {
	if len(list.Thoughts) == 0 {
		return fmt.Sprintf("No DeepDrill thoughts matched in %s.", list.Collection)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "DeepDrill thoughts (%s): %d\n", list.Collection, len(list.Thoughts))
	for _, thought := range list.Thoughts {
		question := strings.TrimSpace(thought.Question)
		if question == "" {
			question = strings.TrimSpace(thought.EvidenceSummary)
		}
		if len(question) > 88 {
			question = question[:88] + "..."
		}
		fmt.Fprintf(&b, "- %s | %s | %s | %s\n", thought.ThoughtID, thought.Type, tools.NormalizeDeepDrillArtifactStatus(thought), thought.EvidenceOrigin)
		if question != "" {
			fmt.Fprintf(&b, "  %s\n", question)
		}
		if len(thought.ParentThoughtIDs) > 0 {
			fmt.Fprintf(&b, "  parents: %s\n", strings.Join(thought.ParentThoughtIDs, ", "))
		}
		if len(thought.Supersedes) > 0 {
			fmt.Fprintf(&b, "  supersedes: %s\n", strings.Join(thought.Supersedes, ", "))
		}
		if strings.TrimSpace(thought.DuplicateOfThought) != "" || strings.EqualFold(strings.TrimSpace(thought.Status), "rediscovery") {
			dup := strings.TrimSpace(thought.DuplicateOfThought)
			if dup == "" {
				dup = "prior artifact"
			}
			fmt.Fprintf(&b, "  rediscovery_of: %s\n", dup)
		}
	}
	return strings.TrimSpace(b.String())
}

func formatDeepDrillThoughtShow(thought tools.DeepDrillThought) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Thought: %s\n", thought.ThoughtID)
	fmt.Fprintf(&b, "Type: %s\n", thought.Type)
	fmt.Fprintf(&b, "Status: %s\n", tools.NormalizeDeepDrillArtifactStatus(thought))
	fmt.Fprintf(&b, "Session: %s\n", thought.SessionID)
	fmt.Fprintf(&b, "Timestamp: %s\n", thought.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(&b, "Evidence origin: %s\n", thought.EvidenceOrigin)
	if thought.InfoGain != "" {
		fmt.Fprintf(&b, "Info gain: %s\n", thought.InfoGain)
	}
	if strings.TrimSpace(thought.Question) != "" {
		fmt.Fprintf(&b, "Question: %s\n", thought.Question)
	}
	if strings.TrimSpace(thought.EvidenceSummary) != "" {
		fmt.Fprintf(&b, "Summary: %s\n", thought.EvidenceSummary)
	}
	if len(thought.HypothesisTargets) > 0 {
		fmt.Fprintf(&b, "Hypotheses: %s\n", strings.Join(thought.HypothesisTargets, ", "))
	}
	if thought.UncertaintyClass != "" {
		fmt.Fprintf(&b, "Uncertainty: %s\n", thought.UncertaintyClass)
	}
	if thought.Strategy != "" {
		fmt.Fprintf(&b, "Strategy: %s\n", thought.Strategy)
	}
	if len(thought.Sources) > 0 {
		fmt.Fprintf(&b, "Evidence/source refs: %s\n", strings.Join(thought.Sources, ", "))
	}
	if len(thought.QuerySpec) > 0 {
		fmt.Fprintf(&b, "Query spec: %s\n", toJSON(thought.QuerySpec))
	}
	if len(thought.ModelBefore) > 0 {
		fmt.Fprintf(&b, "Model before: %s\n", toJSON(thought.ModelBefore))
	}
	if len(thought.ModelAfter) > 0 {
		fmt.Fprintf(&b, "Model after: %s\n", toJSON(thought.ModelAfter))
	}
	if len(thought.ParentThoughtIDs) > 0 {
		fmt.Fprintf(&b, "Parent links: %s\n", strings.Join(thought.ParentThoughtIDs, ", "))
	}
	if len(thought.Supersedes) > 0 {
		fmt.Fprintf(&b, "Supersedes links: %s\n", strings.Join(thought.Supersedes, ", "))
	}
	if strings.TrimSpace(thought.DuplicateOfThought) != "" || strings.EqualFold(strings.TrimSpace(thought.Status), "rediscovery") {
		dup := strings.TrimSpace(thought.DuplicateOfThought)
		if dup == "" {
			dup = "prior artifact"
		}
		fmt.Fprintf(&b, "Rediscovery relation: %s\n", dup)
	}
	return strings.TrimSpace(b.String())
}

func formatDeepDrillThoughtSummary(summary tools.DeepDrillThoughtSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "DeepDrill summary (%s)\n", summary.Collection)
	if strings.TrimSpace(summary.SessionID) != "" {
		fmt.Fprintf(&b, "Session: %s\n", summary.SessionID)
	}
	fmt.Fprintf(&b, "Total thoughts: %d\n", summary.TotalThoughts)
	fmt.Fprintf(&b, "Open questions: %d\n", len(summary.OpenQuestions))
	fmt.Fprintf(&b, "Frozen branches: %d\n", len(summary.FrozenBranches))
	fmt.Fprintf(&b, "Unresolved contradictions: %d\n", len(summary.UnresolvedContradictions))
	fmt.Fprintf(&b, "Recent model revisions: %d\n", len(summary.RecentModelRevisions))
	fmt.Fprintf(&b, "High-information-gain runs: %d\n", len(summary.HighInformationGainRuns))
	fmt.Fprintf(&b, "Negative results: %d\n", len(summary.NegativeResults))
	if len(summary.ThoughtCountByType) > 0 {
		fmt.Fprintf(&b, "Thoughts by type:\n")
		for thoughtType, count := range summary.ThoughtCountByType {
			fmt.Fprintf(&b, "- %s: %d\n", thoughtType, count)
		}
	}
	return strings.TrimSpace(b.String())
}

func formatDeepDrillProvenanceTrace(trace tools.DeepDrillProvenanceTraceResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "SOURCE: %s\n", trace.Source)
	if strings.TrimSpace(trace.Title) != "" {
		fmt.Fprintf(&b, "TITLE: %s\n", trace.Title)
	}
	fmt.Fprintf(&b, "TYPE: %s\n", trace.Type)
	if strings.TrimSpace(trace.AnchorThoughtID) != "" {
		fmt.Fprintf(&b, "ANCHOR_THOUGHT: %s (%s)\n", trace.AnchorThoughtID, trace.AnchorThoughtStatus)
	}
	fmt.Fprintf(&b, "UPSTREAM:\n%s\n", formatProvenanceEdgeList(trace.Upstream))
	fmt.Fprintf(&b, "DOWNSTREAM:\n%s\n", formatProvenanceEdgeList(trace.Downstream))
	fmt.Fprintf(&b, "RELATED:\n%s\n", formatProvenanceEdgeList(trace.RelatedSources))
	if len(trace.ChronologySignals) == 0 {
		fmt.Fprintf(&b, "CHRONOLOGY:\n- none\n")
	} else {
		fmt.Fprintf(&b, "CHRONOLOGY:\n")
		for _, signal := range trace.ChronologySignals {
			fmt.Fprintf(&b, "- %s\n", signal)
		}
	}
	if len(trace.Weaknesses) == 0 {
		fmt.Fprintf(&b, "WEAKNESSES:\n- none\n")
	} else {
		fmt.Fprintf(&b, "WEAKNESSES:\n")
		for _, weakness := range trace.Weaknesses {
			fmt.Fprintf(&b, "- %s\n", weakness)
		}
	}
	fmt.Fprintf(&b, "CONFIDENCE: %.2f", trace.Confidence)
	return strings.TrimSpace(b.String())
}

func formatProvenanceEdgeList(edges []tools.DeepDrillProvenanceEdge) string {
	if len(edges) == 0 {
		return "- none"
	}
	var b strings.Builder
	for _, edge := range edges {
		kind := "inferred"
		if edge.Explicit {
			kind = "explicit"
		}
		target := edge.ToSourceID
		if strings.TrimSpace(target) == "" {
			target = "(unknown)"
		}
		evidence := strings.TrimSpace(edge.Evidence)
		if evidence == "" {
			evidence = "no evidence text"
		}
		basis := strings.TrimSpace(edge.Basis)
		if basis == "" {
			basis = "no basis"
		}
		fmt.Fprintf(&b, "- %s -> %s | %s | %s | confidence=%.2f\n", edge.FromSourceID, target, edge.Relation, kind, edge.Confidence)
		fmt.Fprintf(&b, "  basis: %s\n", basis)
		fmt.Fprintf(&b, "  evidence: %s\n", evidence)
	}
	return strings.TrimSpace(b.String())
}
