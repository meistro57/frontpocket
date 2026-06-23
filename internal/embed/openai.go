package embed

import "context"

type OpenAIEmbedder struct {
	baseURL string
	model   string
	dims    int
}

func NewOpenAIEmbedder(baseURL, model string, dims int) *OpenAIEmbedder {
	if dims <= 0 {
		dims = 1536
	}
	return &OpenAIEmbedder{baseURL: baseURL, model: model, dims: dims}
}

func (e *OpenAIEmbedder) EmbedText(_ context.Context, text string) ([]float32, error) {
	return deterministicVector("openai:"+text, e.dims), nil
}

func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
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

func (e *OpenAIEmbedder) ProviderName() string { return "openai" }

func (e *OpenAIEmbedder) ModelName() string { return e.model }
