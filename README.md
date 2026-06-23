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

## API endpoints

```text
GET  /health
GET  /openapi.json
GET  /memory/stats
POST   /memory/session
DELETE /memory/session
POST   /memory/ingest/chat
POST   /memory/search
POST /memory/context-pack
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

---

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

Attachments and assets are ingested as attachment-aware memory records (including attachment refs) and reported in import stats.
Raw export `.zip` files are gitignored by default (`*.zip`) to reduce accidental commits of private archives.
Embedding responses now support larger payloads (up to 32MB) to avoid truncated JSON decode errors during large imports.

## Docs

- `docs/getting-started.md`
- `docs/architecture.md`
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
