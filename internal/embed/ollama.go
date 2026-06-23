package embed

import (
	"context"
	"hash/fnv"
)

type OllamaEmbedder struct {
	baseURL string
	model   string
	dims    int
}

func NewOllamaEmbedder(baseURL, model string, dims int) *OllamaEmbedder {
	if dims <= 0 {
		dims = 768
	}
	return &OllamaEmbedder{baseURL: baseURL, model: model, dims: dims}
}

func (e *OllamaEmbedder) EmbedText(_ context.Context, text string) ([]float32, error) {
	return deterministicVector(text, e.dims), nil
}

func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		vec, err := e.EmbedText(ctx, t)
		if err != nil {
			return nil, err
		}
		out = append(out, vec)
	}
	return out, nil
}

func (e *OllamaEmbedder) ProviderName() string { return "ollama" }

func (e *OllamaEmbedder) ModelName() string { return e.model }

func deterministicVector(text string, dims int) []float32 {
	if dims <= 0 {
		dims = 384
	}
	vector := make([]float32, dims)
	if text == "" {
		return vector
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	seed := h.Sum64()
	for i := range vector {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		vector[i] = float32(seed%10000) / 10000.0
	}

	return vector
}
