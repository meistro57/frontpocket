package tests

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestChatGPTZipExtractionBlocksZipSlip(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "bad-export.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed creating zip: %v", err)
	}

	writer := zip.NewWriter(file)
	entry, err := writer.Create("../conversations.json")
	if err != nil {
		t.Fatalf("failed creating zip entry: %v", err)
	}
	if _, err := entry.Write([]byte(`[]`)); err != nil {
		t.Fatalf("failed writing zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed closing zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed closing zip file: %v", err)
	}

	_, err = memory.ParseChatGPTExport(zipPath, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
	})
	if err == nil {
		t.Fatal("expected zip-slip path to be rejected")
	}
}

func TestChatGPTImportSupportsFolderAndZipInputs(t *testing.T) {
	payload := sampleConversationPayload()
	folder := t.TempDir()
	writeConversationsJSON(t, folder, payload)
	zipPath := createZipWithConversation(t, payload)

	tests := []struct {
		name             string
		sourcePath       string
		expectedSourceTy string
	}{
		{name: "folder", sourcePath: folder, expectedSourceTy: "chat_export_folder"},
		{name: "zip", sourcePath: zipPath, expectedSourceTy: "chat_export_zip"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := memory.ParseChatGPTExport(tc.sourcePath, memory.ChatGPTImportOptions{
				Project:      "FrontPocket",
				SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
			})
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if result.SourceType != tc.expectedSourceTy {
				t.Fatalf("expected source type %q, got %q", tc.expectedSourceTy, result.SourceType)
			}
			if result.ConversationsFound != 1 {
				t.Fatalf("expected 1 conversation found, got %d", result.ConversationsFound)
			}
			if result.MessagesFound != 2 {
				t.Fatalf("expected 2 messages found, got %d", result.MessagesFound)
			}
			if result.MessagesAccepted != 2 {
				t.Fatalf("expected 2 accepted messages, got %d", result.MessagesAccepted)
			}
			if len(result.Records) != 2 {
				t.Fatalf("expected 2 normalized records, got %d", len(result.Records))
			}
		})
	}
}

func TestChatGPTImportSkipsEmptyMessages(t *testing.T) {
	conversation := []map[string]any{
		{
			"id":    "conv-empty",
			"title": "Empty Test",
			"mapping": map[string]any{
				"n1": map[string]any{
					"id": "n1",
					"message": map[string]any{
						"id":      "m1",
						"author":  map[string]any{"role": "user"},
						"content": map[string]any{"content_type": "text", "parts": []any{"   "}},
					},
				},
				"n2": map[string]any{
					"id":     "n2",
					"parent": "n1",
					"message": map[string]any{
						"id":      "m2",
						"author":  map[string]any{"role": "user"},
						"content": map[string]any{"content_type": "text", "parts": []any{"kept text"}},
					},
				},
			},
		},
	}

	folder := t.TempDir()
	writeConversationsJSON(t, folder, conversation)
	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.MessagesFound != 2 {
		t.Fatalf("expected 2 messages found, got %d", result.MessagesFound)
	}
	if result.MessagesAccepted != 1 {
		t.Fatalf("expected 1 accepted message, got %d", result.MessagesAccepted)
	}
	if result.MessagesSkipped != 1 {
		t.Fatalf("expected 1 skipped message, got %d", result.MessagesSkipped)
	}
}

func TestChatGPTImportRoleFilters(t *testing.T) {
	conversation := []map[string]any{
		{
			"id":    "conv-roles",
			"title": "Role Filter Test",
			"mapping": map[string]any{
				"u": map[string]any{"id": "u", "message": map[string]any{"id": "mu", "author": map[string]any{"role": "user"}, "content": map[string]any{"content_type": "text", "parts": []any{"user text"}}}},
				"a": map[string]any{"id": "a", "message": map[string]any{"id": "ma", "author": map[string]any{"role": "assistant"}, "content": map[string]any{"content_type": "text", "parts": []any{"assistant text"}}}},
				"s": map[string]any{"id": "s", "message": map[string]any{"id": "ms", "author": map[string]any{"role": "system"}, "content": map[string]any{"content_type": "text", "parts": []any{"system text"}}}},
				"t": map[string]any{"id": "t", "message": map[string]any{"id": "mt", "author": map[string]any{"role": "tool"}, "content": map[string]any{"content_type": "text", "parts": []any{"tool text"}}}},
			},
		},
	}

	folder := t.TempDir()
	writeConversationsJSON(t, folder, conversation)
	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: false, StoreSystem: false},
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.MessagesAccepted != 1 {
		t.Fatalf("expected 1 accepted message, got %d", result.MessagesAccepted)
	}
	if len(result.Records) != 1 || result.Records[0].Role != "user" {
		t.Fatalf("expected only user record, got %+v", result.Records)
	}
}

