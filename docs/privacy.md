# Privacy

FrontPocket is local-first by default.

## Guardrails

- Keep `.env` and local data out of version control.
- Do not expose Redis or Qdrant directly to the public internet.
- Use the Go API as the public boundary.
- Keep public endpoints read-only first (`/health`, `/openapi.json`, `/memory/search`, `/memory/context-pack`).
- Keep operational endpoints private (`/memory/stats`, `/memory/session`, `/memory/ingest/chat`).
- Enable API key middleware for exposed deployments.
- Prefer local providers when handling sensitive memory data.
