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
- Embedding responses are bounded with a 32MB decode limit to prevent truncated JSON errors on large batches.
- Qdrant upserts use UUID-compatible point IDs and keep the original `memory_id` in payload metadata.
- `EMBEDDING_DIMENSIONS` is enforced when set.
- Dimension mismatches are surfaced as errors instead of silent fallback.

## Troubleshooting OpenRouter ingest with Qdrant

If OpenRouter usage appears on openrouter.ai but Qdrant stays empty, check for this error in FrontPocket logs:

```text
Format error in JSON body: value <memory_id> is not a valid point ID
```

That indicates Qdrant rejected a non-UUID point ID on insert. Current FrontPocket builds convert memory IDs to deterministic UUID point IDs during upsert and preserve `memory_id` in payload metadata.

## Required credentials

- `OPENAI_API_KEY` is required when `EMBEDDING_PROVIDER=openai`.
- `OPENROUTER_API_KEY` is required when `EMBEDDING_PROVIDER=openrouter`.
- `OPENROUTER_API_KEY` is required when `CHAT_PROVIDER=openrouter`.
- For DeepSeek chat, use `CHAT_PROVIDER=openai` with `OPENAI_BASE_URL=https://api.deepseek.com/v1` and a DeepSeek API key (see https://api-docs.deepseek.com/).

## Vision captioning (ChatGPT attachment ingest)

`frontpocket ingest chatgpt` resolves image attachments and captions them
through a separate vision-capable chat model call — the caption text is
then embedded through the normal text-embedding pipeline, so no embedding
provider needs to support multimodal input directly.

```env
VISION_MODEL=google/gemini-2.5-flash
```

This is intentionally decoupled from `CHAT_PROVIDER`'s model: captioning a
large image corpus is a distinct cost/quality tradeoff from
summarization/reflection, and always goes through OpenRouter (reusing
`OPENROUTER_API_KEY` / `OPENROUTER_BASE_URL` from the embedding config)
regardless of which `EMBEDDING_PROVIDER` is active.

- Non-image attachments (PDF, audio, video, pasted text/markdown) never
  reach this model — they get a metadata-only description with zero
  captioning cost.
- Pass `--no-caption` to `ingest chatgpt` to resolve attachment metadata
  without spending on any vision calls (filenames/mime types still get
  populated; images fall back to the placeholder stub text).
- Always run `--dry-run` first on a new export — it reports resolved vs.
  unresolved counts and how many attachments would actually trigger a
  vision call, before any real API cost is spent.
