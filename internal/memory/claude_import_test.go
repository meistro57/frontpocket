package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseClaudeExportUnescapesToolPayloadNewlinesAndAddsToolTags(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}

	conversations := []map[string]any{
		{
			"uuid":       "conv_1",
			"name":       "Tool Test",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:01Z",
			"chat_messages": []map[string]any{
				{
					"uuid":                "msg_1",
					"sender":              "assistant",
					"parent_message_uuid": claudeRootParentUUID,
					"text":                "baseline",
					"content": []map[string]any{
						{"type": "text", "text": "plain text block"},
						{"type": "tool_use", "name": "artifacts", "input": map[string]any{"new_str": "line1\\nline2"}},
						{"type": "tool_result", "name": "artifacts", "content": []any{map[string]any{"type": "text", "text": "OK\\nDone"}}},
					},
					"attachments": []map[string]any{},
					"files":       []map[string]any{},
				},
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "conversations.json"), conversations)
	writeJSON(t, filepath.Join(dir, "memories.json"), []map[string]any{})

	result, err := ParseClaudeExport(dir, ClaudeImportOptions{
		SpeakerRules: SpeakerRules{StoreAssistant: true, StoreUser: true},
		AIProvider:   "claude",
	})
	if err != nil {
		t.Fatalf("ParseClaudeExport failed: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	rec := result.Records[0]
	if strings.Contains(rec.Text, "\\n") {
		t.Fatalf("expected real newlines, got escaped text: %q", rec.Text)
	}
	if !strings.Contains(rec.Text, "line1\nline2") {
		t.Fatalf("expected unescaped tool_use content in text, got %q", rec.Text)
	}
	if !strings.Contains(rec.Text, "OK\nDone") {
		t.Fatalf("expected unescaped tool_result content in text, got %q", rec.Text)
	}
	if !hasTag(rec.Tags, "content_block:tool_use") || !hasTag(rec.Tags, "content_block:tool_result") {
		t.Fatalf("expected tool tags, got %v", rec.Tags)
	}
}

func TestNormalizePotentiallyEscapedTextLeavesPlainTextIntact(t *testing.T) {
	plain := "line1\nline2"
	if got := normalizePotentiallyEscapedText(plain); got != plain {
		t.Fatalf("plain text changed: %q", got)
	}
	escaped := "line1\\nline2"
	if got := normalizePotentiallyEscapedText(escaped); got != "line1\nline2" {
		t.Fatalf("escaped text not unescaped: %q", got)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func hasTag(tags []string, expected string) bool {
	for _, tag := range tags {
		if tag == expected {
			return true
		}
	}
	return false
}
