package embed

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type OpenAIEmbedder struct {
	baseURL string
	model   string
	dims    int
	apiKey  string
	client  *http.Client
}

type openAIEmbeddingsResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func NewOpenAIEmbedder(baseURL, model, apiKey string, dims int) *OpenAIEmbedder {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = "https://api.openai.com/v1"
	}
	return &OpenAIEmbedder{
		baseURL: trimmed,
		model:   strings.TrimSpace(model),
		dims:    dims,
		apiKey:  strings.TrimSpace(apiKey),
		client:  newHTTPClient(),
	}
}

func (e *OpenAIEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("openai returned %d vectors for single input", len(vectors))
	}
	return vectors[0], nil
}

func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

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
	}
	if err := postJSON(ctx, e.client, e.baseURL+"/embeddings", headers, payload, &response); err != nil {
		return nil, err
	}
	if len(response.Data) != len(texts) {
		return nil, fmt.Errorf("openai returned %d vectors for %d inputs", len(response.Data), len(texts))
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

func (e *OpenAIEmbedder) ProviderName() string { return "openai" }

func (e *OpenAIEmbedder) ModelName() string { return e.model }
