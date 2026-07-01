package tests

import (
	"strings"
	"testing"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestConversationChunkerPairsAdjacentTurns(t *testing.T) {
	chunker := memory.ConversationChunker{
		TextChunker: memory.Chunker{Size: 900, Overlap: 150, MinSize: 120},
		WindowTurns: 2,
	}

	turns := []memory.ConversationTurn{
		{Speaker: "user", Text: "is there a resource monitor for node red?"},
		{Speaker: "assistant", Text: "Yes — the node-red-dashboard palette has a system usage widget you can drop in."},
	}

	windows := chunker.ChunkConversation(turns)
	if len(windows) != 1 {
		t.Fatalf("expected a single paired window, got %d", len(windows))
	}

	w := windows[0]
	if !strings.Contains(w.Text, "USER: is there a resource monitor") {
		t.Fatalf("expected user turn in window text, got %q", w.Text)
	}
	if !strings.Contains(w.Text, "ASSISTANT: Yes") {
		t.Fatalf("expected assistant turn in window text, got %q", w.Text)
	}
	if len(w.Speakers) != 2 {
		t.Fatalf("expected 2 distinct speakers, got %d: %v", len(w.Speakers), w.Speakers)
	}
	if len(w.TurnIndices) != 2 || w.TurnIndices[0] != 0 || w.TurnIndices[1] != 1 {
		t.Fatalf("expected turn indices [0 1], got %v", w.TurnIndices)
	}
}

func TestConversationChunkerOverlapCarriesContext(t *testing.T) {
	chunker := memory.ConversationChunker{
		TextChunker:        memory.Chunker{Size: 900, Overlap: 150, MinSize: 120},
		WindowTurns:        2,
		WindowOverlapTurns: 1,
	}

	turns := []memory.ConversationTurn{
		{Speaker: "user", Text: "turn one"},
		{Speaker: "assistant", Text: "turn two"},
		{Speaker: "user", Text: "turn three"},
	}

	windows := chunker.ChunkConversation(turns)
	if len(windows) < 2 {
		t.Fatalf("expected overlapping windows, got %d", len(windows))
	}

	// With overlap=1, window 2 should start at turn index 1 (shared with window 1).
	if windows[1].TurnIndices[0] != 1 {
		t.Fatalf("expected second window to start at turn 1 for overlap, got %v", windows[1].TurnIndices)
	}
}

func TestConversationChunkerSkipsEmptyTurns(t *testing.T) {
	chunker := memory.ConversationChunker{
		TextChunker: memory.Chunker{Size: 900, Overlap: 150, MinSize: 120},
		WindowTurns: 1,
	}

	turns := []memory.ConversationTurn{
		{Speaker: "system", Text: "   "},
		{Speaker: "user", Text: "real content here"},
	}

	windows := chunker.ChunkConversation(turns)
	if len(windows) != 1 {
		t.Fatalf("expected empty turn to be skipped, got %d windows", len(windows))
	}
	if !strings.Contains(windows[0].Text, "real content here") {
		t.Fatalf("expected surviving turn's text, got %q", windows[0].Text)
	}
}

func TestConversationChunkerEmptyInput(t *testing.T) {
	chunker := memory.ConversationChunker{
		TextChunker: memory.Chunker{Size: 900, Overlap: 150, MinSize: 120},
		WindowTurns: 2,
	}

	windows := chunker.ChunkConversation(nil)
	if len(windows) != 0 {
		t.Fatalf("expected no windows for empty input, got %d", len(windows))
	}
}
