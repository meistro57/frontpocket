# FrontPocket — Phase 1 Enrichment: Codex Task Prompt

Paste this whole file as your task brief. Read it fully before writing any code.

## Context (read AGENTS.md first)

FrontPocket is Mark's (meistro57) Go-first, local-first memory engine at
`~/frontpocket`. **Before doing anything else, read `AGENTS.md` in the repo
root — it is the authoritative style/design guide for this project and
overrides your own defaults on error shapes, naming, and security posture.**

Do not rewrite existing files wholesale. Do not touch `internal/memory/chunker.go`
or `internal/memory/chunker_v2.go` — those are settled, tested, and out of scope
for this task. Do not run anything against the live `frontpocket_memory` or
`fp_reflections` Qdrant collections — those hold 106K+/33K+ real production
points. All testing in this task happens against a new collection name
(`QDRANT_COLLECTION=frontpocket_memory_phase1_test`), set via env var, no code
change needed for that part.

## Mission

A ChatGPT export has been unzipped to `~/frontpocket/unzipped/`. Three files in
there carry explicit, high-value signal that the current importer
(`internal/memory/chatgpt_import.go`) silently discards:

1. **`is_starred`** — a boolean field already present on every conversation
   object inside `unzipped/conversations-*.json`. Currently never read.
2. **`unzipped/shared_conversations.json`** — a flat JSON array of conversations
   Mark explicitly generated a public share link for. Fields: `conversation_id`,
   `id` (share id), `is_anonymous`, `title`.
3. **`unzipped/message_feedback.json`** — a flat JSON array of thumbs up/down
   events. Fields: `conversation_id`, `create_time`, `id`, `rating`
   (`"thumbs_up"` or `"thumbs_down"`), `content` (a JSON-encoded string, usually
   `"{}"` but sometimes `"{\"text\": \"...\"}"` with a real written note),
   `update_time`. **No message-level id — this is conversation-level only.**

None of these require an LLM call, a vision call, or new external dependencies.
This is pure metadata plumbing: parse three additional inputs, join them onto
records by `conversation_id`, carry the result through to the Qdrant payload.

## Exact changes required

### 1. `internal/memory/models.go` (or wherever `NormalizedMemoryRecord` /
`MessageRecord` / `MemoryPoint` are defined — check `chatgpt_import.go` and
`models.go` for the real names before assuming)

Add fields to carry this signal through the pipeline:

```go
// On the conversation-level record (wherever conversation metadata lives):
UserStarred      bool   `json:"user_starred,omitempty"`
UserShared       bool   `json:"user_shared,omitempty"`
ShareID          string `json:"share_id,omitempty"`

// Feedback is conversation-level, so every record from a conversation that
// received feedback carries the same values. If a conversation has multiple
// feedback events, carry the most recent one by update_time.
FeedbackRating   string `json:"feedback_rating,omitempty"`   // "thumbs_up" | "thumbs_down" | ""
FeedbackNote     string `json:"feedback_note,omitempty"`     // from content.text, empty if none
FeedbackAt       string `json:"feedback_at,omitempty"`       // RFC3339, from update_time
```

Follow existing naming conventions in the file exactly — match casing/style
already used for similar fields (e.g. how `ConversationTitle` or `Project` are
named), don't invent a divergent convention.

### 2. `internal/memory/chatgpt_import.go`

- Parse `is_starred` off each conversation object in `ParseChatGPTExport` (it's
  a top-level key on the conversation map, sibling to `title`, `create_time`,
  etc. — same level as where `conversationTitle` is currently read). Handle
  `null`/missing gracefully as `false` — do not error on absence.
- Add a new exported function, e.g. `LoadShareSignals(rootDir string) (map[string]ShareSignal, error)`
  that reads `shared_conversations.json` from the export root (same directory
  `findConversationJSONFiles` walks) and returns a map keyed by `conversation_id`.
  Define `ShareSignal` as a small struct with `ShareID string` and
  `IsAnonymous bool`. If the file doesn't exist, return an empty map and no
  error — this file is optional (not every export will have it).
