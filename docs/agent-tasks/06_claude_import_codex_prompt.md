# FrontPocket — Claude Export Ingestion: Codex Task Prompt

Paste this whole file as your task brief. Read it fully before writing any
code. **Read `AGENTS.md` in the repo root first.**

## Context

A second AI export now sits alongside the ChatGPT one:
`~/frontpocket/Claude-Marks-chat_history_7-1-26/`. Structure, confirmed by
direct inspection (not assumed):

- `users.json` — tiny, account identity (`uuid`, `full_name`, `email_address`)
- `memories.json` — a single record: `conversations_memory` (one large
  cross-conversation synthesis string) + `project_memories` (a map from
  project UUID to a per-project synthesis string) + `account_uuid`. This is
  **pre-synthesized, high-signal-per-byte data** — Claude's own compressed
  model of Mark across every conversation and project, not raw transcript.
- `projects/*.json` — one file per project (17 total): `uuid`, `name`,
  `description`, `is_private`, `prompt_template` (the project's actual custom
  system instructions), `created_at`/`updated_at`, `creator`, `docs[]`
  (attached knowledge files, empty on the one sampled so far — check others).
- `conversations.json` (157MB) — top-level array, one object per
  conversation: `uuid`, `name`, `summary`, `created_at`/`updated_at`,
  `account.uuid`, `chat_messages[]`. Each message has `uuid`, `text`,
  `content[]` (timestamped blocks), `sender` (`"human"` or `"assistant"`),
  `attachments[]`, `files[]`, and **`parent_message_uuid`** — this is a
  **tree, not a flat list** (handles edits/regenerations/branches), rooted at
  a sentinel UUID `00000000-0000-4000-8000-000000000000`.

This is genuinely simpler than the ChatGPT export in most ways (no sharded
files, no `.dat` attachment blobs to resolve, no `library_files.json` dance)
but has one structural wrinkle ChatGPT's export didn't (or at least wasn't
built to handle yet): the message tree with branching.

## Step 1 — Investigate before writing any code (do not assume, confirm each of these against real data)

1. **Branching frequency**: scan `conversations.json` and report how many
   conversations have more than one child pointing to the same
   `parent_message_uuid` (i.e., an actual branch — an edited/regenerated
   message). If branching is rare, a "follow the most recent branch by
   `created_at`" strategy is reasonable. If common, we need a real decision
   (ingest all branches as separate paths, vs. latest-only) — report the
   real number before assuming either way.
2. **Project linkage**: the one sampled conversation had no visible project
   reference. Check a conversation you can confirm belongs to a project
   (cross-reference against `projects/*.json` names/timestamps, or check for
   any field like `project_uuid` you find on closer inspection) — is there a
   real link, or does project association only exist implicitly through
   `docs[]` in the project file? Report what you find.
3. **Content block types beyond plain text**: check for artifacts, tool use,
   tool results, thinking blocks, or attachments-with-real-content in
   `content[].type` across a meaningful sample (not just the first
   conversation). ChatGPT's export needed `unsupported_content_types`
   tracking for exactly this reason (`reasoning_recap`, `thoughts`,
   `multimodal_text`) — Claude's export likely has its own equivalent set.
   Report every distinct `type` value found and how each should be handled
   (ingest as text, skip with a counter, or something else).
4. **`attachments[]` / `files[]` shape**: check whether either array is ever
   non-empty, and if so, what it actually contains (inline data, a
   reference ID needing separate resolution like ChatGPT's export, or
   something else entirely).

Report findings from all four before proceeding to Step 2. This mirrors
exactly the discipline that caught real bugs in the ChatGPT import work
(the `content_type=="text"` short-circuit, the ref-dedup bug) — guessing at
structure here would repeat that same mistake pre-emptively instead of after
the fact.

## Step 2 — New field: `ai_provider` (and `ai_model`)

