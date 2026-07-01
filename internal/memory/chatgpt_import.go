package memory

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type NormalizedMemoryRecord struct {
	SourcePlatform         string            `json:"source_platform"`
	SourceType             string            `json:"source_type"`
	SourceFile             string            `json:"source_file,omitempty"`
	ImportID               string            `json:"import_id"`
	ConversationID         string            `json:"conversation_id"`
	ConversationTitle      string            `json:"conversation_title,omitempty"`
	MessageID              string            `json:"message_id,omitempty"`
	ParentID               string            `json:"parent_id,omitempty"`
	Timestamp              string            `json:"timestamp,omitempty"`
	Speaker                string            `json:"speaker"`
	Role                   string            `json:"role"`
	Project                string            `json:"project,omitempty"`
	Text                   string            `json:"text"`
	Metadata               map[string]string `json:"metadata,omitempty"`
	UserStarred            bool              `json:"user_starred,omitempty"`
	UserShared             bool              `json:"user_shared,omitempty"`
	ShareID                string            `json:"share_id,omitempty"`
	FeedbackRating         string            `json:"feedback_rating,omitempty"`
	FeedbackNote           string            `json:"feedback_note,omitempty"`
	FeedbackAt             string            `json:"feedback_at,omitempty"`
	AttachmentFilename     string            `json:"attachment_filename,omitempty"`
	AttachmentMimeType     string            `json:"attachment_mime_type,omitempty"`
	AttachmentCategory     string            `json:"attachment_category,omitempty"`
	AttachmentSourceSystem string            `json:"attachment_source_system,omitempty"`
}

type ChatGPTImportOptions struct {
	Project            string
	ConversationFilter string
	// ConversationID, when set, scopes the import to exactly this
	// conversation id (exact match, not a substring filter like
	// ConversationFilter). Useful for targeted debugging/verification of a
	// single known conversation instead of scanning the whole export.
	ConversationID string
	Since          time.Time
	SpeakerRules   SpeakerRules
	// Limit caps the number of conversations processed, in file order. Zero
	// or negative means no limit. Useful for a small-scale test ingest
	// before running against the full export.
	Limit int
	// Captioner resolves and captions attachments (images get a real vision
	// caption, other mime types get a metadata-only description). Nil
	// disables resolution entirely and preserves today's placeholder-stub
	// behavior for attachment-bearing messages.
	Captioner Captioner
	// DryRun, when true, still resolves attachments against the mapping
	// files (so AttachmentsWouldCaption is accurate) but never actually
	// invokes Captioner, so a dry run never spends on vision API calls.
	DryRun bool
}

type ChatGPTImportResult struct {
	SourcePath                      string
	ImportID                        string
	ConversationsFound              int
	MessagesFound                   int
	MessagesAccepted                int
	MessagesSkipped                 int
	RolesFound                      []string
	UnsupportedContentTypes         map[string]int
	AttachmentsAssetsDetected       int
	AttachmentsIngested             bool
	StarredConversations            int
	SharedConversations             int
	FeedbackConversations           int
	FeedbackThumbsUp                int
	FeedbackThumbsDown              int
	AttachmentsTotal                int
	AttachmentsResolvedAssetFiles   int
	AttachmentsResolvedLibraryFiles int
	AttachmentsUnresolved           int
	AttachmentsWouldCaption         int
	AttachmentsCaptioned            int
	AttachmentsCaptionFailed        int
	SourceType                      string
	Records                         []NormalizedMemoryRecord
}

type chatGPTSource struct {
	RootDir    string
	SourceType string
	cleanup    func()
}

type chatGPTNode struct {
	NodeID         string
	ParentID       string
	MessageID      string
	Role           string
	ContentType    string
	Text           string
	MessageTime    time.Time
	MessageCreate  string
	MessageUpdate  string
	HasAttachment  bool
	AttachmentRefs []string
	SupportedText  bool
	HasText        bool
}

type ShareSignal struct {
	ShareID     string
	IsAnonymous bool
}

