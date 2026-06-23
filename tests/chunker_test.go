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
