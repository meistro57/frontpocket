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
- Session state can be cached in Redis via `POST /memory/session` and removed with `DELETE /memory/session`.
- Qdrant collection dimensions are validated against embedding output.

## Endpoint groups

Public-safe recall endpoints:

- `GET /health`
- `GET /openapi.json`
- `POST /memory/search`
- `POST /memory/context-pack`

Operational endpoints (trusted/private use):

- `GET /memory/stats`
- `POST /memory/session`
- `DELETE /memory/session`
- `POST /memory/ingest/chat`

Current API version: `0.2.0`
