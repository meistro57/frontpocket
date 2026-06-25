# MindDrill

<img src="../MindDrill_logo.png" alt="MindDrill" width="140" />

MindDrill is FrontPocket's built-in memory explorer: a single-page browser UI for searching
your memory, building context packs, and viewing memory stats. The page is embedded directly
in the binary, so there is nothing to install or build separately beyond `./make_all.sh`.

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

It also exposes `GET /health` returning `{"status":"ok","app":"minddrill"}`.

## Requirements

- The FrontPocket API must be running and reachable at the `--api` URL.
- The MindDrill origin must be allowed by the API's CORS configuration. MindDrill calls the
  API from the browser, so the API must return `Access-Control-Allow-Origin` for its origin.
  The default `CORS_ALLOW_ORIGINS` includes `http://localhost:8089`. If you run MindDrill on a
  different port or host, add that origin to `CORS_ALLOW_ORIGINS` in your `.env`.

## What it uses

MindDrill is a thin client over the public-safe recall surface:

- `GET /health` — connection status
- `GET /memory/stats` — memory counts
- `POST /memory/search` — semantic search
- `POST /memory/context-pack` — assemble session context
