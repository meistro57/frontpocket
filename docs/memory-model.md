# Memory Model

FrontPocket stores memory points with:

- source metadata (`source_title`, `source_type`, `conversation_id`, `timestamp`, `speaker`)
- memory metadata (`project`, `memory_kind`, `tags`, `importance`)
- retrieval fields (`summary`, `source_quote`, `score`)
- canon metadata (`canonical`, `status`, `confidence`)
- canon provenance fields (`source_memory_ids`, `source_quotes`, `reviewed_at`, `reviewed_by`, `created_by_loop`)
- canon lifecycle fields (`supersedes`, `merged_from`, `approximate_date`, `date_basis`, `rejection_reason`, `merge_target_id`)
- ChatGPT export signal metadata, when imported via `frontpocket ingest chatgpt` (`user_starred`, `user_shared`, `share_id`, `feedback_rating`, `feedback_note`, `feedback_at`)
- attachment metadata when present (`attachment_refs`, `attachment_count` in normalized import metadata; `attachment_filename`, `attachment_mime_type`, `attachment_category`, `attachment_source_system` on the stored memory point once resolved)
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
- Aggregate memory totals are exposed through `GET /memory/stats`, using Qdrant collection info/count APIs and cached distinct-field aggregation so large collections do not require full scans per request.
- Collection vector size is validated against embedding output dimensions.
- Filter payload indexes (`project`, `memory_kind`, `speaker`, `source_type`, `conversation_id`, `ai_provider`, `ai_model`, `feedback_rating`, `attachment_source_system`, `tags`, `status`, `user_starred`, `user_shared`, `canonical`, `timestamp`) are created on first use of a collection and re-created idempotently on existing collections, so filtered search, browse, and stats avoid full scans.

## Retrieval tuning

- `hybrid: true` re-ranks the dense candidate pool with lexical overlap (reciprocal rank fusion), so exact names, quotes, and code symbols surface without discarding semantic matches. Scores are normalised to 0-1.
- `dedupe: true` collapses memories with identical text inside the same conversation, keeping the highest-scoring representative and reporting `duplicate_count` / `duplicate_ids`.
- `max_per_conversation` caps how much of one long thread can occupy the result set.
- `min_score` drops weak matches server-side; it is clamped to at least `SEARCH_MIN_SCORE`.
- Query embeddings are cached for five minutes per provider/model, so repeated identical queries skip the embedding round trip.
- `GET /metrics` exposes counters for requests, embedding cache hits/misses, search duration, and rate-limit rejections.

## Context pack behavior

`POST /memory/context-pack` builds a bounded pack rather than forwarding search results:

- `max_characters` sets the budget, clamped to `CONTEXT_PACK_MAX_CHARACTERS`; entries that would exceed it are dropped and counted.
- `quote_only` keeps source quotes and drops full text and summaries.
- `order` accepts `relevance` (default) or `chronological`.
- Responses include `used_memory_ids` in pack order and a `budget` block (`character_limit`, `characters_used`, `results_included`, `dropped`).

## Mutation and audit behavior

- `POST /memory/pin` sets `pinned`, `pinned_at`, and `pinned_by` on the payload.
- `POST /memory/correct` rewrites `text`, `summary`, `source_quote`, `memory_kind`, `project`, or `tags` and stamps `corrected_at` / `corrected_by`. The original source quote is preserved unless explicitly corrected.
- `DELETE /memory/forget` soft-deletes by setting `status=deleted` (excluded from recall by default) and stamps `deleted_at` / `deleted_by`; `hard: true` removes the points.
- `GET /memory/export` streams `application/x-ndjson` for backup and inspection.
- Every accepted mutation is appended to `FRONTPOCKET_MEMORY_MUTATION_LOG` with timestamp, action, actor, memory ids, affected fields, and reason.
- All four endpoints require `ENABLE_ADMIN_ENDPOINTS=true`, which defaults to on only in `FRONTPOCKET_MODE=local`.

## Store failure and degraded reads

- Ingest returns `503` with code `INGEST_FAILED` when points could not be persisted; the detail carries the store error. It never reports success for data that was not written.
- Search, browse, stats, context-pack, and MindDrill search return `503` when the store cannot answer, instead of an empty result set that reads as "no memory".
- A dimension mismatch is a configuration error and stays `500`.
- The in-memory fallback store is off by default (`FRONTPOCKET_ALLOW_MEMORY_FALLBACK=false`). When enabled, fallback writes and reads still succeed, but responses carry `"store": {"degraded": true, "reason": "..."}` and ingest also reports `"persisted": false`.


## Reflection readiness behavior

