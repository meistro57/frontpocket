# Getting Started

## 1) Prepare config

```bash
cp .env.example .env
```

## 2) Install missing local dependencies

```bash
./scripts/install_qdrant_redis.sh
```

## 3) Start FrontPocket stack

```bash
docker compose up --build
```

## 4) Verify health

```bash
curl http://localhost:8088/health
```

## 5) Check OpenAPI schema

```bash
curl http://localhost:8088/openapi.json
```

## 6) Check memory stats

```bash
curl 'http://localhost:8088/memory/stats?project=FrontPocket'
```

## 7) Save session state

```bash
curl -X POST http://localhost:8088/memory/session \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"frontpocket-dev","project":"FrontPocket","active_summary":"Working on memory endpoints"}'
```

## 8) Delete session state (optional)

```bash
curl -X DELETE 'http://localhost:8088/memory/session?session_id=frontpocket-dev'
```

## 9) Ingest a ChatGPT export (zip or folder)

```bash
frontpocket ingest chatgpt ./chatgpt-export.zip --dry-run
frontpocket ingest chatgpt ./chatgpt-export.zip --project FrontPocket
frontpocket ingest chatgpt ./chatgpt-export.zip --out data/processed/chatgpt_normalized.jsonl
frontpocket ingest chatgpt ./unzipped-chatgpt-export/
```

Attachments/assets are detected and reported during import, but they are not ingested yet.

## 10) Run tests

```bash
go test ./...
```

A matching CI check runs the same suite on GitHub Actions for pushes and pull requests (`.github/workflows/test.yml`).
