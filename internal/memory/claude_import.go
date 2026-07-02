package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const claudeRootParentUUID = "00000000-0000-4000-8000-000000000000"

type ClaudeImportOptions struct {
	Project            string
	ConversationFilter string
	ConversationID     string
	SpeakerRules       SpeakerRules
	Limit              int
	AIProvider         string
}

type ClaudeImportResult struct {
	SourcePath              string
	ImportID                string
	ConversationsFound      int
	MessagesFound           int
	MessagesAccepted        int
	MessagesSkipped         int
	RolesFound              []string
	ContentTypesEncountered map[string]int
	UnsupportedContentTypes map[string]int
	BranchingConversations  int
	BranchParentOccurrences int
	MaxChildrenForParent    int
	MemoriesPoints          int
	ProjectsPoints          int
	SourceType              string
	Records                 []NormalizedMemoryRecord
}

type claudeConversation struct {
	UUID         string              `json:"uuid"`
	Name         string              `json:"name"`
	Summary      string              `json:"summary"`
	CreatedAt    string              `json:"created_at"`
	UpdatedAt    string              `json:"updated_at"`
	ChatMessages []claudeChatMessage `json:"chat_messages"`
}

type claudeChatMessage struct {
	UUID              string           `json:"uuid"`
	Text              string           `json:"text"`
	Content           []map[string]any `json:"content"`
	Sender            string           `json:"sender"`
	Attachments       []map[string]any `json:"attachments"`
	Files             []map[string]any `json:"files"`
	ParentMessageUUID string           `json:"parent_message_uuid"`
	CreatedAt         string           `json:"created_at"`
	UpdatedAt         string           `json:"updated_at"`
}

type claudeMemoriesRow struct {
	ConversationsMemory string            `json:"conversations_memory"`
	ProjectMemories     map[string]string `json:"project_memories"`
	AccountUUID         string            `json:"account_uuid"`
}

