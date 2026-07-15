# Agent Task Briefs

Chronological record of scoped task briefs handed to coding agents (Codex,
Crush) for FrontPocket work, plus their self-written completion summaries.
Numbered in the order they actually happened.

Each brief follows the same pattern, established starting with 01:
real investigation before code, test-collection-only verification (never
production without explicit go-ahead), real Qdrant queries required as
proof of "done" rather than a self-reported summary, and an explicit
non-goals section to keep scope tight.

01. **Phase 1** (starred/shared/feedback metadata from ChatGPT export)
02. **Phase 1 correction** — real source code was correct; root cause was a
    stale, un-rebuilt binary. Lesson carried into every brief after this one.
03. **Phase 2** (image/attachment resolution — `asset_file_names` +
    `library_files` dual system, vision captioning)
04. **Phase 2 completion** — forcing real coverage of the two untested paths
    (non-image via `library_files`, image via `asset_file_names`)
05. **Phase 2 completion summary** — Codex's own write-up of what shipped,
    real bugs found, and the one honestly-unreachable case
    (image-via-`library_files`-only doesn't exist in this export)
06. **Claude import** — new source: `memories.json`, `projects/*.json`,
    `conversations.json` (tree-structured, `parent_message_uuid` branching).
    New `ai_provider`/`ai_model` fields, deliberately free-form/open rather
    than a closed enum.
07. **Claude import escaping fix** — small follow-up: double-escaped
    newlines in `tool_use`/`tool_result`-derived text.

See also: `docs/architecture.md`, `docs/memory-model.md` for the permanent
reference documentation this folder deliberately stays separate from.
