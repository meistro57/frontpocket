# FrontPocket — Phase 1 Correction: Stale Binary, Not a Code Bug

## What happened

I reviewed your Phase 1 changes directly — `internal/memory/models.go`,
`internal/memory/chatgpt_import.go`, and `internal/store/qdrant.go`. **The
code is correct.** `LoadShareSignals`, `LoadFeedbackSignals`,
`parseFeedbackNote`, the `is_starred` parsing, the wiring through
`NormalizedMemoryRecord` → `MessageRecord` → `MemoryPoint` →
`toQdrantPayload`, and the new `StarredConversations` /
`SharedConversations` / `FeedbackConversations` / `FeedbackThumbsUp` /
`FeedbackThumbsDown` counters on `ChatGPTImportResult` all match the Phase 1
spec exactly, including the "keep most recent feedback event by
`update_time`" dedup logic and the double-unmarshal needed to pull `text`
out of the nested `content` JSON string in `message_feedback.json`.

The test ingest into `frontpocket_memory_phase1_test` (1,966 points) does
**not** contain any of the six new fields (`user_starred`, `user_shared`,
`share_id`, `feedback_rating`, `feedback_note`, `feedback_at`) — but it does
contain canon-review fields (`canonical`, `confidence`, `status`, etc.) that
already existed in the codebase before this task. That pattern only makes
sense one way: **the ingest command ran against a stale, pre-existing
compiled binary** (`bin/frontpocket`, 8.93 MB, sitting in the repo from an
earlier build) instead of a binary rebuilt from your changes.

## What to do

1. Confirm this diagnosis — check whether `bin/frontpocket`'s mtime predates
   your edits to `internal/memory/chatgpt_import.go`. If your shell/tooling
   doesn't expose file mtimes easily, don't worry about proving it — just
   proceed to step 2 regardless, it's harmless either way.

2. **Rebuild for real**, not just `go build` into a temp location — make
   sure whatever binary actually gets invoked next is the fresh one:

   ```bash
   cd ~/frontpocket
   ./make_all.sh
   # or, if you want to be explicit about exactly what gets built:
   go build -o bin/frontpocket ./cmd/frontpocket
   ```

3. Run `go build ./...` and `go test ./...` first regardless — confirm both
   are clean before touching Qdrant again.

4. **Delete the stale test collection and start clean** — it currently
   holds 1,966 points built with the old binary, none of which reflect your
   actual code:

   ```bash
   curl -X DELETE http://localhost:6333/collections/frontpocket_memory_phase1_test
   ```

5. Re-run the exact same verification sequence from the original task
   brief, using the freshly built binary explicitly (don't rely on
   whatever's on `$PATH` unless you've confirmed it points at the binary you
   just built):

   ```bash
   cd ~/frontpocket

   ./bin/frontpocket ingest chatgpt ./unzipped --dry-run --project FrontPocket
   # Confirm the dry-run output shows non-zero counts for starred/shared/feedback
   # conversations — check ChatGPTImportResult's new counters are being printed.
   # If the CLI doesn't currently print these counts on --dry-run, add that now:
   # it was in scope for the original task ("If --dry-run is passed, print counts").

   QDRANT_COLLECTION=frontpocket_memory_phase1_test ./bin/frontpocket ingest chatgpt \
     ./unzipped --project FrontPocket
   ```

6. Verify directly against Qdrant that real points now carry real values —
   don't just trust exit code 0:

   ```bash
   curl -s -X POST http://localhost:6333/collections/frontpocket_memory_phase1_test/points/scroll \
     -H 'Content-Type: application/json' \
     -d '{"limit": 5, "filter": {"must": [{"key": "user_starred", "match": {"value": true}}]}}'

   curl -s -X POST http://localhost:6333/collections/frontpocket_memory_phase1_test/points/scroll \
     -H 'Content-Type: application/json' \
     -d '{"limit": 5, "filter": {"must": [{"key": "feedback_rating", "match": {"value": "thumbs_down"}}]}}'
   ```

   Each should return real points with non-empty `user_starred: true` /
   `feedback_rating: "thumbs_down"` and (for the feedback query) a populated
   `feedback_note` on at least the "totally lost personality" conversation.

## Do not

- Do not modify `models.go`, `chatgpt_import.go`, or `qdrant.go` further —
  they're correct as written. If step 6's verification still comes back
  empty after a confirmed clean rebuild, stop and report back with the exact
  dry-run output and curl responses rather than guessing at further code
  changes.
- Do not touch `frontpocket_memory` or `fp_reflections` (production).
- Same non-goals as the original task: no image/attachment resolution, no
  `model_slug` capture, no Python reflection pipeline changes.

## Definition of done

- `go build ./...` and `go test ./...` pass clean on a fresh build.
- `frontpocket_memory_phase1_test` recreated from scratch with the rebuilt
  binary.
- The two verification `curl` scroll queries above both return non-empty
  results with correctly populated fields.
- Report back the actual JSON from both verification queries, not just
  "done" — I want to see the real payload before this gets anywhere near
  production data.
