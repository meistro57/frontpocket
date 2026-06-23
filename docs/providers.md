# Embedding Providers

Current provider interface:

```go
type Embedder interface {
    EmbedText(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}
```

Supported provider choices:

- `ollama` (default)
- `openai`
- `openrouter`

Provider selection is config-driven through `EMBEDDING_PROVIDER`.
