<img width="1185" height="435" alt="FrontPocket" src="https://github.com/user-attachments/assets/c8a84478-8e65-4a42-aedc-b854eada137b" />

# FrontPocket

**Source-backed memory for AI companions.**

<p>
  <img src="docs/assets/badge-local-first.svg" alt="Local-first" />
  <img src="docs/assets/badge-go-api.svg" alt="Go API" />
  <img src="docs/assets/badge-qdrant.svg" alt="Qdrant" />
  <img src="docs/assets/badge-redis.svg" alt="Redis" />
  <img src="docs/assets/badge-v020.svg" alt="Version 0.2.0" />
</p>

FrontPocket is a local-first memory engine for AI companions, agents, and creative workflows.
It gives your assistant a searchable, source-backed way to recall conversations, project context, preferences, and decisions without pretending to know things it cannot verify.

No crystal ball. Just retrieval with receipts.

---

## At a glance

- **Architecture:** Go HTTP API in front of Qdrant (long-term semantic memory) and Redis (working/session memory).
- **Retrieval endpoints:** search and context-pack with source metadata.
- **Operational endpoints:** memory stats and session state for trusted usage.
- **OpenAPI support:** served at `GET /openapi.json` for integrations.
- **MindDrill UI:** a built-in, browser-based memory explorer with chat mode, search, and context-pack tools.
- **MindDrill memory isolation:** dedicated MindDrill chat collection (`MINDDRILL_MEMORY_COLLECTION`) in the same Qdrant instance as the main corpus.
- **CORS support:** configurable allowed origins so browser apps (including MindDrill) can call the API directly.

## API endpoints

```text
GET    /health
GET    /openapi.json
GET    /memory/stats
POST   /memory/session
DELETE /memory/session
POST   /memory/ingest/chat
POST   /memory/search
POST   /memory/context-pack
POST   /memory/chat

# Optional local-only debug endpoints (when DEV_DEBUG_ENDPOINTS=true)
GET    /minddrill/memory/stats
POST   /minddrill/memory/search
DELETE /minddrill/memory/session
```

Public-safe recall surface remains:

```text
GET  /health
GET  /openapi.json
POST /memory/search
POST /memory/context-pack
```

---

## Quick start

### 1) Configure

```bash
cp .env.example .env
```

When running with Docker Compose, `frontpocket-api` uses internal service URLs by default:
- `DOCKER_QDRANT_URL` (default `http://qdrant:6333`)
- `DOCKER_REDIS_URL` (default `redis://redis:6379/0`)
- `DOCKER_OLLAMA_BASE_URL` (default `http://ollama:11434`)

Set these only if you need custom container-to-service routing.

To use Gemini embeddings through OpenRouter:

```env
EMBEDDING_PROVIDER=openrouter
OPENROUTER_EMBEDDING_MODEL=google/gemini-embedding-2-preview
```

Browser apps (including the bundled MindDrill UI) call the API directly, so set the
origins they are served from:

```env
CORS_ALLOW_ORIGINS=http://localhost:3000,http://localhost:5173,http://localhost:8080,http://localhost:8089
```

MindDrill chat memory defaults (separate collection, shared Qdrant engine):

```env
MINDDRILL_MEMORY_COLLECTION=minddrill_chat_memory
MINDDRILL_MEMORY_ENABLED=true
MINDDRILL_MEMORY_WRITE_MODE=summary
MINDDRILL_MEMORY_TOP_K=6
MINDDRILL_MEMORY_SESSION_SUMMARY_EVERY=8
```

The API reflects matching origins back via `Access-Control-Allow-Origin`, answers
`OPTIONS` preflight requests with `204`, and supports a `*` wildcard to allow all origins.

### 2) Build + ensure helper scripts are executable

```bash
./make_all.sh
```

### 3) Install Qdrant + Redis if missing

```bash
./scripts/install_qdrant_redis.sh
```

### 4) Run the stack

```bash
docker compose up --build
```

### 5) Health check

```bash
curl http://localhost:8088/health
```

Expected response:

```json
{
  "status": "ok",
  "qdrant": "connected",
  "redis": "connected",
  "version": "0.2.0"
}
```

### 6) OpenAPI schema

```bash
curl http://localhost:8088/openapi.json
```

### 7) Ingest a chat sample

```bash
curl -X POST http://localhost:8088/memory/ingest/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "source_title":"Planning Chat",
    "project":"FrontPocket",
    "records":[
      {
        "conversation_id":"conv_1",
        "timestamp":"2026-06-22T11:13:00Z",
        "speaker":"user",
        "text":"FrontPocket is a local-first, source-backed memory layer in Go."
      }
    ]
  }'
```

### 8) Search memory

```bash
curl -X POST http://localhost:8088/memory/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"source-backed Go memory","limit":5}'
```

### 9) View memory stats

```bash
curl 'http://localhost:8088/memory/stats?project=FrontPocket'
```

### 10) Save session state

```bash
curl -X POST http://localhost:8088/memory/session \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id":"frontpocket-dev",
    "project":"FrontPocket",
    "active_summary":"Implementing memory stats and session APIs.",
    "recent_memory_ids":["conv_1_turn_001_chunk_001"]
  }'
```

Search responses are cached in Redis for `SEARCH_CACHE_TTL_SECONDS` to reduce repeated vector lookups.

Chat endpoint example (dual retrieval: FrontPocket corpus + MindDrill chat memory). The optional `system_prompt` field lets a client pass persona, tone, or other system prompt guidance for that chat turn. When `CHAT_PROVIDER=openrouter`, the endpoint sends the retrieved context pack to OpenRouter and uses Gemma 4 (`OPENROUTER_CHAT_MODEL=google/gemma-4-31b-it`) for the final answer. Keep `CHAT_PROVIDER=none` for retrieval-only local development.

