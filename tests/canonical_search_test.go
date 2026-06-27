package tests

import (
	"context"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestCanonicalMemoriesRankAboveRaw(t *testing.T) {
	store := memory.NewInMemoryStore()
	now := time.Now().UTC()
	points := []memory.MemoryPoint{
		{
			MemoryID:    "raw_1",
			Timestamp:   now.Add(-time.Hour),
			Speaker:     "user",
			SourceType:  "chat_export",
			SourceTitle: "Raw",
			MemoryKind:  memory.KindPreference,
			Text:        "Mark prefers concise responses.",
			Summary:     "Mark prefers concise responses.",
		},
		{
			MemoryID:    "canon_1",
			Timestamp:   now,
			Speaker:     "user",
			SourceType:  "canonical_review",
			SourceTitle: "Canon",
			MemoryKind:  memory.KindPreference,
			Text:        "Mark prefers concise responses.",
			Summary:     "Mark prefers concise responses.",
			Canonical:   true,
			Status:      memory.StatusApprovedByUser,
		},
	}
	if err := store.Upsert(context.Background(), points); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	results, err := store.Search(context.Background(), memory.SearchRequest{Query: "concise responses", Limit: 5})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least two results, got %#v", results)
	}
	if results[0].MemoryID != "canon_1" {
		t.Fatalf("expected canonical memory first, got %q", results[0].MemoryID)
	}
}

func TestRejectedContradictedOutdatedExcludedByDefault(t *testing.T) {
	store := memory.NewInMemoryStore()
	now := time.Now().UTC()
	points := []memory.MemoryPoint{
		{MemoryID: "ok", Timestamp: now, Speaker: "user", SourceType: "chat_export", SourceTitle: "OK", MemoryKind: memory.KindFact, Text: "project uses go", Summary: "project uses go", Status: memory.StatusApprovedByUser},
		{MemoryID: "rej", Timestamp: now, Speaker: "user", SourceType: "chat_export", SourceTitle: "Rejected", MemoryKind: memory.KindFact, Text: "project uses go", Summary: "project uses go", Status: memory.StatusRejected},
		{MemoryID: "con", Timestamp: now, Speaker: "user", SourceType: "chat_export", SourceTitle: "Contradicted", MemoryKind: memory.KindFact, Text: "project uses go", Summary: "project uses go", Status: memory.StatusContradicted},
		{MemoryID: "old", Timestamp: now, Speaker: "user", SourceType: "chat_export", SourceTitle: "Outdated", MemoryKind: memory.KindFact, Text: "project uses go", Summary: "project uses go", Status: memory.StatusOutdated},
	}
	if err := store.Upsert(context.Background(), points); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	results, err := store.Search(context.Background(), memory.SearchRequest{Query: "project uses go", Limit: 10})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 || results[0].MemoryID != "ok" {
		t.Fatalf("expected only non-excluded result, got %#v", results)
	}

	includeRejected, err := store.Search(context.Background(), memory.SearchRequest{Query: "project uses go", Limit: 10, IncludeRejected: true})
	if err != nil {
		t.Fatalf("search with include_rejected failed: %v", err)
	}
	if len(includeRejected) < 4 {
		t.Fatalf("expected rejected/contradicted/outdated included, got %#v", includeRejected)
	}
}
