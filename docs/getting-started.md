# Getting Started

## 1) Prepare config

```bash
cp .env.example .env
```

To use Gemini embeddings through OpenRouter, set:

```env
EMBEDDING_PROVIDER=openrouter
OPENROUTER_EMBEDDING_MODEL=google/gemini-embedding-2-preview
```

If you plan to use the MindDrill UI or another browser app, list its origin so the API
returns CORS headers for it (the default already includes MindDrill's `:8089`):

```env
CORS_ALLOW_ORIGINS=http://localhost:3000,http://localhost:5173,http://localhost:8080,http://localhost:8089
```

## 2) Build + ensure helper scripts are executable

```bash
./make_all.sh
```

## 3) Install missing local dependencies

```bash
./scripts/install_qdrant_redis.sh
```

## 4) Start FrontPocket stack

```bash
docker compose up --build
```

## 5) Verify health

```bash
curl http://localhost:8088/health
```

## 6) Check OpenAPI schema

```bash
curl http://localhost:8088/openapi.json
```

## 7) Check memory stats

```bash
curl 'http://localhost:8088/memory/stats?project=FrontPocket'
```

## 8) Save session state

```bash
curl -X POST http://localhost:8088/memory/session \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"frontpocket-dev","project":"FrontPocket","active_summary":"Working on memory endpoints"}'
```

## 9) Delete session state (optional)

```bash
curl -X DELETE 'http://localhost:8088/memory/session?session_id=frontpocket-dev'
```

## 10) Ingest a ChatGPT export (zip or folder)

```bash
frontpocket ingest chatgpt ./chatgpt-export.zip --dry-run
frontpocket ingest chatgpt ./chatgpt-export.zip --project FrontPocket
frontpocket ingest chatgpt ./chatgpt-export.zip --out data/processed/chatgpt_normalized.jsonl
frontpocket ingest chatgpt ./unzipped-chatgpt-export/
```

Attachments/assets are ingested as attachment-aware records and reported during import.
Raw export `.zip` files are ignored by git (`*.zip`) by default.
If you hit an embedding JSON decode error on large imports, rebuild with `./make_all.sh` to pick up the latest embedding response handling.
If OpenRouter calls succeed but Qdrant remains empty, rebuild and rerun ingest to pick up UUID-compatible Qdrant point IDs (look for `not a valid point ID` in logs when using older binaries).

## 11) Explore memory in the browser (MindDrill)

With the API running, launch the MindDrill memory explorer:

```bash
frontpocket minddrill          # or: minddrill / go run ./cmd/minddrill
```

Open the printed URL (default <http://localhost:8089>). Ensure the MindDrill origin is
listed in `CORS_ALLOW_ORIGINS`. See `docs/minddrill.md` for options.

## 12) Run tests

```bash
go test ./...
```

A matching CI check runs the same suite on GitHub Actions for pushes and pull requests (`.github/workflows/test.yml`).
