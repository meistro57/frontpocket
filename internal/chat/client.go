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
)

const maxResponseBytes int64 = 4 << 20

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client interface {
	Complete(ctx context.Context, messages []Message) (string, error)
	ProviderName() string
	ModelName() string
}

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
	if strings.TrimSpace(c.apiKey) == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY is required when CHAT_PROVIDER=openrouter")
	}
	if strings.TrimSpace(c.model) == "" {
		return "", fmt.Errorf("OPENROUTER_CHAT_MODEL is required when CHAT_PROVIDER=openrouter")
	}
	payload := map[string]any{
		"model":    c.model,
		"messages": messages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.siteURL != "" {
		req.Header.Set("HTTP-Referer", c.siteURL)
	}
	if c.appName != "" {
		req.Header.Set("X-Title", c.appName)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	limited := &io.LimitedReader{R: resp.Body, N: maxResponseBytes + 1}
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if int64(len(respBody)) > maxResponseBytes {
		return "", fmt.Errorf("chat provider response exceeded %d bytes", maxResponseBytes)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("chat provider returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var decoded struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return "", fmt.Errorf("failed to decode chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("chat provider returned no choices")
	}
	answer := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if answer == "" {
		return "", fmt.Errorf("chat provider returned an empty answer")
	}
	return answer, nil
}

func (c *OpenRouterClient) ProviderName() string { return "openrouter" }
func (c *OpenRouterClient) ModelName() string    { return c.model }
