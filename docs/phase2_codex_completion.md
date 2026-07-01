# FrontPocket — Phase 2 Completion Summary

## Status: done, with one honestly-unverifiable case

Phase 2 (image/attachment resolution for `frontpocket ingest chatgpt`) is
implemented, unit-tested, and verified end-to-end against real export data
and a real Qdrant instance. This doc records what shipped, the real bugs
found along the way, and the one case that could not be verified against
real data (and why).

## What shipped

- `internal/memory/assets.go` — `LoadAssetFileNames`, `LoadLibraryFiles`,
  `ResolveAttachment`. Both mapping files are always checked regardless of a
  reference's ID shape (dash- vs. underscore-prefixed), since the two
  systems overlap more than expected in practice (see bugs below). A
  reference only counts as resolved if the `.dat` bytes actually exist on
  disk — a broken cross-reference is treated as unresolved.
- `internal/memory/caption.go` — `Captioner` interface + `VisionCaptioner`.
  Images get a real caption from a vision-capable chat model over
  OpenRouter (`VISION_MODEL`); non-image mime types get a metadata-only
  description with zero API cost.
- New `MemoryPoint`/`MessageRecord`/`NormalizedMemoryRecord` fields:
  `attachment_filename`, `attachment_mime_type`, `attachment_category`,
  `attachment_source_system`. Threaded through `ingest.go` and both Qdrant
  payload directions, following the exact Phase 1 field pattern.
- CLI: `--limit` (cap conversations processed), `--conversation-id` (exact
  match, for targeted single-conversation debugging), `--no-caption`
  (resolve metadata without spending on vision calls). `--dry-run` reports
  resolved/unresolved/would-caption counts before any real cost is spent.
- `VISION_MODEL` config (default `google/gemini-2.5-flash`), documented in
  `.env.example` and `docs/providers.md`. Deliberately separate from
  `CHAT_PROVIDER` — captioning a large image corpus is a distinct
  cost/quality tradeoff from summarization/reflection.

## Real bugs found and fixed while verifying against real data

Every one of these was found by refusing to accept "it built and the tests
pass" as sufficient — each was only visible once checked against the actual
export contents and actual Qdrant output.

1. **Duplicate ref double-counting.** The same underlying asset frequently
   appears on one message in two raw forms — a bare ID (from a nested `id`
   field) and a scheme-qualified `asset_pointer` for the identical file.
   `extractAttachmentRefs` deduped on raw string equality, so both forms
   survived as "two" attachments, double-counting resolution and
   vision-cost estimates. Fixed by deduping on the scheme-stripped
   (normalized) form instead.
2. **Silently dropped pasted-file attachments.** A real, common export
   shape: a large pasted-file upload shows up as `content_type: "text"`
   with an empty `parts` array — the only reference lives in
   `message.metadata.attachments`. `extractMessageText`'s `text` branch
   short-circuited on empty parts and returned `""` regardless of whether
   an attachment was present, so the whole message vanished (`HasText`
   false → skipped) before ever reaching attachment resolution. Fixed by
   checking `hasAttachment` before treating empty parts as "no content."
3. **Meaningless constructed filenames for Library files.**
   `LibraryFileEntry` ignored the real `file_name` field in
   `library_files.json`, always constructing `<file_id>.<ext>` instead.
   Fixed to prefer the real descriptive name when present.

## The one gap: image resolved via `library_files` only

Verified on real data:

- Image via `asset_file_names`: confirmed, real caption landed in Qdrant.
- Non-image via `library_files` (a PDF): confirmed, real filename and
  metadata-only description landed in Qdrant, no vision call made.
- **Image via `library_files` exclusively (not also in
  `asset_file_names`): does not exist as a reachable case in this real
  export.** An exhaustive scan of all 312 real inline `sediment://`
  references in the export that don't resolve via `asset_file_names` was
  cross-checked against every `file_id` in `library_files.json` — zero
  matches. Every genuinely inline-attached Library image in this dataset is
  also duplicated into `conversation_asset_file_names.json` (which turns
  out to contain far more underscore-prefixed Library-style keys than
  originally assumed). The `library_files.json`-only entries that looked
  like image candidates on paper were either pure generation artifacts
  never inline-attached, or cited only via `filecite` content-reference
  markers (a citation mechanism, correctly not treated as an attachment
  upload by the extractor).

This was not faked or waved away — `TestResolveAttachmentLibraryFilesSystem`
proves the resolution code path itself is correct in isolation (synthetic
fixture, both fields checked). It is reported here as an honest gap: this
specific export simply never exercises "image, `library_files`-only" as a
live code path. If a different export ever does contain that shape, the
existing unit test plus this resolution logic should already handle it
correctly — but that claim has not been confirmed against real data, and
shouldn't be treated as equivalent to the cases that have been.

## Verification collections

All verification happened in `frontpocket_memory_phase2_test` (deleted and
rebuilt from scratch at least once during this work; also appended to for
targeted single-conversation follow-up runs). Production collections
(`frontpocket_memory`, `fp_reflections`) were never touched.

## Not done / explicitly out of scope

- No full-corpus run (~1,700 images, real vision API cost) — only small,
  targeted test ingests. Get a `--dry-run` cost estimate and explicit
  go-ahead before running the real thing at scale.
- No `model_slug` / `conversation_template_id` capture (Phase 3 candidate).
- No changes to the Python reflection pipeline (`fp_reflect_loop.py`,
  `memory_cleanup.py`).
