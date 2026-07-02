package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meistro57/frontpocket/internal/memory"
)

type fakeCaptioner struct {
	calls    []memory.ResolvedAttachment
	response string
	err      error
}

// CaptionImage mirrors VisionCaptioner's contract: only image mime types
// trigger a (simulated) vision call, tracked via calls. Non-image mime
// types get a plain metadata description with no recorded call, matching
// real API-cost behavior.
func (f *fakeCaptioner) CaptionImage(ctx context.Context, attachment memory.ResolvedAttachment) (string, error) {
	if !strings.HasPrefix(attachment.MimeType, "image/") {
		return "metadata-only: " + attachment.Filename, nil
	}
	f.calls = append(f.calls, attachment)
	if f.err != nil {
		return "", f.err
	}
	if f.response != "" {
		return f.response, nil
	}
	return "a real caption describing the image", nil
}

func writeAssetFileNamesJSON(t *testing.T, dir string, payload any) {
	t.Helper()
	writeJSONFile(t, filepath.Join(dir, "conversation_asset_file_names.json"), payload)
}

func writeLibraryFilesJSON(t *testing.T, dir string, payload any) {
	t.Helper()
	writeJSONFile(t, filepath.Join(dir, "library_files.json"), payload)
}

func writeJSONFile(t *testing.T, path string, payload any) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed marshaling %s: %v", path, err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("failed writing %s: %v", path, err)
	}
}

func TestLoadAssetFileNamesMissingFileReturnsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	out, err := memory.LoadAssetFileNames(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty map, got %v", out)
	}
}

func TestLoadLibraryFilesMissingFileReturnsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	out, err := memory.LoadLibraryFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty map, got %v", out)
	}
}

func TestResolveAttachmentAssetFileNamesSystem(t *testing.T) {
	dir := t.TempDir()
	writeAssetFileNamesJSON(t, dir, map[string]string{
		"file-abc123.dat": "photo.png",
	})
	if err := os.WriteFile(filepath.Join(dir, "file-abc123.dat"), []byte("fake-bytes"), 0o644); err != nil {
		t.Fatalf("failed writing dat file: %v", err)
	}

	assetNames, err := memory.LoadAssetFileNames(dir)
	if err != nil {
		t.Fatalf("load asset file names: %v", err)
	}
	libraryFiles, err := memory.LoadLibraryFiles(dir)
	if err != nil {
		t.Fatalf("load library files: %v", err)
	}

	attachment, ok := memory.ResolveAttachment("file-service://file-abc123", dir, assetNames, libraryFiles)
	if !ok {
		t.Fatalf("expected attachment to resolve")
	}
	if attachment.Filename != "photo.png" {
		t.Fatalf("expected filename photo.png, got %q", attachment.Filename)
	}
	if attachment.MimeType != "image/png" {
		t.Fatalf("expected mime type image/png, got %q", attachment.MimeType)
	}
	if attachment.SourceSystem != memory.AttachmentSourceAssetFileNames {
		t.Fatalf("expected source system %q, got %q", memory.AttachmentSourceAssetFileNames, attachment.SourceSystem)
	}
	if attachment.DiskPath != filepath.Join(dir, "file-abc123.dat") {
		t.Fatalf("unexpected disk path: %q", attachment.DiskPath)
	}
}

