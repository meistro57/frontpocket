# FrontPocket — Fix Silent Data Loss on Qdrant Write Failure

## Context — this is a confirmed, real data-loss bug, not a hypothesis

During tonight's Claude ingest, the terminal logged two `broken pipe` write
failures on `PUT /collections/frontpocket_memory/points?wait=true` (both on
`127.0.0.1` — a purely local connection, so this isn't a flaky network,
something is actually straining Qdrant or the connection between them).

**Confirmed via direct comparison, not assumed:** at the terminal's own
checkpoint of `1000/12071 records (9843 chunks)`, a live Qdrant count of
`ai_provider: "claude"` points came back as only **9,125** — a real gap of
~718 chunks, in the exact window of those two failures. The ingest kept
running and reporting progress normally the whole time, with no error
surfaced anywhere.

## Root cause, confirmed by reading `internal/store/qdrant.go`

`QdrantMemoryStore.Upsert()`:

```go
_, err := s.client.doJSON(ctx, http.MethodPut, fmt.Sprintf("/collections/%s/points?wait=true", s.collection), payload, nil)
if err != nil {
    if s.fallback != nil {
        if fallbackErr := s.fallback.Upsert(ctx, points); fallbackErr == nil {
            return nil
        }
    }
    return err
}
```

When the Qdrant write fails, it silently falls back to an **in-memory
store** (`memory.NewInMemoryStore()`, wired in by `buildCLIIngestor` in
`cmd/frontpocket/ingest_chatgpt.go` and `ingest_claude.go`). If the
in-memory fallback succeeds — which it always will, it's just a map — this
function returns `nil` (success). The ingestor's own `chunksEmbedded`
counter increments regardless. The in-memory store is ephemeral and dies
with the process. **Any batch that hits a Qdrant write failure is silently,
permanently lost, with the CLI reporting success the entire time.**

This fallback pattern likely makes sense for the live API server (graceful
degradation for user-facing search when Qdrant is briefly unavailable), but
for a one-shot CLI ingest job, silent data loss is unacceptable — durability
*is* the entire point of the operation.

## Fix, in order

### 1. CLI ingest paths must not use the silent fallback at all

In `buildCLIIngestor` (shared by both `ingest_chatgpt.go` and
`ingest_claude.go`), pass `fallback: nil` (or whatever the equivalent is)
to `NewQdrantMemoryStore` for the CLI ingest commands specifically. A write
failure that survives retries (see below) should become a real, hard,
ingest-stopping error — exactly like `Ingestor.Ingest()` was already
designed to do on an embedding failure. This is safe now in a way it wasn't
before today: both the embed-loop resume journal and the new parse-phase
checkpoint mean a hard stop is recoverable, not a full restart. Silent data
loss is not recoverable at all — a loud stop is strictly better here.

Confirm this doesn't regress the live API server's own use of
`QdrantMemoryStore` (`internal/api/`) — that path should very likely keep
its fallback for graceful degradation. Scope this change to the CLI
ingest construction only, not the shared store type's default behavior.

### 2. Add retry-with-backoff on the Qdrant write path for transient network errors

Before removing the fallback, absorb genuine transient blips so a real
network hiccup doesn't turn into a hard-stopped ingest unnecessarily. In
`QdrantClient` (or wherever the PUT to `/points` happens), add retry logic
specifically for connection-level errors — broken pipe, connection reset,
EOF, timeout — distinct from Qdrant returning a real error status (4xx/5xx
with a body, which should NOT be retried, that's a real problem to
surface). Same shape as today's OpenRouter embedding retry: 2-3 attempts,
exponential backoff (e.g. 1s, 3s, 9s). Add a narrow error-matching
predicate (mirroring `isRetryableEmbeddingError`'s pattern) so this doesn't
mask genuine failures.

### 3. Investigate what actually caused the broken pipes

Before assuming this was random, check what else might have been
contending for Qdrant at 11:24–11:30 today. Things to check and report:
- `docker stats kae-qdrant` — was it CPU/memory constrained at that time?
- Was anything else hitting Qdrant concurrently (another ingest, a
  reflection loop, MindDrill, an MCP tool call)?
- Check Qdrant's own container logs (`docker logs kae-qdrant`) around that
  timestamp for anything on its side (e.g. segment optimization, the
  TurboQuant quantization work applied yesterday triggering a background
  reindex under load).
Report findings — this determines whether retry-with-backoff alone is
suf­ficient, or whether there's a real resource contention problem to fix
separately.

### 4. Add a post-ingest integrity check

After a CLI ingest completes (or is interrupted and later resumed to
completion), compare the ingestor's own `chunksEmbedded` total against a
live Qdrant count for that `import_id` (or `ai_provider` + `project`
combination). Print a clear match/mismatch in the final summary — this
becomes the standing verification step for every future ingest, so a
silent gap like tonight's can never go unnoticed again.

## Step 0 — assess tonight's actual damage before restarting anything

The stopped Claude run left a confirmed gap of real, un-recoverable lost
chunks (the ~718 from the two known failures, and possibly more from any
undetected additional failures during the run — check the full log for
every `WARN qdrant request failed` line, not just the two already spotted).

Given the Claude importer doesn't have `--resume` yet, and Phase 1/2's own
lesson today was "verify against real data, don't assume" — the honest
move here is to **wipe `frontpocket_memory`'s Claude data and re-run
clean** once this fix lands, rather than trying to patch a partial,
silently-incomplete import. Confirm with Mark before deciding whether to
wipe just the Claude-tagged points or the whole collection, given the
ChatGPT paused-run situation.

## Verification

```bash
cd ~/frontpocket
go build ./... && go test ./tests/... -v
./make_all.sh
```

Real test: simulate a Qdrant write failure (e.g. temporarily stop
`kae-qdrant` mid-ingest on a small `--limit` test run, or inject a fault in
a test) and confirm the CLI now **hard-fails loudly** instead of silently
continuing — this is the one behavior that must be proven, not just
unit-tested in isolation. Report the actual terminal output showing the
hard failure, plus confirm the retry logic absorbs a *transient* failure
(one that resolves within the retry window) without stopping at all.

## Non-goals

- No changes to the live API server's own fallback behavior — scope this
  to the CLI ingest paths only unless investigation in Step 3 reveals a
  broader fix is needed.
- No re-running the full Claude or ChatGPT ingest as part of this task —
  that's the next step once this is fixed and verified, not part of it.

## Definition of done

- CLI ingest no longer silently falls back to an in-memory store on a
  Qdrant write failure — it hard-fails with a clear error after retries
  are exhausted.
- Retry-with-backoff absorbs genuine transient network failures without
  stopping the ingest.
- Root cause of tonight's specific broken pipes investigated and reported.
- A real, demonstrated test proving the hard-fail behavior works, not just
  unit tests.
- Clear recommendation on how much of tonight's partial Claude import needs
  to be wiped and re-run once this ships.
