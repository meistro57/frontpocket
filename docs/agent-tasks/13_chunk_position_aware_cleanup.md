# FrontPocket — Chunk-Position-Aware Quote Quality (Cleanup Pipeline)

## Context

`memory_cleanup.py`'s quote-quality gate flags things like "ends mid
sentence" or "starts on a lowercase mid-word fragment" as malformed/
truncated. That's correct for a genuinely broken chunk, but wrong for a
completely normal one: **when a long message gets split into many chunks,
every chunk except the very last one is expected to "end mid-sentence" —
that's just where the chunk boundary fell.** Same for "starts lower" —
only the *first* chunk of a message should be expected to start cleanly.

Confirmed with a real example: `"Excel tips for dining services"`,
`memory_id: ..._turn_34186_chunk_013` — its `text` field ends
`"Add column **J – \`"` (mid-formula-name), which today's field-location
fix correctly flagged as `truncated`. But this is chunk 13 of what's
likely a 20+-chunk message — of course it doesn't end cleanly, it's not
the end of the message, it's the end of *this piece* of the message. The
cleanup script currently has no way to know that distinction.

This is the second, deeper reason `safe_for_reflection` barely moved after
today's field fix (166,878 → 169,021) despite `clean`/`malformed_quotes`
improving substantially — a large fraction of "truncated"/"malformed"
flags are probably just ordinary non-terminal chunks, not actually bad data.

## Fix — make quality assessment chunk-position-aware

### Step 1 — confirm the memory_id pattern holds across both sources

`memory_id` format observed: `{conversation_id}_turn_{N}_chunk_{M:03d}}`,
consistent across both ChatGPT-origin points (e.g.
`68085cba-..._turn_21345_chunk_003`) and Claude-origin points (e.g.
`692a306a-..._turn_139_chunk_014` from earlier today's archaeology).
Confirm this holds universally — check for any `memory_id`s that don't
match this pattern (e.g. `memories.json`/`projects.json`-derived points,
which use a different ID scheme entirely and should be exempted from this
chunk-position logic, not force-parsed).

### Step 2 — build a chunk-position map before classifying

Add a cheap pre-scan pass over `frontpocket_memory` (payload-only,
`memory_id` field alone — no need for full payload, keep this fast) that
builds a map: `(conversation_id, turn_number) -> max_chunk_number_seen`.
Since this is local processing with no API calls, a second scroll pass over
the ~322K points should still be fast.

### Step 3 — thread chunk position into `clean_point`/`detect_quote_quality`

Parse `memory_id` for each point to get its own `turn_number`/
`chunk_number`; look up the map to determine `is_first_chunk`
(`chunk_number == 1`) and `is_last_chunk` (`chunk_number ==
max_chunk_number_seen` for that turn). Points whose `memory_id` doesn't
match the turn/chunk pattern at all (e.g. `memories.json`-derived) should
be treated as a single-chunk message (both first and last), so nothing
here changes their existing behavior.

Update the warning-to-classification logic:
- `"possible_midword_start"` / `"starts_lower_fragment"` → only treat as a
  real quality problem when `is_first_chunk`. For a non-first chunk, this
  is expected and should not push `quote_quality` toward `"malformed"`.
- `"ends_mid_sentence"` → only treat as a real quality problem when
  `is_last_chunk`. For a non-last chunk, expected, should not push toward
  `"truncated"`.
- `"unmatched_parentheses"` — leave as-is for now (genuinely ambiguous for
  code content regardless of chunk position, out of scope for this fix).
- `"ellipsis_truncation"`, `"very_short_quote"`, `"low_semantic_content"` —
  unaffected, these are legitimate regardless of chunk position.
- Still record the original warnings in `cleanup_warnings` for
  transparency (don't hide the fact that a chunk is non-terminal), just
  don't let them force a lower `quote_quality`/block `safe_for_reflection`
  when they're expected given position.

Add a real, visible signal for this too — a new field on the cleaned
payload, e.g. `chunk_position: "first" | "middle" | "last" | "single"`, so
downstream (the reflect loop, or manual review) can see this reasoning
rather than it being invisible.

### Step 4 — real before/after verification

Re-run `python -m frontpocket.memory_cleanup --write-cleaned` (wipe
`fp_cleaned_memory` first, same as today's pattern) and report:
- New `safe_for_reflection` count — expect a meaningful jump this time.
- Specifically re-check the "Excel tips for dining services"
  `..._turn_34186_chunk_013` point — confirm it now shows `chunk_position:
  "middle"` (assuming a `chunk_014`+ exists for that turn) and no longer
  gets blocked purely for ending mid-formula.
- Confirm a genuine first-chunk-with-bad-start and genuine
  last-chunk-with-bad-ending example (if any exist) still correctly get
  flagged — this fix should tighten precision, not eliminate quality
  detection entirely.

## Non-goals

- No change to `unmatched_parentheses` handling (separate, harder problem).
- No change to the code-chunking issue from earlier ("The Agency Phase 1"
  starting mid-function-signature) — that's a chunker-level fix, not a
  cleanup-classification fix, and is a separate, already-noted follow-up.
- No changes to the Go ingest pipeline or chunker itself.

## Definition of done

- Chunk-position map built and correctly threaded into classification.
- New `chunk_position` field visible on cleaned payloads.
- Re-run shows a real, reported jump in `safe_for_reflection`, with the
  specific "Excel tips" example confirmed fixed.
- Genuine first/last-chunk quality issues still correctly flagged (not a
  blanket suppression).
