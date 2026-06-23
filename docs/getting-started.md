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

## 11) Run tests

```bash
go test ./...
```

A matching CI check runs the same suite on GitHub Actions for pushes and pull requests (`.github/workflows/test.yml`).
