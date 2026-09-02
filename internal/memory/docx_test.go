package memory

import (
	"os"
	"testing"
)

func TestParseDocxFile(t *testing.T) {
	docxPath := "../../incoming/Master Synthesis Report_ The Unified Theory of Metastable Geometric Resonance (UTMGR) and the New Earth Architecture.docx"
	if _, err := os.Stat(docxPath); os.IsNotExist(err) {
		t.Skip("incoming docx file not found, skipping integration test")
	}

	records, err := DocxToMessageRecords(docxPath, "UTMGR", "test")
	if err != nil {
		t.Fatalf("DocxToMessageRecords failed: %v", err)
	}

	if len(records) == 0 {
		t.Fatalf("expected records, got 0")
	}

	t.Logf("Generated %d records from docx", len(records))
	t.Logf("First record heading/sample: %s", records[0].Text[:min(len(records[0].Text), 150)])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
