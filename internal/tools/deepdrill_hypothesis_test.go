package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meistro57/frontpocket/internal/store"
)

func TestDeepDrillSupportOnlyAffectsTargetHypothesis(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = newScoredHypotheses()
	applyModelUpdate(state, DeepDrillEvidenceGap, []ResearchResult{{MemoryID: "e1", Text: "alpha", SourceType: "document"}}, 1.0)
	h1 := hypothesisByID(state.Hypotheses, "H1")
	h2 := hypothesisByID(state.Hypotheses, "H2")
	if !(h1.Confidence > 0.5) {
		t.Fatalf("expected H1 confidence to increase, got %f", h1.Confidence)
	}
	if h2.Confidence != 0.5 || h2.EvidenceStrength != 0.3 {
		t.Fatalf("expected H2 unchanged, got conf=%f ev=%f", h2.Confidence, h2.EvidenceStrength)
	}
	if hypothesisByID(state.Hypotheses, "H3").Confidence != 0.5 || hypothesisByID(state.Hypotheses, "H4").Confidence != 0.5 {
		t.Fatal("expected H3/H4 unchanged")
	}
}

func TestDeepDrillCounterevidenceOnlyAffectsTargetHypothesis(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = newScoredHypotheses()
	applyModelUpdate(state, DeepDrillContradiction, []ResearchResult{{MemoryID: "e1", Text: "beta however", SourceType: "document"}}, 1.0)
	h2 := hypothesisByID(state.Hypotheses, "H2")
	if !(h2.Confidence < 0.5) {
		t.Fatalf("expected H2 confidence to decrease, got %f", h2.Confidence)
	}
	if h2.Status != "contested" {
		t.Fatalf("expected H2 contested, got %s", h2.Status)
	}
	if hypothesisByID(state.Hypotheses, "H1").Confidence != 0.5 || hypothesisByID(state.Hypotheses, "H3").Confidence != 0.5 || hypothesisByID(state.Hypotheses, "H4").Confidence != 0.5 {
		t.Fatal("expected H1/H3/H4 unchanged")
	}
}

func TestDeepDrillDuplicateEvidenceCountedOnce(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = newScoredHypotheses()
	evidence := []ResearchResult{{MemoryID: "e1", Text: "alpha", SourceType: "document"}}
	applyModelUpdate(state, DeepDrillEvidenceGap, evidence, 1.0)
	first := hypothesisByID(state.Hypotheses, "H1").Confidence
	applyModelUpdate(state, DeepDrillEvidenceGap, evidence, 1.0)
	second := hypothesisByID(state.Hypotheses, "H1").Confidence
	if first != second {
		t.Fatalf("expected duplicate evidence to produce no additional change, got %f then %f", first, second)
	}
}

func TestDeepDrillNoEvidenceDoesNotCollapseConfidence(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = newScoredHypotheses()
	applyModelUpdate(state, DeepDrillEvidenceGap, nil, 0)
	for _, h := range state.Hypotheses {
		if h.Confidence != 0.5 {
			t.Fatalf("expected confidence unchanged without evidence, %s got %f", h.ID, h.Confidence)
		}
	}
}

func TestDeepDrillMixedEvidenceNetReflectsBoth(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = newScoredHypotheses()
	evidence := []ResearchResult{
		{MemoryID: "e1", Text: "alpha", SourceType: "document"},
		{MemoryID: "e2", Text: "alpha extra", SourceType: "document"},
		{MemoryID: "e3", Text: "alpha however", SourceType: "document"},
	}
	applyModelUpdate(state, DeepDrillContradiction, evidence, 1.0)
	h1 := hypothesisByID(state.Hypotheses, "H1")
	// two support (2.0) minus one counter (1.0) = net +1.0 weight.
	if !(h1.Confidence > 0.5) {
		t.Fatalf("expected net positive confidence, got %f", h1.Confidence)
	}
	if h1.Status != "contested" {
		t.Fatalf("expected contested status when counterevidence present, got %s", h1.Status)
	}
	if h1.UniqueSupportCount != 2 || h1.UniqueCounterevidenceCount != 1 {
		t.Fatalf("expected 2 support and 1 counter, got %d/%d", h1.UniqueSupportCount, h1.UniqueCounterevidenceCount)
	}
}

func TestDeepDrillRankRecomputationOvertakesSeed(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = newScoredHypotheses()
	applyModelUpdate(state, DeepDrillEvidenceGap, []ResearchResult{{MemoryID: "e1", Text: "beta", SourceType: "document"}}, 1.0)
	h2 := hypothesisByID(state.Hypotheses, "H2")
	h1 := hypothesisByID(state.Hypotheses, "H1")
	if h2.Rank != 1 || h1.Rank != 2 {
		t.Fatalf("expected H2 to overtake H1, got H2 rank=%d H1 rank=%d", h2.Rank, h1.Rank)
	}
	if h2.RankDelta != -1 {
		t.Fatalf("expected H2 rank_delta -1, got %d", h2.RankDelta)
	}
}