type FeedbackSignal struct {
	Rating string
	Note   string
	At     string
}

type shareSignalRecord struct {
	ConversationID string `json:"conversation_id"`
	ID             string `json:"id"`
	IsAnonymous    bool   `json:"is_anonymous"`
}

type feedbackSignalRecord struct {
	ConversationID string `json:"conversation_id"`
	UpdateTime     any    `json:"update_time"`
	CreateTime     any    `json:"create_time"`
	Rating         string `json:"rating"`
	Content        string `json:"content"`
}

func ParseChatGPTExport(sourcePath string, options ChatGPTImportOptions) (ChatGPTImportResult, error) {
	rules := options.SpeakerRules
	if !rules.StoreAssistant && !rules.StoreUser && !rules.StoreSystem {
		rules = SpeakerRules{StoreAssistant: true, StoreUser: true, StoreSystem: false}
	}

	cleanSource := strings.TrimSpace(sourcePath)
	if cleanSource == "" {
		return ChatGPTImportResult{}, fmt.Errorf("source path is required")
	}

	resolvedSource, err := filepath.Abs(cleanSource)
	if err != nil {
		return ChatGPTImportResult{}, err
	}

	importID := buildImportID(resolvedSource)
	result := ChatGPTImportResult{
		SourcePath:              resolvedSource,
		ImportID:                importID,
		UnsupportedContentTypes: make(map[string]int),
		AttachmentsIngested:     false,
	}

	source, err := prepareChatGPTSource(resolvedSource)
	if err != nil {
		return ChatGPTImportResult{}, err
	}
	defer source.cleanup()
	result.SourceType = source.SourceType

	conversationFiles, err := findConversationJSONFiles(source.RootDir)
	if err != nil {
		return ChatGPTImportResult{}, err
	}

	shareSignals, err := LoadShareSignals(source.RootDir)
	if err != nil {
		return ChatGPTImportResult{}, err
	}
	feedbackSignals, err := LoadFeedbackSignals(source.RootDir)
	if err != nil {
		return ChatGPTImportResult{}, err
	}
	assetFileNames, err := LoadAssetFileNames(source.RootDir)
	if err != nil {
		return ChatGPTImportResult{}, err
	}
	libraryFiles, err := LoadLibraryFiles(source.RootDir)
	if err != nil {
		return ChatGPTImportResult{}, err
	}

	roles := make(map[string]struct{})
	records := make([]NormalizedMemoryRecord, 0)
	conversationFilter := strings.ToLower(strings.TrimSpace(options.ConversationFilter))
	conversationIDFilter := strings.TrimSpace(options.ConversationID)
	ctx := context.Background()

fileLoop:
	for _, conversationFile := range conversationFiles {
		conversations, err := parseConversationFile(conversationFile)
		if err != nil {
			continue
		}

		relFile, relErr := filepath.Rel(source.RootDir, conversationFile)
		if relErr != nil {
			relFile = filepath.Base(conversationFile)
		}
		relFile = filepath.ToSlash(relFile)

		for idx, conversation := range conversations {
			if options.Limit > 0 && result.ConversationsFound >= options.Limit {
				break fileLoop
			}
			conversationID := strings.TrimSpace(stringValue(conversation["id"]))
			if conversationID == "" {
				conversationID = fmt.Sprintf("conversation_%d", idx+1)
			}
			conversationTitle := strings.TrimSpace(stringValue(conversation["title"]))
			if conversationIDFilter != "" && conversationID != conversationIDFilter {
				continue
			}
			if conversationFilter != "" {
				haystack := strings.ToLower(conversationTitle + " " + conversationID)
				if !strings.Contains(haystack, conversationFilter) {
					continue
				}
			}
			result.ConversationsFound++

			conversationCreate := normalizeTimeString(conversation["create_time"])
			conversationUpdate := normalizeTimeString(conversation["update_time"])
			userStarred := boolValue(conversation["is_starred"])
			if userStarred {
				result.StarredConversations++
			}
			shareSignal, hasShareSignal := shareSignals[conversationID]
			if hasShareSignal {
				result.SharedConversations++
			}
			feedbackSignal, hasFeedbackSignal := feedbackSignals[conversationID]
			if hasFeedbackSignal {
				result.FeedbackConversations++
				switch feedbackSignal.Rating {
				case "thumbs_up":
					result.FeedbackThumbsUp++
				case "thumbs_down":
					result.FeedbackThumbsDown++
				}
			}

			nodes := collectMessageNodes(conversation)
			for _, node := range nodes {
				result.MessagesFound++
				if node.Role != "" {
					roles[node.Role] = struct{}{}
				}

				if node.HasAttachment {
					result.AttachmentsAssetsDetected++
				}

				if !node.SupportedText {
					unsupportedType := node.ContentType
					if unsupportedType == "" {
						unsupportedType = "unknown"
					}
					result.UnsupportedContentTypes[unsupportedType]++
					result.MessagesSkipped++
					continue
				}

				if !node.HasText {
					result.MessagesSkipped++
					continue
				}

				if !shouldStoreRole(node.Role, rules) {
					result.MessagesSkipped++
					continue
				}

				if !options.Since.IsZero() && !node.MessageTime.IsZero() && node.MessageTime.Before(options.Since) {
					result.MessagesSkipped++
					continue
				}

				metadata := map[string]string{
					"content_type": node.ContentType,
				}
				if conversationCreate != "" {
					metadata["conversation_create_time"] = conversationCreate
				}
				if conversationUpdate != "" {
					metadata["conversation_update_time"] = conversationUpdate
				}
				if node.MessageCreate != "" {
					metadata["message_create_time"] = node.MessageCreate
				}
				if node.MessageUpdate != "" {
					metadata["message_update_time"] = node.MessageUpdate
				}
				if len(node.AttachmentRefs) > 0 {
					metadata["attachment_refs"] = strings.Join(node.AttachmentRefs, ",")
					metadata["attachment_count"] = strconv.Itoa(len(node.AttachmentRefs))
				}

				nodeText := node.Text
				var chosenAttachment *ResolvedAttachment
				if len(node.AttachmentRefs) > 0 {
					nodeText, chosenAttachment = resolveAttachmentText(
						ctx, node, source.RootDir, assetFileNames, libraryFiles, options.Captioner, options.DryRun, &result,
					)
				}

				timestamp := ""
				if !node.MessageTime.IsZero() {
					timestamp = node.MessageTime.UTC().Format(time.RFC3339)
				}

				record := NormalizedMemoryRecord{
					SourcePlatform:    "chatgpt",
					SourceType:        source.SourceType,
					SourceFile:        relFile,
					ImportID:          importID,
					ConversationID:    conversationID,
					ConversationTitle: conversationTitle,
					MessageID:         node.MessageID,
					ParentID:          node.ParentID,
					Timestamp:         timestamp,
					Speaker:           node.Role,
					Role:              node.Role,
					Project:           strings.TrimSpace(options.Project),
					Text:              nodeText,
					Metadata:          metadata,
					UserStarred:       userStarred,
					UserShared:        hasShareSignal,
					ShareID:           shareSignal.ShareID,
					FeedbackRating:    feedbackSignal.Rating,
					FeedbackNote:      feedbackSignal.Note,
					FeedbackAt:        feedbackSignal.At,
				}
				if chosenAttachment != nil {
					record.AttachmentFilename = chosenAttachment.Filename
					record.AttachmentMimeType = chosenAttachment.MimeType
					record.AttachmentCategory = chosenAttachment.Category
					record.AttachmentSourceSystem = chosenAttachment.SourceSystem
				}

				records = append(records, record)
				if node.HasAttachment && (isAttachmentContentType(node.ContentType) || len(node.AttachmentRefs) > 0) {
					result.AttachmentsIngested = true
				}
				result.MessagesAccepted++
			}
		}
	}

	if result.ConversationsFound == 0 {
		return ChatGPTImportResult{}, fmt.Errorf("no compatible ChatGPT conversation data found in %s", resolvedSource)
	}

	result.Records = records
	result.RolesFound = sortedKeys(roles)

	if len(result.UnsupportedContentTypes) == 0 {
		result.UnsupportedContentTypes = map[string]int{}
	}

	return result, nil
}

