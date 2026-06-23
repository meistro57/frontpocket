package tests

import (
	"context"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestInMemoryStoreSearchFilters(t *testing.T) {
	store := memory.NewInMemoryStore()
	now := time.Now().UTC()
	points := []memory.MemoryPoint{
		{
			MemoryID:       "m1",
			ConversationID: "conv1",
			SourceTitle:    "First",
			SourceType:     "chat_export",
			Timestamp:      now,
			Speaker:        "user",
			Project:        "FrontPocket",
			MemoryKind:     memory.KindProjectContext,
			Text:           "FrontPocket uses Go and Qdrant for memory retrieval.",
			Summary:        "FrontPocket stack",
		},
		{
			MemoryID:       "m2",
			ConversationID: "conv2",
			SourceTitle:    "Second",
			SourceType:     "chat_export",
			Timestamp:      now.Add(-time.Hour),
			Speaker:        "assistant",
			Project:        "Other",
			MemoryKind:     memory.KindFact,
			Text:           "Another project using Python.",
			Summary:        "Other stack",
		},
	}

	if err := store.Upsert(context.Background(), points); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	results, err := store.Search(context.Background(), memory.SearchRequest{
		Query: "go qdrant",
		Limit: 5,
		Filters: memory.SearchFilters{
			Project: "FrontPocket",
		},
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].MemoryID != "m1" {
		t.Fatalf("expected m1, got %s", results[0].MemoryID)
	}
}
