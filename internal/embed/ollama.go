package embed

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type OllamaEmbedder struct {
	baseURL string
	model   string
	dims    int
	client  *http.Client
}

type ollamaBatchResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

type ollamaSingleResponse struct {
	Embedding []float64 `json:"embedding"`
}

func NewOllamaEmbedder(baseURL, model string, dims int) *OllamaEmbedder {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = "http://localhost:11434"
	}
	return &OllamaEmbedder{baseURL: trimmed, model: strings.TrimSpace(model), dims: dims, client: newHTTPClient()}
}

func (e *OllamaEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("ollama returned %d vectors for single input", len(vectors))
	}
	return vectors[0], nil
}

func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	batch, err := e.embedBatchModern(ctx, texts)
	if err == nil {
		return batch, nil
	}
	return e.embedBatchLegacy(ctx, texts)
}

func (e *OllamaEmbedder) ProviderName() string { return "ollama" }

func (e *OllamaEmbedder) ModelName() string { return e.model }

func (e *OllamaEmbedder) embedBatchModern(ctx context.Context, texts []string) ([][]float32, error) {
	payload := map[string]any{
		"model": e.model,
		"input": texts,
	}
	var response ollamaBatchResponse
	if err := postJSON(ctx, e.client, e.baseURL+"/api/embed", nil, payload, &response); err != nil {
		return nil, err
	}

	if len(response.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama returned %d vectors for %d inputs", len(response.Embeddings), len(texts))
	}

	out := make([][]float32, 0, len(response.Embeddings))
	for _, raw := range response.Embeddings {
		vector := toFloat32Slice(raw)
		if err := enforceDimensions(vector, e.dims, e.ProviderName(), e.model); err != nil {
			return nil, err
		}
		out = append(out, vector)
	}
	return out, nil
}

func (e *OllamaEmbedder) embedBatchLegacy(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		payload := map[string]any{
			"model":  e.model,
			"prompt": text,
		}
		var response ollamaSingleResponse
		if err := postJSON(ctx, e.client, e.baseURL+"/api/embeddings", nil, payload, &response); err != nil {
			return nil, err
		}
		vector := toFloat32Slice(response.Embedding)
		if err := enforceDimensions(vector, e.dims, e.ProviderName(), e.model); err != nil {
			return nil, err
		}
		out = append(out, vector)
	}
	return out, nil
}
