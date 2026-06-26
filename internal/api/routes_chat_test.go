package api

import (
	"strings"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestBuildMindDrillPromptIncludesSeparateMemorySections(t *testing.T) {
	front := []memory.SearchResult{{
		MemoryID:       "front_1",
		MemoryKind:     memory.KindProjectContext,
		SourceQuote:    "FrontPocket stays source-backed.",
		Timestamp:      time.Now().UTC(),
		SourceType:     "chat_export",
		SourceTitle:    "Imported Chat",
		Speaker:        "user",
		Summary:        "FrontPocket is source-backed.",
		Score:          0.81,
		ConversationID: "front_conv",
	}}
	mind := []memory.SearchResult{{
		MemoryID:       "mind_1",
		MemoryKind:     memory.KindChatTurn,
		SourceQuote:    "Remember the Eli tone preference.",
		Timestamp:      time.Now().UTC(),
		SourceType:     "minddrill_chat",
		SourceTitle:    "MindDrill Chat",
		Speaker:        "user",
		Summary:        "User prefers direct tone.",
		Score:          0.73,
		ConversationID: "mind_conv",
	}}

	prompt := buildMindDrillPrompt("keep the Eli vibe", "", mind, front)

	if !strings.Contains(prompt, "MINDDRILL CHAT MEMORY:") {
		t.Fatalf("expected prompt to include MindDrill memory section, got: %s", prompt)
	}
	if !strings.Contains(prompt, "FRONTPOCKET SOURCE MEMORY:") {
		t.Fatalf("expected prompt to include FrontPocket section, got: %s", prompt)
	}
	if !strings.Contains(prompt, "USER MESSAGE:") {
		t.Fatalf("expected prompt to include user message section, got: %s", prompt)
	}
}

func TestBuildMindDrillPromptIncludesSystemPromptInfo(t *testing.T) {
	prompt := buildMindDrillPrompt("hello", "Respond as a concise workshop guide.", nil, nil)

	if !strings.Contains(prompt, "User-provided persona/system prompt information:") {
		t.Fatalf("expected prompt to include system prompt heading, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Respond as a concise workshop guide.") {
		t.Fatalf("expected prompt to include supplied system prompt info, got: %s", prompt)
	}
}
