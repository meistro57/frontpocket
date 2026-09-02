package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPDFToMessageRecords(t *testing.T) {
	pdfPath := "../../incoming/Architecting_the_Shift.pdf"
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Skip("incoming PDF not found, skipping")
	}

	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")

	mock := &mockCaptioner{
		reply: "A presentation slide showing sacred geometry and systems architecture of the shift.",
	}

	opts := FolderImportOptions{
		Project:         "UTMGR",
		CacheDir:        cacheDir,
		VisionCaptioner: mock,
		NoVision:        false,
		AIProvider:      "test",
	}

	records, err := PDFToMessageRecords(context.Background(), pdfPath, opts)
	if err != nil {
		t.Fatalf("PDFToMessageRecords failed: %v", err)
	}

	if len(records) != 14 {
		t.Fatalf("expected 14 slide records (14 pages), got %d", len(records))
	}

	first := records[0]
	if first.SourceType != "presentation" {
		t.Errorf("expected presentation, got %s", first.SourceType)
	}
	if first.Speaker != "presentation" {
		t.Errorf("expected presentation speaker, got %s", first.Speaker)
	}
	if first.Tags[1] != "slide" {
		t.Errorf("expected tag slide, got %s", first.Tags[1])
	}
	if mock.lastPrompt != SlideVisionPrompt {
		t.Errorf("expected SlideVisionPrompt, got %s", mock.lastPrompt)
	}
}
