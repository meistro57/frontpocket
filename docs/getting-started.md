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

## 8) Run tests

```bash
go test ./...
```

A matching CI check runs the same suite on GitHub Actions for pushes and pull requests (`.github/workflows/test.yml`).
