package tests

import (
	"strings"
	"testing"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestChunkTextWithOverlap(t *testing.T) {
	chunker := memory.Chunker{Size: 10, Overlap: 2, MinSize: 3}
	text := "abcdefghijklmnopqrstuvwxyz"

	chunks := chunker.ChunkText(text)
	if len(chunks) < 3 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	if chunks[0] != "abcdefghij" {
		t.Fatalf("unexpected first chunk: %q", chunks[0])
	}

	if !strings.Contains(chunks[1], "ijklmnop") {
		t.Fatalf("expected overlap in second chunk, got %q", chunks[1])
	}
}

func TestChunkTextEmptyInput(t *testing.T) {
	chunker := memory.Chunker{Size: 10, Overlap: 2, MinSize: 3}
	chunks := chunker.ChunkText("   ")
	if len(chunks) != 0 {
		t.Fatalf("expected no chunks, got %d", len(chunks))
	}
}

func TestChunkTextPrefersSentenceBoundary(t *testing.T) {
	chunker := memory.Chunker{Size: 40, Overlap: 5, MinSize: 3}
	text := "This is the first sentence. This is the second sentence. This is the third one."

	chunks := chunker.ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	for i, chunk := range chunks[:len(chunks)-1] {
		last := chunk[len(chunk)-1]
		if last != '.' {
			t.Fatalf("chunk %d does not end on a sentence boundary: %q", i, chunk)
		}
	}
}

func TestChunkTextFallsBackToHardCutWithoutBoundaries(t *testing.T) {
	// No spaces or punctuation at all (e.g. an id or unbroken token) should
	// still chunk deterministically via hard character cuts.
	chunker := memory.Chunker{Size: 10, Overlap: 2, MinSize: 3}
	text := strings.Repeat("x", 35)

	chunks := chunker.ChunkText(text)
	if len(chunks) == 0 {
		t.Fatalf("expected chunks for unbroken text, got none")
	}
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk) == "" {
			t.Fatalf("got an empty chunk in fallback mode")
		}
	}
}
