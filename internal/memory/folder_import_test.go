package memory

import (
	"context"
	"os"
	"path/filepath"
	"sort"
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

func TestFolderConversationIDFromPathDistinctSameStem(t *testing.T) {
	root := t.TempDir()

	first := filepath.Join(root, "projects", "steel fab shop", "index.md")
	second := filepath.Join(root, "notes", "steel fab shop", "index.md")

	if err := os.MkdirAll(filepath.Dir(first), 0o755); err != nil {
		t.Fatalf("mkdir first: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatalf("mkdir second: %v", err)
	}
	if err := os.WriteFile(first, []byte("first"), 0o644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatalf("write second: %v", err)
	}

	cidA := folderConversationIDFromPath(root, first)
	cidB := folderConversationIDFromPath(root, second)
	if cidA == cidB {
		t.Fatalf("expected distinct conversation IDs, got %q", cidA)
	}

	idA := buildMemoryID(cidA, "", 0, 0)
	idB := buildMemoryID(cidB, "", 0, 0)
	if idA == idB {
		t.Fatalf("expected distinct fallback memory IDs, got %q", idA)
	}
}

func TestFolderConversationIDFromPathStableAcrossWalks(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "projects", "steel fab shop", "index.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	first := folderConversationIDFromPath(root, filePath)
	second := folderConversationIDFromPath(root, filePath)
	if first != second {
		t.Fatalf("expected stable conversation ID, got %q vs %q", first, second)
	}

	firstID := buildMemoryID(first, "", 0, 0)
	secondID := buildMemoryID(second, "", 0, 0)
	if firstID != secondID {
		t.Fatalf("expected stable fallback memory ID, got %q vs %q", firstID, secondID)
	}
}

func TestParseFolderRecursiveSameStemUniqueConversationIDs(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "projects", "steel fab shop", "index.md"),
		filepath.Join(root, "projects", "machine shop", "index.md"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	res, err := ParseFolder(context.Background(), root, FolderImportOptions{
		Project:    "TestProject",
		CacheDir:   filepath.Join(root, ".cache"),
		NoAudio:    true,
		NoVision:   true,
		AIProvider: "test",
	})
	if err != nil {
		t.Fatalf("ParseFolder failed: %v", err)
	}
	if len(res.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(res.Records))
	}

	ids := []string{res.Records[0].ConversationID, res.Records[1].ConversationID}
	sort.Strings(ids)
	if ids[0] == ids[1] {
		t.Fatalf("expected unique conversation IDs, got %q", ids[0])
	}
	if ids[0] != "projects_machine_shop_index" || ids[1] != "projects_steel_fab_shop_index" {
		t.Fatalf("unexpected conversation IDs: %v", ids)
	}
}
