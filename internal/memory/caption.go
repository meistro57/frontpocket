package memory

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Captioner turns a resolved attachment into text suitable for embedding.
// Implementations should only make a network call for image mime types —
// non-image attachments get a plain metadata-only description with no API
// cost.
type Captioner interface {
	CaptionImage(ctx context.Context, attachment ResolvedAttachment) (string, error)
}

// VisionCaptioner captions images via an OpenRouter vision-capable chat
// model (VISION_MODEL). Captioning is done as a separate chat completion
// call against the image, then the resulting caption text is embedded
// through the existing text-embedding pipeline — this avoids needing to
// re-plumb internal/embed for multimodal input, and works regardless of
// whether the configured embedding model accepts images directly.
type VisionCaptioner struct {
	BaseURL string
	APIKey  string
	Model   string
	SiteURL string
	AppName string
	Client  *http.Client
}

// NewVisionCaptioner builds a VisionCaptioner. baseURL/apiKey follow the
// same OpenRouter conventions as internal/embed's OpenRouterEmbedder.
func NewVisionCaptioner(baseURL, apiKey, model, siteURL, appName string) *VisionCaptioner {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = "https://openrouter.ai/api/v1"
	}
	return &VisionCaptioner{
		BaseURL: trimmed,
		APIKey:  strings.TrimSpace(apiKey),
		Model:   strings.TrimSpace(model),
		SiteURL: strings.TrimSpace(siteURL),
		AppName: strings.TrimSpace(appName),
		Client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// CaptionImage returns a text description suitable for embedding. Non-image
// mime types return a plain metadata-only description ("PDF attachment:
// filename.pdf") without making an API call. Image mime types are sent to
// VISION_MODEL via OpenRouter's chat completions endpoint.
func (c *VisionCaptioner) CaptionImage(ctx context.Context, attachment ResolvedAttachment) (string, error) {
	if !strings.HasPrefix(strings.ToLower(attachment.MimeType), "image/") {
		return nonImageDescription(attachment), nil
	}

	if c == nil || strings.TrimSpace(c.APIKey) == "" || strings.TrimSpace(c.Model) == "" {
		return "", fmt.Errorf("vision captioner not configured (missing VISION_MODEL or OPENROUTER_API_KEY)")
	}

	imageBytes, err := os.ReadFile(attachment.DiskPath)
	if err != nil {
		return "", fmt.Errorf("read attachment bytes: %w", err)
	}

	dataURL := fmt.Sprintf("data:%s;base64,%s", attachment.MimeType, base64.StdEncoding.EncodeToString(imageBytes))

	payload := map[string]any{
		"model": c.Model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": "Describe this image in 1-3 sentences, focused on any concrete, searchable details (text visible in the image, subject matter, diagrams, screenshots, code, UI, etc). Be concrete and specific, not poetic."},
					{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
				},
			},
		},
	}

	caption, err := c.captionRequest(ctx, payload)
	if err != nil {
		return "", err
	}
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return "", fmt.Errorf("vision model returned empty caption for %s", attachment.Filename)
	}
	return caption, nil
}

type visionChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *VisionCaptioner) captionRequest(ctx context.Context, payload map[string]any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	if c.SiteURL != "" {
		req.Header.Set("HTTP-Referer", c.SiteURL)
	}
	if c.AppName != "" {
		req.Header.Set("X-Title", c.AppName)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	limited := &io.LimitedReader{R: resp.Body, N: 8 << 20}
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("vision model returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed visionChatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("decode vision model response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("vision model returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func nonImageDescription(attachment ResolvedAttachment) string {
	label := describeMimeType(attachment.MimeType)
	if attachment.Filename == "" {
		return fmt.Sprintf("%s attachment", label)
	}
	return fmt.Sprintf("%s attachment: %s", label, attachment.Filename)
}

func describeMimeType(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "application/pdf"):
		return "PDF"
	case strings.HasPrefix(mimeType, "audio/"):
		return "Audio"
	case strings.HasPrefix(mimeType, "video/"):
		return "Video"
	case mimeType == "":
		return "File"
	default:
		return mimeType
	}
}
