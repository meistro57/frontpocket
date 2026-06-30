# MindDrill

<img src="../MindDrill_logo.png" alt="MindDrill" width="140" />

MindDrill is FrontPocket's built-in memory explorer: a single-page browser UI for chat mode,
searching your memory, building context packs, and viewing memory stats. The page is embedded
directly in the binary, so there is nothing to install or build separately beyond `./make_all.sh`.

## Running

From the main CLI (uses the standalone `minddrill` binary if it is on `PATH`, otherwise falls
back to `go run ./cmd/minddrill`):

```bash
frontpocket minddrill
```

As the standalone binary built by `./make_all.sh` (binaries are written to `bin/`):

```bash
./make_all.sh                          # builds bin/frontpocket and bin/minddrill
./bin/minddrill                        # serves on http://localhost:8089
./bin/minddrill --port 9000            # custom port
./bin/minddrill --api http://localhost:8088  # point at a non-default API base URL
```

Then open the printed URL (default <http://localhost:8089>) in your browser.

> Build output always lands in `bin/`. Run binaries from there (`./bin/frontpocket`,
> `./bin/minddrill`) rather than from the repo root, to avoid picking up a stale binary
> from an older build path.

## Options

| Flag     | Default                  | Description                          |
| -------- | ------------------------ | ------------------------------------ |
| `--port` | `8089`                   | Port to serve the MindDrill UI on.   |
| `--api`  | `http://localhost:8088`  | FrontPocket API base URL to call.    |

It also exposes:

- `GET /health` returning `{"status":"ok","app":"minddrill"}`
- `GET /config.json` returning `{"api_base_url":"<--api value>"}`

The embedded page loads `/config.json` on startup and uses that runtime value for all
FrontPocket API calls, replacing the old hardcoded API constant.

## Roadmap

The active enhancement plan lives in [MindDrill Roadmap](minddrill-roadmap.md). Sprint 1 focuses on trust and control: non-debug chat session deletion, structured UI errors, richer chat evidence cards, and local search history controls.

## Requirements

- The FrontPocket API must be running and reachable at the `--api` URL.
- The MindDrill origin must be allowed by the API's CORS configuration. MindDrill calls the
  API from the browser, so the API must return `Access-Control-Allow-Origin` for its origin.
  The default `CORS_ALLOW_ORIGINS` includes `http://localhost:8089`. If you run MindDrill on a
  different port or host, add that origin to `CORS_ALLOW_ORIGINS` in your `.env`.

## What it uses

MindDrill is a thin client over the FrontPocket API:

- `GET /health` — connection status
- `GET /memory/stats` — memory counts
- `POST /memory/search` — semantic search
- `POST /memory/context-pack` — assemble session context
- `POST /memory/chat` — chat response with split memory context and optional `system_prompt` persona guidance
- `DELETE /memory/chat/session` — clear the current MindDrill chat session and its chat memory

## Quick probes

The sidebar's "quick probes" are generated dynamically from the corpus, not hardcoded. On load,
MindDrill calls `GET /memory/stats`, which now returns a `top_titles` field — a sample of
distinct `source_title` values pulled from the corpus (capped at 60 titles, sampled from up to
4,000 points). The sidebar renders these as clickable probe buttons, and the browse tab uses a
random probe as its seed query. This means every FrontPocket install shows probes relevant to
that user's own corpus, with no user-specific content shipped in the public repo.

## Conversation thread

Chat mode renders a scrolling thread of bubbles (your messages right-aligned, MindDrill's
replies left-aligned) rather than a single overwritten answer block, so prior turns in a session
stay visible while you converse. A "thinking…" placeholder bubble appears immediately on send
and is replaced in place once the response arrives; on error, the placeholder is removed and
your message is restored to the input box for retry.

## Chat mode memory separation

MindDrill chat keeps continuity in a dedicated Qdrant collection (`MINDDRILL_MEMORY_COLLECTION`,
default `minddrill_chat_memory`) while FrontPocket imported corpus memory remains in the main
`QDRANT_COLLECTION`. Both collections use the same Qdrant instance.

The main chat screen also includes a **persona** button for optional system prompt information such as tone, role, or behavioural boundaries. MindDrill stores that browser setting locally and sends it as `system_prompt` with chat turns; it is not written into memory unless the user explicitly asks chat mode to remember it.

When chat mode runs, it retrieves from both layers:

- **MindDrill chat memory** for conversational continuity and session carry-over.
- **FrontPocket source memory** for stronger source-backed evidence.

The response includes:

- `answer`
- `used_frontpocket_memories`
- `used_minddrill_memories`
- `context_pack`
- `model`
- `provider`

MindDrill writes new chat memory through the Go API only, using `MINDDRILL_MEMORY_WRITE_MODE`
(`summary` by default, or `raw`) and periodic session summaries controlled by
`MINDDRILL_MEMORY_SESSION_SUMMARY_EVERY`.

Chat-memory points are embedded automatically on write if they don't already carry a vector
(`QdrantMemoryStore.Upsert` embeds any point with an empty `Vector` field using the configured
embedder before upserting). This means `minddrill_chat_memory` persists durably in Qdrant across
restarts, rather than only living in the in-process fallback store for the lifetime of a single
server run.

## Request timeouts

A single `POST /memory/chat` turn can make up to **two** sequential LLM calls — an optional
query-refinement call when first-pass retrieval looks thin, followed by the answer-generation
call. The chat client (`internal/chat`) allows each call up to 60s, so a slow turn can legitimately
run close to two minutes. The timeouts are layered so they never abort a turn that is still
working:

- **Backend**: `handleMemoryChat` wraps the whole request in a `context.WithTimeout` of
  `chatRequestTimeout` (120s), bounding the searches and both LLM calls together. If exceeded,
  the server returns a clean `CHAT_COMPLETION_FAILED` error instead of hanging.
- **Frontend**: the MindDrill chat `fetch` uses a 130s `AbortSignal` ceiling — deliberately just
  above the backend bound so the server's error surfaces before the browser aborts.

If you tune the per-call timeout in `internal/chat`, keep `chatRequestTimeout` and the frontend
`fetch` ceiling above the worst-case sum (refinement + answer) so the signal never trips before
the model responds.
