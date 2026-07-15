# FrontPocket / MindDrill — Expose Today's New Fields as Real Filters

## Context

Today's work added `ai_provider`, `ai_model`, `user_starred`, `user_shared`,
`feedback_rating`/`feedback_note`, and `attachment_*` fields to every point
in `frontpocket_memory`. None of these are filterable through the API or
visible in MindDrill yet — the schema moved faster than the UI/API layer
that's supposed to expose it. `docs/minddrill-roadmap.md`'s Sprint 2 filter
list (`project`, `memory_kind`, `source_type`, `speaker`, `tags`, `date
range`) predates all of this and needs updating alongside the actual code.

Confirmed by reading the real code (`internal/store/qdrant.go`,
`internal/store/qdrant_scroll.go`):

- `toQdrantPayload`/`fromQdrantPayload` (used by `Search`) **already**
  read/write `ai_provider`/`ai_model` correctly — search results carry
  these fields fine today.
- `memoryPointFromPayload` (used by `ScrollRaw`, the browse path) does
  **not** read `ai_provider`/`ai_model` at all — a separate, smaller gap.
- `toQdrantFilter(filters memory.SearchFilters)` only builds Qdrant filter
  clauses for `project`, `memory_kind`, `speaker`, `source_type`,
  `conversation_id`, `tags` — nothing for any of today's new fields, even
  though the data itself is fully present and correctly tagged in Qdrant.
- `memory.SearchFilters` (wherever that struct is defined — check
  `internal/memory/models.go` or similar) has no fields for any of this
  either — needs extending before `toQdrantFilter` can use them.

## Task

### Step 1 — Extend `memory.SearchFilters`

Add fields for: `AIProvider string`, `AIModel string`, `UserStarred *bool`,
`UserShared *bool`, `FeedbackRating string`, `HasAttachment *bool` (true =
`attachment_source_system` is non-empty, false = it's empty — use a
pointer so "not specified" is distinguishable from "false").

### Step 2 — Extend `toQdrantFilter` in `internal/store/qdrant.go`

Add clauses for each new filter field. The existing `appendMatch` helper
only handles non-empty strings — add a parallel bool-aware helper for
`UserStarred`/`UserShared` (match `true` or `false` explicitly, only when
the pointer is non-nil), and for `HasAttachment` build a `must_not: {key:
"attachment_source_system", match: {value: ""}}` clause when true, or the
inverse when false.

### Step 3 — Fix the browse-path gap in `qdrant_scroll.go`

Add `AIProvider: asString(payload["ai_provider"])` and
`AIModel: asString(payload["ai_model"])` to `memoryPointFromPayload` —
confirm `memory.MemoryPoint` already has these fields (it's used
elsewhere for ingest, so it likely does) and just wasn't being read here.

### Step 4 — Wire new filters into whatever parses browse/search requests from HTTP

`statsFiltersFromQuery` in `routes_memory.go` handles `/memory/stats`'s
query params — extend it with `ai_provider`, `ai_model`, `starred`,
`shared`, `feedback_rating`, `has_attachment` (parse as bool where
appropriate). **Locate the actual `/memory/browse` handler** (referenced in
`docs/minddrill-roadmap.md` but not seen in the `routes_memory.go` section
read so far — check `internal/api/` for where browse requests get parsed,
likely a similar query-param-to-`SearchFilters` function) and extend it
the same way. Don't guess at the function name — find it, read it, extend
it consistently with `statsFiltersFromQuery`'s pattern.

### Step 5 — MindDrill UI: real filter controls

Per the roadmap's own Sprint 2 goals (already written, just needs updating
for the current schema), add UI controls for:
- **AI Provider** — dropdown: All / Claude / ChatGPT
- **Starred** — checkbox
- **Shared** — checkbox
- **Feedback** — dropdown: Any / Thumbs Up / Thumbs Down
- **Has Image** — checkbox (given real image captions now exist, this is
  a genuinely new, useful way to browse — "show me photos I've discussed")

Wire these directly to the corresponding query params from Step 4. Follow
whatever pattern the existing project/memory_kind/speaker filters already
use in the MindDrill frontend for consistency — don't invent a new UI
pattern for these specifically.

### Step 6 — Update the roadmap doc

Update `docs/minddrill-roadmap.md`'s Sprint 2 filter list to reflect what
actually shipped, so it stops being stale the moment this lands.

## Verification

```bash
cd ~/frontpocket
go build ./... && go test ./tests/... -v
./make_all.sh
```

Real test: start the API server, hit `/memory/stats?ai_provider=claude`
and `/memory/stats?ai_provider=chatgpt` directly, confirm the counts are
disjoint and sum to the total (322,260 as of today). Then hit
`?starred=true` and `?feedback_rating=thumbs_down` and confirm non-zero,
sane results against the real data — same standard as every other
verification today, real query results, not just a clean compile.

## Non-goals

- No changes to the ingest pipeline, chunker, cleanup script, or reflection
  loop — this is purely the API/MindDrill exposure layer.
- No new memory-lens presets beyond what's listed in Step 5 — that's a
  separate, later Sprint 2 item if wanted.

## Definition of done

- All six new filter fields work end-to-end: HTTP query param → parsed
  filter → correct Qdrant `must`/`must_not` clause → correct results.
- `ai_provider`/`ai_model` now correctly populated on browse results too
  (Step 3), not just search results.
- MindDrill UI has working controls for all six, verified by hand against
  real data (e.g. toggling "Claude only" actually changes what's shown).
- Roadmap doc updated to match reality.
