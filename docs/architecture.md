# FrontPocket Architecture

```text
Client / Agent / Chat Bridge
        ↓
FrontPocket Go HTTP API
        ↓
Embedder (Ollama/OpenAI/OpenRouter)
        ↓
Qdrant (long-term semantic memory)
        ↘
         Redis (working/session cache)
```

## Notes

- The API is the only public boundary.
- Qdrant and Redis should remain private in non-local deployments.
- Search responses can be cached in Redis (`SEARCH_CACHE_TTL_SECONDS`).
- Qdrant collection dimensions are validated against embedding output.

Current API version: `0.1.0`