func LoadShareSignals(rootDir string) (map[string]ShareSignal, error) {
	path := filepath.Join(rootDir, "shared_conversations.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]ShareSignal{}, nil
		}
		return nil, err
	}

	var rows []shareSignalRecord
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, fmt.Errorf("parse shared_conversations.json: %w", err)
	}

	out := make(map[string]ShareSignal, len(rows))
	for _, row := range rows {
		conversationID := strings.TrimSpace(row.ConversationID)
		if conversationID == "" {
			continue
		}
		out[conversationID] = ShareSignal{
			ShareID:     strings.TrimSpace(row.ID),
			IsAnonymous: row.IsAnonymous,
		}
	}
	return out, nil
}

func LoadFeedbackSignals(rootDir string) (map[string]FeedbackSignal, error) {
	path := filepath.Join(rootDir, "message_feedback.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]FeedbackSignal{}, nil
		}
		return nil, err
	}

	var rows []feedbackSignalRecord
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, fmt.Errorf("parse message_feedback.json: %w", err)
	}

	out := make(map[string]FeedbackSignal)
	latest := make(map[string]time.Time)
	for _, row := range rows {
		conversationID := strings.TrimSpace(row.ConversationID)
		if conversationID == "" {
			continue
		}

		updatedAt := parseTimeValue(row.UpdateTime)
		if updatedAt.IsZero() {
			updatedAt = parseTimeValue(row.CreateTime)
		}

		if currentLatest, ok := latest[conversationID]; ok {
			if !updatedAt.IsZero() && !updatedAt.After(currentLatest) {
				continue
			}
			if updatedAt.IsZero() && !currentLatest.IsZero() {
				continue
			}
		}

		latest[conversationID] = updatedAt
		out[conversationID] = FeedbackSignal{
			Rating: strings.TrimSpace(row.Rating),
			Note:   parseFeedbackNote(row.Content),
			At:     normalizeTimeString(row.UpdateTime),
		}
		if out[conversationID].At == "" {
			out[conversationID] = FeedbackSignal{
				Rating: strings.TrimSpace(row.Rating),
				Note:   parseFeedbackNote(row.Content),
				At:     normalizeTimeString(row.CreateTime),
			}
		}
	}
	return out, nil
}

