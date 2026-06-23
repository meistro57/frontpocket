package embed

import "context"

type OpenRouterEmbedder struct {
	baseURL string
	model   string
	dims    int
}

func NewOpenRouterEmbedder(baseURL, model string, dims int) *OpenRouterEmbedder {
	if dims <= 0 {
		dims = 1536
	}
	return &OpenRouterEmbedder{baseURL: baseURL, model: model, dims: dims}
}

func (e *OpenRouterEmbedder) EmbedText(_ context.Context, text string) ([]float32, error) {
	return deterministicVector("openrouter:"+text, e.dims), nil
}

func (e *OpenRouterEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
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

func (e *OpenRouterEmbedder) ProviderName() string { return "openrouter" }

func (e *OpenRouterEmbedder) ModelName() string { return e.model }
