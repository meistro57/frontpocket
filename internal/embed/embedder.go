package embed

import "context"

type Embedder interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	ProviderName() string
	ModelName() string
}
