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

func TestDeepDrillProvenanceDoesNotGloballyPenalize(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = []DeepDrillHypothesisState{
		{ID: "H3", Rank: 1, Confidence: 0.6, EvidenceStrength: 0.5},
		{ID: "H2", Rank: 2, Confidence: 0.55, EvidenceStrength: 0.4},
	}
	// A weak-provenance investigation with no hypothesis-relevant evidence
	// must not mechanically drive every hypothesis confidence toward zero.
	applyModelUpdate(state, DeepDrillProvenanceGap, []ResearchResult{{SourceType: "audio_overview"}}, 0.3)
	if state.Hypotheses[0].Confidence != 0.6 || state.Hypotheses[1].Confidence != 0.55 {
		t.Fatalf("expected no global provenance penalty, got %#v", state.Hypotheses)
	}
	if state.Hypotheses[0].Rank != 1 || state.Hypotheses[1].Rank != 2 {
		t.Fatalf("expected rank order preserved, got %#v", state.Hypotheses)
	}
}

func newScoredHypotheses() []DeepDrillHypothesisState {
	return []DeepDrillHypothesisState{
		{ID: "H1", Statement: "alpha", Rank: 1, Confidence: 0.5, EvidenceStrength: 0.3, Status: "active"},
		{ID: "H2", Statement: "beta", Rank: 2, Confidence: 0.5, EvidenceStrength: 0.3, Status: "active"},
		{ID: "H3", Statement: "gamma", Rank: 3, Confidence: 0.5, EvidenceStrength: 0.3, Status: "active"},
		{ID: "H4", Statement: "delta", Rank: 4, Confidence: 0.5, EvidenceStrength: 0.3, Status: "active"},
	}
}

func hypothesisByID(hypotheses []DeepDrillHypothesisState, id string) DeepDrillHypothesisState {
	for _, h := range hypotheses {
		if h.ID == id {
			return h
		}
	}
	return DeepDrillHypothesisState{}
}

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

