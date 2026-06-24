package memory

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// stubEmbedder returns a fixed-width vector for each input text.
type stubEmbedder struct{ dims int }

func (s stubEmbedder) EmbedText(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, s.dims), nil
}

func (s stubEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, s.dims)
	}
	return out, nil
}

func (s stubEmbedder) ProviderName() string { return "stub" }
func (s stubEmbedder) ModelName() string    { return "stub-model" }

// countingStore records every batch it receives and can be set to fail once it
// has accepted a given number of batches, simulating a crash mid-import.
type countingStore struct {
	*InMemoryStore
	batches    int
	failAfter  int // 0 == never fail
	batchSizes []int
}

func (c *countingStore) Upsert(ctx context.Context, points []MemoryPoint) error {
	if c.failAfter > 0 && c.batches >= c.failAfter {
		return errors.New("simulated store failure")
	}
	c.batches++
	c.batchSizes = append(c.batchSizes, len(points))
	return c.InMemoryStore.Upsert(ctx, points)
}

func makeRecords(n int) []MessageRecord {
	recs := make([]MessageRecord, n)
	for i := range recs {
		recs[i] = MessageRecord{
			ConversationID: "conv",
			Speaker:        "user",
			Text:           "hello world message",
			Timestamp:      "2026-06-24T00:00:00Z",
		}
	}
	return recs
}

func newIngestor(store MemoryStore) Ingestor {
	return Ingestor{
		Chunker:      Chunker{Size: 100, Overlap: 0, MinSize: 1},
		Embedder:     stubEmbedder{dims: 4},
		Store:        store,
		BatchSize:    10,
		SpeakerRules: SpeakerRules{StoreUser: true, StoreAssistant: true},
	}
}

func TestIngestFlushesInBatches(t *testing.T) {
	store := &countingStore{InMemoryStore: NewInMemoryStore()}
	ing := newIngestor(store)

	points, err := ing.Ingest(context.Background(), makeRecords(25))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if len(points) != 25 {
		t.Fatalf("expected 25 inserted points, got %d", len(points))
	}
	if store.batches < 3 {
		t.Fatalf("expected multiple batches for 25 records with batch size 10, got %d", store.batches)
	}
	// Returned points must not retain vectors (freed after flush).
	for _, p := range points {
		if p.Vector != nil {
			t.Fatalf("expected vectors to be cleared on returned points")
		}
	}
}

func TestIngestResumesFromJournal(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "progress.json")
	meta := JournalMeta{Source: "export.zip", Collection: "mem", EmbeddingModel: "stub-model"}

	records := makeRecords(25)

	// First run crashes after 2 successful batches (20 records committed).
	j1, resumed, err := OpenFileJournal(journalPath, meta)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	if resumed {
		t.Fatalf("expected fresh journal, got resumed")
	}
	store1 := &countingStore{InMemoryStore: NewInMemoryStore(), failAfter: 2}
	ing1 := newIngestor(store1)
	ing1.Journal = j1
	if _, err := ing1.Ingest(context.Background(), records); err == nil {
		t.Fatalf("expected simulated failure on first run")
	}
	if got := j1.LastRecordIndex(); got != 19 {
		t.Fatalf("expected journal at record index 19 after 2 batches, got %d", got)
	}

	// Second run resumes: only the remaining records should be embedded/stored.
	j2, resumed, err := OpenFileJournal(journalPath, meta)
	if err != nil {
		t.Fatalf("reopen journal: %v", err)
	}
	if !resumed {
		t.Fatalf("expected to resume from existing journal")
	}
	store2 := &countingStore{InMemoryStore: NewInMemoryStore()}
	ing2 := newIngestor(store2)
	ing2.Journal = j2
	points, err := ing2.Ingest(context.Background(), records)
	if err != nil {
		t.Fatalf("resume ingest: %v", err)
	}
	if len(points) != 5 {
		t.Fatalf("expected 5 records on resume (records 20-24), got %d", len(points))
	}
	if j2.LastRecordIndex() != 24 {
		t.Fatalf("expected journal to reach final record index 24, got %d", j2.LastRecordIndex())
	}
}

func TestOpenFileJournalRejectsMismatch(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "progress.json")

	j, _, err := OpenFileJournal(journalPath, JournalMeta{Source: "a.zip", Collection: "mem", EmbeddingModel: "m1"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := j.Commit(Checkpoint{LastRecordIndex: 3, TotalRecords: 10}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if _, _, err := OpenFileJournal(journalPath, JournalMeta{Source: "b.zip", Collection: "mem", EmbeddingModel: "m1"}); err == nil {
		t.Fatalf("expected mismatch error for different source")
	}
}
