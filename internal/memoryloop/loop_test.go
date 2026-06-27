package memoryloop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

type fakeSource struct {
	batches [][]memory.MemoryPoint
	calls   int
}

func (f *fakeSource) ScrollRaw(_ context.Context, _ int, offset string, _ memory.SearchFilters, _, _ time.Time, _ bool) ([]memory.MemoryPoint, string, error) {
	if offset == "end" {
		return []memory.MemoryPoint{}, "", nil
	}
	if f.calls >= len(f.batches) {
		return []memory.MemoryPoint{}, "", nil
	}
	points := f.batches[f.calls]
	f.calls++
	next := ""
	if f.calls < len(f.batches) {
		next = "next"
	}
	if f.calls >= len(f.batches) {
		next = "end"
	}
	return points, next, nil
}

func TestMemoryHarvesterLoadsBatches(t *testing.T) {
	source := &fakeSource{batches: [][]memory.MemoryPoint{{{MemoryID: "1"}}, {{MemoryID: "2"}}}}
	harvester := NewMemoryHarvester(source)
	count, err := harvester.Harvest(context.Background(), HarvestFilter{BatchSize: 1}, func(_ []memory.MemoryPoint) error { return nil })
	if err != nil {
		t.Fatalf("harvest failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 processed records, got %d", count)
	}
	if source.calls != 2 {
		t.Fatalf("expected 2 source calls, got %d", source.calls)
	}
}

func TestRunnerDryRunDoesNotWriteQueue(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "proposed.json")
	source := &fakeSource{batches: [][]memory.MemoryPoint{{samplePoint("m1", "user", "FrontPocket")}}}
	runner := Runner{
		Harvester:   NewMemoryHarvester(source),
		Clusterer:   MemoryClusterer{MaxClusterSize: 25},
		Extractor:   FactExtractor{},
		Scout:       ContradictionScout{},
		Timeline:    TimelineBuilder{},
		Curator:     CanonCurator{},
		ReviewQueue: NewFileReviewQueue(tmp),
	}
	result, err := runner.Run(context.Background(), HarvestFilter{BatchSize: 50}, false)
	if err != nil {
		t.Fatalf("runner failed: %v", err)
	}
	if len(result.Candidates) == 0 {
		t.Fatal("expected candidates in dry-run")
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("expected no queue file in dry-run, got err=%v", err)
	}
}

func TestRunnerWriteCandidatesPersistsQueue(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "proposed.json")
	source := &fakeSource{batches: [][]memory.MemoryPoint{{samplePoint("m1", "user", "FrontPocket")}}}
	runner := Runner{
		Harvester:   NewMemoryHarvester(source),
		Clusterer:   MemoryClusterer{MaxClusterSize: 25},
		Extractor:   FactExtractor{},
		Scout:       ContradictionScout{},
		Timeline:    TimelineBuilder{},
		Curator:     CanonCurator{},
		ReviewQueue: NewFileReviewQueue(tmp),
	}
	result, err := runner.Run(context.Background(), HarvestFilter{BatchSize: 50}, true)
	if err != nil {
		t.Fatalf("runner failed: %v", err)
	}
	if len(result.Candidates) == 0 {
		t.Fatal("expected candidates in write mode")
	}
	queue := NewFileReviewQueue(tmp)
	records, err := queue.List(CandidateFilter{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected persisted candidates")
	}
	if len(records[0].SourceMemoryIDs) == 0 || len(records[0].SourceQuotes) == 0 {
		t.Fatalf("expected provenance on candidate: %#v", records[0])
	}
	if records[0].Confidence == "" || records[0].Status == "" {
		t.Fatalf("expected confidence and status: %#v", records[0])
	}
}

func TestExtractorDoesNotMarkAssistantOnlyAsDirectUserStatement(t *testing.T) {
	cluster := Cluster{ClusterID: "c1", Label: "test", Points: []memory.MemoryPoint{samplePoint("m1", "assistant", "FrontPocket")}}
	candidates := (FactExtractor{}).Extract(cluster)
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(candidates))
	}
	if candidates[0].Status == memory.StatusDirectUserStatement {
		t.Fatalf("expected inferred status for assistant-only cluster, got %q", candidates[0].Status)
	}
}