func TestDeepDrillWeakProvenanceContributesLess(t *testing.T) {
	raw := newDefaultDeepDrillState("minddrill_research_thoughts")
	raw.Hypotheses = newScoredHypotheses()
	applyModelUpdate(raw, DeepDrillEvidenceGap, []ResearchResult{{MemoryID: "e1", Text: "alpha", SourceType: "document"}}, 1.0)
	rawDelta := hypothesisByID(raw.Hypotheses, "H1").Confidence - 0.5

	weak := newDefaultDeepDrillState("minddrill_research_thoughts")
	weak.Hypotheses = newScoredHypotheses()
	applyModelUpdate(weak, DeepDrillEvidenceGap, []ResearchResult{{MemoryID: "e1", Text: "alpha", SourceType: "audio_overview"}}, 1.0)
	weakDelta := hypothesisByID(weak.Hypotheses, "H1").Confidence - 0.5

	if !(weakDelta < rawDelta) {
		t.Fatalf("expected weak provenance to contribute less, raw=%f weak=%f", rawDelta, weakDelta)
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

func TestDeepDrillOppositeMeaningCounter(t *testing.T) {
	h := DeepDrillHypothesisState{ID: "H2", Statement: "branches developed independently then recombined"}
	got := classifyEvidenceForHypothesis(h, ResearchResult{MemoryID: "e1", Text: "the branches never developed independently and were never recombined."})
	if !got.Relevant || got.Polarity != deepDrillPolarityCounter {
		t.Fatalf("expected relevant COUNTER, got relevant=%v polarity=%s reason=%q negation=%v", got.Relevant, got.Polarity, got.Reason, got.NegationSignals)
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

func TestDeepDrillRepeatedProvenanceInspectionNoRepeatPenalty(t *testing.T) {
	state := newDefaultDeepDrillState("minddrill_research_thoughts")
	state.Hypotheses = newScoredHypotheses()
	evidence := []ResearchResult{{MemoryID: "e1", Text: "alpha", SourceType: "audio_overview"}}
	applyModelUpdate(state, DeepDrillProvenanceGap, evidence, 0.42)
	first := hypothesisByID(state.Hypotheses, "H1").Confidence
	applyModelUpdate(state, DeepDrillProvenanceGap, evidence, 0.42)
	second := hypothesisByID(state.Hypotheses, "H1").Confidence
	if second < first {
		t.Fatalf("expected no repeated provenance penalty, got %f then %f", first, second)
	}
	if second != first {
		t.Fatalf("expected duplicate evidence to be ignored on re-inspection, got %f then %f", first, second)
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

func seedExhaustedDeepDrillSession(t *testing.T, runtime *MindDrillResearchRuntime, sessionID string, frozen bool) *DeepDrillState {
	t.Helper()
	_, _, state, err := runtime.prepareDeepDrillSession(context.Background(), DeepDrillPlanRequest{SessionID: sessionID, Collection: "corpus", ResearchQuestion: "trace", KnownBlockers: []DeepDrillUncertainty{DeepDrillProvenanceGap}})
	if err != nil {
		t.Fatalf("prepare session failed: %v", err)
	}
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
	if frozen {
		state.BranchStatus = DeepDrillFrozen
		state.FrozenReason = "test freeze"
	}
	session, _, err := runtime.requireBoundCollection(sessionID)
	if err != nil {
		t.Fatalf("require bound session failed: %v", err)
	}
	session.DeepDrill = state
	runtime.sessions.put(session)
	return state
}

func TestDeepDrillFreezeTerminatesEarlyAndUniqueSteps(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, false)
	defer fixture.Close()
	sessionPath := filepath.Join(t.TempDir(), "deepdrill_sessions.json")
	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 1)
	seedExhaustedDeepDrillSession(t, runtime, "s1", false)
	result, err := runtime.DeepDrillRun(context.Background(), DeepDrillRunRequest{DeepDrillPlanRequest: DeepDrillPlanRequest{SessionID: "s1"}, Steps: 5})
	if err != nil {
		t.Fatalf("deepdrill run failed: %v", err)
	}
	if result.State.BranchStatus != DeepDrillFrozen {
		t.Fatalf("expected frozen branch, got %s", result.State.BranchStatus)
	}
	if len(result.Steps) >= 5 {
		t.Fatalf("expected run to terminate before max steps, got %d steps", len(result.Steps))
	}
	seen := map[int]bool{}
	for _, step := range result.Steps {
		if seen[step.Step] {
			t.Fatalf("duplicate step number %d in results %#v", step.Step, result.Steps)
		}
		seen[step.Step] = true
	}
}

func TestDeepDrillEmitsExactlyOneTerminalFreezeArtifact(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, false)
	defer fixture.Close()
	sessionPath := filepath.Join(t.TempDir(), "deepdrill_sessions.json")
	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 1)
	seedExhaustedDeepDrillSession(t, runtime, "s1", false)
	_, err := runtime.DeepDrillRun(context.Background(), DeepDrillRunRequest{DeepDrillPlanRequest: DeepDrillPlanRequest{SessionID: "s1"}, Steps: 5})
	if err != nil {
		t.Fatalf("deepdrill run failed: %v", err)
	}
	stopCount := 0
	for _, payload := range fixture.storedThoughtPayloads {
		if strings.TrimSpace(asString(payload["type"])) == string(DeepDrillThoughtStopDecision) && strings.TrimSpace(asString(payload["strategy"])) == string(DeepDrillFreezeBranch) {
			stopCount++
		}
	}
	if stopCount != 1 {
		t.Fatalf("expected exactly one terminal FREEZE_BRANCH artifact, got %d", stopCount)
	}
}

func TestDeepDrillRerunningFrozenSessionDoesNoWork(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, false)
	defer fixture.Close()
	sessionPath := filepath.Join(t.TempDir(), "deepdrill_sessions.json")
	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 1)
	state := seedExhaustedDeepDrillSession(t, runtime, "s1", true)
	iterationsBefore := state.Iterations
	result, err := runtime.DeepDrillRun(context.Background(), DeepDrillRunRequest{DeepDrillPlanRequest: DeepDrillPlanRequest{SessionID: "s1"}, Steps: 5})
	if err != nil {
		t.Fatalf("deepdrill run failed: %v", err)
	}
	if len(result.Steps) != 0 {
		t.Fatalf("expected zero new research steps for frozen session, got %d", len(result.Steps))
	}
	if result.State.BranchStatus != DeepDrillFrozen {
		t.Fatalf("expected branch to stay frozen, got %s", result.State.BranchStatus)
	}
	if result.State.Iterations != iterationsBefore {
		t.Fatalf("expected iterations unchanged, got %d want %d", result.State.Iterations, iterationsBefore)
	}
}

func TestDeepDrillForceReopenPermitsExecution(t *testing.T) {
	fixture := newDeepDrillQdrantFixture(t, false)
	defer fixture.Close()
	sessionPath := filepath.Join(t.TempDir(), "deepdrill_sessions.json")
	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(fixture.server.URL), researchTestEmbedder{}, "", nil)
	runtime.ConfigureDeepDrill("minddrill_research_thoughts", 1)
	seedExhaustedDeepDrillSession(t, runtime, "s1", true)
	result, err := runtime.DeepDrillRun(context.Background(), DeepDrillRunRequest{DeepDrillPlanRequest: DeepDrillPlanRequest{SessionID: "s1", ForceReopen: true}, Steps: 1})
	if err != nil {
		t.Fatalf("deepdrill run failed: %v", err)
	}
	if len(result.Steps) == 0 {
		t.Fatal("expected force reopen to permit execution again")
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
