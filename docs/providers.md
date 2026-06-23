# Embedding Providers

Current provider interface:

```go
type Embedder interface {
    EmbedText(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
    ProviderName() string
    ModelName() string
}
```

Supported provider choices:

- `ollama` (default)
- `openai`
- `openrouter`

Provider selection is config-driven through `EMBEDDING_PROVIDER`.

## Implementation details

- Providers call their HTTP embedding endpoints directly.
- Batch embedding is supported for ingestion throughput.
- `EMBEDDING_DIMENSIONS` is enforced when set.
- Dimension mismatches are surfaced as errors instead of silent fallback.

## Required credentials

- `OPENAI_API_KEY` is required when `EMBEDDING_PROVIDER=openai`.
- `OPENROUTER_API_KEY` is required when `EMBEDDING_PROVIDER=openrouter`.
