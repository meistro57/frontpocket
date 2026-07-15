# FrontPocket — Root Cause: Recurring Broken-Pipe Failures Around Record ~700-800

## Context

Two consecutive full Claude ingest runs have now failed at nearly the exact
same point: record ~700-800, ~8 minutes elapsed, both times with a burst of
`broken pipe` errors on `PUT /collections/frontpocket_memory/points?wait=true`
against `127.0.0.1:6333` — a purely local connection. This is not random
noise; it's reproducible enough to have a real, findable cause.

Quantization background reindexing has been ruled out — confirmed via
`fp_list_collections` that every collection (including `mb_claims` and
`meta_reflections`, which got TurboQuant applied yesterday) shows
`"status": "green"`, not `"yellow"`/optimizing.

Last night's write-reliability fix (removing the silent in-memory fallback,
adding retry-with-backoff, adding the hard integrity-check gate) is working
correctly — it turned this same failure from a silent 718-chunk data loss
into a loud, honest, partial-but-correct stop. That fix stays. This task is
about finding why the failure keeps happening at this specific point, not
about the failure-handling behavior itself.

## Leading hypothesis, to confirm or rule out with real evidence

Given the failure clusters around a consistent record range, the most
likely cause is an **unusually large batch payload** around that point in
the Claude export — not a random network blip. Earlier archaeology on this
same export found some Claude messages (`tool_use`/`tool_result` blocks
tied to large project docs, e.g. the "Advance Steel API" project's
`prompt_template` + `docs[]` content) produced hundreds of chunks from a
single source message. If a batch happens to land on one of these
outsized records, the resulting HTTP PUT body could be large enough to hit
a timeout, a connection-level limit, or simply take long enough that a
pooled/keep-alive connection gets invalidated mid-write — producing exactly
the broken-pipe pattern seen.

## Step 1 — Confirm what's actually at record ~700-800 (real data, not guesswork)

Using `--dry-run` or `--limit`/`--conversation-id` on the Claude importer,
identify which specific conversation(s) and message(s) correspond to
records ~650-800 in the processing order. For each, report:
- Raw message size (character count) before chunking
- Number of chunks it expands into
- Whether it's a `tool_use`/`tool_result`-heavy message (matching the
  pattern already found to produce outsized chunk counts)

## Step 2 — Measure actual batch payload sizes around the failure point

Add temporary instrumentation (or use existing logging if the batch size in
bytes is already knowable) to log the byte size of each PUT payload sent to
Qdrant during a real run. Re-run a limited/targeted ingest covering the
suspect record range and report the actual payload sizes — confirm whether
the batch(es) immediately before the failure point are meaningfully larger
than typical batches elsewhere in the run.

## Step 3 — Check for a real size/timeout ceiling

- Check the Go `http.Client` used for Qdrant writes (`internal/store/qdrant.go`)
  for any configured `Timeout`, and compare against how long an oversized
  batch write might realistically take.
- Check Qdrant's own default limits — is there a request body size limit or
  a default request timeout that a sufficiently large batch could exceed?
  (Check `kae-qdrant`'s actual config/env, not just Qdrant's generic docs —
  confirm what's really configured on this instance.)
- Check `docker logs kae-qdrant` around both failure timestamps
  (11:24-11:30 from the first run, and 12:07:46-12:08:02 from tonight's
  second run) specifically for any size/timeout-related log lines on the
  server side, not just absence of a "broken pipe" line (already checked
  and found nothing server-side last time — look harder for anything at
  all logged in that exact window, even seemingly unrelated).

## Step 4 — Fix, once the real cause is confirmed

Depending on what Steps 1-3 find, the fix is one of:
- **If it's genuinely an oversized single batch**: split batches by payload
  byte size as well as point count (e.g. cap at both 128 points AND some
  reasonable byte ceiling, flushing early if either limit is hit) — so one
  outsized record doesn't produce one outsized write.
- **If it's a client-side timeout**: increase the Go `http.Client` timeout
  specifically for the Qdrant write path (not embedding calls), since a
  large legitimate batch may just need more time, not a smaller size.
- **If it's a connection pool/keep-alive staleness issue** (worth checking
  regardless — the burst-of-4-failures-within-seconds pattern in tonight's
  log looks consistent with several pooled connections all going stale at
  once): check `http.Transport`'s `IdleConnTimeout`/`MaxIdleConnsPerHost`
  settings for the Qdrant client and compare against Qdrant's own
  keep-alive behavior; consider disabling keep-alive for this specific
  client if that proves to be the real cause.

Don't guess at which of these it is — Steps 1-3 should produce enough real
evidence to know which fix actually applies, rather than trying all three
speculatively.

## Verification

```bash
cd ~/frontpocket
go build ./... && go test ./tests/... -v
./make_all.sh

# Real test: re-run specifically targeting the suspect record range and
# confirm it now completes without a broken-pipe failure.
./bin/frontpocket ingest claude ./Claude-Marks-chat_history_7-1-26 \
  --project FrontPocket --ai-provider claude
```

Report the actual real numbers found in Steps 1-2 (message sizes, chunk
counts, payload byte sizes) as part of the completion report — this is a
root-cause investigation task, the numbers found ARE the deliverable, not
just whether the fix works.

## Non-goals

- No changes to the fallback-removal or retry-with-backoff logic from last
  night's fix — that behavior is correct and stays as-is.
- No `--resume` support added to the Claude importer as part of this task
  (separate, already-identified follow-up — worth doing next, but keep
  this task focused on root cause).

## Definition of done

- Real evidence from Steps 1-3 identifying the actual cause (oversized
  batch, client timeout, or connection staleness) — not a guess.
- A fix applied that matches the confirmed cause.
- A real full Claude ingest run completes past record 800 without a
  broken-pipe failure, reported with actual terminal output.