Two new fields on `MemoryPoint` / `MessageRecord` (same pattern as Phase 1's
six fields — add to `models.go`, thread through `ingest.go`, include in
`internal/store/qdrant.go`'s `toQdrantPayload`):

- `ai_provider` — **free-form string, not a closed enum**. No `const` list
  restricting values. Values used so far: `"chatgpt"`, `"claude"`. Must stay
  open for `"gemini"`, `"deepseek"`, or anything else added later without a
  schema change.
- `ai_model` — fine-grained, populated when the export actually provides it.
  ChatGPT's export has `metadata.model_slug` per message (never wired into
  the importer despite being available — real, separate gap, wire it up now
  alongside this work since it's the same kind of field). Claude's export,
  per Step 1's investigation, may or may not have an equivalent — report
  what you find rather than leaving this silently empty without knowing why.

**Do not conflate this with `source_type`** — that field means export
format (`"chat_export_zip"`, `"chat_export_folder"`), not which AI generated
the assistant's replies. Keep them orthogonal.

Wire a new CLI flag `--ai-provider` on both the existing `ingest chatgpt`
command (should now be passed explicitly going forward, not left implicit)
and the new `ingest claude` command below.

## Step 3 — New file: `internal/memory/claude_import.go`

Sibling to `chatgpt_import.go`, same overall shape (`ParseClaudeExport`,
`ClaudeImportOptions`, `ClaudeImportResult` mirroring the ChatGPT
equivalents). Handles all three source types found in the export root:

1. **`memories.json`** — two point types:
   - One canonical point from `conversations_memory` (the whole string,
     `memory_kind` something like `"ai_generated_synthesis"`, `ai_provider:
     "claude"`, high default `evidence_strength`/`confidence` since it's
     Claude's own synthesis, not raw transcript).
   - One point per `project_memories` entry, tagged with the project UUID
     (`conversation_id` or a new `project_id` field — decide which fits the
     existing schema better and justify it) and cross-referenced against
     `projects/*.json` for the real project name.
2. **`projects/*.json`** — one point per project file: name, description,
   `prompt_template` as the embeddable text (this is real, valuable
   context — a project's actual custom instructions), tagged
   `memory_kind: "project_config"` or similar.
3. **`conversations.json`** — the main import. Walk each conversation's
   message tree properly per Step 1's branching findings (don't assume
   linear order). Normalize `sender: "human"` → `speaker: "user"` and
   `sender: "assistant"` → `speaker: "assistant"` so `speaker` stays a
   clean, provider-agnostic filter across the whole corpus regardless of
   which importer produced a point. Chunk/embed exactly like the ChatGPT
   path (reuse `Chunker`/`Ingestor` — this new importer only needs to
   produce `MessageRecord`s correctly, not duplicate the embedding/write
   pipeline).

## Step 4 — CLI wiring

```
frontpocket ingest claude <export-folder> [--dry-run] [--project <n>] [--ai-provider claude] [--limit <n>]
```

Dry-run should print the same category of summary as `ingest chatgpt
--dry-run`: conversations found, messages found/accepted/skipped, content
types encountered (from Step 1's findings), branching stats, memories.json
point count, projects point count.

## Step 5 — Backfill existing production data

The 86,198 points currently in `frontpocket_memory` (from last night's full
ChatGPT ingest) have **no `ai_provider` set** — that field didn't exist yet
at ingest time. Write a small one-time backfill: scroll all points where
`ai_provider` is absent/empty, set it to `"chatgpt"`, write back via
`points/set_payload`. Confirm the count matches 86,198 (or whatever the
current true count is — check first) before and after, and confirm zero
points get touched twice.

## Verification (same standard as every prior phase — do not skip)

```bash
cd ~/frontpocket
go build ./...
go test ./tests/... -v
./make_all.sh   # always rebuild before testing

# Dry run first
./bin/frontpocket ingest claude ./Claude-Marks-chat_history_7-1-26 --dry-run --project FrontPocket

# Small real test into a NEW test collection — do not reuse phase1/phase2 test collections
QDRANT_COLLECTION=frontpocket_claude_test ./bin/frontpocket ingest claude \
  ./Claude-Marks-chat_history_7-1-26 --project FrontPocket --limit 20
```

Then pull real points directly and report actual JSON, not a summary claim:

```bash
curl -s -X POST http://localhost:6333/collections/frontpocket_claude_test/points/scroll \
  -H 'Content-Type: application/json' \
  -d '{"limit": 5, "filter": {"must": [{"key": "ai_provider", "match": {"value": "claude"}}]}}'
```

Confirm: real `ai_provider: "claude"`, correctly normalized `speaker`
values, real text (not truncated/mangled), and — if `memories.json` got
ingested in this test run — confirm the `conversations_memory` /
`project_memories` points look right too.

## Explicit non-goals

- No changes to the ChatGPT importer's actual parsing logic — only add the
  `--ai-provider` flag wiring and the `model_slug` capture noted in Step 2.
- No changes to the chunker, cleanup pipeline, or reflection pipeline.
- No full-corpus Claude ingest without reviewing the dry-run numbers first.
- No backfill of production data until the new field/schema is verified
  working correctly on the test collection first.
- No touching `frontpocket_memory_phase1_test` or `frontpocket_memory_phase2_test`.

## Definition of done

- Step 1's four investigation questions answered with real numbers/findings,
  not assumptions.
- `go build ./...` / `go test ./...` clean on a freshly rebuilt binary.
- Dry-run against the real export prints accurate, sane-looking counts.
- Small test ingest into `frontpocket_claude_test` verified via real Qdrant
  queries — actual JSON reported back, matching this brief's Step 5
  verification format.
- Backfill script written and tested (dry-run mode showing what it *would*
  change) but **not yet run against production** — that's a separate
  go-ahead once everything above is confirmed working.
