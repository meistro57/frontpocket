// internal/chat/client.go
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/meistro57/loadout"
)

const maxResponseBytes int64 = 4 << 20

// Message represents one turn in a chat completion request. Role is one of
// "system", "user", "assistant", or "tool". A tool-role message reports the
// result of a prior tool call and must set ToolCallID to match the id the
// model gave that call. An assistant message that wants to call tools sets
// ToolCalls instead of (or alongside a possibly-empty) Content.
type Message struct {
	Role       string             `json:"role"`
	Content    string             `json:"content"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	ToolCalls  []loadout.ToolCall `json:"tool_calls,omitempty"`
}

// ToolDefinition and ToolCall are aliases for Loadout's types. Package chat
// doesn't own this vocabulary — Loadout does — so callers building a tool
// registry and callers building chat messages are working with the exact
// same types with no conversion at the boundary.
type ToolDefinition = loadout.ToolDefinition
type ToolCall = loadout.ToolCall

// CompletionResult is what a tool-aware completion call returns. If the
// model wants to call tools, ToolCalls is non-empty and Content is usually
// empty; a final answer has Content set and ToolCalls empty. Both being
// non-empty is legal (some models emit trailing commentary alongside a tool
// call) — callers should treat a non-empty ToolCalls as "not done yet"
// regardless of Content.
type CompletionResult struct {
	Content   string
	ToolCalls []loadout.ToolCall
}

// Client is implemented by every chat backend (OpenRouter, DeepSeek, Ollama).
type Client interface {
	// Complete is a convenience wrapper for tool-free calls (used by
	// query-refinement and other internal prompts that only need text back).
	// It errors if the model returns an empty answer.
	Complete(ctx context.Context, messages []Message) (string, error)
	// CompleteWithTools is the full call: pass tool definitions (nil/empty
	// for none) and get back either a text answer or a set of tool calls to
	// execute and feed back as tool-role messages.
	CompleteWithTools(ctx context.Context, messages []Message, tools []ToolDefinition) (CompletionResult, error)
	ProviderName() string
	ModelName() string
}

// ── Shared OpenAI-compatible wire format ────────────────────────────────────
// OpenRouter, DeepSeek, and Ollama's /v1 endpoint all speak the same
// /chat/completions request/response shape, tools included. One request
// builder and one response decoder cover all three.

type apiTool struct {
	Type     string          `json:"type"`
	Function apiToolFunction `json:"function"`
}

type apiToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type apiMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	ToolCalls  []apiMessageToolCall `json:"tool_calls,omitempty"`
}

type apiMessageToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function apiMessageToolFunction `json:"function"`
}

type apiMessageToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type apiChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func toAPITools(tools []ToolDefinition) []apiTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]apiTool, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, apiTool{
			Type: "function",
			Function: apiToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

func toAPIMessages(messages []Message) []apiMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]apiMessage, 0, len(messages))
	for _, m := range messages {
		msg := apiMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]apiMessageToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, apiMessageToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: apiMessageToolFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
		}
		out = append(out, msg)
	}
	return out
}

func cleanThinkingTags(s string) string {
	s = strings.TrimSpace(s)
	for {
		start := strings.Index(s, "<think>")
		if start == -1 {
			break
		}
		end := strings.Index(s, "</think>")
		if end == -1 {
			s = strings.TrimSpace(s[:start])
			break
		}
		s = strings.TrimSpace(s[:start] + s[end+len("</think>"):])
	}
	return s
}

// doChatCompletion sends one /chat/completions request and decodes the
// result into a CompletionResult. Shared by OpenRouterClient and
// openAICompatibleClient (Ollama, DeepSeek) since they differ only in base
// URL, auth headers, and how strictly they validate model/key presence.
func doChatCompletion(ctx context.Context, httpClient *http.Client, url string, headers map[string]string, model string, messages []Message, tools []ToolDefinition) (CompletionResult, error) {
	payload := map[string]any{
		"model":    model,
		"messages": toAPIMessages(messages),
	}
	if apiTools := toAPITools(tools); len(apiTools) > 0 {
		payload["tools"] = apiTools
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return CompletionResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return CompletionResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return CompletionResult{}, err
	}
	defer resp.Body.Close()

	limited := &io.LimitedReader{R: resp.Body, N: maxResponseBytes + 1}
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return CompletionResult{}, err
	}
	if int64(len(respBody)) > maxResponseBytes {
		return CompletionResult{}, fmt.Errorf("chat provider response exceeded %d bytes", maxResponseBytes)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return CompletionResult{}, fmt.Errorf("chat provider returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded apiChatResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return CompletionResult{}, fmt.Errorf("failed to decode chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return CompletionResult{}, fmt.Errorf("chat provider returned no choices")
	}

	msg := decoded.Choices[0].Message
	result := CompletionResult{Content: cleanThinkingTags(msg.Content)}
	for _, tc := range msg.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return result, nil
}

// ── OpenRouter ───────────────────────────────────────────────────────────

type OpenRouterClient struct {
	baseURL string
	model   string
	apiKey  string
	siteURL string
	appName string
	client  *http.Client
}

func NewOpenRouterClient(baseURL, model, apiKey, siteURL, appName string) *OpenRouterClient {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = "https://openrouter.ai/api/v1"
	}
	return &OpenRouterClient{
		baseURL: trimmed,
		model:   strings.TrimSpace(model),
		apiKey:  strings.TrimSpace(apiKey),
		siteURL: strings.TrimSpace(siteURL),
		appName: strings.TrimSpace(appName),
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *OpenRouterClient) Complete(ctx context.Context, messages []Message) (string, error) {
	result, err := c.CompleteWithTools(ctx, messages, nil)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Content) == "" {
		return "", fmt.Errorf("chat provider returned an empty answer")
	}
	return result.Content, nil
}

func (c *OpenRouterClient) CompleteWithTools(ctx context.Context, messages []Message, tools []ToolDefinition) (CompletionResult, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return CompletionResult{}, fmt.Errorf("OPENROUTER_API_KEY is required when CHAT_PROVIDER=openrouter")
	}
	if strings.TrimSpace(c.model) == "" {
		return CompletionResult{}, fmt.Errorf("OPENROUTER_CHAT_MODEL is required when CHAT_PROVIDER=openrouter")
	}
	headers := map[string]string{"Authorization": "Bearer " + c.apiKey}
	if c.siteURL != "" {
		headers["HTTP-Referer"] = c.siteURL
	}
	if c.appName != "" {
		headers["X-Title"] = c.appName
	}
	return doChatCompletion(ctx, c.client, c.baseURL+"/chat/completions", headers, c.model, messages, tools)
}

func (c *OpenRouterClient) ProviderName() string { return "openrouter" }
func (c *OpenRouterClient) ModelName() string    { return c.model }

// ── Ollama / DeepSeek (OpenAI-compatible /chat/completions) ────────────────

// openAICompatibleClient talks to any OpenAI-compatible /chat/completions
// endpoint. DeepSeek's native API and Ollama's /v1 API both expose this exact
// shape, so they share one implementation. OpenRouter keeps its own type
// because it sends extra ranking headers (HTTP-Referer / X-Title).
type openAICompatibleClient struct {
	baseURL      string
	model        string
	apiKey       string
	providerName string
	client       *http.Client
}

func (c *openAICompatibleClient) Complete(ctx context.Context, messages []Message) (string, error) {
	result, err := c.CompleteWithTools(ctx, messages, nil)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Content) == "" {
		return "", fmt.Errorf("chat provider returned an empty answer")
	}
	return result.Content, nil
}

func (c *openAICompatibleClient) CompleteWithTools(ctx context.Context, messages []Message, tools []ToolDefinition) (CompletionResult, error) {
	if strings.TrimSpace(c.model) == "" {
		return CompletionResult{}, fmt.Errorf("%s_CHAT_MODEL is required when CHAT_PROVIDER=%s", strings.ToUpper(c.providerName), c.providerName)
	}
	headers := map[string]string{}
	// Ollama runs locally and needs no key; DeepSeek requires one (enforced
	// in config.Validate). Only send the header when we actually have a key.
	if strings.TrimSpace(c.apiKey) != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}
	return doChatCompletion(ctx, c.client, c.baseURL+"/chat/completions", headers, c.model, messages, tools)
}

func (c *openAICompatibleClient) ProviderName() string { return c.providerName }
func (c *openAICompatibleClient) ModelName() string    { return c.model }

// NewDeepSeekClient builds a chat client for DeepSeek's native OpenAI-compatible
// API (https://api.deepseek.com/v1).
func NewDeepSeekClient(baseURL, model, apiKey string) Client {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = "https://api.deepseek.com/v1"
	}
	return &openAICompatibleClient{
		baseURL:      trimmed,
		model:        strings.TrimSpace(model),
		apiKey:       strings.TrimSpace(apiKey),
		providerName: "deepseek",
		client:       &http.Client{Timeout: 60 * time.Second},
	}
}

// NewOllamaChatClient builds a chat client for a local Ollama server via its
// OpenAI-compatible /v1 API, including tool-calling support for models that
// support it (llama3.1, qwen2.5/qwen3, mistral-nemo, command-r, etc.). Models
// without tool support simply never emit ToolCalls — CompleteWithTools still
// works, it just always returns a plain answer.
// baseURL is the plain Ollama URL (e.g. http://localhost:11434); the /v1
// suffix is added if absent.
func NewOllamaChatClient(baseURL, model string) Client {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = "http://localhost:11434"
	}
	if !strings.HasSuffix(trimmed, "/v1") {
		trimmed += "/v1"
	}
	return &openAICompatibleClient{
		baseURL:      trimmed,
		model:        strings.TrimSpace(model),
		apiKey:       "",
		providerName: "ollama",
		// Local models can be slow to load/generate, so allow more time.
		client: &http.Client{Timeout: 120 * time.Second},
	}
}
