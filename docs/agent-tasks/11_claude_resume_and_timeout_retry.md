# FrontPocket — Claude Importer: Add `--resume` + Extend Embedding Retry to Cover Timeouts

## Context

Two real gaps have now cost real restart time on the Claude ingest:

1. The Claude importer (`cmd/frontpocket/ingest_claude.go`) has no `--resume`
   support at all — confirmed by reading the code. Every crash or manual
   stop means a full restart from record 0, even though `memory_id`s are
   deterministic (safe to re-upsert, just wasteful to re-embed/re-write
   everything already done).
2. Tonight's run got past the batch-size wall (fixed earlier today) and
   then stopped at record 2645 on an OpenRouter **embedding timeout** — a
   different failure shape than the "0 vectors returned" case that already
   has retry logic (`isRetryableEmbeddingError` in
   `internal/embed/http_util.go`). A genuine timeout isn't currently
   retried at all.

Fix both before the next attempt, so this is the last full restart needed.

## Part 1 — `--resume` for the Claude importer

Mirror what already works correctly in `ingest_chatgpt.go` — don't
reinvent the journal format, reuse `internal/memory/resume.go`'s
`OpenFileJournal`/`ResumeJournal` exactly as-is.

- Add `--resume <path>` flag to `ingest_claude.go`'s flag parsing, same
  shape as the ChatGPT command.
- Wire the journal into the embed/write loop the same way
  `runIngestChatGPT` does — `OpenFileJournal(path, JournalMeta{...})`,
  check `resumed` bool, print the matching log lines
  (`"resume: tracking progress in ..."` vs
  `"resume: continuing from record X"`) so behavior and observability
  match the ChatGPT path exactly.
- **Important, given today's earlier resume bug**: confirm the journal's
  `Source` field resolution is consistent regardless of CWD (this was the
  root cause of the ChatGPT resume silently not working earlier today —
  make sure the Claude path doesn't inherit the same fragility). Add an
  explicit log line if `--resume` is passed but the file isn't found,
  distinguishing "starting fresh because no journal exists yet" from any
  future path-mismatch confusion — this was flagged as a good idea
  earlier today and never implemented; do it now for both importers if
  it's a small shared change.
- Real test: run with `--limit 200`, interrupt partway (Ctrl+C), restart
  with the same `--resume` path and same CWD, confirm it actually prints
  `"continuing from record X"` and doesn't re-embed/re-write already-done
  records. Report the real evidence, not just that it compiled.

## Part 2 — Extend embedding retry to cover genuine timeouts

In `internal/embed/http_util.go`, `isRetryableEmbeddingError` currently
only matches the "0 vectors returned" text pattern. Extend it (or add a
sibling predicate, whichever keeps the code cleaner) to also treat a real
timeout as retryable — mirror the pattern already proven correct today in
`internal/store/qdrant.go`'s `isRetryableQdrantConnectionError`:

```go
if errors.Is(err, context.DeadlineExceeded) {
    return true
}
var netErr net.Error
if errors.As(err, &netErr) && netErr.Timeout() {
    return true
}
```

Keep this narrow — timeouts and the existing 0-vector case, not a blanket
retry-on-any-error. Same backoff schedule as the existing retry logic
(`1s, 3s, 9s`) unless there's a good reason to differ.

Investigate briefly why record 2645 specifically timed out — check if it
corresponds to another oversized chunk (like record #798's 1,012-chunk
monster from earlier today) that might genuinely need a longer timeout
rather than just a retry, similar to how the Qdrant write path needed a
real batch-size cap rather than just retry logic. If the same record
reliably times out even after retries, that's a sign the fix needs to be
"give it more time" rather than "try again," and worth reporting rather
than just wrapping it in a retry that will predictably also fail.

## Verification

```bash
cd ~/frontpocket
go build ./... && go test ./tests/... -v
./make_all.sh

# Resume test
./bin/frontpocket ingest claude ./Claude-Marks-chat_history_7-1-26 \
  --project FrontPocket --ai-provider claude --limit 200 \
  --resume ./claude_resume_test.json
# interrupt partway, then re-run the identical command and confirm real
# "continuing from record X" behavior, not a silent restart.

# Full real run once both fixes are verified
./bin/frontpocket ingest claude ./Claude-Marks-chat_history_7-1-26 \
  --project FrontPocket --ai-provider claude \
  --resume ./ingest_progress_claude.json
```

## Non-goals

- No changes to the ChatGPT importer's own resume logic (already working).
- No changes to the Qdrant write batching/retry logic from earlier today
  (that's confirmed working, stays as-is).
- No full production run as part of this task — verify on `--limit 200`
  first, report real evidence, then hand back for the real run.

## Definition of done

- `--resume` works on the Claude importer, proven via a real
  interrupt-and-restart test with actual "continuing from record X" output.
- Embedding timeout errors are now retried with the same backoff pattern
  as the Qdrant write path, verified via a real or simulated timeout
  scenario.
- Real investigation into whether record 2645 needs more than a retry
  (oversized chunk) or whether a retry alone resolves it.