func TestDeepDrillTiedEvidenceRemainsDeterministic(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = newScoredHypotheses()
	applyModelUpdate(state, DeepDrillEvidenceGap, []ResearchResult{
		{MemoryID: "e1", Text: "alpha", SourceType: "document"},
		{MemoryID: "e2", Text: "beta", SourceType: "document"},
	}, 1.0)
	h1 := hypothesisByID(state.Hypotheses, "H1")
	h2 := hypothesisByID(state.Hypotheses, "H2")
	if h1.Confidence != h2.Confidence {
		t.Fatalf("expected tied confidence, got H1=%f H2=%f", h1.Confidence, h2.Confidence)
	}
	if h1.Rank != 1 || h2.Rank != 2 {
		t.Fatalf("expected deterministic tie order H1 then H2, got H1=%d H2=%d", h1.Rank, h2.Rank)
	}
}

func TestDeepDrillUnassociatedEvidenceDoesNotScore(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = newScoredHypotheses()
	applyModelUpdate(state, DeepDrillEvidenceGap, []ResearchResult{{MemoryID: "e1", Text: "unrelated zzz", SourceType: "document"}}, 1.0)
	for _, h := range state.Hypotheses {
		if h.Confidence != 0.5 || h.EvidenceStrength != 0.3 {
			t.Fatalf("expected %s unchanged by unassociated evidence, got conf=%f ev=%f", h.ID, h.Confidence, h.EvidenceStrength)
		}
	}
}

func TestDeepDrillEvidenceClassificationDiagnostics(t *testing.T) {
	hypotheses := []DeepDrillHypothesisState{
		{ID: "H1", Statement: "The sacred-symbolic vocabulary was directly translated into E8 technical terminology"},
		{ID: "H2", Statement: "The sacred-symbolic and E8 traditions developed independently and were later recombined in a synthesis"},
		{ID: "H3", Statement: "The apparent bridge between sacred-symbolic and E8 terms is a retrieval artifact"},
		{ID: "H4", Statement: "Generated summaries and chunking produced an apparent lineage that is absent from the underlying transcripts"},
	}
	evidence := []ResearchResult{
		{MemoryID: "e-direct-support", Text: "the two traditions developed independently and were later recombined in a synthesis"},
		{MemoryID: "e-direct-contradiction", Text: "the sacred vocabulary does not map onto E8 technical terminology; the bridge is contradicted by the transcripts"},
		{MemoryID: "e-unrelated-thematic", Text: "the E8 lattice has 240 root vectors in eight dimensions"},
		{MemoryID: "e-relational-paraphrase", Text: "the two traditions arose separately and were joined in the later synthesis"},
		{MemoryID: "e-retrieval-artifact", Text: "the connection appears only in generated overviews and is absent from the underlying transcripts"},
		{MemoryID: "e-opposite-meaning", Text: "the traditions never developed independently and were never recombined"},
		{MemoryID: "e-empty-source", Text: ""},
	}

	rows := diagnoseEvidenceClassification(hypotheses, evidence)

	if want := len(hypotheses) * len(evidence); len(rows) != want {
		t.Fatalf("expected %d diagnostic rows, got %d", want, len(rows))
	}

	for _, row := range rows {
		t.Logf("evidence=%-24s hypothesis=%s fields=%v lexical=%.2f concepts=%v relations=%v negation=%v relevance=%.2f relevant=%v polarity=%s reason=%q",
			row.EvidenceID, row.HypothesisID, row.UsableTextFields, row.LexicalOverlap, row.ConceptMatches, row.RelationMatches, row.NegationSignals, row.RelevanceScore, row.Relevant, row.Polarity, row.Reason)
	}

	row := func(evidenceID, hypothesisID string) deepDrillClassificationRow {
		for _, r := range rows {
			if r.EvidenceID == evidenceID && r.HypothesisID == hypothesisID {
				return r
			}
		}
		t.Fatalf("missing diagnostic row for %s x %s", evidenceID, hypothesisID)
		return deepDrillClassificationRow{}
	}

	if got := row("e-direct-support", "H2"); got.Polarity != "support" {
		t.Fatalf("expected direct lexical support to classify H2 as support, got %s (relevance=%.2f)", got.Polarity, got.RelevanceScore)
	}
	if got := row("e-direct-contradiction", "H1"); got.Polarity != "counter" {
		t.Fatalf("expected direct contradiction to classify H1 as counter, got %s (relevance=%.2f)", got.Polarity, got.RelevanceScore)
	}
	if got := row("e-unrelated-thematic", "H3"); got.Polarity != "none" {
		t.Fatalf("expected unrelated thematic text to leave H3 unclassified, got %s", got.Polarity)
	}
	if got := row("e-relational-paraphrase", "H2"); got.Polarity != "support" {
		t.Fatalf("expected relational paraphrase to support H2, got %s relations=%v", got.Polarity, got.RelationMatches)
	}
	if got := row("e-retrieval-artifact", "H4"); got.Polarity != "support" {
		t.Fatalf("expected retrieval-artifact paraphrase to support H4, got %s relations=%v", got.Polarity, got.RelationMatches)
	}
	if got := row("e-opposite-meaning", "H2"); got.Polarity != "counter" {
		t.Fatalf("expected opposite-meaning evidence to counter H2, got %s negation=%v", got.Polarity, got.NegationSignals)
	}
	if got := row("e-empty-source", "H1"); got.Reason != "no classifiable source text" {
		t.Fatalf("expected empty source text to surface a diagnostic reason, got %q", got.Reason)
	}
}

