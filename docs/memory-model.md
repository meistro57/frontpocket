# Memory Model

FrontPocket stores memory points with:

- source metadata (`source_title`, `source_type`, `conversation_id`, `timestamp`, `speaker`)
- memory metadata (`project`, `memory_kind`, `tags`, `importance`)
- retrieval fields (`summary`, `source_quote`, `score`)
- attachment metadata when present (`attachment_refs`, `attachment_count`)
- embedding metadata (`embedding_provider`, `embedding_model`, `embedding_dimensions`)

## Storage behavior

- Long-term semantic memory lives in Qdrant.
- Working/session recall cache lives in Redis.
- Search results may be cached for `SEARCH_CACHE_TTL_SECONDS`.
- Session state can be saved/loaded through `POST /memory/session` and cleared through `DELETE /memory/session`.
- Aggregate memory totals are exposed through `GET /memory/stats`.
- Collection vector size is validated against embedding output dimensions.

This keeps recall source-backed, auditable, and consistent.
