# FrontPocket Architecture

```text
Client / Agent / Chat Bridge / MindDrill UI (browser)
        ↓
FrontPocket Go HTTP API  (CORS-aware)
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
- ChatGPT imports now ingest attachment-aware memory records when attachment refs are present.
- Qdrant collection dimensions are validated against embedding output.
- FrontPocket corpus memory and MindDrill chat memory use separate Qdrant collections on the same Qdrant instance (`QDRANT_COLLECTION` and `MINDDRILL_MEMORY_COLLECTION`).
- The API applies CORS as the outermost middleware (configured via `CORS_ALLOW_ORIGINS`), so browser clients can call it directly and `OPTIONS` preflight is handled before auth.
- MindDrill is a separate, static browser UI (`cmd/minddrill`) that is purely an API client — it stores nothing itself and never bypasses the API boundary.
- MindDrill serves `GET /config.json` from its own origin to publish the configured FrontPocket API base URL (`--api`) at runtime, so the UI has no hardcoded backend URL.
- MindDrill chat calls `POST /memory/chat`, which retrieves both memory layers and writes back through the Go API only.

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
- `POST /memory/chat`

Optional local development helpers (enable `DEV_DEBUG_ENDPOINTS=true`):

- `GET /minddrill/memory/stats`
- `POST /minddrill/memory/search`
- `DELETE /minddrill/memory/session`

Current API version: `0.2.0`