func WriteNormalizedJSONL(path string, records []NormalizedMemoryRecord) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return fmt.Errorf("output path is required")
	}

	parent := filepath.Dir(trimmed)
	if parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}

	file, err := os.Create(trimmed)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return nil
}

func ToMessageRecords(records []NormalizedMemoryRecord) []MessageRecord {
	out := make([]MessageRecord, 0, len(records))
	for _, record := range records {
		timestamp := strings.TrimSpace(record.Timestamp)
		if timestamp == "" {
			timestamp = time.Now().UTC().Format(time.RFC3339)
		}
		out = append(out, MessageRecord{
			ConversationID:         record.ConversationID,
			Timestamp:              timestamp,
			Speaker:                record.Speaker,
			Text:                   record.Text,
			SourceType:             record.SourceType,
			SourceTitle:            record.ConversationTitle,
			Project:                record.Project,
			UserStarred:            record.UserStarred,
			UserShared:             record.UserShared,
			ShareID:                record.ShareID,
			FeedbackRating:         record.FeedbackRating,
			FeedbackNote:           record.FeedbackNote,
			FeedbackAt:             record.FeedbackAt,
			AttachmentFilename:     record.AttachmentFilename,
			AttachmentMimeType:     record.AttachmentMimeType,
			AttachmentCategory:     record.AttachmentCategory,
			AttachmentSourceSystem: record.AttachmentSourceSystem,
		})
	}
	return out
}

