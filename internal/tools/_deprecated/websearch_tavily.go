package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/chat"
)

// TavilyWebSearchTool calls Tavily's search API (https://tavily.com), which
// returns cleaned, extracted result content rather than raw snippets — one
// call gets Eli both the links and usable page content, no separate
// scraping step. Auth is a bearer token per Tavily's current docs
// (Authorization: Bearer tvly-...), not a body field.
type TavilyWebSearchTool struct {
	apiKey string
	client *http.Client
}

// NewTavilyWebSearchTool builds the tool. If apiKey is empty, Execute
// returns a clear "not configured" message instead of failing at startup —
// Eli should still boot and chat with memory-only context if TAVILY_API_KEY
// isn't set yet.
func NewTavilyWebSearchTool(apiKey string) *TavilyWebSearchTool {
	return &TavilyWebSearchTool{
		apiKey: strings.TrimSpace(apiKey),
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (t *TavilyWebSearchTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "web_search",
		Description: "Search the live web for current information. Use this for anything that might have changed recently or isn't in the memory archive — current events, prices, docs, releases, anything outside personal history.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "Search query"},
				"max_results": {"type": "integer", "description": "Max results to return (default 5, max 10)"}
			},
			"required": ["query"]
		}`),
	}
}

type tavilySearchArgs struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

type tavilyRequest struct {
	Query         string `json:"query"`
	MaxResults    int    `json:"max_results"`
	SearchDepth   string `json:"search_depth"`
	IncludeAnswer bool   `json:"include_answer"`
}

type tavilyResponse struct {
	Results []struct {
		Title   string  `json:"title"`
		URL     string  `json:"url"`
		Content string  `json:"content"`
		Score   float64 `json:"score"`
	} `json:"results"`
}

func (t *TavilyWebSearchTool) Execute(ctx context.Context, argumentsJSON string) (string, error) {
	if t.apiKey == "" {
		return "error: web search is not configured (TAVILY_API_KEY missing)", nil
	}

	var args tavilySearchArgs
	if strings.TrimSpace(argumentsJSON) != "" {
		if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
			return fmt.Sprintf("error: could not parse arguments: %v", err), nil
		}
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "error: query is required", nil
	}
	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 10 {
		maxResults = 10
	}

	payload := tavilyRequest{
		Query:         query,
		MaxResults:    maxResults,
		SearchDepth:   "basic",
		IncludeAnswer: false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Sprintf("error: web search request failed: %v", err), nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Sprintf("error: web search returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))), nil
	}

	var decoded tavilyResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return "", fmt.Errorf("decode tavily response: %w", err)
	}
	if len(decoded.Results) == 0 {
		return "no results found", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d web results for %q:\n\n", len(decoded.Results), query)
	for i, r := range decoded.Results {
		fmt.Fprintf(&b, "[%d] %s\n    %s\n", i+1, strings.TrimSpace(r.Title), strings.TrimSpace(r.URL))
		content := strings.TrimSpace(r.Content)
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		if content != "" {
			fmt.Fprintf(&b, "    %s\n", content)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
