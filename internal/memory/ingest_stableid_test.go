package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/embed"
)

type stableIDTestEmbedder struct{}

func (stableIDTestEmbedder) EmbedText(_ context.Context, text string) ([]float32, error) {
	return []float32{float32(len(text))}, nil
}

func (stableIDTestEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		out = append(out, []float32{float32(len(t))})
	}
	return out, nil
}

func (stableIDTestEmbedder) ProviderName() string { return "test" }
func (stableIDTestEmbedder) ModelName() string    { return "test" }

var _ embed.Embedder = stableIDTestEmbedder{}

type fixedChunker struct{ chunks []string }

func (c fixedChunker) ChunkText(_ string) []string {
	out := make([]string, 0, len(c.chunks))
	out = append(out, c.chunks...)
	return out
}

func newStableIDIngestor(store MemoryStore, chunker Chunker) Ingestor {
	return Ingestor{
		Chunker:      chunker,
		Embedder:     stableIDTestEmbedder{},
		Store:        store,
		SpeakerRules: SpeakerRules{StoreAssistant: true, StoreUser: true, StoreSystem: true},
		LegacyMode:   "mixed",
	}
}

func TestBuildMemoryIDStableWithMessageID(t *testing.T) {
	first := buildMemoryID("conv-a", "msg-1", 0, 0)
	second := buildMemoryID("conv-a", "msg-1", 17, 0)
	if first != second {
		t.Fatalf("expected stable message-based id, got %q vs %q", first, second)
	}
}

