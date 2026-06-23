# Memory Model

FrontPocket stores memory points with:

- source metadata (`source_title`, `source_type`, `conversation_id`, `timestamp`, `speaker`)
- memory metadata (`project`, `memory_kind`, `tags`, `importance`)
- retrieval fields (`summary`, `source_quote`, `score`)
- embedding metadata (`embedding_provider`, `embedding_model`, `embedding_dimensions`)

## Storage behavior

- Long-term semantic memory lives in Qdrant.
- Working/session recall cache lives in Redis.
- Search results may be cached for `SEARCH_CACHE_TTL_SECONDS`.
- Collection vector size is validated against embedding output dimensions.

This keeps recall source-backed, auditable, and consistent.
