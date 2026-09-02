package memory

import (
	"context"
	"os"
	"testing"
)

type mockCaptioner struct {
	lastPrompt string
	reply      string
}

func (m *mockCaptioner) CaptionImage(ctx context.Context, attachment ResolvedAttachment) (string, error) {
	return m.reply, nil
}

func (m *mockCaptioner) CaptionImageWithPrompt(ctx context.Context, attachment ResolvedAttachment, prompt string) (string, error) {
	m.lastPrompt = prompt
	return m.reply, nil
}

func TestImageToMessageRecords(t *testing.T) {
	imgPath := "../../incoming/Love_Archetypes_Geometric_Diagram.png"
	if _, err := os.Stat(imgPath); os.IsNotExist(err) {
		t.Skip("test image not found, skipping")
	}

	mock := &mockCaptioner{
		reply: "A sacred geometry diagram showing 9 love archetypes arranged symmetrically.",
	}

	opts := FolderImportOptions{
		Project:         "UTMGR",
		VisionCaptioner: mock,
		NoVision:        false,
		AIProvider:      "test",
	}

	records, err := ImageToMessageRecords(context.Background(), imgPath, opts)
	if err != nil {
		t.Fatalf("ImageToMessageRecords failed: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	rec := records[0]
	if rec.SourceType != "diagram" {
		t.Errorf("expected source_type diagram, got %s", rec.SourceType)
	}
	if rec.Speaker != "diagram" {
		t.Errorf("expected speaker diagram, got %s", rec.Speaker)
	}
	if rec.MemoryKind != KindCreativeArtifact {
		t.Errorf("expected kind creative_artifact, got %s", rec.MemoryKind)
	}
	if mock.lastPrompt != DiagramVisionPrompt {
		t.Errorf("expected DiagramVisionPrompt, got %s", mock.lastPrompt)
	}
}
