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

## OpenRouter Gemini embedding model

To use Gemini embeddings through OpenRouter:

```env
EMBEDDING_PROVIDER=openrouter
OPENROUTER_EMBEDDING_MODEL=google/gemini-embedding-2-preview
# Leave EMBEDDING_DIMENSIONS empty unless you need strict dimension enforcement.
```

## Implementation details

- Providers call their HTTP embedding endpoints directly.
- Batch embedding is supported for ingestion throughput.
- `EMBEDDING_DIMENSIONS` is enforced when set.
- Dimension mismatches are surfaced as errors instead of silent fallback.

## Required credentials

- `OPENAI_API_KEY` is required when `EMBEDDING_PROVIDER=openai`.
- `OPENROUTER_API_KEY` is required when `EMBEDDING_PROVIDER=openrouter`.
