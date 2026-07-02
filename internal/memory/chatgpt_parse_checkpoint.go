package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const chatGPTParseCheckpointVersion = 1

type ChatGPTParseCheckpointMeta struct {
	Source            string `json:"source"`
	ConversationID    string `json:"conversation_id,omitempty"`
	ConversationMatch string `json:"conversation_match,omitempty"`
	Since             string `json:"since,omitempty"`
	Limit             int    `json:"limit,omitempty"`
	CaptioningEnabled bool   `json:"captioning_enabled"`
}

type chatGPTParseCheckpointState struct {
	Version             int      `json:"version"`
	Source              string   `json:"source"`
	ConversationID      string   `json:"conversation_id,omitempty"`
	ConversationMatch   string   `json:"conversation_match,omitempty"`
	Since               string   `json:"since,omitempty"`
	Limit               int      `json:"limit,omitempty"`
	CaptioningEnabled   bool     `json:"captioning_enabled"`
	DoneConversationIDs []string `json:"done_conversation_ids"`
	UpdatedAt           string   `json:"updated_at,omitempty"`
}

type ChatGPTParseCheckpoint struct {
	path  string
	state chatGPTParseCheckpointState
	done  map[string]struct{}
}

func OpenChatGPTParseCheckpoint(path string, meta ChatGPTParseCheckpointMeta) (*ChatGPTParseCheckpoint, bool, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return nil, false, fmt.Errorf("parse checkpoint path is required")
	}
	normalizedMeta := normalizeChatGPTParseCheckpointMeta(meta)
	cp := &ChatGPTParseCheckpoint{
		path: trimmedPath,
		state: chatGPTParseCheckpointState{
			Version:           chatGPTParseCheckpointVersion,
			Source:            normalizedMeta.Source,
			ConversationID:    normalizedMeta.ConversationID,
			ConversationMatch: normalizedMeta.ConversationMatch,
			Since:             normalizedMeta.Since,
			Limit:             normalizedMeta.Limit,
			CaptioningEnabled: normalizedMeta.CaptioningEnabled,
		},
		done: map[string]struct{}{},
	}

	data, err := os.ReadFile(trimmedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cp, false, nil
		}
		return nil, false, fmt.Errorf("read parse checkpoint: %w", err)
	}

	var existing chatGPTParseCheckpointState
	if err := json.Unmarshal(data, &existing); err != nil {
		return nil, false, fmt.Errorf("parse parse checkpoint %s: %w", trimmedPath, err)
	}
	if existing.Version != chatGPTParseCheckpointVersion {
		return nil, false, fmt.Errorf("parse checkpoint %s has unsupported version %d (want %d)", trimmedPath, existing.Version, chatGPTParseCheckpointVersion)
	}
	normalizedExisting := normalizeChatGPTParseCheckpointMeta(ChatGPTParseCheckpointMeta{
		Source:            existing.Source,
		ConversationID:    existing.ConversationID,
		ConversationMatch: existing.ConversationMatch,
		Since:             existing.Since,
		Limit:             existing.Limit,
		CaptioningEnabled: existing.CaptioningEnabled,
	})
	if normalizedExisting != normalizedMeta {
		return nil, false, fmt.Errorf(
			"parse checkpoint %s is for a different import scope (source=%q conversation_id=%q conversation_match=%q since=%q limit=%d captioning_enabled=%t); delete it to start over",
			trimmedPath,
			existing.Source,
			existing.ConversationID,
			existing.ConversationMatch,
			existing.Since,
			existing.Limit,
			existing.CaptioningEnabled,
		)
	}

	cp.state = existing
	cp.state.Source = normalizedExisting.Source
	cp.state.ConversationID = normalizedExisting.ConversationID
	cp.state.ConversationMatch = normalizedExisting.ConversationMatch
	cp.state.Since = normalizedExisting.Since
	cp.state.Limit = normalizedExisting.Limit
	cp.state.CaptioningEnabled = normalizedExisting.CaptioningEnabled
	cp.done = make(map[string]struct{}, len(existing.DoneConversationIDs))
	for _, id := range existing.DoneConversationIDs {
		normalizedID := strings.TrimSpace(id)
		if normalizedID == "" {
			continue
		}
		cp.done[normalizedID] = struct{}{}
	}
	cp.state.DoneConversationIDs = sortedConversationIDs(cp.done)
	return cp, true, nil
}

func (c *ChatGPTParseCheckpoint) IsDone(conversationID string) bool {
	if c == nil {
		return false
	}
	_, ok := c.done[strings.TrimSpace(conversationID)]
	return ok
}

func (c *ChatGPTParseCheckpoint) MarkDone(conversationID string) error {
	if c == nil {
		return nil
	}
	id := strings.TrimSpace(conversationID)
	if id == "" {
		return nil
	}
	if _, exists := c.done[id]; exists {
		return nil
	}
	c.done[id] = struct{}{}
	c.state.DoneConversationIDs = sortedConversationIDs(c.done)
	c.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode parse checkpoint: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return fmt.Errorf("prepare parse checkpoint directory: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write parse checkpoint: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("commit parse checkpoint: %w", err)
	}
	return nil
}

func (c *ChatGPTParseCheckpoint) DoneCount() int {
	if c == nil {
		return 0
	}
	return len(c.done)
}

func (c *ChatGPTParseCheckpoint) Remove() error {
	if c == nil {
		return nil
	}
	if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func normalizeChatGPTParseCheckpointMeta(meta ChatGPTParseCheckpointMeta) ChatGPTParseCheckpointMeta {
	normalized := ChatGPTParseCheckpointMeta{
		ConversationID:    strings.TrimSpace(meta.ConversationID),
		ConversationMatch: strings.ToLower(strings.TrimSpace(meta.ConversationMatch)),
		Since:             strings.TrimSpace(meta.Since),
		Limit:             meta.Limit,
		CaptioningEnabled: meta.CaptioningEnabled,
	}
	source := strings.TrimSpace(meta.Source)
	if source == "" {
		return normalized
	}
	if abs, err := filepath.Abs(source); err == nil {
		normalized.Source = filepath.Clean(abs)
	} else {
		normalized.Source = filepath.Clean(source)
	}
	if normalized.Limit < 0 {
		normalized.Limit = 0
	}
	if normalized.Since != "" {
		if parsed, err := time.Parse(time.RFC3339, normalized.Since); err == nil {
			normalized.Since = parsed.UTC().Format(time.RFC3339)
		}
	}
	return normalized
}

func sortedConversationIDs(done map[string]struct{}) []string {
	ids := make([]string, 0, len(done))
	for id := range done {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func ChatGPTParseCheckpointSinceValue(since time.Time) string {
	if since.IsZero() {
		return ""
	}
	return since.UTC().Format(time.RFC3339)
}
