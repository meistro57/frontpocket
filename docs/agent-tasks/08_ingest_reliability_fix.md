# FrontPocket — Ingest Reliability: Fix the Real Waste, Not Just the Symptom

## Context

The full ChatGPT re-ingest has now crashed twice today at nearly the same
point (~record 6,333) with an identical transient error:
`openrouter returned 0 vectors for 1 inputs`. That alone is a known,
fixable class of problem (transient API blip, needs retry-with-backoff).

But the bigger issue, which Mark caught by ear ("it builds everything up
then dumps, meta-bridge does it better"): the actual architecture wastes
real work — and real money — on every restart, independent of the crash
bug. This task fixes the structural problem, not just adds a retry.

## The real problem, confirmed by reading the code

`ParseChatGPTExport()` (called once, at the very start of `runIngestChatGPT`
in `cmd/frontpocket/ingest_chatgpt.go`) does **all** of the following in one
monolithic, entirely-in-memory pass, for the whole export, before the
resumable embedding/write loop (`Ingestor.Ingest()`) ever starts:

- Walks all 2,585 conversations
- Resolves all ~2,023 attachment references
- Runs **all ~1,310 real vision-captioning API calls**

The `ResumeJournal` only tracks progress through `Ingestor.Ingest()` (the
embed-and-write loop). It has zero visibility into the parse+caption phase.
**Practical consequence, confirmed today: every restart re-runs all ~1,310
vision captions from scratch — real OpenRouter spend, completely wasted,
twice already today.** This is the actual cost of the "build everything up
then dump" pattern, independent of whether the embedding resume even works.

Compare to meta-bridge's own architecture (already in this codebase's
neighborhood): typed pipeline stages (source → chunker → extractor → store),
each with its own persistence and content-hash dedup, so a re-run skips
work already done instead of redoing it blind. That's the shape to move
toward here.

## Step 1 — Investigate before fixing (real root causes, not assumptions)

1. **Why didn't `--resume` actually resume today?** After the crash at
   record 6,333, the first restart attempt failed immediately on
   `--ai-provider is required` (a flag validation error, before
   `OpenFileJournal` is ever called — so it shouldn't have touched the
   journal file). The *second* restart, with correct flags, printed
   `"resume: tracking progress in ./ingest_progress.json"` — the **fresh**
   tracking message, not `"resume: continuing from record X"` — meaning it
   started over from scratch despite `--resume ./ingest_progress.json`
   being passed both times. Check the actual current contents of
   `./ingest_progress.json` (if it still exists) and trace through
   `OpenFileJournal`'s logic in `internal/memory/resume.go` for why it
   decided this was a fresh run rather than a resume. Consider: does
   `JournalMeta` (Source/Collection/EmbeddingModel) comparison reject a
   resume if any field differs even slightly between runs? Report the real
   cause with evidence, not a guess.
2. **Confirm the caption re-run cost precisely.** Check whether
   `Captioner`/`VisionCaptioner` has any caching at all currently (it
   shouldn't, based on the code reviewed for Phase 2, but confirm). Report
   the real count of vision API calls made across today's two crashed runs
   (should be ~1,310 × 2 if the hypothesis above is correct) so the actual
   wasted spend is known, not estimated.

## Step 2 — Cache captions by content hash (kills the repeated-cost problem regardless of anything else)

Add a persistent, on-disk cache keyed by the attachment's actual content
hash (not filename — filenames can collide across conversations, but
content hash can't false-negative). Suggested: a simple JSON or SQLite
cache file, e.g. `.caption_cache.json` in the export root or a configurable
path via a new `--caption-cache <path>` flag (default alongside the
`--resume` journal path).

- Before calling the vision API for any attachment, hash its bytes and
  check the cache.
- On a hit: use the cached caption, zero API cost, zero vision call.
- On a miss: caption normally, then write the result to the cache
  immediately (not batched — a crash right after should still preserve
  what was already captioned).
- This fix alone means a third crash-and-restart today would cost zero
  additional vision spend for anything already captioned once.

## Step 3 — Checkpoint the parse phase itself, not just the embed phase

Right now the resumable unit is "record index within `Ingestor.Ingest()`."
Extend real checkpointing to cover the parse+caption phase too, at
per-conversation granularity (matching meta-bridge's per-source-unit
persistence pattern):

- After each conversation is fully parsed (messages extracted, attachments
  resolved, captions generated/cache-hit), mark that conversation as done
  in a checkpoint file distinct from the embed-loop journal (or extend the
  existing `ResumeJournal` interface to cover both phases — pick whichever
  is the smaller, cleaner change, and justify the choice).
- On restart, skip conversations already fully parsed+captioned; only
  resume embedding/writing from where that phase left off.
- Report the real end-to-end behavior: crash mid-parse, restart, confirm
  zero re-work for conversations already fully processed.

## Step 4 — Retry-with-backoff for transient embedding failures

Separate from the caching/checkpointing fix, add resilience to the actual
failure that's been triggering these crashes:

- In the embedder's OpenRouter call path (`internal/embed/`, wherever
  `EmbedBatch`/`EmbedText` live), add retry logic for a transient "0
  vectors returned" response: 2-3 retries with exponential backoff
  (e.g. 1s, 3s, 9s) before giving up and returning the error.
- Only retry on this specific transient-looking failure shape, not on
  genuine auth/config errors — don't mask real problems.
- Add a regression test simulating a transient 0-vector response followed
  by a successful retry, confirming the batch completes rather than
  aborting the whole ingest.

## Verification

```bash
cd ~/frontpocket
go build ./... && go test ./tests/... -v
./make_all.sh

# Small real test proving the caching fix works: run a limited ingest,
# let it "crash" (or just interrupt it) partway through, restart, and
# confirm via real OpenRouter activity (or a call counter in the caption
# cache) that already-captioned attachments do NOT trigger new vision calls.
QDRANT_COLLECTION=frontpocket_memory_reliability_test ./bin/frontpocket ingest chatgpt \
  ./unzipped --project FrontPocket --ai-provider chatgpt --limit 50 \
  --resume ./test_progress.json --caption-cache ./test_caption_cache.json
# interrupt with Ctrl+C partway through, then re-run the identical command
# and confirm real evidence (cache hit count, or absence of new gen-* rows
# in a fresh OpenRouter activity pull) that captions weren't re-run.
```

Report real evidence — cache hit/miss counts, actual resume behavior
confirmed via the journal file contents, not just "tests pass."

## Non-goals

- No changes to the Claude importer.
- No full-corpus production run as part of this task — this is a
  reliability fix, verified on a small/interrupted test run, not a
  green-light to re-run the full 45K-record ingest yet.
- Don't touch `frontpocket_memory` (still needs a clean successful run
  once this is fixed — that's the next step after this, not part of it).

## Definition of done

- Real root cause found and explained for why today's `--resume` runs
  didn't actually resume.
- Caption caching implemented, verified to prevent re-captioning identical
  attachments across a real interrupt-and-restart test.
- Parse-phase checkpointing implemented at per-conversation granularity,
  verified the same way.
- Retry-with-backoff added for transient embedding failures, with a
  passing regression test.
- A real interrupt-and-restart test run reported with actual evidence
  (not a summary claim) that restarting costs meaningfully less than
  starting over.
