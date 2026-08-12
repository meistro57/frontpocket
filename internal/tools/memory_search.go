package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/loadout"
)

// MemorySearchTool exposes an explicit, model-callable version of the same
// search FrontPocket already runs automatically before every chat turn. The
// automatic pass searches on the user's raw message; this tool lets the
// model search again mid-conversation with a sharper, self-chosen query —
// e.g. after a first answer reveals a more specific angle worth digging
// into, or when it wants to check a different project/filter than the
// auto-injected context used.
type MemorySearchTool struct {
	name         string
	description  string
	store        memory.MemoryStore
	defaultLimit int
	maxLimit     int
}

// NewMemorySearchTool builds a search tool over a single memory store. Build
// one per store you want the model to be able to search explicitly (e.g. one
// named "search_source_memory" over FrontPocket's main store, one named
// "search_chat_history" over the MindDrill store) and register both.
func NewMemorySearchTool(name, description string, store memory.MemoryStore, defaultLimit, maxLimit int) *MemorySearchTool {
	if defaultLimit <= 0 {
		defaultLimit = 8
	}
	if maxLimit <= 0 {
		maxLimit = 20
	}
	return &MemorySearchTool{
		name:         name,
		description:  description,
		store:        store,
		defaultLimit: defaultLimit,
		maxLimit:     maxLimit,
	}
}

func (t *MemorySearchTool) Definition() loadout.ToolDefinition {
	return loadout.ToolDefinition{
		Name:        t.name,
		Description: t.description,
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "Search query — a concrete topic, theme, project name, or phrase to search for"},
				"limit": {"type": "integer", "description": "Max results to return (default 8, capped at 20)"},
				"project": {"type": "string", "description": "Optional project name to filter results by"}
			},
			"required": ["query"]
		}`),
	}
}

type memorySearchArgs struct {
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
	Project string `json:"project"`
}

func (t *MemorySearchTool) Execute(ctx context.Context, argumentsJSON string) (string, error) {
	var args memorySearchArgs
	if strings.TrimSpace(argumentsJSON) != "" {
		if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
			return fmt.Sprintf("error: could not parse arguments: %v", err), nil
		}
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "error: query is required", nil
	}
	limit := args.Limit
	if limit <= 0 {
		limit = t.defaultLimit
	}
	if limit > t.maxLimit {
		limit = t.maxLimit
	}

	results, err := t.store.Search(ctx, memory.SearchRequest{
		Query: query,
		Limit: limit,
		Filters: memory.SearchFilters{
			Project: strings.TrimSpace(args.Project),
		},
	})
	if err != nil {
		return fmt.Sprintf("error: memory search failed: %v", err), nil
	}
	if len(results) == 0 {
		return "no results found", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d results for %q:\n\n", len(results), query)
	for i, r := range results {
		fmt.Fprintf(&b, "[%d] memory_id=%s kind=%s score=%.3f\n", i+1, r.MemoryID, r.MemoryKind, r.Score)
		line := strings.TrimSpace(r.SourceQuote)
		if line == "" {
			line = strings.TrimSpace(r.Summary)
		}
		if line == "" {
			line = clampRunes(r.Text, 300)
		}
		if line != "" {
			fmt.Fprintf(&b, "    %s\n", line)
		}
	}
	return b.String(), nil
}

func clampRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "..."
}
