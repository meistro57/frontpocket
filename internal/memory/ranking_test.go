package memory

import (
	"testing"
	"time"
)

func rankingFixture() []SearchResult {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	return []SearchResult{
		{
			MemoryID:       "m_semantic",
			ConversationID: "conv_a",
			Timestamp:      now,
			SourceTitle:    "Architecture chat",
			Text:           "The retrieval layer keeps embeddings in a vector database for semantic recall.",
			Score:          0.91,
		},
		{
			MemoryID:       "m_exact",
			ConversationID: "conv_b",
			Timestamp:      now.Add(-time.Hour),
			SourceTitle:    "Debugging chat",
			Text:           "Run `frontpocket ingest chatgpt --conversation 42` to reimport that thread.",
			Score:          0.72,
		},
		{
			MemoryID:       "m_unrelated",
			ConversationID: "conv_c",
			Timestamp:      now.Add(-2 * time.Hour),
			SourceTitle:    "Groceries",
			Text:           "Buy oat milk and coffee beans.",
			Score:          0.55,
		},
	}
}

func TestLexicalScoreRewardsExactTerms(t *testing.T) {
	results := rankingFixture()
	query := "frontpocket ingest chatgpt"

	if score := LexicalScore(query, results[1]); score < 0.99 {
		t.Fatalf("expected full lexical coverage for the exact command, got %f", score)
	}
	if score := LexicalScore(query, results[0]); score != 0 {
		t.Fatalf("expected no lexical overlap for the semantic-only result, got %f", score)
	}
}

func TestFuseHybridResultsPromotesExactTermMatch(t *testing.T) {
	results := rankingFixture()
	query := "frontpocket ingest chatgpt"

	denseOnly := append([]SearchResult(nil), results...)
	if denseOnly[0].MemoryID != "m_semantic" {
		t.Fatalf("expected dense ordering to put the semantic result first, got %q", denseOnly[0].MemoryID)
	}

	hybrid := FuseHybridResults(query, results, 3)
	if hybrid[0].MemoryID != "m_exact" {
		t.Fatalf("expected hybrid ranking to surface the exact-term match, got %q", hybrid[0].MemoryID)
	}
	if hybrid[len(hybrid)-1].MemoryID != "m_unrelated" {
		t.Fatalf("expected the unrelated result to rank last, got %q", hybrid[len(hybrid)-1].MemoryID)
	}
	for _, result := range hybrid {
		if result.Score > 1.0 || result.Score < 0 {
			t.Fatalf("expected normalised scores in [0,1], got %f for %q", result.Score, result.MemoryID)
		}
	}
}

func TestFuseHybridResultsKeepsDenseOrderWithoutLexicalOverlap(t *testing.T) {
	results := rankingFixture()
	hybrid := FuseHybridResults("quantum chromodynamics", results, 3)
	if hybrid[0].MemoryID != "m_semantic" {
		t.Fatalf("expected dense ordering to survive when nothing matches lexically, got %q", hybrid[0].MemoryID)
	}
}

func TestDedupeResultsCollapsesRepeatedText(t *testing.T) {
	now := time.Now().UTC()
	results := []SearchResult{
		{MemoryID: "a", ConversationID: "conv_1", Text: "FrontPocket is  local-first.", Score: 0.9, Timestamp: now},
		{MemoryID: "a_copy", ConversationID: "conv_1", Text: "frontpocket is local-first.", Score: 0.8, Timestamp: now},
		{MemoryID: "b", ConversationID: "conv_2", Text: "Redis caches session state.", Score: 0.7, Timestamp: now},
	}

	deduped := DedupeResults(results, 0)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 results after dedupe, got %d", len(deduped))
	}
	if deduped[0].MemoryID != "a" {
		t.Fatalf("expected highest-scoring representative to win, got %q", deduped[0].MemoryID)
	}
	if len(deduped[0].DuplicateIDs) != 1 || deduped[0].DuplicateIDs[0] != "a_copy" {
		t.Fatalf("expected collapsed id to be recorded, got %#v", deduped[0].DuplicateIDs)
	}
	if deduped[0].DuplicateCount != 1 {
		t.Fatalf("expected duplicate count 1, got %d", deduped[0].DuplicateCount)
	}
}

func TestDedupeResultsKeepsSameTextFromDifferentConversations(t *testing.T) {
	now := time.Now().UTC()
	results := []SearchResult{
		{MemoryID: "a", ConversationID: "conv_1", Text: "thanks", Score: 0.9, Timestamp: now},
		{MemoryID: "b", ConversationID: "conv_2", Text: "thanks", Score: 0.8, Timestamp: now},
	}

	if deduped := DedupeResults(results, 0); len(deduped) != 2 {
		t.Fatalf("expected both conversations to survive, got %d results", len(deduped))
	}
}

func TestDedupeResultsCapsPerConversation(t *testing.T) {
	now := time.Now().UTC()
	results := []SearchResult{
		{MemoryID: "a1", ConversationID: "conv_1", Text: "one", Score: 0.9, Timestamp: now},
		{MemoryID: "a2", ConversationID: "conv_1", Text: "two", Score: 0.85, Timestamp: now},
		{MemoryID: "a3", ConversationID: "conv_1", Text: "three", Score: 0.8, Timestamp: now},
		{MemoryID: "b1", ConversationID: "conv_2", Text: "four", Score: 0.7, Timestamp: now},
	}

	capped := DedupeResults(results, 2)
	if len(capped) != 3 {
		t.Fatalf("expected 3 results after capping, got %d", len(capped))
	}
	conversationCounts := map[string]int{}
	for _, result := range capped {
		conversationCounts[result.ConversationID]++
	}
	if conversationCounts["conv_1"] != 2 {
		t.Fatalf("expected conv_1 capped at 2, got %d", conversationCounts["conv_1"])
	}
}

func TestApplyMinScoreDropsLowScores(t *testing.T) {
	results := []SearchResult{
		{MemoryID: "high", Score: 0.8},
		{MemoryID: "low", Score: 0.2},
	}
	filtered := ApplyMinScore(results, 0.5)
	if len(filtered) != 1 || filtered[0].MemoryID != "high" {
		t.Fatalf("expected only the high score to survive, got %#v", filtered)
	}
	if len(ApplyMinScore(results, 0)) != 2 {
		t.Fatal("expected a zero floor to keep every result")
	}
}