func prepareChatGPTSource(sourcePath string) (chatGPTSource, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return chatGPTSource{}, err
	}

	if info.IsDir() {
		return chatGPTSource{
			RootDir:    sourcePath,
			SourceType: "chat_export_folder",
			cleanup:    func() {},
		}, nil
	}

	if strings.EqualFold(filepath.Ext(sourcePath), ".zip") {
		tempDir, err := extractZipToTemp(sourcePath)
		if err != nil {
			return chatGPTSource{}, err
		}
		return chatGPTSource{
			RootDir:    tempDir,
			SourceType: "chat_export_zip",
			cleanup: func() {
				_ = os.RemoveAll(tempDir)
			},
		}, nil
	}

	return chatGPTSource{}, fmt.Errorf("unsupported source path %q: expected .zip file or directory", sourcePath)
}

func extractZipToTemp(zipPath string) (string, error) {
	file, err := os.Open(zipPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return "", err
	}

	reader, err := zip.NewReader(file, fileInfo.Size())
	if err != nil {
		return "", err
	}

	tempDir, err := os.MkdirTemp("", "frontpocket-chatgpt-")
	if err != nil {
		return "", err
	}

	for _, entry := range reader.File {
		entryName := strings.TrimSpace(entry.Name)
		if entryName == "" {
			continue
		}

		if filepath.IsAbs(entryName) {
			_ = os.RemoveAll(tempDir)
			return "", fmt.Errorf("zip entry %q is not allowed", entryName)
		}

		cleanName := filepath.Clean(entryName)
		if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, "../") || strings.HasPrefix(cleanName, "..\\") {
			_ = os.RemoveAll(tempDir)
			return "", fmt.Errorf("zip entry %q is not allowed", entryName)
		}

		targetPath := filepath.Join(tempDir, cleanName)
		rel, relErr := filepath.Rel(tempDir, targetPath)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			_ = os.RemoveAll(tempDir)
			return "", fmt.Errorf("zip entry %q escapes extraction directory", entryName)
		}

		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				_ = os.RemoveAll(tempDir)
				return "", err
			}
			continue
		}

		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			_ = os.RemoveAll(tempDir)
			return "", fmt.Errorf("zip entry %q symlinks are not allowed", entryName)
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			_ = os.RemoveAll(tempDir)
			return "", err
		}

		source, err := entry.Open()
		if err != nil {
			_ = os.RemoveAll(tempDir)
			return "", err
		}

		target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			_ = source.Close()
			_ = os.RemoveAll(tempDir)
			return "", err
		}

		if _, err := io.Copy(target, source); err != nil {
			_ = source.Close()
			_ = target.Close()
			_ = os.RemoveAll(tempDir)
			return "", err
		}

		_ = source.Close()
		_ = target.Close()
	}

	return tempDir, nil
}

func findConversationJSONFiles(root string) ([]string, error) {
	jsonFiles := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".json") {
			jsonFiles = append(jsonFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(jsonFiles) == 0 {
		return nil, fmt.Errorf("no JSON files found in %s", root)
	}

	sort.Strings(jsonFiles)
	return jsonFiles, nil
}

func parseConversationFile(path string) ([]map[string]any, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}

	conversations := extractConversationObjects(decoded)
	if len(conversations) == 0 {
		return nil, fmt.Errorf("no conversation objects found")
	}
	return conversations, nil
}

func extractConversationObjects(payload any) []map[string]any {
	out := make([]map[string]any, 0)
	switch data := payload.(type) {
	case []any:
		for _, item := range data {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if _, ok := itemMap["mapping"]; ok {
				out = append(out, itemMap)
			}
		}
	case map[string]any:
		if _, ok := data["mapping"]; ok {
			out = append(out, data)
		}
		if conversations, ok := data["conversations"].([]any); ok {
			for _, item := range conversations {
				itemMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if _, ok := itemMap["mapping"]; ok {
					out = append(out, itemMap)
				}
			}
		}
	}
	return out
}

