package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestParseJSONL(t *testing.T) {
	jsonl := "{\"conversation_id\":\"c1\",\"timestamp\":\"2026-06-22T11:13:00Z\",\"speaker\":\"user\",\"text\":\"hello\"}\n" +
		"{\"conversation_id\":\"c1\",\"timestamp\":\"2026-06-22T11:14:00Z\",\"speaker\":\"assistant\",\"text\":\"hi\"}\n"

	records, err := memory.ParseJSONL(jsonl)
	if err != nil {
		t.Fatalf("expected valid JSONL, got %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].ConversationID != "c1" {
		t.Fatalf("unexpected conversation id: %q", records[0].ConversationID)
	}
}

func TestParseJSONLRejectsInvalidLine(t *testing.T) {
	jsonl := "{\"conversation_id\":\"c1\"}\nnot-json\n"
	_, err := memory.ParseJSONL(jsonl)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseJSONLLineLargerThanScannerLimit(t *testing.T) {
	longText := strings.Repeat("x", 200*1024)
	line, err := json.Marshal(map[string]string{
		"conversation_id": "c1",
		"timestamp":       "2026-06-22T11:13:00Z",
		"speaker":         "user",
		"text":            longText,
	})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	records, err := memory.ParseJSONL(string(line) + "\n")
	if err != nil {
		t.Fatalf("expected long line to parse, got %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if len(records[0].Text) != len(longText) {
		t.Fatalf("expected text of %d bytes, got %d", len(longText), len(records[0].Text))
	}
}