func TestBuildMemoryIDLegacyFallback(t *testing.T) {
	got := buildMemoryID("conv-a", "", 4, 1)
	want := "conv-a_turn_005_chunk_002"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestIngestSkipsExistingOnReingest(t *testing.T) {
	store := NewInMemoryStore()
	ing := newStableIDIngestor(store, Chunker{Size: 8, Overlap: 0, MinSize: 1})
	recs := []MessageRecord{{ConversationID: "conv-a", MessageID: "msg-1", Timestamp: time.Now().UTC().Format(time.RFC3339), Speaker: "user", Text: "hello world"}}

	first, err := ing.Ingest(context.Background(), recs)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if len(first.Points) == 0 {
		t.Fatalf("expected points on first ingest")
	}

	second, err := ing.Ingest(context.Background(), recs)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if len(second.Points) != 0 {
		t.Fatalf("expected zero new points on reingest, got %d", len(second.Points))
	}
	if second.RecordsSkippedExisting != 1 {
		t.Fatalf("expected 1 skipped record, got %d", second.RecordsSkippedExisting)
	}
}

func TestIngestPartialChunksOnlyMissingUpserted(t *testing.T) {
	store := NewInMemoryStore()
	chunker := Chunker{Size: 4, Overlap: 0, MinSize: 1}
	ing := newStableIDIngestor(store, chunker)

	rec := MessageRecord{ConversationID: "conv-a", MessageID: "msg-1", Timestamp: time.Now().UTC().Format(time.RFC3339), Speaker: "user", Text: "abcdefgh"}
	chunks := chunker.ChunkText(rec.Text)
	if len(chunks) < 2 {
		t.Fatalf("expected at least two chunks")
	}

	pre := MemoryPoint{
		MemoryID:            buildMemoryID(rec.ConversationID, rec.MessageID, 0, 0),
		ConversationID:      rec.ConversationID,
		MessageID:           rec.MessageID,
		SourceType:          "chat_export",
		SourceTitle:         "test",
		Timestamp:           time.Now().UTC(),
		Speaker:             "user",
		MemoryKind:          KindChatTurn,
		Text:                chunks[0],
		SourceQuote:         chunks[0],
		Summary:             chunks[0],
		EmbeddingProvider:   "test",
		EmbeddingModel:      "test",
		EmbeddingDimensions: 1,
		Vector:              []float32{1},
	}
	if err := store.Upsert(context.Background(), []MemoryPoint{pre}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := ing.Ingest(context.Background(), []MessageRecord{rec})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.ChunksSkippedExisting != 1 {
		t.Fatalf("expected one skipped existing chunk, got %d", result.ChunksSkippedExisting)
	}
	if len(result.Points) != len(chunks)-1 {
		t.Fatalf("expected %d new chunks, got %d", len(chunks)-1, len(result.Points))
	}
}

func TestLegacySkipConversation(t *testing.T) {
	store := NewInMemoryStore()
	old := MemoryPoint{
		MemoryID:            "conv-legacy_turn_001_chunk_001",
		ConversationID:      "conv-legacy",
		SourceType:          "chat_export",
		SourceTitle:         "legacy",
		Timestamp:           time.Now().UTC(),
		Speaker:             "user",
		MemoryKind:          KindChatTurn,
		Text:                "legacy",
		SourceQuote:         "legacy",
		Summary:             "legacy",
		EmbeddingProvider:   "test",
		EmbeddingModel:      "test",
		EmbeddingDimensions: 1,
		Vector:              []float32{1},
	}
	if err := store.Upsert(context.Background(), []MemoryPoint{old}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ing := newStableIDIngestor(store, Chunker{Size: 8, Overlap: 0, MinSize: 1})
	ing.LegacyMode = "skip"
	rec := MessageRecord{ConversationID: "conv-legacy", MessageID: "msg-1", Timestamp: time.Now().UTC().Format(time.RFC3339), Speaker: "user", Text: "new text"}

	result, err := ing.Ingest(context.Background(), []MessageRecord{rec})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.ConversationsLegacySkipped != 1 {
		t.Fatalf("expected one legacy skipped conversation, got %d", result.ConversationsLegacySkipped)
	}
	if len(result.Points) != 0 {
		t.Fatalf("expected no inserted points, got %d", len(result.Points))
	}
}

func TestLegacyGuardIgnoredForRecordsWithoutMessageID(t *testing.T) {
	store := NewInMemoryStore()
	seed := MemoryPoint{
		MemoryID:            "conv-legacy_turn_042_chunk_001",
		ConversationID:      "conv-legacy",
		SourceType:          "chat_export",
		SourceTitle:         "legacy",
		Timestamp:           time.Now().UTC(),
		Speaker:             "user",
		MemoryKind:          KindChatTurn,
		Text:                "legacy",
		SourceQuote:         "legacy",
		Summary:             "legacy",
		EmbeddingProvider:   "test",
		EmbeddingModel:      "test",
		EmbeddingDimensions: 1,
		Vector:              []float32{1},
	}
	if err := store.Upsert(context.Background(), []MemoryPoint{seed}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ing := newStableIDIngestor(store, Chunker{Size: 256, Overlap: 0, MinSize: 1})
	ing.LegacyMode = "skip"
	// API/JSONL records carry no MessageID: they must ingest even into a
	// conversation that already holds legacy-shaped points (existence-check
	// dedupe only). Regression guard for the stats_test by_speaker bug.
	recs := []MessageRecord{
		{ConversationID: "conv-legacy", Timestamp: time.Now().UTC().Format(time.RFC3339), Speaker: "user", Text: "first text"},
		{ConversationID: "conv-legacy", Timestamp: time.Now().UTC().Format(time.RFC3339), Speaker: "assistant", Text: "second text"},
	}

	result, err := ing.Ingest(context.Background(), recs)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.ConversationsLegacySkipped != 0 {
		t.Fatalf("expected zero legacy skips for MessageID-less records, got %d", result.ConversationsLegacySkipped)
	}
	if len(result.Points) != 2 {
		t.Fatalf("expected both records inserted, got %d points", len(result.Points))
	}
}

func TestLegacyRewriteReplacesConversationAndProducesMap(t *testing.T) {
	store := NewInMemoryStore()
	seed := []MemoryPoint{
		{
			MemoryID:            "conv-r_turn_001_chunk_001",
			ConversationID:      "conv-r",
			SourceType:          "chat_export",
			SourceTitle:         "legacy",
			Timestamp:           time.Now().UTC(),
			Speaker:             "user",
			MemoryKind:          KindChatTurn,
			Text:                "hello",
			SourceQuote:         "hello",
			Summary:             "hello",
			EmbeddingProvider:   "test",
			EmbeddingModel:      "test",
			EmbeddingDimensions: 1,
			Vector:              []float32{1},
		},
		{
			MemoryID:            "conv-r_turn_002_chunk_001",
			ConversationID:      "conv-r",
			SourceType:          "chat_export",
			SourceTitle:         "legacy",
			Timestamp:           time.Now().UTC(),
			Speaker:             "assistant",
			MemoryKind:          KindChatTurn,
			Text:                "world",
			SourceQuote:         "world",
			Summary:             "world",
			EmbeddingProvider:   "test",
			EmbeddingModel:      "test",
			EmbeddingDimensions: 1,
			Vector:              []float32{1},
		},
	}
	if err := store.Upsert(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ing := newStableIDIngestor(store, Chunker{Size: 32, Overlap: 0, MinSize: 1})
	ing.LegacyMode = "rewrite"
	recs := []MessageRecord{
		{ConversationID: "conv-r", MessageID: "m1", Timestamp: time.Now().UTC().Format(time.RFC3339), Speaker: "user", Text: "hello"},
		{ConversationID: "conv-r", MessageID: "m2", Timestamp: time.Now().UTC().Format(time.RFC3339), Speaker: "assistant", Text: "world"},
	}

	result, err := ing.Ingest(context.Background(), recs)
	if err != nil {
		t.Fatalf("rewrite ingest: %v", err)
	}
	if len(result.Points) != 2 {
		t.Fatalf("expected 2 inserted replacement points, got %d", len(result.Points))
	}
	if len(result.RewriteMappings) != 2 {
		t.Fatalf("expected 2 mapping rows, got %d", len(result.RewriteMappings))
	}
	for _, row := range result.RewriteMappings {
		if row.OldMemoryID == "" || row.NewMemoryID == "" {
			t.Fatalf("expected populated mapping row, got %+v", row)
		}
		if !strings.Contains(row.NewMemoryID, "_msg_") {
			t.Fatalf("expected new id to be msg-shaped, got %q", row.NewMemoryID)
		}
	}

	all, _, err := store.ScrollRaw(context.Background(), 100, "", SearchFilters{ConversationID: "conv-r"}, time.Time{}, time.Time{}, true)
	if err != nil {
		t.Fatalf("scroll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected conversation count preserved at 2, got %d", len(all))
	}
	for _, p := range all {
		if !strings.Contains(p.MemoryID, "_msg_") {
			t.Fatalf("legacy id survived rewrite: %q", p.MemoryID)
		}
	}
}
