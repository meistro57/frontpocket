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
- cleanup metadata in cleaned-memory layer (`cleanup_status`, `cleanup_warnings`, `reflection_blockers`, `safe_for_reflection`, `quote_quality`, `source_quote_cleaned`, `timestamp_normalized`, `speaker_normalized`, `source_role`, `domain`, `phase_applicability`)
- reflection eligibility scopes (`usable_for_user_profile`, `usable_for_project_history`, `usable_for_assistant_guidance`, `usable_for_persona_memory`, `usable_for_canon`)
- role-aware classification fields (`memory_kind`, `canon_blockers`, `project_hint`, `project_hint_basis`, `project_confidence`)
- vector visibility metadata (`vector_present`, `vector_names`, `vector_dimensions`)

## Storage behavior

- Long-term semantic memory lives in Qdrant.
- Working/session recall cache lives in Redis.
- Raw memory (`frontpocket_memory`) is never overwritten by cleanup.
- Pre-reflection cleanup writes normalized records into `fp_cleaned_memory`.
- Reflection writes LLM outputs into `fp_reflections`, using cleaned records by default.
- Proposed canon candidates live in a JSON review queue file (`FRONTPOCKET_PROPOSED_CANON_PATH`, default `data/proposed_canon.json`).
- Search results may be cached for `SEARCH_CACHE_TTL_SECONDS`.
- Session state can be saved/loaded through `POST /memory/session` and cleared through `DELETE /memory/session`.
- Aggregate memory totals are exposed through `GET /memory/stats`.
- Collection vector size is validated against embedding output dimensions.

## Reflection readiness behavior

- `safe_for_reflection=true` means a cleaned record passed mechanical checks and can be reflected by default.
- Missing/malformed quotes and schema issues populate `reflection_blockers` and mark records unsafe.
- `cleanup_status=needs_review` keeps records reviewable while still preserving source provenance.
- `speaker=mixed` is not canon-eligible until source separation (`usable_for_canon=false`, `canon_blockers` populated).
- Assistant-origin records are routed to `assistant_guidance` or `project_support_history`; they are not user facts unless nearby user support exists.
- Technical assistant chunks are marked `domain=technical`, `phase_applicability=not_applicable`, and avoid awakening-phase assignment.
- Reflection confidence caps are quote-quality aware: partial/truncated max `0.75`, malformed max `0.35`, missing quote blocked by default.
- Vectors are hidden by default in cleanup/review/query output unless explicitly requested via CLI flags.

## Retrieval behavior

- `status=rejected`, `status=contradicted`, and `status=outdated` are excluded by default unless `include_rejected=true` is requested.
- Canonical and approved records receive a modest ranking boost in search and context-pack results.
- `canonical_first=true` forces canonical records to the top of result ordering before score tie-breaking.
- `include_proposed=true` adds proposed canon candidates from the review queue into search/context-pack responses.

This keeps recall source-backed, auditable, and consistent.
