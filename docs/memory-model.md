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

## Retrieval behavior

- `status=rejected`, `status=contradicted`, and `status=outdated` are excluded by default unless `include_rejected=true` is requested.
- Canonical and approved records receive a modest ranking boost in search and context-pack results.
- `canonical_first=true` forces canonical records to the top of result ordering before score tie-breaking.
- `include_proposed=true` adds proposed canon candidates from the review queue into search/context-pack responses.

This keeps recall source-backed, auditable, and consistent.
