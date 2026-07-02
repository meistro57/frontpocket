package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunIngestChatGPTDryRunOutputsStats(t *testing.T) {
	source := writeChatGPTConversationDir(t)
	output := captureStdout(t, func() {
		err := runIngestChatGPT([]string{source, "--dry-run", "--project", "FrontPocket", "--ai-provider", "chatgpt"})
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
		"dry-run signals:",
		"dry-run attachments:",
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

	err := runIngestChatGPT([]string{source, "--dry-run", "--out", outPath, "--conversation", "FrontPocket", "--ai-provider", "chatgpt"})
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

func TestRunIngestChatGPTDryRunSignalCounts(t *testing.T) {
	source := writeChatGPTConversationDirWithSignals(t)
	output := captureStdout(t, func() {
		err := runIngestChatGPT([]string{source, "--dry-run", "--ai-provider", "chatgpt"})
		if err != nil {
			t.Fatalf("runIngestChatGPT failed: %v", err)
		}
	})
	if !strings.Contains(output, "dry-run signals: starred=1 shared=1 feedback=1 (thumbs_up=0, thumbs_down=1)") {
		t.Fatalf("expected dry-run signal counts, got:\n%s", output)
	}
}

func TestRunIngestChatGPTLimitCapsConversations(t *testing.T) {
	source := writeTwoChatGPTConversationsDir(t)

	output := captureStdout(t, func() {
		err := runIngestChatGPT([]string{source, "--dry-run", "--limit", "1", "--ai-provider", "chatgpt"})
		if err != nil {
			t.Fatalf("runIngestChatGPT failed: %v", err)
		}
	})
	if !strings.Contains(output, "conversations found: 1") {
		t.Fatalf("expected --limit 1 to cap conversations found at 1, got:\n%s", output)
	}

	unlimited := captureStdout(t, func() {
		err := runIngestChatGPT([]string{source, "--dry-run", "--ai-provider", "chatgpt"})
		if err != nil {
			t.Fatalf("runIngestChatGPT failed: %v", err)
		}
	})
	if !strings.Contains(unlimited, "conversations found: 2") {
		t.Fatalf("expected no limit to process both conversations, got:\n%s", unlimited)
	}
}

func writeTwoChatGPTConversationsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	payload := []map[string]any{
		{
			"id":    "conv_one",
			"title": "First",
			"mapping": map[string]any{
				"node_1": map[string]any{
					"id": "node_1",
					"message": map[string]any{
						"id":      "msg_1",
						"author":  map[string]any{"role": "user"},
						"content": map[string]any{"content_type": "text", "parts": []any{"first"}},
					},
				},
			},
		},
		{
			"id":    "conv_two",
			"title": "Second",
			"mapping": map[string]any{
				"node_2": map[string]any{
					"id": "node_2",
					"message": map[string]any{
						"id":      "msg_2",
						"author":  map[string]any{"role": "user"},
						"content": map[string]any{"content_type": "text", "parts": []any{"second"}},
					},
				},
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed marshaling conversations payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "conversations.json"), encoded, 0o644); err != nil {
		t.Fatalf("failed writing conversations.json: %v", err)
	}
	return dir
}

func TestRunIngestChatGPTHardFailsOnQdrantWriteError(t *testing.T) {
	embedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed decoding embeddings request: %v", err)
		}
		response := map[string]any{"data": make([]map[string]any, 0, len(req.Input))}
		for idx := range req.Input {
			response["data"] = append(response["data"].([]map[string]any), map[string]any{
				"index":     idx,
				"embedding": []float64{0.1, 0.2, 0.3, 0.4},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer embedServer.Close()

	qdrantServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/collections/frontpocket_memory/points/count":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"result":{"count":0}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_memory":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/frontpocket_memory/points":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"status":"error","result":null}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrantServer.Close()

	t.Setenv("EMBEDDING_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", embedServer.URL)
	t.Setenv("OPENAI_EMBEDDING_MODEL", "test-model")
	t.Setenv("EMBEDDING_DIMENSIONS", "4")
	t.Setenv("QDRANT_URL", qdrantServer.URL)
	t.Setenv("QDRANT_COLLECTION", "frontpocket_memory")
	t.Setenv("QDRANT_DISTANCE", "Cosine")
	t.Setenv("STORE_ASSISTANT_MESSAGES", "true")
	t.Setenv("STORE_USER_MESSAGES", "true")
	t.Setenv("STORE_SYSTEM_MESSAGES", "false")

	source := writeChatGPTConversationDir(t)
	err := runIngestChatGPT([]string{source, "--ai-provider", "chatgpt"})
	if err == nil {
		t.Fatal("expected ingest to fail when qdrant upsert fails")
	}
	t.Logf("received error: %v", err)
	message := err.Error()
	if !strings.Contains(message, "storage ingest failed") {
		t.Fatalf("expected hard-fail storage error, got: %v", err)
	}
	if !strings.Contains(message, "qdrant responded with status 500") {
		t.Fatalf("expected surfaced qdrant status error, got: %v", err)
	}
}

func TestRunIngestCommandHelp(t *testing.T) {
	output := captureStdout(t, func() {
		err := runIngestCommand([]string{"--help"})
		if err != nil {
			t.Fatalf("runIngestCommand --help failed: %v", err)
		}
	})
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("expected help output to contain Usage, got:\n%s", output)
	}
	if !strings.Contains(output, "Subcommands:") || !strings.Contains(output, "chatgpt      Import from a ChatGPT export zip or folder.") || !strings.Contains(output, "claude       Import from a Claude export folder.") {
		t.Fatalf("expected help output to contain ingest subcommand reference, got:\n%s", output)
	}
	if !strings.Contains(output, "frontpocket ingest --help") || !strings.Contains(output, "frontpocket ingest chatgpt --help") || !strings.Contains(output, "frontpocket ingest claude --help") {
		t.Fatalf("expected help output to contain nested help references, got:\n%s", output)
	}
}

func TestRunIngestChatGPTHelp(t *testing.T) {
	output := captureStdout(t, func() {
		err := runIngestChatGPT([]string{"--help"})
		if err != nil {
			t.Fatalf("runIngestChatGPT --help failed: %v", err)
		}
	})
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("expected help output to contain Usage, got:\n%s", output)
	}
	if !strings.Contains(output, "--dry-run") {
		t.Fatalf("expected help output to contain flags, got:\n%s", output)
	}
	if !strings.Contains(output, "Command Reference:") || !strings.Contains(output, "frontpocket ingest --help") || !strings.Contains(output, "frontpocket ingest chatgpt --help") {
		t.Fatalf("expected help output to contain command references, got:\n%s", output)
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

func writeChatGPTConversationDirWithSignals(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	payload := []map[string]any{
		{
			"id":          "conv_frontpocket_signal",
			"title":       "FrontPocket Signals",
			"is_starred":  true,
			"create_time": 1735689600,
			"update_time": 1735689700,
			"mapping": map[string]any{
				"node_1": map[string]any{
					"id": "node_1",
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
			},
		},
	}
	conversations, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed marshaling conversations payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "conversations.json"), conversations, 0o644); err != nil {
		t.Fatalf("failed writing conversations.json: %v", err)
	}

	shared := []map[string]any{{
		"conversation_id": "conv_frontpocket_signal",
		"id":              "share-123",
		"is_anonymous":    false,
	}}
	sharedPayload, err := json.Marshal(shared)
	if err != nil {
		t.Fatalf("failed marshaling shared_conversations payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shared_conversations.json"), sharedPayload, 0o644); err != nil {
		t.Fatalf("failed writing shared_conversations.json: %v", err)
	}

	feedback := []map[string]any{{
		"conversation_id": "conv_frontpocket_signal",
		"create_time":     1735689602,
		"id":              "fb-1",
		"rating":          "thumbs_down",
		"content":         "{\"text\": \"totally lost personality\"}",
		"update_time":     1735689700,
	}}
	feedbackPayload, err := json.Marshal(feedback)
	if err != nil {
		t.Fatalf("failed marshaling message_feedback payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "message_feedback.json"), feedbackPayload, 0o644); err != nil {
		t.Fatalf("failed writing message_feedback.json: %v", err)
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
