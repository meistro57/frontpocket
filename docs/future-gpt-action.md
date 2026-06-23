# Future GPT Action Path

FrontPocket is designed to expose a clean OpenAPI schema and read-only recall endpoints for future Custom GPT Action integration.

Planned public-safe surface:

- `GET /health`
- `GET /openapi.json`
- `POST /memory/search`
- `POST /memory/context-pack`

Operational endpoints (keep private or trusted-only):

- `GET /memory/stats`
- `POST /memory/session`

Admin and mutation endpoints should remain private.

The service now exposes `/openapi.json` for schema discovery and Action wiring.
