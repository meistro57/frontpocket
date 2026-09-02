package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/store"
)

type deepDrillQdrantFixture struct {
	server                 *httptest.Server
	thoughtCollectionReady bool
	thoughtUpserts         int
	upsertedCollections    []string
	fingerprintCounts      map[string]int
	storedThoughtPayloads  []map[string]any
	sourceEvidenceEnabled  bool
}

func newDeepDrillQdrantFixture(t *testing.T, sourceEvidence bool) *deepDrillQdrantFixture {
	t.Helper()
	fixture := &deepDrillQdrantFixture{fingerprintCounts: map[string]int{}, sourceEvidenceEnabled: sourceEvidence}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/corpus":
			_, _ = io.WriteString(w, `{"result":{"points_count":3,"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/corpus/points/search":
			if fixture.sourceEvidenceEnabled {
				_, _ = io.WriteString(w, `{"result":[{"score":0.93,"payload":{"memory_id":"m1","source_document_id":"doc-A","source_title":"Doc A","source_type":"audio_overview","chunk_id":"c1","chunk_index":1,"text":"first explicit bridge between sacred geometry and E8 appears in this synthesized summary"}}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"result":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/corpus/points/scroll":
			if fixture.sourceEvidenceEnabled {
				_, _ = io.WriteString(w, `{"result":{"points":[{"payload":{"memory_id":"m1","source_document_id":"doc-A","source_title":"Doc A","source_type":"audio_overview","chunk_id":"c1","chunk_index":1,"text":"first explicit bridge between sacred geometry and E8 appears in this synthesized summary","created_at":"2026-01-01T00:00:00Z"}}],"next_page_offset":null}}`)
				return
			}
			_, _ = io.WriteString(w, `{"result":{"points":[],"next_page_offset":null}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/collections/minddrill_research_thoughts":
			if !fixture.thoughtCollectionReady {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"status":"error"}`)
				return
			}
			_, _ = io.WriteString(w, `{"result":{"points_count":1,"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/minddrill_research_thoughts":
			fixture.thoughtCollectionReady = true
			fixture.upsertedCollections = append(fixture.upsertedCollections, "minddrill_research_thoughts")
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/minddrill_research_thoughts/points/scroll":
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			_ = json.Unmarshal(body, &req)
			offsetValue := strings.TrimSpace(asString(req["offset"]))
			mustSession := ""
			mustSourceType := ""
			if filter, ok := req["filter"].(map[string]any); ok {
				if must, ok := filter["must"].([]any); ok {
					for _, rawClause := range must {
						clause, _ := rawClause.(map[string]any)
						key, _ := clause["key"].(string)
						match, _ := clause["match"].(map[string]any)
						value, _ := match["value"].(string)
						switch key {
						case "session_id":
							mustSession = strings.TrimSpace(value)
						case "source_type":
							mustSourceType = strings.TrimSpace(value)
						}
					}
				}
			}
			if offsetValue != "" {
				_, _ = io.WriteString(w, `{"result":{"points":[],"next_page_offset":null}}`)
				return
			}
			points := make([]map[string]any, 0, len(fixture.storedThoughtPayloads))
			for _, payload := range fixture.storedThoughtPayloads {
				if mustSourceType != "" && strings.TrimSpace(asString(payload["source_type"])) != mustSourceType {
					continue
				}
				if mustSession != "" && strings.TrimSpace(asString(payload["session_id"])) != mustSession {
					continue
				}
				points = append(points, map[string]any{"payload": payload})
			}
			nextOffset := ""
			if len(points) > 0 {
				nextOffset = "done"
			}
			encoded, _ := json.Marshal(map[string]any{"result": map[string]any{"points": points, "next_page_offset": nextOffset}})
			_, _ = w.Write(encoded)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/minddrill_research_thoughts/points/count":
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			_ = json.Unmarshal(body, &req)
			fingerprint := ""
			if filter, ok := req["filter"].(map[string]any); ok {
				if must, ok := filter["must"].([]any); ok {
					for _, clause := range must {
						cl, _ := clause.(map[string]any)
						key, _ := cl["key"].(string)
						match, _ := cl["match"].(map[string]any)
						value, _ := match["value"].(string)
						if key == "fingerprint" {
							fingerprint = value
						}
					}
				}
			}
			count := fixture.fingerprintCounts[fingerprint]
			_, _ = io.WriteString(w, `{"result":{"count":`+strconvItoa(count)+`}}`)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/collections/minddrill_research_thoughts/points"):
			fixture.thoughtUpserts++
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Points []struct {
					Payload map[string]any `json:"payload"`
				} `json:"points"`
			}
			_ = json.Unmarshal(body, &req)
			for _, point := range req.Points {
				fingerprint := strings.TrimSpace(asString(point.Payload["fingerprint"]))
				if fingerprint != "" {
					fixture.fingerprintCounts[fingerprint]++
				}
				copied := map[string]any{}
				for key, value := range point.Payload {
					copied[key] = value
				}
				fixture.storedThoughtPayloads = append(fixture.storedThoughtPayloads, copied)
			}
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	return fixture
}

func (f *deepDrillQdrantFixture) Close() { f.server.Close() }

func strconvItoa(v int) string {
	return strconv.Itoa(v)
}

func TestDeepDrillThoughtSerialization(t *testing.T) {
	thought := DeepDrillThought{
		ThoughtID:         "t1",
		SessionID:         "s1",
		Timestamp:         time.Now().UTC(),
		Type:              DeepDrillThoughtEvidence,
		Question:          "What changed?",
		UncertaintyClass:  DeepDrillChronologyGap,
		Strategy:          DeepDrillChronologyTrace,
		EvidenceSummary:   "A source-backed sequence was found.",
		ProvenanceScore:   0.5,
		InfoGain:          DeepDrillInfoMedium,
		Fingerprint:       "abc",
		EvidenceOrigin:    DeepDrillSourceCorpus,
		HypothesisTargets: []string{"H1", "H2"},
	}
	encoded, err := json.Marshal(thought)
	if err != nil {
		t.Fatalf("marshal thought failed: %v", err)
	}
	var decoded DeepDrillThought
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal thought failed: %v", err)
	}
	if decoded.Type != DeepDrillThoughtEvidence || decoded.EvidenceOrigin != DeepDrillSourceCorpus {
		t.Fatalf("unexpected decoded thought: %#v", decoded)
	}
}

func TestDeepDrillStrategySelectionByUncertaintyType(t *testing.T) {
	strategy, _, _ := selectStrategy(newDefaultDeepDrillState("minddrill_research_thoughts"), DeepDrillChronologyGap)
	if strategy != DeepDrillProvenanceTrace {
		t.Fatalf("expected provenance trace strategy, got %s", strategy)
	}
	strategy, _, _ = selectStrategy(newDefaultDeepDrillState("minddrill_research_thoughts"), DeepDrillProvenanceGap)
	if strategy != DeepDrillProvenanceTrace {
		t.Fatalf("expected provenance trace strategy, got %s", strategy)
	}
	strategy, _, _ = selectStrategy(newDefaultDeepDrillState("minddrill_research_thoughts"), DeepDrillSourceQuality)
	if strategy != DeepDrillProvenanceTrace {
		t.Fatalf("expected provenance trace strategy, got %s", strategy)
	}
}

func TestDeepDrillProvenanceDowngradeKeepsRankOrder(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = []DeepDrillHypothesisState{
		{ID: "H3", Rank: 1, Confidence: 0.6, EvidenceStrength: 0.5},
		{ID: "H2", Rank: 2, Confidence: 0.55, EvidenceStrength: 0.4},
	}
	applyModelUpdate(state, DeepDrillProvenanceGap, []ResearchResult{{SourceType: "audio_overview"}}, 0.3, false)
	if state.Hypotheses[0].Rank != 1 || state.Hypotheses[1].Rank != 2 {
		t.Fatalf("expected ranks unchanged, got %#v", state.Hypotheses)
	}
	if !(state.Hypotheses[0].Confidence < 0.6 && state.Hypotheses[1].Confidence < 0.55) {
		t.Fatalf("expected confidence downgrade, got %#v", state.Hypotheses)
	}
}

func TestDeepDrillDuplicateFingerprintDetection(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, true)
	defer fixture.Close()
	sessionPath := filepath.Join(t.TempDir(), "deepdrill_sessions.json")
	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 2)
	_, _, state, err := runtime.prepareDeepDrillSession(context.Background(), DeepDrillPlanRequest{SessionID: "s1", Collection: "corpus"})
	if err != nil {
		t.Fatalf("prepare session failed: %v", err)
	}
	session, _, err := runtime.requireBoundCollection("s1")
	if err != nil {
		t.Fatalf("require bound session failed: %v", err)
	}
	thought := DeepDrillThought{Type: DeepDrillThoughtQuestion, Question: "Q", Strategy: DeepDrillKeywordTargeted, EvidenceSummary: "S", EvidenceOrigin: DeepDrillResearchThought}
	if _, dup, err := runtime.storeDeepDrillThought(context.Background(), session, state, thought); err != nil || dup {
		t.Fatalf("first thought should not be duplicate, err=%v dup=%v", err, dup)
	}
	if _, dup, err := runtime.storeDeepDrillThought(context.Background(), session, state, thought); err != nil || !dup {
		t.Fatalf("second thought should be duplicate, err=%v dup=%v", err, dup)
	}
}

func TestDeepDrillExhaustedStrategyNotReselected(t *testing.T) {
	state := &DeepDrillState{CurrentUncertainty: DeepDrillProvenanceGap, ThoughtCollection: "minddrill_research_thoughts"}
	markStrategyExhausted(state, DeepDrillProvenanceGap, DeepDrillProvenanceTrace)
	selected, _, _ := selectStrategy(state, DeepDrillProvenanceGap)
	if selected == DeepDrillProvenanceTrace {
		t.Fatalf("exhausted PROVENANCE_TRACE must not be selected again, got %s", selected)
	}
	if selected != DeepDrillProvenanceProfile {
		t.Fatalf("expected next remaining strategy PROVENANCE_PROFILE, got %s", selected)
	}
}

func TestDeepDrillProvenanceGapProgression(t *testing.T) {
	state := &DeepDrillState{CurrentUncertainty: DeepDrillProvenanceGap, ThoughtCollection: "minddrill_research_thoughts"}
	want := []DeepDrillStrategy{
		DeepDrillProvenanceTrace,
		DeepDrillProvenanceProfile,
		DeepDrillRawSourceLookup,
		DeepDrillConfidenceDowngrade,
	}
	for idx, expected := range want {
		selected, _, _ := selectStrategy(state, DeepDrillProvenanceGap)
		if selected != expected {
			t.Fatalf("step %d: expected %s, got %s", idx, expected, selected)
		}
		markStrategyExhausted(state, DeepDrillProvenanceGap, expected)
	}
	selected, _, _ := selectStrategy(state, DeepDrillProvenanceGap)
	if selected != DeepDrillFreezeBranch {
		t.Fatalf("expected freeze fallback when class exhausted, got %s", selected)
	}
}

func TestDeepDrillLeavingAndReturningPreservesExhaustion(t *testing.T) {
	state := &DeepDrillState{CurrentUncertainty: DeepDrillProvenanceGap, ThoughtCollection: "minddrill_research_thoughts"}
	markStrategyExhausted(state, DeepDrillProvenanceGap, DeepDrillProvenanceTrace)
	// Leave the uncertainty and come back without a model change.
	state.CurrentUncertainty = DeepDrillContradiction
	state.CurrentUncertainty = DeepDrillProvenanceGap
	if !isStrategyExhausted(state, DeepDrillProvenanceGap, DeepDrillProvenanceTrace) {
		t.Fatal("expected PROVENANCE_TRACE to remain exhausted after leaving and returning")
	}
	selected, _, _ := selectStrategy(state, DeepDrillProvenanceGap)
	if selected == DeepDrillProvenanceTrace {
		t.Fatalf("expected PROVENANCE_TRACE to stay excluded, got %s", selected)
	}
}

func TestDeepDrillNewEvidenceTierReopensStrategy(t *testing.T) {
	state := &DeepDrillState{CurrentUncertainty: DeepDrillProvenanceGap, ThoughtCollection: "minddrill_research_thoughts", LastSeenProvenanceTier: DeepDrillAudioOverview}
	markStrategyExhausted(state, DeepDrillProvenanceGap, DeepDrillProvenanceTrace)
	if !isStrategyExhausted(state, DeepDrillProvenanceGap, DeepDrillProvenanceTrace) {
		t.Fatal("expected PROVENANCE_TRACE exhausted at derived provenance tier")
	}
	state.LastSeenProvenanceTier = DeepDrillRawSource
	if isStrategyExhausted(state, DeepDrillProvenanceGap, DeepDrillProvenanceTrace) {
		t.Fatal("expected higher provenance tier to reopen PROVENANCE_TRACE")
	}
}

func TestDetectStrategyCycle(t *testing.T) {
	mk := func(u DeepDrillUncertainty, s DeepDrillStrategy, fp string) DeepDrillStrategyOutcome {
		return DeepDrillStrategyOutcome{Uncertainty: u, Strategy: s, EvidenceFingerprint: fp}
	}
	cyclic := []DeepDrillStrategyOutcome{
		mk(DeepDrillContradiction, DeepDrillSourceComparison, "fp-A"),
		mk(DeepDrillProvenanceGap, DeepDrillProvenanceTrace, "fp-B"),
		mk(DeepDrillContradiction, DeepDrillSourceComparison, "fp-A"),
		mk(DeepDrillProvenanceGap, DeepDrillProvenanceTrace, "fp-B"),
	}
	if !detectStrategyCycle(cyclic) {
		t.Fatal("expected A->B->A->B cycle to be detected")
	}
	acyclic := []DeepDrillStrategyOutcome{
		mk(DeepDrillContradiction, DeepDrillSourceComparison, "fp-A"),
		mk(DeepDrillProvenanceGap, DeepDrillProvenanceTrace, "fp-B"),
		mk(DeepDrillContradiction, DeepDrillCounterevidenceSearch, "fp-C"),
		mk(DeepDrillProvenanceGap, DeepDrillProvenanceProfile, "fp-D"),
	}
	if detectStrategyCycle(acyclic) {
		t.Fatal("expected non-repeating sequence to not be a cycle")
	}
}

func TestDeepDrillMovesToNextStrategyAfterDuplicateLowGain(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, false)
	defer fixture.Close()
	sessionPath := filepath.Join(t.TempDir(), "deepdrill_sessions.json")
	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 2)
	result, err := runtime.DeepDrillRun(context.Background(), DeepDrillRunRequest{DeepDrillPlanRequest: DeepDrillPlanRequest{SessionID: "s1", Collection: "corpus", ResearchQuestion: "trace", KnownBlockers: []DeepDrillUncertainty{DeepDrillProvenanceGap}}, Steps: 2})
	if err != nil {
		t.Fatalf("deepdrill run failed: %v", err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[0].Strategy != DeepDrillProvenanceTrace {
		t.Fatalf("expected first strategy PROVENANCE_TRACE, got %s", result.Steps[0].Strategy)
	}
	if result.Steps[1].Strategy == DeepDrillProvenanceTrace {
		t.Fatalf("expected exhausted PROVENANCE_TRACE not to recur, got %s", result.Steps[1].Strategy)
	}
	exhaustedSet := map[DeepDrillStrategy]bool{}
	for _, strategy := range result.State.StrategiesExhausted {
		exhaustedSet[strategy] = true
	}
	if !exhaustedSet[DeepDrillProvenanceTrace] {
		t.Fatalf("expected PROVENANCE_TRACE in strategies_exhausted, got %#v", result.State.StrategiesExhausted)
	}
}

func TestDeepDrillFreezeAfterStrategiesExhausted(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, false)
	defer fixture.Close()
	sessionPath := filepath.Join(t.TempDir(), "deepdrill_sessions.json")
	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 1)
	_, _, state, err := runtime.prepareDeepDrillSession(context.Background(), DeepDrillPlanRequest{SessionID: "s1", Collection: "corpus", ResearchQuestion: "trace", KnownBlockers: []DeepDrillUncertainty{DeepDrillProvenanceGap}})
	if err != nil {
		t.Fatalf("prepare session failed: %v", err)
	}
	// Exhaust every strategy across every uncertainty so reclassification has nowhere to go.
	for _, uncertainty := range []DeepDrillUncertainty{
		DeepDrillEvidenceGap, DeepDrillProvenanceGap, DeepDrillChronologyGap, DeepDrillContradiction,
		DeepDrillAmbiguousClassification, DeepDrillGenericSimilarity, DeepDrillRetrievalLimit, DeepDrillSourceQuality,
	} {
		for _, strategy := range strategyCandidates(uncertainty) {
			state.StrategyHistory = append(state.StrategyHistory, DeepDrillStrategyOutcome{Strategy: strategy, Uncertainty: uncertainty, Question: "trace", InfoGain: DeepDrillInfoLow})
			markStrategyExhausted(state, uncertainty, strategy)
		}
	}
	state.CurrentUncertainty = DeepDrillProvenanceGap
	session, _, err := runtime.requireBoundCollection("s1")
	if err != nil {
		t.Fatalf("require bound session failed: %v", err)
	}
	session.DeepDrill = state
	runtime.sessions.put(session)
	result, err := runtime.DeepDrillRun(context.Background(), DeepDrillRunRequest{DeepDrillPlanRequest: DeepDrillPlanRequest{SessionID: "s1"}, Steps: 1})
	if err != nil {
		t.Fatalf("deepdrill run failed: %v", err)
	}
	if result.State.BranchStatus != DeepDrillFrozen {
		t.Fatalf("expected freeze when no unused strategy class remains, got %s", result.State.BranchStatus)
	}
}

func TestDeepDrillRunBoundedAndResumable(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, true)
	defer fixture.Close()
	sessionPath := filepath.Join(t.TempDir(), "deepdrill_sessions.json")
	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 2)
	seed := []DeepDrillHypothesisState{{ID: "H3", Statement: "parallel", Rank: 1, Confidence: 0.4}, {ID: "H2", Statement: "downstream", Rank: 2, Confidence: 0.4}}
	first, err := runtime.DeepDrillRun(context.Background(), DeepDrillRunRequest{DeepDrillPlanRequest: DeepDrillPlanRequest{SessionID: "s1", Collection: "corpus", ResearchQuestion: "earliest bridge", Hypotheses: seed, KnownBlockers: []DeepDrillUncertainty{DeepDrillProvenanceGap}}, Steps: 1})
	if err != nil {
		t.Fatalf("first deepdrill run failed: %v", err)
	}
	if len(first.Steps) != 1 || first.State.Iterations != 1 {
		t.Fatalf("expected first run to execute one step, got steps=%d iterations=%d", len(first.Steps), first.State.Iterations)
	}
	second, err := runtime.DeepDrillRun(context.Background(), DeepDrillRunRequest{DeepDrillPlanRequest: DeepDrillPlanRequest{SessionID: "s1"}, Steps: 1})
	if err != nil {
		t.Fatalf("second deepdrill run failed: %v", err)
	}
	if second.State.Iterations < 2 {
		t.Fatalf("expected resumed run to continue iteration count, got %d", second.State.Iterations)
	}
}

func TestDeepDrillStoresNegativeResultThought(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, false)
	defer fixture.Close()
	sessionPath := filepath.Join(t.TempDir(), "deepdrill_sessions.json")
	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 2)
	result, err := runtime.DeepDrillRun(context.Background(), DeepDrillRunRequest{DeepDrillPlanRequest: DeepDrillPlanRequest{SessionID: "s1", Collection: "corpus", ResearchQuestion: "no hit expected", KnownBlockers: []DeepDrillUncertainty{DeepDrillEvidenceGap}}, Steps: 1})
	if err != nil {
		t.Fatalf("deepdrill run failed: %v", err)
	}
	if len(result.Steps) != 1 || !result.Steps[0].NegativeResult {
		t.Fatalf("expected negative result step, got %#v", result.Steps)
	}
	if fixture.thoughtUpserts == 0 {
		t.Fatal("expected thought artifacts to be written to thought collection")
	}
}

func TestDeepDrillThoughtCollectionSeparatedFromSourceEvidence(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, true)
	defer fixture.Close()
	sessionPath := filepath.Join(t.TempDir(), "deepdrill_sessions.json")
	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 2)
	run, err := runtime.DeepDrillRun(context.Background(), DeepDrillRunRequest{DeepDrillPlanRequest: DeepDrillPlanRequest{SessionID: "s1", Collection: "corpus", ResearchQuestion: "trace", KnownBlockers: []DeepDrillUncertainty{DeepDrillEvidenceGap}}, Steps: 1})
	if err != nil {
		t.Fatalf("deepdrill run failed: %v", err)
	}
	if run.Steps[0].EvidenceCount == 0 {
		t.Fatal("expected source evidence count to be recorded")
	}
	if fixture.thoughtUpserts == 0 {
		t.Fatal("expected thought artifacts to be written separately")
	}
}

func TestNormalizeDeepDrillArtifactStatus(t *testing.T) {
	if NormalizeDeepDrillArtifactStatus(DeepDrillThought{Status: "resolved"}) != "RESOLVED" {
		t.Fatal("expected resolved normalization")
	}
	if NormalizeDeepDrillArtifactStatus(DeepDrillThought{Status: "superseded"}) != "SUPERSEDED" {
		t.Fatal("expected superseded normalization")
	}
	if NormalizeDeepDrillArtifactStatus(DeepDrillThought{Status: "frozen"}) != "FROZEN" {
		t.Fatal("expected frozen normalization")
	}
	if NormalizeDeepDrillArtifactStatus(DeepDrillThought{Type: DeepDrillThoughtQuestion}) != "OPEN" {
		t.Fatal("expected open default normalization")
	}
}

func TestDeepDrillReopenConditions(t *testing.T) {
	state := &DeepDrillState{BranchStatus: DeepDrillFrozen, CurrentQuestion: "old question", ThoughtCollection: "minddrill_research_thoughts", LastSeenProvenanceTier: DeepDrillAudioOverview}
	if !shouldReopenFrozenBranch(state, "new question", "corpus") {
		t.Fatal("expected new question to reopen frozen branch")
	}
	if shouldReopenFrozenBranch(state, "old question", "") {
		t.Fatal("expected unchanged frozen branch to stay frozen")
	}
}

func TestDeepDrillThoughtsFilteringAndShow(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, true)
	defer fixture.Close()
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 2)
	fixture.thoughtCollectionReady = true
	fixture.storedThoughtPayloads = []map[string]any{
		{
			"thought_id":         "t-open",
			"session_id":         "s1",
			"timestamp":          "2026-01-02T00:00:00Z",
			"type":               "QUESTION",
			"question":           "Open question",
			"status":             "open",
			"uncertainty_class":  "PROVENANCE_GAP",
			"hypothesis_targets": []any{"H2"},
			"source_type":        "deepdrill_thought",
			"evidence_origin":    "RESEARCH_THOUGHTS",
		},
		{
			"thought_id":        "t-frozen",
			"session_id":        "s1",
			"timestamp":         "2026-01-03T00:00:00Z",
			"type":              "STOP_DECISION",
			"status":            "frozen",
			"uncertainty_class": "DIMINISHING_RETURNS",
			"source_type":       "deepdrill_thought",
			"evidence_origin":   "SOURCE_CORPUS",
		},
		{
			"thought_id":           "t-dup",
			"session_id":           "s2",
			"timestamp":            "2026-01-04T00:00:00Z",
			"type":                 "EVIDENCE",
			"status":               "rediscovery",
			"duplicate_of_thought": "t-open",
			"source_type":          "deepdrill_thought",
			"evidence_origin":      "SOURCE_CORPUS",
		},
	}
	list, err := runtime.DeepDrillThoughts(context.Background(), DeepDrillThoughtQuery{SessionID: "s1", Status: "FROZEN", Limit: 10})
	if err != nil {
		t.Fatalf("DeepDrillThoughts failed: %v", err)
	}
	if len(list.Thoughts) != 1 || list.Thoughts[0].ThoughtID != "t-frozen" {
		t.Fatalf("unexpected filtered thoughts: %#v", list.Thoughts)
	}
	show, err := runtime.DeepDrillShowThought(context.Background(), "t-dup")
	if err != nil {
		t.Fatalf("DeepDrillShowThought failed: %v", err)
	}
	if show.DuplicateOfThought != "t-open" {
		t.Fatalf("expected duplicate relation, got %#v", show)
	}
}

func TestDeepDrillThoughtFiltersByTypeUncertaintyHypothesisAndInfoGain(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, true)
	defer fixture.Close()
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 2)
	fixture.thoughtCollectionReady = true
	fixture.storedThoughtPayloads = []map[string]any{
		{"thought_id": "a", "session_id": "s1", "timestamp": "2026-01-01T00:00:00Z", "type": "EVIDENCE", "uncertainty_class": "PROVENANCE_GAP", "hypothesis_targets": []any{"H3"}, "info_gain": "HIGH", "source_type": "deepdrill_thought", "evidence_origin": "SOURCE_CORPUS"},
		{"thought_id": "b", "session_id": "s1", "timestamp": "2026-01-02T00:00:00Z", "type": "QUESTION", "uncertainty_class": "CHRONOLOGY_GAP", "hypothesis_targets": []any{"H2"}, "info_gain": "LOW", "source_type": "deepdrill_thought", "evidence_origin": "RESEARCH_THOUGHTS"},
	}
	list, err := runtime.DeepDrillThoughts(context.Background(), DeepDrillThoughtQuery{SessionID: "s1", Type: DeepDrillThoughtEvidence, Uncertainty: DeepDrillProvenanceGap, Hypothesis: "H3", InfoGain: DeepDrillInfoHigh, Limit: 10})
	if err != nil {
		t.Fatalf("DeepDrillThoughts failed: %v", err)
	}
	if len(list.Thoughts) != 1 || list.Thoughts[0].ThoughtID != "a" {
		t.Fatalf("unexpected filter result: %#v", list.Thoughts)
	}
}

func TestDeepDrillThoughtSummaryReportsRequiredSections(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, true)
	defer fixture.Close()
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 2)
	fixture.thoughtCollectionReady = true
	fixture.storedThoughtPayloads = []map[string]any{
		{"thought_id": "q1", "session_id": "s1", "timestamp": "2026-01-01T00:00:00Z", "type": "QUESTION", "status": "open", "source_type": "deepdrill_thought", "evidence_origin": "RESEARCH_THOUGHTS"},
		{"thought_id": "f1", "session_id": "s1", "timestamp": "2026-01-02T00:00:00Z", "type": "STOP_DECISION", "status": "frozen", "source_type": "deepdrill_thought", "evidence_origin": "RESEARCH_THOUGHTS"},
		{"thought_id": "c1", "session_id": "s1", "timestamp": "2026-01-03T00:00:00Z", "type": "CONTRADICTION", "status": "open", "source_type": "deepdrill_thought", "evidence_origin": "SOURCE_CORPUS"},
		{"thought_id": "m1", "session_id": "s1", "timestamp": "2026-01-04T00:00:00Z", "type": "MODEL_REVISION", "info_gain": "HIGH", "source_type": "deepdrill_thought", "evidence_origin": "RESEARCH_THOUGHTS"},
		{"thought_id": "n1", "session_id": "s1", "timestamp": "2026-01-05T00:00:00Z", "type": "NEGATIVE_RESULT", "info_gain": "LOW", "source_type": "deepdrill_thought", "evidence_origin": "SOURCE_CORPUS"},
	}
	summary, err := runtime.DeepDrillThoughtSummary(context.Background(), DeepDrillThoughtQuery{SessionID: "s1"})
	if err != nil {
		t.Fatalf("DeepDrillThoughtSummary failed: %v", err)
	}
	if len(summary.OpenQuestions) == 0 {
		t.Fatal("expected open questions")
	}
	if len(summary.FrozenBranches) == 0 {
		t.Fatal("expected frozen branches")
	}
	if len(summary.UnresolvedContradictions) == 0 {
		t.Fatal("expected unresolved contradictions")
	}
	if len(summary.RecentModelRevisions) == 0 {
		t.Fatal("expected model revisions")
	}
	if len(summary.HighInformationGainRuns) == 0 {
		t.Fatal("expected high-information-gain runs")
	}
	if len(summary.NegativeResults) == 0 {
		t.Fatal("expected negative results")
	}
	if summary.ThoughtCountByType[string(DeepDrillThoughtModelRevision)] == 0 {
		t.Fatalf("expected count by type for model revision, got %#v", summary.ThoughtCountByType)
	}
}

func TestBuildDeepDrillProvenanceTraceExplicitAndInferredEdges(t *testing.T) {
	anchor := deepDrillProvenanceNode{
		SourceID:          "doc-A",
		DocumentID:        "doc-A",
		SourceType:        "document",
		SourceTitle:       "Lecture Notes",
		FamilyID:          sourceFamilyKey("Lecture Notes"),
		OriginalTimestamp: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		DerivedFrom:       []string{"raw-notes"},
	}
	nodes := []deepDrillProvenanceNode{
		anchor,
		{
			SourceID:           "summary-A",
			DocumentID:         "summary-A",
			SourceType:         "audio_overview",
			SourceTitle:        "Lecture Notes",
			FamilyID:           anchor.FamilyID,
			DerivedFrom:        []string{"doc-A"},
			CitedSources:       []string{"doc-A"},
			OriginalTimestamp:  time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC),
			IngestionTimestamp: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			SourceID:          "raw-notes",
			DocumentID:        "raw-notes",
			SourceType:        "document",
			SourceTitle:       "Lecture Notes",
			FamilyID:          anchor.FamilyID,
			OriginalTimestamp: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		},
	}
	trace := buildDeepDrillProvenanceTrace("doc-A", DeepDrillProvenanceTraceResult{Collection: "corpus"}, nodes)
	if len(trace.Upstream) == 0 {
		t.Fatalf("expected upstream edges, got %#v", trace)
	}
	if len(trace.Downstream) == 0 {
		t.Fatalf("expected downstream edges, got %#v", trace)
	}
	if len(trace.RelatedSources) == 0 {
		t.Fatalf("expected related edges, got %#v", trace)
	}
	hasExplicit := false
	for _, edge := range append(append([]DeepDrillProvenanceEdge{}, trace.Upstream...), trace.Downstream...) {
		if edge.Explicit && (edge.Relation == DeepDrillDerivedFrom || edge.Relation == DeepDrillCites) {
			hasExplicit = true
			break
		}
	}
	if !hasExplicit {
		t.Fatalf("expected explicit provenance relation, got %#v", trace)
	}
	hasInferred := false
	for _, edge := range trace.RelatedSources {
		if !edge.Explicit && (edge.Relation == DeepDrillSameSourceFamily || edge.Relation == DeepDrillSummarizes || edge.Relation == DeepDrillPresents || edge.Relation == DeepDrillTranscribes) {
			hasInferred = true
			break
		}
	}
	if !hasInferred {
		t.Fatalf("expected inferred provenance relation, got %#v", trace)
	}
}

func TestBuildDeepDrillProvenanceTraceChronologySafeguard(t *testing.T) {
	family := sourceFamilyKey("archive transcript")
	nodes := []deepDrillProvenanceNode{
		{SourceID: "doc-A", DocumentID: "doc-A", SourceType: "document", SourceTitle: "Archive Transcript", FamilyID: family, IngestionTimestamp: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)},
		{SourceID: "doc-B", DocumentID: "doc-B", SourceType: "document", SourceTitle: "Archive Transcript", FamilyID: family, IngestionTimestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	trace := buildDeepDrillProvenanceTrace("doc-A", DeepDrillProvenanceTraceResult{Collection: "corpus"}, nodes)
	if len(trace.ChronologySignals) != 0 {
		t.Fatalf("expected no chronology signal from ingestion timestamps, got %#v", trace.ChronologySignals)
	}
	hasGuard := false
	for _, weakness := range trace.Weaknesses {
		if strings.Contains(strings.ToLower(weakness), "ingestion") {
			hasGuard = true
			break
		}
	}
	if !hasGuard {
		t.Fatalf("expected ingestion chronology safeguard weakness, got %#v", trace.Weaknesses)
	}
}

func TestBuildDeepDrillProvenanceTraceUnknownRelationWhenNoLinks(t *testing.T) {
	nodes := []deepDrillProvenanceNode{
		{SourceID: "doc-A", DocumentID: "doc-A", SourceType: "document", SourceTitle: "Primary", FamilyID: sourceFamilyKey("Primary")},
	}
	trace := buildDeepDrillProvenanceTrace("doc-A", DeepDrillProvenanceTraceResult{Collection: "corpus"}, nodes)
	foundUnknown := false
	for _, edge := range trace.RelatedSources {
		if edge.Relation == DeepDrillUnknownRelation {
			foundUnknown = true
			break
		}
	}
	if !foundUnknown {
		t.Fatalf("expected unknown relation fallback, got %#v", trace.RelatedSources)
	}
}

func TestBuildDeepDrillProvenanceTraceRawSourceHasHigherConfidenceThanDerived(t *testing.T) {
	raw := buildDeepDrillProvenanceTrace("doc-raw", DeepDrillProvenanceTraceResult{Collection: "corpus"}, []deepDrillProvenanceNode{{SourceID: "doc-raw", DocumentID: "doc-raw", SourceType: "document", SourceTitle: "Core Notes", FamilyID: sourceFamilyKey("Core Notes")}})
	derived := buildDeepDrillProvenanceTrace("doc-derived", DeepDrillProvenanceTraceResult{Collection: "corpus"}, []deepDrillProvenanceNode{{SourceID: "doc-derived", DocumentID: "doc-derived", SourceType: "audio_overview", SourceTitle: "Core Notes", FamilyID: sourceFamilyKey("Core Notes")}})
	if raw.Confidence <= derived.Confidence {
		t.Fatalf("expected raw source confidence (%f) to exceed derived confidence (%f)", raw.Confidence, derived.Confidence)
	}
}

func TestReclassifyUncertaintyIsEvidenceDriven(t *testing.T) {
	state := &DeepDrillState{CurrentUncertainty: DeepDrillEvidenceGap, ThoughtCollection: "minddrill_research_thoughts"}
	// Weak derived provenance reclassifies an evidence gap into a provenance gap.
	got, reason, changed := reclassifyUncertainty(state, []ResearchResult{{SourceType: "audio_overview"}}, 0.42, DeepDrillAudioOverview, false, "trace earliest bridge")
	if !changed || got != DeepDrillProvenanceGap || reason == "" {
		t.Fatalf("expected provenance gap reclassification, got %s changed=%v reason=%q", got, changed, reason)
	}
	// Contradiction signals take priority over provenance.
	got, _, changed = reclassifyUncertainty(state, []ResearchResult{{Text: "this contradicts the earlier claim"}}, 0.9, DeepDrillRawSource, true, "which claim holds")
	if !changed || got != DeepDrillContradiction {
		t.Fatalf("expected contradiction reclassification, got %s changed=%v", got, changed)
	}
	// No coverage points to retrieval limit.
	got, _, changed = reclassifyUncertainty(state, nil, 0, DeepDrillUnknownSourceTier, false, "trace")
	if !changed || got != DeepDrillRetrievalLimit {
		t.Fatalf("expected retrieval limit reclassification, got %s changed=%v", got, changed)
	}
	// Same class does not count as a reclassification.
	state.CurrentUncertainty = DeepDrillProvenanceGap
	_, _, changed = reclassifyUncertainty(state, []ResearchResult{{SourceType: "audio_overview"}}, 0.42, DeepDrillAudioOverview, false, "trace")
	if changed {
		t.Fatalf("expected no reclassification when class is unchanged")
	}
}

func TestStrategyAvailabilityTracksExhaustedAndRemaining(t *testing.T) {
	state := &DeepDrillState{CurrentUncertainty: DeepDrillProvenanceGap, ThoughtCollection: "minddrill_research_thoughts"}
	markStrategyExhausted(state, DeepDrillProvenanceGap, DeepDrillProvenanceTrace)
	remaining, exhausted := strategyAvailability(state, DeepDrillProvenanceGap)
	if len(exhausted) != 1 || exhausted[0] != DeepDrillProvenanceTrace {
		t.Fatalf("expected provenance trace exhausted, got %#v", exhausted)
	}
	if len(remaining) == 0 || remaining[0] == DeepDrillProvenanceTrace {
		t.Fatalf("expected other provenance strategies to remain, got %#v", remaining)
	}
}

func TestBuildDeepDrillProvenanceTraceDuplicateFamilyDetection(t *testing.T) {
	family := sourceFamilyKey("Field Notes")
	trace := buildDeepDrillProvenanceTrace("doc-A", DeepDrillProvenanceTraceResult{Collection: "corpus"}, []deepDrillProvenanceNode{
		{SourceID: "doc-A", DocumentID: "doc-A", SourceType: "document", SourceTitle: "Field Notes", FamilyID: family},
		{SourceID: "doc-B", DocumentID: "doc-B", SourceType: "document", SourceTitle: "Field Notes", FamilyID: family},
	})
	hasDuplicateFamily := false
	for _, edge := range trace.RelatedSources {
		if edge.Relation == DeepDrillSameSourceFamily && strings.Contains(strings.ToLower(edge.Basis), "duplicate source family") {
			hasDuplicateFamily = true
			break
		}
	}
	if !hasDuplicateFamily {
		t.Fatalf("expected duplicate source family relation, got %#v", trace.RelatedSources)
	}
}
