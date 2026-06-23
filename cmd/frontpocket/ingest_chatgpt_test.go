package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunIngestChatGPTDryRunOutputsStats(t *testing.T) {
	source := writeChatGPTConversationDir(t)
	output := captureStdout(t, func() {
		err := runIngestChatGPT([]string{source, "--dry-run", "--project", "FrontPocket"})
		if err != nil {
			t.Fatalf("runIngestChatGPT failed: %v", err)
		}
	})

	expected := []string{
		"source path:",
		"import_id:",
		"conversations found:",
		"messages found:",
		"messages accepted:",
		"messages skipped:",
		"roles found:",
		"unsupported content types:",
		"attachments/assets detected:",
		"attachments ingested: no",
		"storage write: skipped (--dry-run)",
	}
	for _, fragment := range expected {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got:\n%s", fragment, output)
		}
	}
}

func TestRunIngestChatGPTPathBeforeFlagsAndOutFile(t *testing.T) {
	source := writeChatGPTConversationDir(t)
	outPath := filepath.Join(t.TempDir(), "processed", "chatgpt_normalized.jsonl")

	err := runIngestChatGPT([]string{source, "--dry-run", "--out", outPath, "--conversation", "FrontPocket"})
	if err != nil {
		t.Fatalf("runIngestChatGPT failed: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected output JSONL file, got error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d", len(lines))
	}
}

func writeChatGPTConversationDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	payload := []map[string]any{
		{
			"id":          "conv_frontpocket_1",
			"title":       "FrontPocket Planning",
			"create_time": 1735689600,
			"update_time": 1735689700,
			"mapping": map[string]any{
				"node_1": map[string]any{
					"id":       "node_1",
					"children": []any{"node_2"},
					"message": map[string]any{
						"id":          "msg_1",
						"create_time": 1735689601,
						"author":      map[string]any{"role": "user"},
						"content": map[string]any{
							"content_type": "text",
							"parts":        []any{"Hello"},
						},
					},
				},
				"node_2": map[string]any{
					"id":       "node_2",
					"parent":   "node_1",
					"children": []any{},
					"message": map[string]any{
						"id":          "msg_2",
						"create_time": 1735689602,
						"author":      map[string]any{"role": "assistant"},
						"content": map[string]any{
							"content_type": "text",
							"parts":        []any{"Hi"},
						},
					},
				},
			},
		},
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	path := filepath.Join(dir, "conversations.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("failed writing conversations.json: %v", err)
	}
	return dir
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed creating stdout pipe: %v", err)
	}
	os.Stdout = writer

	fn()

	_ = writer.Close()
	os.Stdout = original

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, reader)
	_ = reader.Close()
	return buf.String()
}