- Add a new exported function, e.g. `LoadFeedbackSignals(rootDir string) (map[string]FeedbackSignal, error)`
  that reads `message_feedback.json` and returns a map keyed by
  `conversation_id`, keeping only the most recent event per conversation (by
  `update_time`). Define `FeedbackSignal` with `Rating string`, `Note string`
  (parsed out of the `content` field's nested JSON — `content` is itself a
  JSON-encoded string, so you'll unmarshal twice: once for the outer object,
  once for `content` into `{Text string \`json:"text"\`}`), and `At string`.
  Same optional-file handling as above.
- Wire both maps into `ParseChatGPTExport`: after building each
  `NormalizedMemoryRecord`, look up its `ConversationID` in both maps and set
  the new fields. Wire `is_starred` directly from the conversation object.

### 3. `internal/memory/ingest.go`

Thread `UserStarred`, `UserShared`, `ShareID`, `FeedbackRating`, `FeedbackNote`,
`FeedbackAt` from the source record onto the constructed `MemoryPoint` in
`Ingestor.Ingest`, the same way `Project` and `Tags` are currently passed
through. Do not change chunking behavior, batching behavior, or the
`MemoryID` scheme — this task only adds payload fields.

### 4. CLI wiring (`cmd/frontpocket`, wherever `ingest chatgpt` is wired up)

No new flags required. The three new files should just be picked up
automatically when they exist alongside `conversations-*.json` in the same
export root. If `--dry-run` is passed, print counts: how many conversations
have `is_starred=true`, how many have a share signal, how many have a
feedback signal (broken down by thumbs_up/thumbs_down), before doing any
embedding calls.

## Tests (required — do not skip)

Add table-driven Go tests, following the existing style in `tests/` (see
`tests/chatgpt_import_test.go` for conventions already in use):

- `is_starred: true` on a conversation → the resulting records carry
  `UserStarred: true`.
- A conversation present in `shared_conversations.json` → records carry
  `UserShared: true` and the correct `ShareID`.
- A conversation absent from `shared_conversations.json` → `UserShared: false`,
  `ShareID: ""`.
- A conversation with two feedback events at different `update_time`s → the
  record carries the *later* one.
- A feedback event with `content: "{\"text\": \"totally lost personality\"}"` →
  `FeedbackNote` is parsed out correctly (use this exact string as a test
  fixture — it's real data from the export sample already reviewed).
- Missing `shared_conversations.json` / `message_feedback.json` entirely →
  no error, empty maps, all records get zero-value fields.

## Verification steps (run yourself before calling this done)

```bash
cd ~/frontpocket
go build ./...
go test ./tests/... -v

# Dry run against the real export — confirms counts without writing anything
QDRANT_COLLECTION=frontpocket_memory_phase1_test frontpocket ingest chatgpt \
  ./unzipped --dry-run --project FrontPocket

# Small real ingest into the TEST collection only — never the production one
QDRANT_COLLECTION=frontpocket_memory_phase1_test frontpocket ingest chatgpt \
  ./unzipped --project FrontPocket
```

Then hand back to me (or run yourself if you have Qdrant MCP access) a
`qdrant_scroll_points` sample from `frontpocket_memory_phase1_test` filtered
on `user_starred=true` and separately on `feedback_rating=thumbs_down`, to
confirm the fields actually landed correctly in the payload.

## Explicit non-goals for this task

- No image/attachment resolution (`conversation_asset_file_names.json` /
  `library_files.json`) — that's Phase 2, separate task.
- No `model_slug` / `conversation_template_id` capture — Phase 3.
- No changes to `fp_reflect_loop.py`, `memory_cleanup.py`, or anything in the
  Python reflection pipeline.
- No writes to `frontpocket_memory` or `fp_reflections` (production
  collections) under any circumstance.
- No changes to chunking logic of any kind.

## Definition of done

- `go build ./...` and `go test ./...` both pass clean.
- Dry-run prints correct counts against the real export in `unzipped/`.
- A real test ingest into `frontpocket_memory_phase1_test` shows the six new
  fields populated correctly on spot-checked points.
- Production collections (`frontpocket_memory`, `fp_reflections`) are
  untouched — confirmed by point count before/after matching.