func collectMessageNodes(conversation map[string]any) []chatGPTNode {
	rawMapping, ok := conversation["mapping"].(map[string]any)
	if !ok {
		return nil
	}

	nodes := make([]chatGPTNode, 0, len(rawMapping))
	for nodeID, rawNode := range rawMapping {
		nodeMap, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}

		messageMap, ok := nodeMap["message"].(map[string]any)
		if !ok {
			continue
		}

		role := normalizeRole(stringValue(mapValue(messageMap, "author", "role")))
		if role == "" {
			role = "user"
		}

		contentMap, _ := messageMap["content"].(map[string]any)
		contentType := strings.ToLower(strings.TrimSpace(stringValue(mapValue(contentMap, "content_type"))))
		if contentType == "" {
			contentType = "text"
		}
		attachmentRefs := extractAttachmentRefs(nodeMap, messageMap, contentMap)
		hasAttachment := len(attachmentRefs) > 0 || detectAttachments(nodeMap, messageMap, contentMap)
		text, supported := extractMessageText(contentMap, contentType, attachmentRefs, hasAttachment)
		hasText := strings.TrimSpace(text) != ""

		messageCreate := normalizeTimeString(messageMap["create_time"])
		messageUpdate := normalizeTimeString(messageMap["update_time"])

		messageTime := parseTimeValue(messageMap["create_time"])
		if messageTime.IsZero() {
			messageTime = parseTimeValue(messageMap["update_time"])
		}
		if messageTime.IsZero() {
			messageTime = parseTimeValue(conversation["create_time"])
		}

		nodes = append(nodes, chatGPTNode{
			NodeID:         nodeID,
			ParentID:       strings.TrimSpace(stringValue(nodeMap["parent"])),
			MessageID:      strings.TrimSpace(stringValue(messageMap["id"])),
			Role:           role,
			ContentType:    contentType,
			Text:           strings.TrimSpace(text),
			MessageTime:    messageTime,
			MessageCreate:  messageCreate,
			MessageUpdate:  messageUpdate,
			HasAttachment:  hasAttachment,
			AttachmentRefs: attachmentRefs,
			SupportedText:  supported,
			HasText:        hasText,
		})
	}

	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].MessageTime.Equal(nodes[j].MessageTime) {
			if nodes[i].NodeID == nodes[j].NodeID {
				return nodes[i].MessageID < nodes[j].MessageID
			}
			return nodes[i].NodeID < nodes[j].NodeID
		}
		if nodes[i].MessageTime.IsZero() {
			return false
		}
		if nodes[j].MessageTime.IsZero() {
			return true
		}
		return nodes[i].MessageTime.Before(nodes[j].MessageTime)
	})

	return nodes
}

func isAttachmentContentType(contentType string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(contentType))
	return strings.Contains(trimmed, "asset") || strings.Contains(trimmed, "image")
}

