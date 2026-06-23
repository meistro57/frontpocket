# Privacy

FrontPocket is local-first by default.

Guidelines:

- Keep `.env` and local data out of version control.
- Do not expose Redis or Qdrant directly.
- Keep public endpoints read-only first (`/health`, `/memory/search`, `/memory/context-pack`).
- Use API key middleware for exposed API deployments.
