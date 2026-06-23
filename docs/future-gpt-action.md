# Future GPT Action Path

FrontPocket is designed to expose a clean OpenAPI schema and read-only recall endpoints for future Custom GPT Action integration.

Planned public-safe surface:

- `GET /health`
- `GET /openapi.json`
- `POST /memory/search`
- `POST /memory/context-pack`

Admin and mutation endpoints should remain private.

The service now exposes `/openapi.json` for schema discovery and Action wiring.