func extractAttachmentRefs(node map[string]any, message map[string]any, content map[string]any) []string {
	refs := make(map[string]string)
	appendAttachmentRefsFromValue(refs, node["asset_pointer"])
	appendAttachmentRefsFromValue(refs, node["attachments"])
	appendAttachmentRefsFromValue(refs, node["assets"])
	appendAttachmentRefsFromValue(refs, message["asset_pointer"])
	appendAttachmentRefsFromValue(refs, message["attachments"])
	appendAttachmentRefsFromValue(refs, message["assets"])

	metadata, _ := message["metadata"].(map[string]any)
	appendAttachmentRefsFromValue(refs, metadata["asset_pointer"])
	appendAttachmentRefsFromValue(refs, metadata["attachments"])
	appendAttachmentRefsFromValue(refs, metadata["assets"])

	parts, _ := content["parts"].([]any)
	for _, part := range parts {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		appendAttachmentRefsFromValue(refs, partMap["asset_pointer"])
		appendAttachmentRefsFromValue(refs, partMap["asset"])
		appendAttachmentRefsFromValue(refs, partMap["file_id"])
		appendAttachmentRefsFromValue(refs, partMap["attachments"])
	}

	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

// appendAttachmentRefsFromValue collects attachment reference strings,
// deduping by normalized (scheme-stripped) form. The same underlying asset
// often appears twice in a message — once as a bare ID (e.g. via a nested
// "id" field) and once scheme-qualified (e.g. "file-service://<id>" from
// asset_pointer) — and both forms resolve to the same file, so without this
// normalization they'd be double-counted as two distinct attachments.
func appendAttachmentRefsFromValue(refs map[string]string, value any) {
	switch v := value.(type) {
	case nil:
		return
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return
		}
		key := normalizeAttachmentRef(trimmed)
		if key == "" {
			key = trimmed
		}
		if _, exists := refs[key]; !exists {
			refs[key] = trimmed
		}
	case []any:
		for _, item := range v {
			appendAttachmentRefsFromValue(refs, item)
		}
	case map[string]any:
		appendAttachmentRefsFromValue(refs, v["asset_pointer"])
		appendAttachmentRefsFromValue(refs, v["asset"])
		appendAttachmentRefsFromValue(refs, v["file_id"])
		appendAttachmentRefsFromValue(refs, v["id"])
		appendAttachmentRefsFromValue(refs, v["attachments"])
		appendAttachmentRefsFromValue(refs, v["assets"])
	}
}

func extractTextParts(content map[string]any) []string {
	parts, ok := content["parts"].([]any)
	if !ok {
		return nil
	}

	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		textPart, ok := part.(string)
		if !ok {
			continue
		}
		textPart = strings.TrimSpace(textPart)
		if textPart == "" {
			continue
		}
		lines = append(lines, textPart)
	}
	return lines
}

