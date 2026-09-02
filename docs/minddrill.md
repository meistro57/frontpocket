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
./bin/minddrill inspect frontpocket_memory
./bin/minddrill inspect --sample-limit 4000 centerstone
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

`minddrill inspect` options:

| Flag             | Default | Description                                       |
| ---------------- | ------- | ------------------------------------------------- |
| `--sample-limit` | `1200`  | Max vectors to sample while profiling collection. |

It also exposes:

- `GET /health` returning `{"status":"ok","app":"minddrill"}`
- `GET /config.json` returning `{"api_base_url":"<--api value>"}`

The embedded page loads `/config.json` on startup and uses that runtime value for all
FrontPocket API calls, replacing the old hardcoded API constant.

## DeepDrill provenance tracing

MindDrill includes DeepDrill CLI subcommands for bounded research loops and source provenance tracing.

```bash
# Trace from a thought artifact
./bin/minddrill deepdrill provenance <thought_id> --collection frontpocket_memory

# Trace directly from a source id
./bin/minddrill deepdrill provenance --collection frontpocket_memory --source <source_id>

# Resolve collection from a bound session
./bin/minddrill deepdrill provenance <thought_id> --session centerstone

# Machine-readable output
./bin/minddrill deepdrill provenance <thought_id> --collection frontpocket_memory --json
```

The report distinguishes explicit and inferred edges and prints:

- `SOURCE` and `TYPE` for the anchor
- `UPSTREAM`, `DOWNSTREAM`, and `RELATED` provenance edges
- `CHRONOLOGY` signals (source-local chronology only)
- `WEAKNESSES` when metadata is incomplete or chronology is unsafe
- `CONFIDENCE` for the assembled trace

DeepDrill strategy scheduling now auto-considers `PROVENANCE_TRACE` when uncertainty is
`PROVENANCE_GAP`, `CHRONOLOGY_GAP`, or `SOURCE_QUALITY`.

## Roadmap

The active enhancement plan lives in [MindDrill Roadmap](minddrill-roadmap.md). Current focus is stable, trustable retrieval UX: stale-response guards, explicit per-mode rendering, duplicate grouping controls, richer context-pack output, and browse-first filtering.

## Requirements

- The FrontPocket API must be running and reachable at the `--api` URL.
- The MindDrill origin must be allowed by the API's CORS configuration. MindDrill calls the
  API from the browser, so the API must return `Access-Control-Allow-Origin` for its origin.
  The default `CORS_ALLOW_ORIGINS` includes `http://localhost:8089`. If you run MindDrill on a
  different port or host, add that origin to `CORS_ALLOW_ORIGINS` in your `.env`.

## What it uses

MindDrill is a thin client over the FrontPocket API for UI use, and now includes a corpus inspection CLI plus research retrieval tools for the chat model.

- `GET /health` — connection status
- `GET /memory/stats` — memory counts
- `POST /memory/search` — semantic search
- `POST /memory/context-pack` — assemble session context
- `POST /memory/chat` — chat response with split memory context and optional `system_prompt` persona guidance
- `DELETE /memory/chat/session` — clear the current MindDrill chat session and its chat memory

Research retrieval tools exposed to the model include:

- `inspect_collection`
- `bind_research_collection`
- `semantic_search`
- `keyword_search`
- `search_with_metadata`
- `get_document_chunks`
- `get_neighbor_chunks`
- `find_related_outside_document`
- `search_excluding_documents`

## Corpus research workflow

MindDrill research sessions now bind explicitly to one collection before retrieval. There is no silent fallback to an arbitrary collection.

- Start with `minddrill inspect --list` and `minddrill inspect <collection>` to confirm collection shape and embedding compatibility.
- Bind a research session with `bind_research_collection` before calling retrieval tools.
- Retrieval calls can cap per-document dominance with `max_chunks_per_document` and can run raw nearest-neighbor mode when explicitly requested.
- Every research search can append a ledger entry (query, mode, filters, exclusions, timestamp, result sources, reason).
- Session state persists to `data/minddrill_research_sessions.json` by default, configurable via `MINDDRILL_RESEARCH_SESSION_FILE`.
- Similarity scores remain retrieval metadata only, not proof of causality or agreement.

## Quick probes

The sidebar's "quick probes" are generated dynamically from the corpus, not hardcoded. On load,
MindDrill calls `GET /memory/stats`, which returns a `top_titles` field (up to 60 distinct
`source_title` values). The sidebar renders these as clickable probe buttons. This means every
FrontPocket install shows probes relevant to that user's own corpus, with no user-specific
content shipped in the public repo.

If stats are unavailable, MindDrill now falls back to a small static probe list and shows an
inline retry control instead of leaving the panel stuck in a perpetual loading message.

## Rendering stability and duplicate control

MindDrill now uses strict per-mode render targets and clears each target before append, so
re-running queries or switching tabs does not accumulate stale result cards.

Search, browse, and context-pack requests also carry client request IDs; if an older request
finishes after a newer one has started, the older response is ignored.

Search and browse include a `group duplicates` toggle (default on):

- duplicates are grouped by `conversation_id` and normalized text;
- the highest-scoring item is kept as the visible primary card;
- collapsed groups show a clickable `×N similar` badge;
- turning grouping off shows all rows.

## Conversation thread

Chat mode renders a scrolling thread of bubbles (your messages right-aligned, MindDrill's
replies left-aligned) rather than a single overwritten answer block, so prior turns in a session
stay visible while you converse. A "thinking…" placeholder bubble appears immediately on send
and is replaced in place once the response arrives; on error, the placeholder is removed and
your message is restored to the input box for retry.

Assistant responses now render Markdown in the thread (headings, lists, blockquotes, inline
code, fenced code blocks, links, `---` horizontal rules, and emphasis), while user turns stay
plain text.

Expanded result-card full text in search/browse now uses the same sanitized Markdown renderer as
chat mode (no raw HTML injection from memory text), including fenced code blocks with per-block
copy buttons.

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

## Context-pack output UX

Context-pack output now includes:

- character count and rough token estimate;
- copy options for JSON and Markdown formats;
- collapsible JSON output to avoid giant always-open payload blocks.

Search/browse/context-pack actions disable their submit button while requests are in flight and
show a lightweight inline loading state. Zero-result responses now use a friendly empty-state
message (`No memories matched — try broadening your query`).

Chat-memory points are embedded automatically on write if they don't already carry a vector
(`QdrantMemoryStore.Upsert` embeds any point with an empty `Vector` field using the configured
embedder before upserting). This means `minddrill_chat_memory` persists durably in Qdrant across
restarts, rather than only living in the in-process fallback store for the lifetime of a single
server run.

## Request timeouts

`GET /memory/stats` now uses a short frontend timeout and a scan-free backend path:

- **Frontend**: stats fetch uses an `AbortController` timeout of ~5s. On timeout/error, MindDrill
  shows a visible "stats unavailable" fallback with retry.
- **Backend**: totals come from Qdrant collection info/count APIs, and distinct field groups come
  from cached aggregation instead of per-request full collection scans.
- **Fallback behavior**: if the backend still fails, the handler can return stale cached stats or
  a structured error, rather than hanging the request.

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
