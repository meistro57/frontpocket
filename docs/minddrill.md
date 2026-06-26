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

As the standalone binary built by `./make_all.sh`:

```bash
minddrill                              # serves on http://localhost:8089
minddrill --port 9000                  # custom port
minddrill --api http://localhost:8088  # point at a non-default API base URL
```

Then open the printed URL (default <http://localhost:8089>) in your browser.

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