func TestResolveAttachmentLibraryFilesSystem(t *testing.T) {
	dir := t.TempDir()
	writeLibraryFilesJSON(t, dir, []map[string]any{
		{
			"file_id":                     "file_abc123",
			"file_extension":              "png",
			"mime_type":                   "image/png",
			"library_file_category":       "image",
			"origination_conversation_id": "conv-1",
			"origination_message_id":      "msg-1",
			"origination_thread_id":       "thread-1",
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "file_abc123.dat"), []byte("fake-bytes"), 0o644); err != nil {
		t.Fatalf("failed writing dat file: %v", err)
	}

	assetNames, err := memory.LoadAssetFileNames(dir)
	if err != nil {
		t.Fatalf("load asset file names: %v", err)
	}
	libraryFiles, err := memory.LoadLibraryFiles(dir)
	if err != nil {
		t.Fatalf("load library files: %v", err)
	}

	attachment, ok := memory.ResolveAttachment("sediment://file_abc123", dir, assetNames, libraryFiles)
	if !ok {
		t.Fatalf("expected attachment to resolve")
	}
	if attachment.SourceSystem != memory.AttachmentSourceLibraryFiles {
		t.Fatalf("expected source system %q, got %q", memory.AttachmentSourceLibraryFiles, attachment.SourceSystem)
	}
	if attachment.Category != "image" {
		t.Fatalf("expected category image, got %q", attachment.Category)
	}
	if attachment.MimeType != "image/png" {
		t.Fatalf("expected mime type image/png, got %q", attachment.MimeType)
	}
}

func TestResolveAttachmentLibraryFilesPrefersRealFileNameOverConstructedOne(t *testing.T) {
	// Real export shape: library_files.json entries often carry a real,
	// descriptive file_name (e.g. a user-uploaded PDF's original name)
	// distinct from the UUID-shaped file_id. Prefer it when present instead
	// of always constructing "<file_id>.<ext>", which is a meaningless
	// filename for anything the resolved caption/summary should reference.
	dir := t.TempDir()
	writeLibraryFilesJSON(t, dir, []map[string]any{
		{
			"file_id":               "file_xyz789",
			"file_name":             "Knowledge Base - Real Document.pdf",
			"file_extension":        "pdf",
			"mime_type":             "application/pdf",
			"library_file_category": "pdf",
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "file_xyz789.dat"), []byte("fake-bytes"), 0o644); err != nil {
		t.Fatalf("failed writing dat file: %v", err)
	}

	assetNames, _ := memory.LoadAssetFileNames(dir)
	libraryFiles, err := memory.LoadLibraryFiles(dir)
	if err != nil {
		t.Fatalf("load library files: %v", err)
	}

	attachment, ok := memory.ResolveAttachment("sediment://file_xyz789", dir, assetNames, libraryFiles)
	if !ok {
		t.Fatalf("expected attachment to resolve")
	}
	if attachment.Filename != "Knowledge Base - Real Document.pdf" {
		t.Fatalf("expected real file_name to be used, got %q", attachment.Filename)
	}
}

func TestResolveAttachmentUnresolvedWhenMissingFromBothMaps(t *testing.T) {
	dir := t.TempDir()
	assetNames, _ := memory.LoadAssetFileNames(dir)
	libraryFiles, _ := memory.LoadLibraryFiles(dir)

	_, ok := memory.ResolveAttachment("file-service://file-doesnotexist", dir, assetNames, libraryFiles)
	if ok {
		t.Fatalf("expected unresolved reference to return false")
	}
}

func TestResolveAttachmentUnresolvedWhenDiskFileMissing(t *testing.T) {
	dir := t.TempDir()
	writeAssetFileNamesJSON(t, dir, map[string]string{
		"file-abc123.dat": "photo.png",
	})
	assetNames, _ := memory.LoadAssetFileNames(dir)
	libraryFiles, _ := memory.LoadLibraryFiles(dir)

	_, ok := memory.ResolveAttachment("file-service://file-abc123", dir, assetNames, libraryFiles)
	if ok {
		t.Fatalf("expected resolution to fail when .dat bytes don't exist on disk")
	}
}

func TestParseChatGPTExportCaptionsResolvedImageAttachment(t *testing.T) {
	folder := t.TempDir()
	payload := []map[string]any{
		{
			"id":    "conv-image",
			"title": "Has Image",
			"mapping": map[string]any{
				"n1": map[string]any{
					"id": "n1",
					"message": map[string]any{
						"id":     "m1",
						"author": map[string]any{"role": "user"},
						"content": map[string]any{
							"content_type": "multimodal_text",
							"parts": []any{
								map[string]any{
									"asset_pointer": "file-service://file-abc123",
									"content_type":  "image_asset_pointer",
								},
							},
						},
					},
				},
			},
		},
	}
	writeConversationsJSON(t, folder, payload)
	writeAssetFileNamesJSON(t, folder, map[string]string{
		"file-abc123.dat": "photo.png",
	})
	if err := os.WriteFile(filepath.Join(folder, "file-abc123.dat"), []byte("fake-bytes"), 0o644); err != nil {
		t.Fatalf("failed writing dat file: %v", err)
	}

	captioner := &fakeCaptioner{response: "a screenshot of a terminal showing a go test run"}

	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		Captioner:    captioner,
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	record := result.Records[0]
	if record.Text != "a screenshot of a terminal showing a go test run" {
		t.Fatalf("expected caption text, got %q", record.Text)
	}
	if record.AttachmentFilename != "photo.png" {
		t.Fatalf("expected attachment_filename photo.png, got %q", record.AttachmentFilename)
	}
	if record.AttachmentMimeType != "image/png" {
		t.Fatalf("expected attachment_mime_type image/png, got %q", record.AttachmentMimeType)
	}
	if record.AttachmentSourceSystem != memory.AttachmentSourceAssetFileNames {
		t.Fatalf("expected attachment_source_system asset_file_names, got %q", record.AttachmentSourceSystem)
	}
	if len(captioner.calls) != 1 {
		t.Fatalf("expected exactly 1 captioning call, got %d", len(captioner.calls))
	}
	if result.AttachmentsCaptioned != 1 {
		t.Fatalf("expected AttachmentsCaptioned=1, got %d", result.AttachmentsCaptioned)
	}
	if result.AttachmentsResolvedAssetFiles != 1 {
		t.Fatalf("expected AttachmentsResolvedAssetFiles=1, got %d", result.AttachmentsResolvedAssetFiles)
	}
	if result.AttachmentsWouldCaption != 1 {
		t.Fatalf("expected AttachmentsWouldCaption=1, got %d", result.AttachmentsWouldCaption)
	}
}

func TestParseChatGPTExportDryRunSkipsCaptioningCalls(t *testing.T) {
	folder := t.TempDir()
	payload := []map[string]any{
		{
			"id":    "conv-image",
			"title": "Has Image",
			"mapping": map[string]any{
				"n1": map[string]any{
					"id": "n1",
					"message": map[string]any{
						"id":     "m1",
						"author": map[string]any{"role": "user"},
						"content": map[string]any{
							"content_type": "multimodal_text",
							"parts": []any{
								map[string]any{
									"asset_pointer": "file-service://file-abc123",
									"content_type":  "image_asset_pointer",
								},
							},
						},
					},
				},
			},
		},
	}
	writeConversationsJSON(t, folder, payload)
	writeAssetFileNamesJSON(t, folder, map[string]string{
		"file-abc123.dat": "photo.png",
	})
	if err := os.WriteFile(filepath.Join(folder, "file-abc123.dat"), []byte("fake-bytes"), 0o644); err != nil {
		t.Fatalf("failed writing dat file: %v", err)
	}

	captioner := &fakeCaptioner{}

	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		Captioner:    captioner,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(captioner.calls) != 0 {
		t.Fatalf("expected no captioning calls during dry-run, got %d", len(captioner.calls))
	}
	if result.AttachmentsWouldCaption != 1 {
		t.Fatalf("expected AttachmentsWouldCaption=1, got %d", result.AttachmentsWouldCaption)
	}
	if result.AttachmentsCaptioned != 0 {
		t.Fatalf("expected AttachmentsCaptioned=0 during dry-run, got %d", result.AttachmentsCaptioned)
	}
	if result.Records[0].Text == "" {
		t.Fatalf("expected fallback placeholder text to remain when dry-run skips captioning")
	}
}

func TestParseChatGPTExportNonImageAttachmentSkipsCaptioner(t *testing.T) {
	folder := t.TempDir()
	payload := []map[string]any{
		{
			"id":    "conv-pdf",
			"title": "Has PDF",
			"mapping": map[string]any{
				"n1": map[string]any{
					"id": "n1",
					"message": map[string]any{
						"id":     "m1",
						"author": map[string]any{"role": "user"},
						"content": map[string]any{
							"content_type": "multimodal_text",
							"parts": []any{
								map[string]any{
									"asset_pointer": "file-service://file-doc123",
									"content_type":  "image_asset_pointer",
								},
							},
						},
					},
				},
			},
		},
	}
	writeConversationsJSON(t, folder, payload)
	writeAssetFileNamesJSON(t, folder, map[string]string{
		"file-doc123.dat": "notes.pdf",
	})
	if err := os.WriteFile(filepath.Join(folder, "file-doc123.dat"), []byte("fake-bytes"), 0o644); err != nil {
		t.Fatalf("failed writing dat file: %v", err)
	}

	captioner := &fakeCaptioner{}

	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		Captioner:    captioner,
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.AttachmentsWouldCaption != 0 {
		t.Fatalf("expected AttachmentsWouldCaption=0 for a PDF, got %d", result.AttachmentsWouldCaption)
	}
	if len(captioner.calls) != 0 {
		t.Fatalf("non-image attachments should not invoke the captioner at all, got %d calls", len(captioner.calls))
	}
	if result.Records[0].AttachmentMimeType != "application/pdf" {
		t.Fatalf("expected attachment_mime_type application/pdf, got %q", result.Records[0].AttachmentMimeType)
	}
}

func TestParseChatGPTExportUnresolvedAttachmentFallsBackToPlaceholder(t *testing.T) {
	folder := t.TempDir()
	payload := []map[string]any{
		{
			"id":    "conv-unresolved",
			"title": "Unresolved",
			"mapping": map[string]any{
				"n1": map[string]any{
					"id": "n1",
					"message": map[string]any{
						"id":     "m1",
						"author": map[string]any{"role": "user"},
						"content": map[string]any{
							"content_type": "multimodal_text",
							"parts": []any{
								map[string]any{
									"asset_pointer": "file-service://file-ghost",
									"content_type":  "image_asset_pointer",
								},
							},
						},
					},
				},
			},
		},
	}
	writeConversationsJSON(t, folder, payload)

	captioner := &fakeCaptioner{}
	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		Captioner:    captioner,
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(captioner.calls) != 0 {
		t.Fatalf("expected no captioning calls for unresolvable reference, got %d", len(captioner.calls))
	}
	if result.AttachmentsUnresolved != 1 {
		t.Fatalf("expected AttachmentsUnresolved=1, got %d", result.AttachmentsUnresolved)
	}
	if result.Records[0].AttachmentSourceSystem != "" {
		t.Fatalf("expected no attachment_source_system for unresolved ref, got %q", result.Records[0].AttachmentSourceSystem)
	}
	if result.Records[0].Text == "" {
		t.Fatalf("expected fallback placeholder text for unresolved attachment")
	}
}

func TestParseChatGPTExportDedupesSchemeQualifiedAndBareRefToSameAttachment(t *testing.T) {
	// Real ChatGPT exports frequently surface the same underlying asset
	// twice on one message: once scheme-qualified via asset_pointer (e.g.
	// "file-service://file-abc123") and once bare via a nested "id" field
	// picked up by appendAttachmentRefsFromValue's map-walk. Both forms
	// resolve to the same file and must count as ONE attachment, not two.
	folder := t.TempDir()
	payload := []map[string]any{
		{
			"id":    "conv-dup",
			"title": "Duplicate Ref Forms",
			"mapping": map[string]any{
				"n1": map[string]any{
					"id": "n1",
					"message": map[string]any{
						"id":     "m1",
						"author": map[string]any{"role": "user"},
						"content": map[string]any{
							"content_type": "multimodal_text",
							"parts": []any{
								map[string]any{
									"asset_pointer": "file-service://file-abc123",
									"content_type":  "image_asset_pointer",
									"id":            "file-abc123",
								},
							},
						},
					},
				},
			},
		},
	}
	writeConversationsJSON(t, folder, payload)
	writeAssetFileNamesJSON(t, folder, map[string]string{
		"file-abc123.dat": "photo.png",
	})
	if err := os.WriteFile(filepath.Join(folder, "file-abc123.dat"), []byte("fake-bytes"), 0o644); err != nil {
		t.Fatalf("failed writing dat file: %v", err)
	}

	captioner := &fakeCaptioner{response: "a single real caption"}
	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		Captioner:    captioner,
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.AttachmentsTotal != 1 {
		t.Fatalf("expected AttachmentsTotal=1 (deduped), got %d", result.AttachmentsTotal)
	}
	if result.AttachmentsResolvedAssetFiles != 1 {
		t.Fatalf("expected AttachmentsResolvedAssetFiles=1 (deduped), got %d", result.AttachmentsResolvedAssetFiles)
	}
	if result.AttachmentsWouldCaption != 1 {
		t.Fatalf("expected AttachmentsWouldCaption=1 (deduped), got %d", result.AttachmentsWouldCaption)
	}
	if len(captioner.calls) != 1 {
		t.Fatalf("expected exactly 1 captioning call for the deduped attachment, got %d", len(captioner.calls))
	}
}

func TestParseChatGPTExportEmptyTextMessageWithMetadataAttachmentIsNotDropped(t *testing.T) {
	// Real export shape: a large pasted-file upload appears as a message
	// with content_type "text" and an empty parts array — the only
	// reference to the actual file lives in message.metadata.attachments.
	// Before this was fixed, extractMessageText's content_type=="text"
	// branch short-circuited on empty parts and returned "" regardless of
	// hasAttachment, so the whole message silently vanished (HasText false
	// -> skipped) before ever reaching attachment resolution.
	folder := t.TempDir()
	payload := []map[string]any{
		{
			"id":    "conv-pasted-file",
			"title": "Pasted File Upload",
			"mapping": map[string]any{
				"n1": map[string]any{
					"id": "n1",
					"message": map[string]any{
						"id":      "m1",
						"author":  map[string]any{"role": "user"},
						"content": map[string]any{"content_type": "text", "parts": []any{""}},
						"metadata": map[string]any{
							"attachments": []any{
								map[string]any{
									"id":        "file_abc123",
									"mime_type": "text/markdown",
									"name":      "Pasted markdown(10).md",
								},
							},
						},
					},
				},
			},
		},
	}
	writeConversationsJSON(t, folder, payload)
	writeLibraryFilesJSON(t, folder, []map[string]any{
		{
			"file_id":               "file_abc123",
			"file_extension":        "md",
			"mime_type":             "text/markdown",
			"library_file_category": "text",
		},
	})
	if err := os.WriteFile(filepath.Join(folder, "file_abc123.dat"), []byte("fake-bytes"), 0o644); err != nil {
		t.Fatalf("failed writing dat file: %v", err)
	}

	captioner := &fakeCaptioner{}
	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		Captioner:    captioner,
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected the message to survive as 1 record, got %d", len(result.Records))
	}
	record := result.Records[0]
	if record.AttachmentSourceSystem != memory.AttachmentSourceLibraryFiles {
		t.Fatalf("expected attachment_source_system library_files, got %q", record.AttachmentSourceSystem)
	}
	if record.AttachmentMimeType != "text/markdown" {
		t.Fatalf("expected attachment_mime_type text/markdown, got %q", record.AttachmentMimeType)
	}
	if strings.TrimSpace(record.Text) == "" {
		t.Fatalf("expected non-empty metadata-only text, got empty")
	}
	if len(captioner.calls) != 0 {
		t.Fatalf("non-image attachment should not trigger a vision call, got %d calls", len(captioner.calls))
	}
}

func TestParseChatGPTExportConversationIDFiltersExactMatchOnly(t *testing.T) {
	folder := t.TempDir()
	payload := []map[string]any{
		conversationWithSingleTextMessage("conv-target", "target content"),
		conversationWithSingleTextMessage("conv-target-extra", "should not match"),
		conversationWithSingleTextMessage("conv-other", "other content"),
	}
	writeConversationsJSON(t, folder, payload)

	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules:   memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		ConversationID: "conv-target",
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.ConversationsFound != 1 {
		t.Fatalf("expected exactly 1 conversation matched by exact id, got %d", result.ConversationsFound)
	}
	if len(result.Records) != 1 || result.Records[0].ConversationID != "conv-target" {
		t.Fatalf("expected only conv-target's record, got %#v", result.Records)
	}
}

func TestParseChatGPTExportLimitCapsConversations(t *testing.T) {
	folder := t.TempDir()
	payload := []map[string]any{
		conversationWithSingleTextMessage("conv-1", "first"),
		conversationWithSingleTextMessage("conv-2", "second"),
		conversationWithSingleTextMessage("conv-3", "third"),
	}
	writeConversationsJSON(t, folder, payload)

	result, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		Limit:        2,
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.ConversationsFound != 2 {
		t.Fatalf("expected ConversationsFound=2 with --limit 2, got %d", result.ConversationsFound)
	}
}

func TestParseChatGPTExportCaptionCacheAvoidsSecondVisionCall(t *testing.T) {
	folder := t.TempDir()
	payload := []map[string]any{
		{
			"id":    "conv-cache",
			"title": "Caption Cache",
			"mapping": map[string]any{
				"n1": map[string]any{
					"id": "n1",
					"message": map[string]any{
						"id":     "m1",
						"author": map[string]any{"role": "user"},
						"content": map[string]any{
							"content_type": "multimodal_text",
							"parts": []any{
								map[string]any{
									"asset_pointer": "file-service://file-abc123",
									"content_type":  "image_asset_pointer",
								},
							},
						},
					},
				},
			},
		},
	}
	writeConversationsJSON(t, folder, payload)
	writeAssetFileNamesJSON(t, folder, map[string]string{
		"file-abc123.dat": "photo.png",
	})
	if err := os.WriteFile(filepath.Join(folder, "file-abc123.dat"), []byte("identical-bytes"), 0o644); err != nil {
		t.Fatalf("failed writing dat file: %v", err)
	}

	cachePath := filepath.Join(folder, "caption_cache.json")
	cache, err := memory.OpenCaptionCache(cachePath)
	if err != nil {
		t.Fatalf("open caption cache: %v", err)
	}
	firstUnderlying := &fakeCaptioner{response: "cached image caption"}
	firstResult, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		Captioner:    memory.NewCachingCaptioner(firstUnderlying, cache),
	})
	if err != nil {
		t.Fatalf("first parse failed: %v", err)
	}
	if len(firstUnderlying.calls) != 1 {
		t.Fatalf("expected exactly 1 vision call on first parse, got %d", len(firstUnderlying.calls))
	}
	if firstResult.CaptionCacheMisses != 1 || firstResult.CaptionCacheWrites != 1 || firstResult.CaptionCacheHits != 0 {
		t.Fatalf("unexpected first-run cache stats: hits=%d misses=%d writes=%d", firstResult.CaptionCacheHits, firstResult.CaptionCacheMisses, firstResult.CaptionCacheWrites)
	}

	cacheReloaded, err := memory.OpenCaptionCache(cachePath)
	if err != nil {
		t.Fatalf("reopen caption cache: %v", err)
	}
	secondUnderlying := &fakeCaptioner{response: "should not be used"}
	secondResult, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules: memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		Captioner:    memory.NewCachingCaptioner(secondUnderlying, cacheReloaded),
	})
	if err != nil {
		t.Fatalf("second parse failed: %v", err)
	}
	if len(secondUnderlying.calls) != 0 {
		t.Fatalf("expected 0 vision calls on second parse due to cache hit, got %d", len(secondUnderlying.calls))
	}
	if secondResult.CaptionCacheHits != 1 || secondResult.CaptionCacheMisses != 0 {
		t.Fatalf("unexpected second-run cache stats: hits=%d misses=%d", secondResult.CaptionCacheHits, secondResult.CaptionCacheMisses)
	}
	if secondResult.Records[0].Text != "cached image caption" {
		t.Fatalf("expected cached caption text to be reused, got %q", secondResult.Records[0].Text)
	}
}