```bash
curl -X POST http://localhost:8088/memory/chat \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"minddrill-dev","message":"remember this: keep responses concise","system_prompt":"Use a concise, practical persona and call out uncertainty.","remember_this":true}'
```

---

## MindDrill memory explorer

<img src="MindDrill_logo.png" alt="MindDrill" width="120" align="right" />

MindDrill is a built-in, single-page browser UI for exploring your memory. It talks to the
FrontPocket API to run semantic searches, build context packs, and run chat mode with split
memory context (FrontPocket corpus memory + MindDrill chat memory) — no extra dependencies,
the page is embedded directly in the binary.

Start it from the main CLI (it uses the standalone `minddrill` binary if present on `PATH`,
otherwise falls back to `go run ./cmd/minddrill`):

```bash
frontpocket minddrill
```

Or run the standalone binary built by `./make_all.sh`:

```bash
minddrill                              # serves on http://localhost:8089
minddrill --port 9000                  # custom port
minddrill --api http://localhost:8088  # point at a non-default API base URL
```

Then open the printed URL (default <http://localhost:8089>) in your browser. MindDrill serves
`/config.json` from its own origin and the page loads it at startup, so `--api` is applied to all
FrontPocket API calls at runtime. Make sure the FrontPocket API is running and that the MindDrill
origin is listed in `CORS_ALLOW_ORIGINS` (port `8089` is included in the default list).

MindDrill chat mode now shows:

- assistant answer
- expandable "FrontPocket memories used"
- expandable "MindDrill memories used"
- "remember this" action
- "forget this session" action (uses local-only debug delete when enabled)
- status text (`using X FrontPocket memories + Y MindDrill memories`)

```bash
frontpocket minddrill --help
```

## Examples

- Request samples: `examples/search_request.json`
- OpenAPI action schema sample: `examples/openapi_action_schema.yaml`
- JSONL sample import: `examples/chat_export_sample.jsonl`

## ChatGPT export ingest (CLI)

FrontPocket can parse ChatGPT exports from either a `.zip` file or an extracted folder and normalize them into JSONL-compatible records.

### Ingest directly from a `.zip`

1. Download your ChatGPT export zip to your machine.
2. Run a dry-run first to verify parsing stats.
3. Re-run without `--dry-run` to write to memory storage.
4. Use `--out` if you also want normalized JSONL on disk.

```bash
frontpocket ingest chatgpt ./chatgpt-export.zip --dry-run
frontpocket ingest chatgpt ./chatgpt-export.zip --project FrontPocket
frontpocket ingest chatgpt ./chatgpt-export.zip --project FrontPocket --out data/processed/chatgpt_normalized.jsonl
```

Additional filters:

```bash
frontpocket ingest chatgpt ./chatgpt-export.zip --since 2026-01-01
frontpocket ingest chatgpt ./chatgpt-export.zip --conversation "FrontPocket"
frontpocket ingest chatgpt ./unzipped-chatgpt-export/
```

### Resumable, memory-bounded imports

Large exports are embedded and written to Qdrant in **batches** rather than buffered entirely in memory, so a multi-gigabyte archive won't exhaust RAM. Each batch is flushed on a record boundary as soon as it fills.

Pass `--resume <path>` to track progress in a small JSON journal. If an import is interrupted (crash, `Ctrl-C`, or a transient embedding/store failure), re-running the same command continues from the last committed batch instead of starting over:

```bash
frontpocket ingest chatgpt ./chatgpt-export.zip --project FrontPocket --resume .frontpocket-progress.json
# ...interrupted...
frontpocket ingest chatgpt ./chatgpt-export.zip --project FrontPocket --resume .frontpocket-progress.json  # picks up where it stopped
```

The journal is keyed to the source, collection, and embedding model, so a stale journal can't silently skip records from a different import. It is removed automatically once the import completes successfully. Because `memory_id`s are deterministic, any re-processed records upsert idempotently.

### CLI help

```bash
frontpocket --help
frontpocket ingest --help
frontpocket ingest chatgpt --help
frontpocket minddrill --help
```

### Notes

- Attachments and assets are ingested as attachment-aware memory records (including attachment refs) and reported in import stats.
- Raw export `.zip` files are gitignored by default (`*.zip`) to reduce accidental commits of private archives.
- Embedding responses support larger payloads (up to 32MB) to avoid truncated JSON decode errors during large imports.
- Qdrant writes use UUID-compatible point IDs while preserving the original `memory_id` in payload metadata, preventing rejected inserts when source IDs are not UUIDs.

## Docs

- `docs/getting-started.md`
- `docs/architecture.md`
- `docs/minddrill.md`
- `docs/memory-model.md`
- `docs/providers.md`
- `docs/privacy.md`
- `docs/local-first.md`
- `docs/future-gpt-action.md`

---

## Local development

Run tests:

```bash
go test ./...
```

CI runs on every push and pull request through `.github/workflows/test.yml`.

Run API directly:

```bash
go run ./cmd/frontpocket
```

Run the MindDrill UI directly:

```bash
go run ./cmd/minddrill
```

Print version:

```bash
go run ./cmd/frontpocket --version
```

---

## Project direction

FrontPocket remains:

- Local-first by default
- Source-backed recall focused
- Retrieval-first before mutation-heavy workflows
- API-shaped for future hosted/GPT Action integrations
- Explicit about provenance and metadata

---

## License

MIT License. See `LICENSE`.