type claudeProjectFile struct {
	UUID           string             `json:"uuid"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	PromptTemplate string             `json:"prompt_template"`
	CreatedAt      string             `json:"created_at"`
	UpdatedAt      string             `json:"updated_at"`
	Docs           []claudeProjectDoc `json:"docs"`
}

type claudeProjectDoc struct {
	UUID     string `json:"uuid"`
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

func ParseClaudeExport(sourcePath string, options ClaudeImportOptions) (ClaudeImportResult, error) {
	rules := options.SpeakerRules
	if !rules.StoreAssistant && !rules.StoreUser && !rules.StoreSystem {
		rules = SpeakerRules{StoreAssistant: true, StoreUser: true, StoreSystem: false}
	}

	cleanSource := strings.TrimSpace(sourcePath)
	if cleanSource == "" {
		return ClaudeImportResult{}, fmt.Errorf("source path is required")
	}
	resolvedSource, err := filepath.Abs(cleanSource)
	if err != nil {
		return ClaudeImportResult{}, err
	}
	info, err := os.Stat(resolvedSource)
	if err != nil {
		return ClaudeImportResult{}, err
	}
	if !info.IsDir() {
		return ClaudeImportResult{}, fmt.Errorf("unsupported source path %q: expected export directory", sourcePath)
	}

	aiProvider := strings.TrimSpace(options.AIProvider)
	if aiProvider == "" {
		aiProvider = "claude"
	}

	result := ClaudeImportResult{
		SourcePath:              resolvedSource,
		ImportID:                buildClaudeImportID(resolvedSource),
		ContentTypesEncountered: make(map[string]int),
		UnsupportedContentTypes: make(map[string]int),
		SourceType:              "chat_export_folder",
	}

	projects, projectByID, err := loadClaudeProjects(resolvedSource)
	if err != nil {
		return ClaudeImportResult{}, err
	}

	records := make([]NormalizedMemoryRecord, 0, 1024)

	memoryRecords, memoriesCount, err := parseClaudeMemories(resolvedSource, result.ImportID, options.Project, aiProvider, projectByID)
	if err != nil {
		return ClaudeImportResult{}, err
	}
	records = append(records, memoryRecords...)
	result.MemoriesPoints = memoriesCount

	projectRecords := buildClaudeProjectRecords(projects, result.ImportID, options.Project, aiProvider)
	records = append(records, projectRecords...)
	result.ProjectsPoints = len(projectRecords)

	conversationPayload, err := os.ReadFile(filepath.Join(resolvedSource, "conversations.json"))
	if err != nil {
		return ClaudeImportResult{}, err
	}
	var conversations []claudeConversation
	if err := json.Unmarshal(conversationPayload, &conversations); err != nil {
		return ClaudeImportResult{}, fmt.Errorf("parse conversations.json: %w", err)
	}

	roles := make(map[string]struct{})
	conversationFilter := strings.ToLower(strings.TrimSpace(options.ConversationFilter))
	conversationIDFilter := strings.TrimSpace(options.ConversationID)

	for _, conversation := range conversations {
		if options.Limit > 0 && result.ConversationsFound >= options.Limit {
			break
		}
		conversationID := strings.TrimSpace(conversation.UUID)
		if conversationID == "" {
			continue
		}
		conversationTitle := strings.TrimSpace(conversation.Name)
		if conversationIDFilter != "" && conversationID != conversationIDFilter {
			continue
		}
		if conversationFilter != "" {
			haystack := strings.ToLower(conversationTitle + " " + conversationID)
			if !strings.Contains(haystack, conversationFilter) {
				continue
			}
		}

		orderedMessages, branchParents, maxChildren := orderClaudeMessages(conversation.ChatMessages)
		if branchParents > 0 {
			result.BranchingConversations++
			result.BranchParentOccurrences += branchParents
			if maxChildren > result.MaxChildrenForParent {
				result.MaxChildrenForParent = maxChildren
			}
		}

		result.ConversationsFound++
		for _, message := range orderedMessages {
			result.MessagesFound++
			speaker := normalizeClaudeSender(message.Sender)
			if speaker != "" {
				roles[speaker] = struct{}{}
			}
			if !shouldStoreRole(speaker, rules) {
				result.MessagesSkipped++
				continue
			}

			text := extractClaudeMessageText(message, &result)
			if strings.TrimSpace(text) == "" {
				result.MessagesSkipped++
				continue
			}
			tags := claudeMessageTags(message)

			timestamp := claudeMessageTimestamp(message, conversation)
			records = append(records, NormalizedMemoryRecord{
				SourcePlatform:    "claude",
				SourceType:        result.SourceType,
				SourceFile:        "conversations.json",
				ImportID:          result.ImportID,
				ConversationID:    conversationID,
				ConversationTitle: defaultValue(conversationTitle, "Untitled Conversation"),
				MessageID:         strings.TrimSpace(message.UUID),
				ParentID:          strings.TrimSpace(message.ParentMessageUUID),
				Timestamp:         timestamp,
				Speaker:           speaker,
				Role:              speaker,
				Project:           strings.TrimSpace(options.Project),
				MemoryKind:        KindChatTurn,
				Tags:              tags,
				Text:              text,
				AIProvider:        aiProvider,
			})
			result.MessagesAccepted++
		}
	}

	if result.ConversationsFound == 0 {
		return ClaudeImportResult{}, fmt.Errorf("no Claude conversations found in %s", resolvedSource)
	}

	if len(result.ContentTypesEncountered) == 0 {
		result.ContentTypesEncountered = map[string]int{}
	}
	if len(result.UnsupportedContentTypes) == 0 {
		result.UnsupportedContentTypes = map[string]int{}
	}

	result.Records = records
	result.RolesFound = sortedKeys(roles)
	return result, nil
}

func buildClaudeImportID(sourcePath string) string {
	seed := fmt.Sprintf("%s|%d", sourcePath, time.Now().UTC().UnixNano())
	sum := sha256.Sum256([]byte(seed))
	return "claude_" + hex.EncodeToString(sum[:8])
}

func loadClaudeProjects(rootDir string) ([]claudeProjectFile, map[string]claudeProjectFile, error) {
	matches, err := filepath.Glob(filepath.Join(rootDir, "projects", "*.json"))
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(matches)
	projects := make([]claudeProjectFile, 0, len(matches))
	projectByID := make(map[string]claudeProjectFile, len(matches))
	for _, match := range matches {
		payload, err := os.ReadFile(match)
		if err != nil {
			return nil, nil, err
		}
		var project claudeProjectFile
		if err := json.Unmarshal(payload, &project); err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", filepath.Base(match), err)
		}
		projects = append(projects, project)
		if strings.TrimSpace(project.UUID) != "" {
			projectByID[strings.TrimSpace(project.UUID)] = project
		}
	}
	return projects, projectByID, nil
}

func parseClaudeMemories(rootDir, importID, projectLabel, aiProvider string, projectByID map[string]claudeProjectFile) ([]NormalizedMemoryRecord, int, error) {
	payload, err := os.ReadFile(filepath.Join(rootDir, "memories.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	var rows []claudeMemoriesRow
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, 0, fmt.Errorf("parse memories.json: %w", err)
	}
	if len(rows) == 0 {
		return nil, 0, nil
	}

	records := make([]NormalizedMemoryRecord, 0, 1+len(rows[0].ProjectMemories))
	first := rows[0]
	now := time.Now().UTC().Format(time.RFC3339)
	if text := strings.TrimSpace(first.ConversationsMemory); text != "" {
		records = append(records, NormalizedMemoryRecord{
			SourcePlatform:    "claude",
			SourceType:        "chat_export_folder",
			SourceFile:        "memories.json",
			ImportID:          importID,
			ConversationID:    "claude_conversations_memory",
			ConversationTitle: "Claude Conversations Memory",
			Timestamp:         now,
			Speaker:           "assistant",
			Role:              "assistant",
			Project:           strings.TrimSpace(projectLabel),
			MemoryKind:        "ai_generated_synthesis",
			Tags:              []string{"claude", "synthesis", "conversations_memory"},
			Text:              text,
			AIProvider:        aiProvider,
		})
	}

	projectIDs := make([]string, 0, len(first.ProjectMemories))
	for projectID := range first.ProjectMemories {
		projectIDs = append(projectIDs, projectID)
	}
	sort.Strings(projectIDs)
	for _, projectID := range projectIDs {
		text := strings.TrimSpace(first.ProjectMemories[projectID])
		if text == "" {
			continue
		}
		projectName := strings.TrimSpace(projectByID[projectID].Name)
		if projectName == "" {
			projectName = projectID
		}
		records = append(records, NormalizedMemoryRecord{
			SourcePlatform:    "claude",
			SourceType:        "chat_export_folder",
			SourceFile:        "memories.json",
			ImportID:          importID,
			ConversationID:    strings.TrimSpace(projectID),
			ConversationTitle: "Claude Project Memory: " + projectName,
			Timestamp:         now,
			Speaker:           "assistant",
			Role:              "assistant",
			Project:           projectName,
			MemoryKind:        "ai_generated_synthesis",
			Tags:              []string{"claude", "synthesis", "project_memory", strings.TrimSpace(projectID)},
			Text:              text,
			AIProvider:        aiProvider,
		})
	}
	return records, len(records), nil
}

func buildClaudeProjectRecords(projects []claudeProjectFile, importID, projectLabel, aiProvider string) []NormalizedMemoryRecord {
	records := make([]NormalizedMemoryRecord, 0, len(projects))
	for _, project := range projects {
		segments := make([]string, 0, 4)
		if name := strings.TrimSpace(project.Name); name != "" {
			segments = append(segments, name)
		}
		if desc := strings.TrimSpace(project.Description); desc != "" {
			segments = append(segments, desc)
		}
		if prompt := strings.TrimSpace(project.PromptTemplate); prompt != "" {
			segments = append(segments, prompt)
		}
		if len(project.Docs) > 0 {
			docNames := make([]string, 0, len(project.Docs))
			for _, doc := range project.Docs {
				if name := strings.TrimSpace(doc.Filename); name != "" {
					docNames = append(docNames, name)
				}
			}
			if len(docNames) > 0 {
				segments = append(segments, "project_docs: "+strings.Join(docNames, ", "))
			}
		}
		text := strings.TrimSpace(strings.Join(segments, "\n\n"))
		if text == "" {
			continue
		}

		timestamp := strings.TrimSpace(project.UpdatedAt)
		if timestamp == "" {
			timestamp = strings.TrimSpace(project.CreatedAt)
		}
		if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
			timestamp = time.Now().UTC().Format(time.RFC3339)
		}

		records = append(records, NormalizedMemoryRecord{
			SourcePlatform:    "claude",
			SourceType:        "chat_export_folder",
			SourceFile:        filepath.ToSlash(filepath.Join("projects", strings.TrimSpace(project.UUID)+".json")),
			ImportID:          importID,
			ConversationID:    strings.TrimSpace(project.UUID),
			ConversationTitle: defaultValue(strings.TrimSpace(project.Name), strings.TrimSpace(project.UUID)),
			Timestamp:         timestamp,
			Speaker:           "assistant",
			Role:              "assistant",
			Project:           defaultValue(strings.TrimSpace(project.Name), strings.TrimSpace(projectLabel)),
			MemoryKind:        "project_config",
			Tags:              []string{"claude", "project", strings.TrimSpace(project.UUID)},
			Text:              text,
			AIProvider:        aiProvider,
		})
	}
	return records
}

func orderClaudeMessages(messages []claudeChatMessage) ([]claudeChatMessage, int, int) {
	if len(messages) == 0 {
		return nil, 0, 0
	}

	childrenByParent := make(map[string][]claudeChatMessage)
	byID := make(map[string]claudeChatMessage, len(messages))
	for _, message := range messages {
		msgID := strings.TrimSpace(message.UUID)
		if msgID == "" {
			continue
		}
		byID[msgID] = message
		parentID := strings.TrimSpace(message.ParentMessageUUID)
		if parentID == "" {
			parentID = claudeRootParentUUID
		}
		childrenByParent[parentID] = append(childrenByParent[parentID], message)
	}

	branchParents := 0
	maxChildren := 0
	for parentID, children := range childrenByParent {
		sort.SliceStable(children, func(i, j int) bool {
			left := claudeMessageSortTime(children[i])
			right := claudeMessageSortTime(children[j])
			if left.Equal(right) {
				return strings.TrimSpace(children[i].UUID) < strings.TrimSpace(children[j].UUID)
			}
			if left.IsZero() {
				return false
			}
			if right.IsZero() {
				return true
			}
			return left.Before(right)
		})
		childrenByParent[parentID] = children
		if len(children) > 1 {
			branchParents++
			if len(children) > maxChildren {
				maxChildren = len(children)
			}
		}
	}

	ordered := make([]claudeChatMessage, 0, len(byID))
	visited := make(map[string]struct{}, len(byID))
	var walk func(parentID string)
	walk = func(parentID string) {
		for _, child := range childrenByParent[parentID] {
			id := strings.TrimSpace(child.UUID)
			if id == "" {
				continue
			}
			if _, seen := visited[id]; seen {
				continue
			}
			visited[id] = struct{}{}
			ordered = append(ordered, child)
			walk(id)
		}
	}
	walk(claudeRootParentUUID)

	if len(ordered) < len(byID) {
		remainder := make([]claudeChatMessage, 0, len(byID)-len(ordered))
		for id, message := range byID {
			if _, seen := visited[id]; seen {
				continue
			}
			remainder = append(remainder, message)
		}
		sort.SliceStable(remainder, func(i, j int) bool {
			left := claudeMessageSortTime(remainder[i])
			right := claudeMessageSortTime(remainder[j])
			if left.Equal(right) {
				return strings.TrimSpace(remainder[i].UUID) < strings.TrimSpace(remainder[j].UUID)
			}
			if left.IsZero() {
				return false
			}
			if right.IsZero() {
				return true
			}
			return left.Before(right)
		})
		ordered = append(ordered, remainder...)
	}

	return ordered, branchParents, maxChildren
}

func normalizeClaudeSender(sender string) string {
	switch strings.ToLower(strings.TrimSpace(sender)) {
	case "human":
		return "user"
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	default:
		return strings.ToLower(strings.TrimSpace(sender))
	}
}

func claudeMessageTimestamp(message claudeChatMessage, conversation claudeConversation) string {
	for _, raw := range []string{message.CreatedAt, message.UpdatedAt} {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return trimmed
		}
	}
	for _, block := range message.Content {
		for _, key := range []string{"start_timestamp", "stop_timestamp"} {
			raw := strings.TrimSpace(stringValue(block[key]))
			if raw == "" {
				continue
			}
			if _, err := time.Parse(time.RFC3339, raw); err == nil {
				return raw
			}
		}
	}
	for _, raw := range []string{conversation.UpdatedAt, conversation.CreatedAt} {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return trimmed
		}
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func claudeMessageSortTime(message claudeChatMessage) time.Time {
	ts := parseTimeValue(message.CreatedAt)
	if ts.IsZero() {
		ts = parseTimeValue(message.UpdatedAt)
	}
	if ts.IsZero() {
		for _, block := range message.Content {
			ts = parseTimeValue(block["start_timestamp"])
			if !ts.IsZero() {
				break
			}
			ts = parseTimeValue(block["stop_timestamp"])
			if !ts.IsZero() {
				break
			}
		}
	}
	return ts
}

func extractClaudeMessageText(message claudeChatMessage, result *ClaudeImportResult) string {
	seen := make(map[string]struct{})
	segments := make([]string, 0, 8)
	appendUnique := func(text string) {
		trimmed := strings.TrimSpace(normalizePotentiallyEscapedText(text))
		if trimmed == "" {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		segments = append(segments, trimmed)
	}

	appendUnique(message.Text)

	for _, block := range message.Content {
		contentType := strings.ToLower(strings.TrimSpace(stringValue(block["type"])))
		if contentType == "" {
			contentType = "unknown"
		}
		result.ContentTypesEncountered[contentType]++
		switch contentType {
		case "text":
			appendUnique(stringValue(block["text"]))
		case "voice_note":
			voice := strings.TrimSpace(strings.Join([]string{stringValue(block["title"]), stringValue(block["text"])}, "\n"))
			appendUnique(voice)
		case "tool_use":
			appendUnique("tool_use")
			appendUnique("tool_name: " + stringValue(block["name"]))
			appendUnique("tool_message: " + stringValue(block["message"]))
			appendUnique("tool_display: " + stringValue(block["display_content"]))
			appendToolPayload(block["input"], appendUnique)
		case "tool_result":
			appendUnique("tool_result")
			appendUnique("tool_name: " + stringValue(block["name"]))
			appendToolPayload(block["content"], appendUnique)
			appendToolPayload(block["structured_content"], appendUnique)
		case "thinking", "token_budget":
			result.UnsupportedContentTypes[contentType]++
		default:
			result.UnsupportedContentTypes[contentType]++
		}
	}

	for _, attachment := range message.Attachments {
		if extracted := strings.TrimSpace(stringValue(attachment["extracted_content"])); extracted != "" {
			appendUnique(extracted)
			continue
		}
		if fileName := strings.TrimSpace(stringValue(attachment["file_name"])); fileName != "" {
			appendUnique("attachment: " + fileName)
		}
	}
	for _, file := range message.Files {
		if fileName := strings.TrimSpace(stringValue(file["file_name"])); fileName != "" {
			appendUnique("file: " + fileName)
		}
	}

	return strings.TrimSpace(strings.Join(segments, "\n\n"))
}

func appendToolPayload(value any, appendUnique func(string)) {
	switch v := value.(type) {
	case nil:
		return
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return
		}
		decoded := normalizePotentiallyEscapedText(trimmed)
		if json.Valid([]byte(decoded)) {
			var nested any
			if err := json.Unmarshal([]byte(decoded), &nested); err == nil {
				appendToolPayload(nested, appendUnique)
				return
			}
		}
		appendUnique(decoded)
	case []any:
		for _, item := range v {
			appendToolPayload(item, appendUnique)
		}
	case map[string]any:
		priority := []string{"text", "content", "new_str", "old_str", "command", "title", "name", "id", "language", "type"}
		for _, key := range priority {
			if child, ok := v[key]; ok {
				appendToolPayload(child, appendUnique)
			}
		}
		rest := make([]string, 0, len(v))
		for key := range v {
			isPriority := false
			for _, pkey := range priority {
				if key == pkey {
					isPriority = true
					break
				}
			}
			if isPriority {
				continue
			}
			rest = append(rest, key)
		}
		sort.Strings(rest)
		for _, key := range rest {
			appendToolPayload(v[key], appendUnique)
		}
	default:
		appendUnique(fmt.Sprintf("%v", v))
	}
}

func normalizePotentiallyEscapedText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "\\n") || strings.Contains(trimmed, "\\t") || strings.Contains(trimmed, "\\r") {
		quoted := "\"" + strings.ReplaceAll(trimmed, "\"", "\\\"") + "\""
		if unquoted, err := strconv.Unquote(quoted); err == nil {
			return unquoted
		}
	}
	return trimmed
}

func claudeMessageTags(message claudeChatMessage) []string {
	tagSet := map[string]struct{}{}
	for _, block := range message.Content {
		typeName := strings.ToLower(strings.TrimSpace(stringValue(block["type"])))
		switch typeName {
		case "tool_use", "tool_result", "voice_note":
			tagSet["content_block:"+typeName] = struct{}{}
		}
	}
	if len(tagSet) == 0 {
		return nil
	}
	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}