func TestParseChatGPTExportCheckpointSkipsCompletedConversations(t *testing.T) {
	folder := t.TempDir()
	payload := []map[string]any{
		conversationWithSingleTextMessage("conv-1", "first"),
		conversationWithSingleTextMessage("conv-2", "second"),
	}
	writeConversationsJSON(t, folder, payload)

	checkpointPath := filepath.Join(folder, "parse_checkpoint.json")
	checkpoint, resumed, err := memory.OpenChatGPTParseCheckpoint(checkpointPath, memory.ChatGPTParseCheckpointMeta{
		Source:            folder,
		CaptioningEnabled: false,
	})
	if err != nil {
		t.Fatalf("open parse checkpoint: %v", err)
	}
	if resumed {
		t.Fatalf("expected fresh parse checkpoint")
	}

	firstResult, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules:    memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		Limit:           1,
		ParseCheckpoint: checkpoint,
	})
	if err != nil {
		t.Fatalf("first parse failed: %v", err)
	}
	if firstResult.ConversationsCheckpointed != 1 {
		t.Fatalf("expected 1 conversation checkpointed on first run, got %d", firstResult.ConversationsCheckpointed)
	}

	reopened, resumed, err := memory.OpenChatGPTParseCheckpoint(checkpointPath, memory.ChatGPTParseCheckpointMeta{
		Source:            folder,
		CaptioningEnabled: false,
	})
	if err != nil {
		t.Fatalf("reopen parse checkpoint: %v", err)
	}
	if !resumed {
		t.Fatalf("expected parse checkpoint resume")
	}

	secondResult, err := memory.ParseChatGPTExport(folder, memory.ChatGPTImportOptions{
		SpeakerRules:    memory.SpeakerRules{StoreUser: true, StoreAssistant: true},
		ParseCheckpoint: reopened,
	})
	if err != nil {
		t.Fatalf("second parse failed: %v", err)
	}
	if secondResult.ConversationsSkippedByCheckpoint != 1 {
		t.Fatalf("expected 1 skipped conversation from checkpoint, got %d", secondResult.ConversationsSkippedByCheckpoint)
	}
	if len(secondResult.Records) != 1 {
		t.Fatalf("expected only 1 new conversation record after resume, got %d", len(secondResult.Records))
	}
	if secondResult.Records[0].ConversationID != "conv-2" {
		t.Fatalf("expected remaining conversation conv-2, got %q", secondResult.Records[0].ConversationID)
	}
}