- `safe_for_reflection=true` means a cleaned record passed mechanical checks and can be reflected by default.
- Missing/malformed quotes and schema issues populate `reflection_blockers` and mark records unsafe.
- `cleanup_status=needs_review` keeps records reviewable while still preserving source provenance.
- `speaker=mixed` is not canon-eligible until source separation (`usable_for_canon=false`, `canon_blockers` populated).
- Assistant-origin records are routed to `assistant_guidance` or `project_support_history`; they are not user facts unless nearby user support exists.
- Technical assistant chunks are marked `domain=technical`, `phase_applicability=not_applicable`, and avoid awakening-phase assignment.
- Reflection confidence caps are quote-quality aware: partial/truncated max `0.75`, malformed max `0.35`, missing quote blocked by default.
- Vectors are hidden by default in cleanup/review/query output unless explicitly requested via CLI flags.

## Attachment resolution behavior

`frontpocket ingest chatgpt` resolves attachment references found in a ChatGPT
export against two mapping files before falling back to a placeholder stub:

- `conversation_asset_file_names.json` — inline chat uploads (and, in
  practice, many Library-backed uploads too; the two systems overlap more
  than their ID-prefix shapes suggest, so both maps are always checked
  regardless of a reference's shape).
- `library_files.json` — ChatGPT's persistent "Library" files, with richer
  metadata (`mime_type`, `library_file_category`, real `file_name`).

Resolution behavior:

- A reference only counts as resolved if the mapping file has an entry
  **and** the corresponding `.dat` bytes actually exist on disk. A broken
  cross-reference (mapping entry present, bytes missing) is treated as
  unresolved, not a partial success.
- `image/*` mime types get a real caption from a vision-capable chat model
  (`VISION_MODEL`, via OpenRouter) — the caption becomes the point's `text`
  and gets chunked/embedded normally, so it is semantically searchable.
- Non-image mime types (PDF, audio, video, plain text/markdown pastes, etc.)
  get a plain metadata-only description (`"PDF attachment: <filename>"`)
  with no vision API call — captioning cost only applies to images.
- Unresolvable references keep today's original placeholder-stub text
  (`attachment_refs: <id>`) rather than blocking the whole import.
- `--dry-run` resolves attachments and reports accurate
  resolved/unresolved/would-caption counts (so vision API cost is knowable
  upfront) but never actually invokes the captioner.
- The same underlying asset can appear on a message in more than one raw
  reference form (e.g. a bare ID from a nested `id` field alongside a
  scheme-qualified `asset_pointer` for the identical file); references are
  deduplicated by their scheme-stripped form before resolution so this
  doesn't double-count attachments or double-spend on vision calls.

## Folder and multimodal ingestion behavior

`frontpocket ingest folder <path>` normalizes mixed documents, presentations, diagrams, and media directly into `frontpocket_memory`:

- **Word Documents (`.docx`)**: Parsed natively from OpenXML packages. Each document heading and section becomes a structured record with `source_type=document`, `speaker=document`, and `memory_kind=research_note`.
- **Presentation Decks (`.pdf`)**: Detects slide presentations (such as NotebookLM slide exports). For decks without embedded text streams, pages are rendered at native resolution and transcribed/described using `VISION_MODEL` with slide-focused prompting. Stored with `source_type=presentation`, `speaker=presentation`, and tags including `slide-N`.
- **Diagrams and Images (`.png`, `.jpg`, `.jpeg`, `.webp`)**: Transcribed with diagram-focused vision prompting capturing node connections, formulas, labels, and conceptual relationships. Stored with `source_type=diagram`, `speaker=diagram`, and `memory_kind=creative_artifact`.
- **Audio & Video (`.m4a`, `.mp3`, `.wav`, `.mp4`)**: Transcribed via GPU-accelerated WhisperX/Whisper, segmented into natural turns with timestamp ranges (`[01:15 - 02:45]`), stored with `source_type=audio_overview` or `video_presentation`, `speaker=audio_overview`, and `memory_kind=research_note`.
- **Persistent Caching**: Slide image extractions, diagram vision captions, and audio transcripts are saved in `.frontpocket_cache/` so interrupted runs or repeated imports never incur redundant API spend or transcription time.

## Retrieval behavior

- `status=rejected`, `status=contradicted`, and `status=outdated` are excluded by default unless `include_rejected=true` is requested.
- Canonical and approved records receive a modest ranking boost in search and context-pack results.
- `canonical_first=true` forces canonical records to the top of result ordering before score tie-breaking.
- `include_proposed=true` adds proposed canon candidates from the review queue into search/context-pack responses.

This keeps recall source-backed, auditable, and consistent.