func TestReviewQueueApprovalAndRejection(t *testing.T) {
	queue := NewFileReviewQueue(filepath.Join(t.TempDir(), "proposed.json"))
	candidate := Candidate{
		ID:              "cand_1",
		Summary:         "Mark prefers concise responses.",
		MemoryKind:      memory.KindPreference,
		Confidence:      memory.ConfidenceHigh,
		Status:          memory.StatusInferredFromSources,
		SourceMemoryIDs: []string{"m1"},
		SourceQuotes:    []string{"keep responses concise"},
		CreatedByLoop:   true,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := queue.Upsert([]Candidate{candidate}); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	store := memory.NewInMemoryStore()
	approved, err := queue.Approve(context.Background(), candidate.ID, "mark", store, nil)
	if err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if !approved.Canonical || approved.Status != memory.StatusApprovedByUser {
		t.Fatalf("expected approved canonical status, got %#v", approved)
	}
	results, err := store.Search(context.Background(), memory.SearchRequest{Query: "concise", Limit: 5})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 || !results[0].Canonical {
		t.Fatalf("expected canonical memory after approval, got %#v", results)
	}

	rejected, err := queue.Reject(candidate.ID, "too vague", "mark")
	if err != nil {
		t.Fatalf("reject failed: %v", err)
	}
	if rejected.Status != memory.StatusRejected || len(rejected.SourceMemoryIDs) == 0 || len(rejected.SourceQuotes) == 0 {
		t.Fatalf("expected rejected candidate preserving provenance, got %#v", rejected)
	}
}

func TestReviewQueueMergePreservesProvenance(t *testing.T) {
	queue := NewFileReviewQueue(filepath.Join(t.TempDir(), "proposed.json"))
	candidate := Candidate{ID: "cand_2", Summary: "project moved to go", MemoryKind: memory.KindProjectDecision, SourceMemoryIDs: []string{"m1"}, SourceQuotes: []string{"moved to go"}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := queue.Upsert([]Candidate{candidate}); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	merged, err := queue.Merge("cand_2", "canon_1", "mark")
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if merged.MergeTargetID != "canon_1" || len(merged.MergedFrom) == 0 {
		t.Fatalf("expected merge provenance, got %#v", merged)
	}
}

func TestTimelineBuilderAddsDateBasis(t *testing.T) {
	points := []memory.MemoryPoint{samplePoint("m1", "user", "FrontPocket"), samplePoint("m2", "assistant", "FrontPocket")}
	points[0].Timestamp = time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	points[1].Timestamp = time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	candidates := (TimelineBuilder{}).Build(points)
	if len(candidates) == 0 {
		t.Fatal("expected timeline candidates")
	}
	if candidates[0].ApproximateDate == "" || candidates[0].DateBasis == "" {
		t.Fatalf("expected approximate date and date basis, got %#v", candidates[0])
	}
}

func TestContradictionScoutMarksNeedsReview(t *testing.T) {
	input := []Candidate{
		{ID: "a", Summary: "Project uses Python", Project: "FrontPocket", MemoryKind: memory.KindProjectDecision, SourceMemoryIDs: []string{"m1"}, SourceQuotes: []string{"uses Python"}},
		{ID: "b", Summary: "Project moved to Go", Project: "FrontPocket", MemoryKind: memory.KindProjectDecision, SourceMemoryIDs: []string{"m2"}, SourceQuotes: []string{"moved to Go"}},
	}
	output, conflicts := (ContradictionScout{}).Check(input)
	if conflicts == 0 {
		t.Fatal("expected conflict count")
	}
	found := false
	for _, candidate := range output {
		if candidate.MemoryKind == memory.KindContradictionNote && candidate.Status == memory.StatusNeedsReview {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected contradiction candidate, got %#v", output)
	}
}

func samplePoint(id, speaker, project string) memory.MemoryPoint {
	now := time.Now().UTC()
	return memory.MemoryPoint{
		MemoryID:       id,
		ConversationID: "conv_1",
		SourceType:     "chat_export",
		SourceTitle:    "Test",
		Timestamp:      now,
		Speaker:        speaker,
		Project:        project,
		MemoryKind:     memory.KindProjectDecision,
		Text:           "Mark prefers paste-ready implementation prompts.",
		SourceQuote:    "Mark prefers paste-ready prompts.",
		Summary:        "Mark prefers paste-ready prompts.",
	}
}
