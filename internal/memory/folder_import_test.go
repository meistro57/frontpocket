package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFolderWithTextAndJsonl(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a text note
	notePath := filepath.Join(tempDir, "research_notes.txt")
	if err := os.WriteFile(notePath, []byte("Metastable geometric resonance in E8 lattice possibility space."), 0o644); err != nil {
		t.Fatalf("failed to write text note: %v", err)
	}

	// 2. Create a JSONL file
	jsonlPath := filepath.Join(tempDir, "sample.jsonl")
	jsonlContent := `{"conversation_id":"test_conv","speaker":"user","text":"Testing folder import"}`
	if err := os.WriteFile(jsonlPath, []byte(jsonlContent), 0o644); err != nil {
		t.Fatalf("failed to write jsonl: %v", err)
	}

	opts := FolderImportOptions{
		Project:    "TestProject",
		CacheDir:   filepath.Join(tempDir, ".cache"),
		NoAudio:    true,
		NoVision:   true,
		AIProvider: "test",
	}

	res, err := ParseFolder(context.Background(), tempDir, opts)
	if err != nil {
		t.Fatalf("ParseFolder failed: %v", err)
	}

	if res.FilesProcessed != 2 {
		t.Errorf("expected 2 files processed, got %d", res.FilesProcessed)
	}

	if len(res.Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(res.Records))
	}
}
