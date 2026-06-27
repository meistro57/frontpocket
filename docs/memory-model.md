# Memory Model

FrontPocket stores memory points with:

- source metadata (`source_title`, `source_type`, `conversation_id`, `timestamp`, `speaker`)
- memory metadata (`project`, `memory_kind`, `tags`, `importance`)
- retrieval fields (`summary`, `source_quote`, `score`)
- canon metadata (`canonical`, `status`, `confidence`)
- canon provenance fields (`source_memory_ids`, `source_quotes`, `reviewed_at`, `reviewed_by`, `created_by_loop`)
- canon lifecycle fields (`supersedes`, `merged_from`, `approximate_date`, `date_basis`, `rejection_reason`, `merge_target_id`)
- attachment metadata when present (`attachment_refs`, `attachment_count`)
- embedding metadata (`embedding_provider`, `embedding_model`, `embedding_dimensions`)

## Storage behavior

- Long-term semantic memory lives in Qdrant.
- Working/session recall cache lives in Redis.
- Proposed canon candidates live in a JSON review queue file (`FRONTPOCKET_PROPOSED_CANON_PATH`, default `data/proposed_canon.json`).
- Search results may be cached for `SEARCH_CACHE_TTL_SECONDS`.
- Session state can be saved/loaded through `POST /memory/session` and cleared through `DELETE /memory/session`.
- Aggregate memory totals are exposed through `GET /memory/stats`.
- Collection vector size is validated against embedding output dimensions.

## Retrieval behavior

- `status=rejected`, `status=contradicted`, and `status=outdated` are excluded by default unless `include_rejected=true` is requested.
- Canonical and approved records receive a modest ranking boost in search and context-pack results.
- `canonical_first=true` forces canonical records to the top of result ordering before score tie-breaking.
- `include_proposed=true` adds proposed canon candidates from the review queue into search/context-pack responses.

This keeps recall source-backed, auditable, and consistent.
