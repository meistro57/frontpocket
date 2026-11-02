package memory

import (
	"context"
	"testing"
	"time"
)

func packFixture() []SearchResult {
	base := time.Date(2026, 6, 22, 1, 0, 0, 0, time.UTC)
	return []SearchResult{
		{
			MemoryID:       "newer",
			ConversationID: "conv_1",
			Timestamp:      base.Add(2 * time.Hour),
			SourceTitle:    "Planning",
			SourceQuote:    "we should ship the retrieval layer first",
			Text:           "we should ship the retrieval layer first, then polish",
			Score:          0.9,
		},
		{
			MemoryID:       "older",
			ConversationID: "conv_1",
			Timestamp:      base,
			SourceTitle:    "Kickoff",
			SourceQuote:    "start with source-backed recall",
			Text:           "start with source-backed recall before adding automation",
			Score:          0.8,
		},
	}
}

func TestAssembleContextPackKeepsAllWithinBudget(t *testing.T) {
	resp := AssembleContextPack("what did we decide", packFixture(), ContextPackOptions{MaxCharacters: 10000})

	if len(resp.MemoryPack) != 2 {
		t.Fatalf("expected both memories, got %d", len(resp.MemoryPack))
	}
	if len(resp.UsedMemoryIDs) != 2 {
		t.Fatalf("expected two used memory ids, got %#v", resp.UsedMemoryIDs)
	}
	if resp.Budget == nil || resp.Budget.Dropped != 0 {
		t.Fatalf("expected no drops, got %#v", resp.Budget)
	}
	if resp.Budget.CharactersUsed <= 0 || resp.Budget.CharactersUsed > 10000 {
		t.Fatalf("unexpected characters used: %d", resp.Budget.CharactersUsed)
	}
}

func TestAssembleContextPackDropsOverBudgetEntries(t *testing.T) {
	fixture := packFixture()
	tightBudget := packItemCharacters(fixture[0]) + 1

	resp := AssembleContextPack("what did we decide", fixture, ContextPackOptions{MaxCharacters: tightBudget})

	if len(resp.MemoryPack) != 1 {
		t.Fatalf("expected one memory to fit, got %d", len(resp.MemoryPack))
	}
	if resp.MemoryPack[0].MemoryID != "newer" {
		t.Fatalf("expected the top-scored memory to be kept, got %q", resp.MemoryPack[0].MemoryID)
	}
	if resp.Budget.Dropped != 1 {
		t.Fatalf("expected one dropped entry, got %d", resp.Budget.Dropped)
	}
}

func TestAssembleContextPackQuoteOnlyStripsFullText(t *testing.T) {
	resp := AssembleContextPack("decide", packFixture(), ContextPackOptions{QuoteOnly: true})

	for _, result := range resp.MemoryPack {
		if result.Text != "" {
			t.Fatalf("expected text to be stripped, got %q", result.Text)
		}
		if result.Summary != "" {
			t.Fatalf("expected summary to be stripped, got %q", result.Summary)
		}
		if result.SourceQuote == "" {
			t.Fatal("expected the source quote to survive")
		}
	}
}

func TestAssembleContextPackChronologicalOrder(t *testing.T) {
	resp := AssembleContextPack("decide", packFixture(), ContextPackOptions{Chronological: true})

	if resp.MemoryPack[0].MemoryID != "older" {
		t.Fatalf("expected oldest memory first, got %q", resp.MemoryPack[0].MemoryID)
	}
}

func TestContextPackerBuildAppliesBudgetAndSearchOptions(t *testing.T) {
	store := NewInMemoryStore()
	now := time.Now().UTC()
	if err := store.Upsert(context.Background(), []MemoryPoint{
		{
			MemoryID: "pack_1", ConversationID: "conv_1", SourceTitle: "Packing", SourceType: "chat_export",
			Timestamp: now, Speaker: "user", MemoryKind: KindProjectContext,
			Text: "the context pack keeps a character budget", SourceQuote: "the context pack keeps a character budget",
		},
	}); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	packer := ContextPacker{Store: store}
	resp, err := packer.Build(context.Background(), ContextPackRequest{
		Query:         "context pack budget",
		Limit:         5,
		MaxCharacters: 5000,
		QuoteOnly:     true,
	})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if len(resp.MemoryPack) != 1 {
		t.Fatalf("expected one packed memory, got %d", len(resp.MemoryPack))
	}
	if resp.MemoryPack[0].Text != "" {
		t.Fatal("expected quote-only packing to strip text")
	}
	if resp.Budget == nil || resp.Budget.CharactersUsed == 0 {
		t.Fatalf("expected budget accounting, got %#v", resp.Budget)
	}
}