func TestCaptionImageReturnsErrorWhenUnconfigured(t *testing.T) {
	captioner := memory.NewVisionCaptioner("", "", "", "", "")
	_, err := captioner.CaptionImage(context.Background(), memory.ResolvedAttachment{
		MimeType: "image/png",
		Filename: "photo.png",
	})
	if err == nil {
		t.Fatalf("expected error when vision captioner has no API key/model configured")
	}
}

func TestCaptionImageNonImageSkipsNetworkCall(t *testing.T) {
	captioner := memory.NewVisionCaptioner("", "", "", "", "")
	caption, err := captioner.CaptionImage(context.Background(), memory.ResolvedAttachment{
		MimeType: "application/pdf",
		Filename: "notes.pdf",
	})
	if err != nil {
		t.Fatalf("unexpected error for non-image attachment: %v", err)
	}
	if caption != "PDF attachment: notes.pdf" {
		t.Fatalf("expected metadata-only PDF description, got %q", caption)
	}
}

func conversationWithSingleTextMessage(id, text string) map[string]any {
	return map[string]any{
		"id":    id,
		"title": id,
		"mapping": map[string]any{
			"n1": map[string]any{
				"id": "n1",
				"message": map[string]any{
					"id":      "m-" + id,
					"author":  map[string]any{"role": "user"},
					"content": map[string]any{"content_type": "text", "parts": []any{text}},
				},
			},
		},
	}
}