func buildAttachmentText(contentType string, lines []string, refs []string) string {
	out := make([]string, 0, len(lines)+2)
	out = append(out, lines...)
	if len(refs) > 0 {
		out = append(out, "attachment_refs: "+strings.Join(refs, ", "))
	}
	if len(out) == 0 {
		out = append(out, "attachment content: "+contentType)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// resolveAttachmentText resolves a node's attachment references against the
// export's mapping files, tallying resolution/captioning counters on
// result, and returns the text that should actually be stored/embedded for
// this message plus the attachment chosen to represent it (the first
// resolved reference; most attachment-bearing messages carry exactly one).
// If nothing resolves, or captioner is nil, it falls back to the original
// placeholder-stub text from node.Text so existing behavior for
// unresolvable references is preserved.
func resolveAttachmentText(
	ctx context.Context,
	node chatGPTNode,
	rootDir string,
	assetNames map[string]string,
	libraryFiles map[string]LibraryFileEntry,
	captioner Captioner,
	dryRun bool,
	result *ChatGPTImportResult,
) (string, *ResolvedAttachment) {
	var chosen *ResolvedAttachment

	for _, ref := range node.AttachmentRefs {
		result.AttachmentsTotal++
		attachment, ok := ResolveAttachment(ref, rootDir, assetNames, libraryFiles)
		if !ok {
			result.AttachmentsUnresolved++
			continue
		}

		switch attachment.SourceSystem {
		case AttachmentSourceAssetFileNames:
			result.AttachmentsResolvedAssetFiles++
		case AttachmentSourceLibraryFiles:
			result.AttachmentsResolvedLibraryFiles++
		}
		if strings.HasPrefix(strings.ToLower(attachment.MimeType), "image/") {
			result.AttachmentsWouldCaption++
		}
		if chosen == nil {
			chosen = attachment
		}
	}

	if chosen == nil {
		return node.Text, nil
	}
	if captioner == nil || dryRun {
		return node.Text, chosen
	}

	caption, err := captioner.CaptionImage(ctx, *chosen)
	if err != nil {
		result.AttachmentsCaptionFailed++
		return node.Text, chosen
	}
	result.AttachmentsCaptioned++
	return caption, chosen
}

func extractMessageText(content map[string]any, contentType string, attachmentRefs []string, hasAttachment bool) (string, bool) {
	lines := extractTextParts(content)
	if contentType == "text" {
		if len(lines) == 0 {
			// A real export shape: a pasted-file upload shows up as
			// content_type "text" with an empty parts array — the actual
			// reference lives only in message.metadata.attachments. Without
			// this check, such messages silently vanish (empty text ->
			// HasText false -> skipped) before ever reaching attachment
			// resolution, even though a real attachment is present.
			if hasAttachment {
				return buildAttachmentText(contentType, lines, attachmentRefs), true
			}
			return "", true
		}
		return strings.Join(lines, "\n"), true
	}
	if isAttachmentContentType(contentType) || hasAttachment {
		return buildAttachmentText(contentType, lines, attachmentRefs), true
	}
	return "", false
}

func detectAttachments(node map[string]any, message map[string]any, content map[string]any) bool {
	if hasKey(node, "attachments") || hasKey(node, "assets") || hasKey(node, "asset_pointer") {
		return true
	}
	if hasKey(message, "attachments") || hasKey(message, "assets") || hasKey(message, "asset_pointer") {
		return true
	}

	metadata, _ := message["metadata"].(map[string]any)
	if hasKey(metadata, "attachments") || hasKey(metadata, "assets") || hasKey(metadata, "asset_pointer") {
		return true
	}

	contentType := strings.ToLower(strings.TrimSpace(stringValue(mapValue(content, "content_type"))))
	if strings.Contains(contentType, "asset") || strings.Contains(contentType, "image") {
		return true
	}

	parts, ok := content["parts"].([]any)
	if !ok {
		return false
	}
	for _, part := range parts {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if hasKey(partMap, "asset_pointer") || hasKey(partMap, "asset") || hasKey(partMap, "attachments") || hasKey(partMap, "file_id") {
			return true
		}
	}

	return false
}

func shouldStoreRole(role string, rules SpeakerRules) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return rules.StoreAssistant
	case "system", "tool":
		return rules.StoreSystem
	case "user":
		return rules.StoreUser
	default:
		return rules.StoreUser
	}
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	case "tool":
		return "tool"
	default:
		return strings.ToLower(strings.TrimSpace(role))
	}
}

func parseTimeValue(value any) time.Time {
	switch v := value.(type) {
	case nil:
		return time.Time{}
	case float64:
		seconds := int64(v)
		nanos := int64((v - float64(seconds)) * float64(time.Second))
		return time.Unix(seconds, nanos).UTC()
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return time.Time{}
		}
		seconds := int64(f)
		nanos := int64((f - float64(seconds)) * float64(time.Second))
		return time.Unix(seconds, nanos).UTC()
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return time.Time{}
		}
		if asFloat, err := strconv.ParseFloat(raw, 64); err == nil {
			seconds := int64(asFloat)
			nanos := int64((asFloat - float64(seconds)) * float64(time.Second))
			return time.Unix(seconds, nanos).UTC()
		}
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func normalizeTimeString(value any) string {
	ts := parseTimeValue(value)
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}

func buildImportID(sourcePath string) string {
	seed := fmt.Sprintf("%s|%d", sourcePath, time.Now().UTC().UnixNano())
	sum := sha256.Sum256([]byte(seed))
	return "chatgpt_" + hex.EncodeToString(sum[:8])
}

func boolValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return false
		}
		return parsed
	default:
		return false
	}
}

func parseFeedbackNote(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(trimmed), &content); err != nil {
		return ""
	}
	return strings.TrimSpace(content.Text)
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func mapValue(m map[string]any, keys ...string) any {
	current := any(m)
	for _, key := range keys {
		next, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = next[key]
	}
	return current
}

func hasKey(m map[string]any, key string) bool {
	if len(m) == 0 {
		return false
	}
	_, ok := m[key]
	return ok
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
