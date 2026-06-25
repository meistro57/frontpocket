package memory

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	SourcePlatform    string            `json:"source_platform"`
	SourceType        string            `json:"source_type"`
	SourceFile        string            `json:"source_file,omitempty"`
	ImportID          string            `json:"import_id"`
	ConversationID    string            `json:"conversation_id"`
	ConversationTitle string            `json:"conversation_title,omitempty"`
	MessageID         string            `json:"message_id,omitempty"`
	ParentID          string            `json:"parent_id,omitempty"`
	Timestamp         string            `json:"timestamp,omitempty"`
	Speaker           string            `json:"speaker"`
	Role              string            `json:"role"`
	Project           string            `json:"project,omitempty"`
	Text              string            `json:"text"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type ChatGPTImportOptions struct {
	Project            string
	ConversationFilter string
	Since              time.Time
	SpeakerRules       SpeakerRules
}

type ChatGPTImportResult struct {
	SourcePath                string
	ImportID                  string
	ConversationsFound        int
	MessagesFound             int
	MessagesAccepted          int
	MessagesSkipped           int
	RolesFound                []string
	UnsupportedContentTypes   map[string]int
	AttachmentsAssetsDetected int
	AttachmentsIngested       bool
	SourceType                string
	Records                   []NormalizedMemoryRecord
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

	roles := make(map[string]struct{})
	records := make([]NormalizedMemoryRecord, 0)
	conversationFilter := strings.ToLower(strings.TrimSpace(options.ConversationFilter))

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
			conversationID := strings.TrimSpace(stringValue(conversation["id"]))
			if conversationID == "" {
				conversationID = fmt.Sprintf("conversation_%d", idx+1)
			}
			conversationTitle := strings.TrimSpace(stringValue(conversation["title"]))
			if conversationFilter != "" {
				haystack := strings.ToLower(conversationTitle + " " + conversationID)
				if !strings.Contains(haystack, conversationFilter) {
					continue
				}
			}
			result.ConversationsFound++

			conversationCreate := normalizeTimeString(conversation["create_time"])
			conversationUpdate := normalizeTimeString(conversation["update_time"])

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
					Text:              node.Text,
					Metadata:          metadata,
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
			ConversationID: record.ConversationID,
			Timestamp:      timestamp,
			Speaker:        record.Speaker,
			Text:           record.Text,
			SourceType:     record.SourceType,
			SourceTitle:    record.ConversationTitle,
			Project:        record.Project,
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
	refs := make(map[string]struct{})
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
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func appendAttachmentRefsFromValue(refs map[string]struct{}, value any) {
	switch v := value.(type) {
	case nil:
		return
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			refs[trimmed] = struct{}{}
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

func extractMessageText(content map[string]any, contentType string, attachmentRefs []string, hasAttachment bool) (string, bool) {
	lines := extractTextParts(content)
	if contentType == "text" {
		if len(lines) == 0 {
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