func TestChatGPTImportDryRunStatsAndUnsupportedContent(t *testing.T) {
	conversation := []map[string]any{
		{
			"id":          "conv-stats",
			"title":       "Stats Test",
			"create_time": 1735689600,
			"update_time": 1735689700,
			"mapping": map[string]any{
				"u": map[string]any{"id": "u", "message": map[string]any{"id": "mu", "author": map[string]any{"role": "user"}, "content": map[string]any{"content_type": "text", "parts": []any{"kept"}}, "create_time": 1735689601}},
				"x": map[string]any{"id": "x", "message": map[string]any{"id": "mx", "author": map[string]any{"role": "assistant"}, "content": map[string]any{"content_type": "image_asset_pointer", "parts": []any{map[string]any{"asset_pointer": "asset-1"}}}, "metadata": map[string]any{"attachments": []any{"asset-1"}}}},
			},
		},
	}

	folder := t.TempDir()
	writeConversationsJSON(t, folder, conversation)
	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.MessagesFound != 2 {
		t.Fatalf("expected 2 messages found, got %d", result.MessagesFound)
	}
	if result.MessagesAccepted != 2 {
		t.Fatalf("expected 2 accepted messages, got %d", result.MessagesAccepted)
	}
	if result.MessagesSkipped != 0 {
		t.Fatalf("expected 0 skipped messages, got %d", result.MessagesSkipped)
	}
	if _, ok := result.UnsupportedContentTypes["image_asset_pointer"]; ok {
		t.Fatalf("expected image_asset_pointer to be supported, got unsupported count %d", result.UnsupportedContentTypes["image_asset_pointer"])
	}
	if result.AttachmentsAssetsDetected != 1 {
		t.Fatalf("expected 1 attachment detected, got %d", result.AttachmentsAssetsDetected)
	}
	if !result.AttachmentsIngested {
		t.Fatal("expected attachments ingested to be true")
	}
	if len(result.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result.Records))
	}
	var attachmentRecord memory.NormalizedMemoryRecord
	foundAttachmentRecord := false
	for _, record := range result.Records {
		if record.Metadata["content_type"] == "image_asset_pointer" {
			attachmentRecord = record
			foundAttachmentRecord = true
			break
		}
	}
	if !foundAttachmentRecord {
		t.Fatalf("expected one image_asset_pointer record, got %+v", result.Records)
	}
	if attachmentRecord.Metadata["attachment_count"] != "1" {
		t.Fatalf("expected attachment_count 1, got %q", attachmentRecord.Metadata["attachment_count"])
	}
	if attachmentRecord.Metadata["attachment_refs"] != "asset-1" {
		t.Fatalf("expected attachment_refs asset-1, got %q", attachmentRecord.Metadata["attachment_refs"])
	}
	if !strings.Contains(attachmentRecord.Text, "attachment_refs: asset-1") {
		t.Fatalf("expected attachment text to include asset ref, got %q", attachmentRecord.Text)
	}
}

func TestChatGPTImportIncludesSourceMetadata(t *testing.T) {
	payload := sampleConversationPayload()
	folder := t.TempDir()
	writeConversationsJSON(t, folder, payload)

	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		Project:      "FrontPocket",
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Records) == 0 {
		t.Fatal("expected at least one normalized record")
	}
	record := result.Records[0]
	if record.SourcePlatform != "chatgpt" {
		t.Fatalf("expected source_platform chatgpt, got %q", record.SourcePlatform)
	}
	if record.Metadata["conversation_create_time"] == "" {
		t.Fatal("expected conversation_create_time metadata")
	}
	if record.Metadata["message_create_time"] == "" {
		t.Fatal("expected message_create_time metadata")
	}
	if record.Metadata["content_type"] != "text" {
		t.Fatalf("expected content_type text, got %q", record.Metadata["content_type"])
	}
}

