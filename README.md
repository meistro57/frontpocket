<img width="1185" height="435" alt="FrontPocket" src="https://github.com/user-attachments/assets/c8a84478-8e65-4a42-aedc-b854eada137b" />

# FrontPocket

**Source-backed memory for AI companions.**

FrontPocket is a local-first memory engine for AI companions, agents, and creative workflows.

It gives your assistant a searchable, source-backed way to recall past conversations, project context, preferences, decisions, and long-running threads without pretending to know things it cannot verify.

No crystal ball. Just retrieval with receipts.

FrontPocket uses a **Go HTTP API** in front of **Qdrant** (long-term semantic memory) and **Redis** (working/session memory).

Current version: **0.1.0**

---

## Why FrontPocket

- Keeps recall grounded in source metadata.
- Prioritizes local-first defaults.
- Retrieves what matters now instead of prompt-stuffing everything.
- Stays practical: boring architecture, useful outcomes.

---

## Current Status

FrontPocket has a working Go-first foundation with:

- Go module + command entrypoint (`cmd/frontpocket/main.go`)
- Config loading from environment (`internal/config`)
- HTTP API server + API key middleware (`internal/api`)
- Memory ingest/search/context-pack handlers
- JSONL parser + chunking (`internal/memory`)
- Ollama/OpenAI/OpenRouter HTTP embedders (`internal/embed`)
- Qdrant-backed memory store with vector-size validation (`internal/store`)
- Redis-backed search caching (`internal/store` + `internal/api`)
- Dockerfile + Docker Compose + `.env.example`

---

## API Endpoints

Implemented endpoints:

```text
GET  /health
GET  /openapi.json
POST /memory/ingest/chat
POST /memory/search
POST /memory/context-pack
```

Example search request:

```json
{
  "query": "What did we decide FrontPocket is?",
  "limit": 5,
  "filters": {
    "project": "FrontPocket"
  }
}
```

Example search response:

```json
{
  "query": "What did we decide FrontPocket is?",
  "results": [
    {
      "memory_id": "conv_1_turn_001_chunk_001",
      "conversation_id": "conv_1",
      "source_title": "Planning Chat",
      "source_type": "chat_export",
      "timestamp": "2026-06-22T11:13:00Z",
      "speaker": "user",
      "project": "FrontPocket",
      "memory_kind": "project_context",
      "summary": "FrontPocket is a local-first, source-backed memory layer in Go.",
      "source_quote": "FrontPocket is a local-first, source-backed memory layer in Go.",
      "text": "FrontPocket is a local-first, source-backed memory layer in Go.",
      "score": 1,
      "embedding_provider": "ollama",
      "embedding_model": "nomic-embed-text",
      "embedding_dimensions": 768
    }
  ]
}
```

---

## Quick Start

### 1) Configure

```bash
cp .env.example .env
```

### 2) Install Qdrant + Redis if missing

```bash
./scripts/install_qdrant_redis.sh
```

### 3) Run the stack

```bash
docker compose up --build
```

### 4) Health check

```bash
curl http://localhost:8088/health
```

Expected response:

```json
{
  "status": "ok",
  "qdrant": "connected",
  "redis": "connected",
  "version": "0.1.0"
}
```

OpenAPI schema:

```bash
curl http://localhost:8088/openapi.json
```

### 5) Ingest a chat sample

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

### 6) Search memory

```bash
curl -X POST http://localhost:8088/memory/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"source-backed Go memory","limit":5}'
```

Search responses are cached in Redis for `SEARCH_CACHE_TTL_SECONDS` to reduce repeated vector lookups.

---

## Local Development

Run tests:

```bash
go test ./...
```

CI tests run automatically on every push and pull request via `.github/workflows/test.yml`.

Run API directly:

```bash
go run ./cmd/frontpocket
```

Print version:

```bash
go run ./cmd/frontpocket --version
```

---

## Project Direction

FrontPocket remains:

- Local-first by default
- Source-backed recall focused
- Retrieval-first before mutation-heavy workflows
- API-shaped for future hosted/GPT Action integrations
- Explicit about provenance and metadata

---

## License

MIT License. See `LICENSE`.
