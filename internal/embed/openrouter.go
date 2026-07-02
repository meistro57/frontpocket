package embed

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

var openRouterZeroVectorRetryDelays = []time.Duration{time.Second, 3 * time.Second, 9 * time.Second}

type OpenRouterEmbedder struct {
	baseURL string
	model   string
	dims    int
	apiKey  string
	siteURL string
	appName string
	client  *http.Client
}

func NewOpenRouterEmbedder(baseURL, model, apiKey, siteURL, appName string, dims int) *OpenRouterEmbedder {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = "https://openrouter.ai/api/v1"
	}
	return &OpenRouterEmbedder{
		baseURL: trimmed,
		model:   strings.TrimSpace(model),
		dims:    dims,
		apiKey:  strings.TrimSpace(apiKey),
		siteURL: strings.TrimSpace(siteURL),
		appName: strings.TrimSpace(appName),
		client:  newHTTPClient(),
	}
}

func (e *OpenRouterEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("openrouter returned %d vectors for single input", len(vectors))
	}
	return vectors[0], nil
}

func (e *OpenRouterEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embedBatchWithRetry(ctx, texts, true)
}

func (e *OpenRouterEmbedder) embedBatchWithRetry(ctx context.Context, texts []string, allowSplit bool) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	vectors, err := e.embedBatchOnce(ctx, texts)
	if err == nil {
		return vectors, nil
	}
	lastErr := err
	if isRetryableEmbeddingError(err) {
		for _, delay := range openRouterZeroVectorRetryDelays {
			if waitErr := sleepWithContext(ctx, delay); waitErr != nil {
				return nil, waitErr
			}
			vectors, err = e.embedBatchOnce(ctx, texts)
			if err == nil {
				return vectors, nil
			}
			lastErr = err
			if !isRetryableEmbeddingError(err) {
				break
			}
		}
	}

	if allowSplit && len(texts) > 1 {
		vectors, err := e.embedBatchSerial(ctx, texts)
		if err == nil {
			return vectors, nil
		}
		if lastErr != nil {
			return nil, fmt.Errorf("%w; serial fallback failed: %v", lastErr, err)
		}
		return nil, err
	}

	return nil, lastErr
}

func (e *OpenRouterEmbedder) embedBatchSerial(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for idx, text := range texts {
		chunkVectors, err := e.embedBatchWithRetry(ctx, []string{text}, false)
		if err != nil {
			return nil, fmt.Errorf("openrouter serial embed failed at input %d: %w", idx, err)
		}
		if len(chunkVectors) != 1 {
			return nil, fmt.Errorf("openrouter serial embed returned %d vectors for single input", len(chunkVectors))
		}
		vectors = append(vectors, chunkVectors[0])
	}
	return vectors, nil
}

func (e *OpenRouterEmbedder) embedBatchOnce(ctx context.Context, texts []string) ([][]float32, error) {
	payload := map[string]any{
		"model":           e.model,
		"input":           texts,
		"encoding_format": "float",
	}
	if e.dims > 0 {
		payload["dimensions"] = e.dims
	}

	var response openAIEmbeddingsResponse
	headers := map[string]string{
		"Authorization": "Bearer " + e.apiKey,
		"HTTP-Referer":  e.siteURL,
		"X-Title":       e.appName,
	}
	if err := postJSON(ctx, e.client, e.baseURL+"/embeddings", headers, payload, &response); err != nil {
		return nil, err
	}
	if len(response.Data) != len(texts) {
		return nil, fmt.Errorf("openrouter returned %d vectors for %d inputs", len(response.Data), len(texts))
	}

	sort.Slice(response.Data, func(i, j int) bool { return response.Data[i].Index < response.Data[j].Index })
	vectors := make([][]float32, 0, len(response.Data))
	for _, item := range response.Data {
		vector := toFloat32Slice(item.Embedding)
		if err := enforceDimensions(vector, e.dims, e.ProviderName(), e.model); err != nil {
			return nil, err
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func (e *OpenRouterEmbedder) ProviderName() string { return "openrouter" }

func (e *OpenRouterEmbedder) ModelName() string { return e.model }