func TestDeepDrillRelationalParaphraseRelevantSupport(t *testing.T) {
	h := DeepDrillHypothesisState{ID: "H2", Statement: "branches developed independently then recombined"}
	got := classifyEvidenceForHypothesis(h, ResearchResult{MemoryID: "e1", Text: "the two traditions arose separately and were joined in the later synthesis."})
	if !got.Relevant || got.Polarity != deepDrillPolaritySupport {
		t.Fatalf("expected relevant SUPPORT, got relevant=%v polarity=%s reason=%q concepts=%v relations=%v", got.Relevant, got.Polarity, got.Reason, got.ConceptMatches, got.RelationMatches)
	}
}

func TestDeepDrillRetrievalArtifactParaphraseSupport(t *testing.T) {
	h := DeepDrillHypothesisState{ID: "H4", Statement: "hybrid lineage may be produced by summaries/chunking/ingestion/retrieval"}
	got := classifyEvidenceForHypothesis(h, ResearchResult{MemoryID: "e1", Text: "the connection appears only in generated overviews and is absent from the underlying transcripts."})
	if !got.Relevant || got.Polarity != deepDrillPolaritySupport {
		t.Fatalf("expected relevant SUPPORT, got relevant=%v polarity=%s reason=%q concepts=%v relations=%v", got.Relevant, got.Polarity, got.Reason, got.ConceptMatches, got.RelationMatches)
	}
}

func TestDeepDrillE8SurvivesNormalization(t *testing.T) {
	concepts := normalizeConcepts("the E8 technical terminology")
	found := false
	for _, concept := range concepts {
		if concept == "e8" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected e8 to survive normalization, got %v", concepts)
	}
}

func TestDeepDrillDevelopedDevelopmentNormalizeTogether(t *testing.T) {
	a := normalizeConcepts("developed")
	b := normalizeConcepts("development")
	if len(a) != 1 || len(b) != 1 || a[0] != "develop" || b[0] != "develop" {
		t.Fatalf("expected developed/development to normalize to develop, got %v and %v", a, b)
	}
}

func TestDeepDrillIndependentlySeparatelySameFamily(t *testing.T) {
	a := normalizeConcepts("independently")
	b := normalizeConcepts("separately")
	if len(a) != 1 || len(b) != 1 || a[0] != "independent" || b[0] != "independent" {
		t.Fatalf("expected independently/separately to normalize to independent, got %v and %v", a, b)
	}
}

func TestDeepDrillRecombinedJoinedSameFamily(t *testing.T) {
	a := normalizeConcepts("recombined")
	b := normalizeConcepts("joined")
	if len(a) != 1 || len(b) != 1 || a[0] != "recombine" || b[0] != "recombine" {
		t.Fatalf("expected recombined/joined to normalize to recombine, got %v and %v", a, b)
	}
}

func TestDeepDrillAmbiguousPolarity(t *testing.T) {
	h := DeepDrillHypothesisState{ID: "H1", Statement: "alpha"}
	got := classifyEvidenceForHypothesis(h, ResearchResult{MemoryID: "e1", Text: "alpha may have been the source"})
	if !got.Relevant || got.Polarity != deepDrillPolarityAmbiguous {
		t.Fatalf("expected relevant AMBIGUOUS, got relevant=%v polarity=%s reason=%q", got.Relevant, got.Polarity, got.Reason)
	}
}

func TestDeepDrillClassifiesFromSummaryAndQuote(t *testing.T) {
	h := DeepDrillHypothesisState{ID: "H2", Statement: "branches developed independently then recombined"}
	item := ResearchResult{
		MemoryID:    "e1",
		Summary:     "the two traditions developed independently and recombined",
		SourceQuote: "developed independently and recombined",
	}
	got := classifyEvidenceForHypothesis(h, item)
	if !got.Relevant || got.Polarity != deepDrillPolaritySupport {
		t.Fatalf("expected summary/quote-only evidence to support, got relevant=%v polarity=%s fields=%v reason=%q", got.Relevant, got.Polarity, got.UsableTextFields, got.Reason)
	}
	if len(got.UsableTextFields) < 2 {
		t.Fatalf("expected multiple usable text fields, got %v", got.UsableTextFields)
	}
}

func TestDeepDrillPolarityNotDerivedFromStrategy(t *testing.T) {
	h := DeepDrillHypothesisState{ID: "H1", Statement: "alpha"}
	// A counterevidence search can retrieve a chunk that merely restates the
	// hypothesis vocabulary. Without a negation signal it must not be counter.
	got := classifyEvidenceForHypothesis(h, ResearchResult{MemoryID: "e1", Text: "alpha present in the source"})
	if got.Polarity != deepDrillPolaritySupport {
		t.Fatalf("expected support without a negation signal, got %s", got.Polarity)
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
