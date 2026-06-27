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

Pre-reflection cleanup loop (Python)
  frontpocket_memory -> fp_cleaned_memory
        ↓
Reflection loop (Python)
  fp_cleaned_memory -> fp_reflections

Proposed canon queue (JSON review file)
        ↑
frontpocket memory-loop + /memory/canon/proposed/*
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
- The memory loop proposes canon candidates but never auto-promotes them; human review is required via CLI or API approve/reject/merge actions.
- The pre-reflection cleanup loop is mechanical (normalization/validation only), writes to `fp_cleaned_memory`, and does not mutate raw `frontpocket_memory` points.
- Cleanup normalizes speaker/source role to `user | assistant | system | tool | mixed | unknown` and stamps role-aware `memory_kind` plus usability scopes.
- Reflection reads `fp_cleaned_memory` by default (`--raw-input` restores legacy raw-source mode).
- Assistant technical chunks are classified as `domain=technical` with `phase_applicability=not_applicable` and no awakening-phase assignment.
- Reflection confidence is capped by quote quality (`partial|truncated <= 0.75`, `malformed <= 0.35`, missing quote blocked by default).
- Project hints are inferred from `source_title` when project is empty, while final project assignment remains manual/approved.
- Reflection queries and cleanup review paths keep vectors hidden by default unless explicitly requested.
- Query/review surfaces expose vector metadata (`vector_present`, `vector_names`, `vector_dimensions`) even when vectors are hidden.
- `FRONTPOCKET_PROPOSED_CANON_PATH` controls where proposed canon queue state is persisted (default `data/proposed_canon.json`).

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

Canon review endpoints (trusted/private use):

- `GET /memory/canon/proposed`
- `GET /memory/canon/proposed/{id}`
- `POST /memory/canon/proposed/{id}/approve`
- `POST /memory/canon/proposed/{id}/reject`
- `POST /memory/canon/proposed/{id}/merge`

Optional local development helpers (enable `DEV_DEBUG_ENDPOINTS=true`):

- `GET /minddrill/memory/stats`
- `POST /minddrill/memory/search`
- `DELETE /minddrill/memory/session`

Current API version: `0.2.0`