func TestChatGPTImportSinceAndConversationFilters(t *testing.T) {
	payload := sampleConversationPayload()
	folder := t.TempDir()
	writeConversationsJSON(t, folder, payload)

	since := time.Unix(1735689602, 0).UTC()
	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		ConversationFilter: "frontpocket",
		Since:              since,
		SpeakerRules:       memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.MessagesAccepted != 1 {
		t.Fatalf("expected 1 accepted message after since filter, got %d", result.MessagesAccepted)
	}
	if len(result.Records) != 1 || result.Records[0].Role != "assistant" {
		t.Fatalf("expected assistant record after filters, got %+v", result.Records)
	}
}

func TestChatGPTImportSupportsWrappedConversationsPayload(t *testing.T) {
	folder := t.TempDir()
	wrapped := map[string]any{
		"conversations": sampleConversationPayload(),
	}
	encoded, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("failed marshaling wrapped payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "wrapped_conversations.json"), encoded, 0o644); err != nil {
		t.Fatalf("failed writing wrapped payload file: %v", err)
	}

	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.ConversationsFound != 1 {
		t.Fatalf("expected 1 conversation found, got %d", result.ConversationsFound)
	}
	if result.MessagesAccepted != 2 {
		t.Fatalf("expected 2 accepted messages, got %d", result.MessagesAccepted)
	}
}

func TestChatGPTImportConversationSignals(t *testing.T) {
	folder := t.TempDir()
	payload := []map[string]any{
		{
			"id":         "conv-signals",
			"title":      "Signals",
			"is_starred": true,
			"mapping": map[string]any{
				"n1": map[string]any{
					"id": "n1",
					"message": map[string]any{
						"id":      "m1",
						"author":  map[string]any{"role": "user"},
						"content": map[string]any{"content_type": "text", "parts": []any{"signal text"}},
					},
				},
			},
		},
		{
			"id":    "conv-no-share",
			"title": "No Share",
			"mapping": map[string]any{
				"n2": map[string]any{
					"id": "n2",
					"message": map[string]any{
						"id":      "m2",
						"author":  map[string]any{"role": "user"},
						"content": map[string]any{"content_type": "text", "parts": []any{"plain text"}},
					},
				},
			},
		},
	}
	writeConversationsJSON(t, folder, payload)
	writeSharedConversationsJSON(t, folder, []map[string]any{{
		"conversation_id": "conv-signals",
		"id":              "share-123",
		"is_anonymous":    false,
	}})
	writeMessageFeedbackJSON(t, folder, []map[string]any{
		{
			"conversation_id": "conv-signals",
			"create_time":     1735689602,
			"id":              "fb-1",
			"rating":          "thumbs_up",
			"content":         "{}",
			"update_time":     1735689603,
		},
		{
			"conversation_id": "conv-signals",
			"create_time":     1735689604,
			"id":              "fb-2",
			"rating":          "thumbs_down",
			"content":         "{\"text\": \"totally lost personality\"}",
			"update_time":     1735689700,
		},
	})

	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	byConversation := make(map[string]memory.NormalizedMemoryRecord)
	for _, record := range result.Records {
		if _, exists := byConversation[record.ConversationID]; !exists {
			byConversation[record.ConversationID] = record
		}
	}

	tests := []struct {
		name           string
		conversationID string
		assert         func(t *testing.T, rec memory.NormalizedMemoryRecord)
	}{
		{
			name:           "is_starred true carries through",
			conversationID: "conv-signals",
			assert: func(t *testing.T, rec memory.NormalizedMemoryRecord) {
				if !rec.UserStarred {
					t.Fatalf("expected UserStarred true, got false")
				}
			},
		},
		{
			name:           "shared conversation carries share fields",
			conversationID: "conv-signals",
			assert: func(t *testing.T, rec memory.NormalizedMemoryRecord) {
				if !rec.UserShared {
					t.Fatalf("expected UserShared true, got false")
				}
				if rec.ShareID != "share-123" {
					t.Fatalf("expected ShareID share-123, got %q", rec.ShareID)
				}
			},
		},
		{
			name:           "feedback uses latest update_time with parsed note",
			conversationID: "conv-signals",
			assert: func(t *testing.T, rec memory.NormalizedMemoryRecord) {
				if rec.FeedbackRating != "thumbs_down" {
					t.Fatalf("expected FeedbackRating thumbs_down, got %q", rec.FeedbackRating)
				}
				if rec.FeedbackNote != "totally lost personality" {
					t.Fatalf("expected FeedbackNote parsed text, got %q", rec.FeedbackNote)
				}
				expectedAt := time.Unix(1735689700, 0).UTC().Format(time.RFC3339)
				if rec.FeedbackAt != expectedAt {
					t.Fatalf("expected FeedbackAt %q, got %q", expectedAt, rec.FeedbackAt)
				}
			},
		},
		{
			name:           "absent shared conversation stays zero-value",
			conversationID: "conv-no-share",
			assert: func(t *testing.T, rec memory.NormalizedMemoryRecord) {
				if rec.UserShared {
					t.Fatalf("expected UserShared false, got true")
				}
				if rec.ShareID != "" {
					t.Fatalf("expected empty ShareID, got %q", rec.ShareID)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := byConversation[tc.conversationID]
			if !ok {
				t.Fatalf("missing record for conversation %q", tc.conversationID)
			}
			tc.assert(t, rec)
		})
	}
}

func TestChatGPTImportOptionalSignalFiles(t *testing.T) {
	folder := t.TempDir()
	writeConversationsJSON(t, folder, sampleConversationPayload())

	shareSignals, err := memory.LoadShareSignals(folder)
	if err != nil {
		t.Fatalf("LoadShareSignals returned error for missing file: %v", err)
	}
	if len(shareSignals) != 0 {
		t.Fatalf("expected empty share signals for missing file, got %d", len(shareSignals))
	}

	feedbackSignals, err := memory.LoadFeedbackSignals(folder)
	if err != nil {
		t.Fatalf("LoadFeedbackSignals returned error for missing file: %v", err)
	}
	if len(feedbackSignals) != 0 {
		t.Fatalf("expected empty feedback signals for missing file, got %d", len(feedbackSignals))
	}

	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	for _, record := range result.Records {
		if record.UserStarred {
			t.Fatalf("expected UserStarred false by default, got true for %q", record.ConversationID)
		}
		if record.UserShared {
			t.Fatalf("expected UserShared false, got true for %q", record.ConversationID)
		}
		if record.ShareID != "" || record.FeedbackRating != "" || record.FeedbackNote != "" || record.FeedbackAt != "" {
			t.Fatalf("expected zero-value signal fields, got %+v", record)
		}
	}
}

func writeConversationsJSON(t *testing.T, dir string, payload any) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed marshaling payload: %v", err)
	}
	path := filepath.Join(dir, "conversations.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("failed writing conversations.json: %v", err)
	}
}

func writeSharedConversationsJSON(t *testing.T, dir string, payload any) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed marshaling shared_conversations payload: %v", err)
	}
	path := filepath.Join(dir, "shared_conversations.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("failed writing shared_conversations.json: %v", err)
	}
}

func writeMessageFeedbackJSON(t *testing.T, dir string, payload any) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed marshaling message_feedback payload: %v", err)
	}
	path := filepath.Join(dir, "message_feedback.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("failed writing message_feedback.json: %v", err)
	}
}

func createZipWithConversation(t *testing.T, payload any) string {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed marshaling payload: %v", err)
	}

	zipPath := filepath.Join(t.TempDir(), "chatgpt-export.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed creating zip file: %v", err)
	}

	writer := zip.NewWriter(file)
	entry, err := writer.Create("conversations.json")
	if err != nil {
		t.Fatalf("failed creating zip entry: %v", err)
	}
	if _, err := entry.Write(encoded); err != nil {
		t.Fatalf("failed writing zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed closing zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed closing zip file: %v", err)
	}
	return zipPath
}

func sampleConversationPayload() []map[string]any {
	return []map[string]any{
		{
			"id":          "conv_frontpocket_1",
			"title":       "FrontPocket Planning",
			"create_time": 1735689600,
			"update_time": 1735689700,
			"mapping": map[string]any{
				"node_1": map[string]any{
					"id":       "node_1",
					"parent":   nil,
					"children": []any{"node_2"},
					"message": map[string]any{
						"id":          "msg_1",
						"create_time": 1735689601,
						"update_time": 1735689601,
						"author":      map[string]any{"role": "user"},
						"content": map[string]any{
							"content_type": "text",
							"parts":        []any{"FrontPocket should stay local-first."},
						},
					},
				},
				"node_2": map[string]any{
					"id":       "node_2",
					"parent":   "node_1",
					"children": []any{},
					"message": map[string]any{
						"id":          "msg_2",
						"create_time": 1735689603,
						"update_time": 1735689603,
						"author":      map[string]any{"role": "assistant"},
						"content": map[string]any{
							"content_type": "text",
							"parts":        []any{"Acknowledged. We'll keep it local-first."},
						},
					},
				},
			},
		},
	}
}
