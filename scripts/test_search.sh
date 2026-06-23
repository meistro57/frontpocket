#!/usr/bin/env sh
set -eu

BASE_URL="${1:-http://localhost:8088}"

curl -sS -X POST "$BASE_URL/memory/search" \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "What is FrontPocket?",
    "limit": 5,
    "filters": {
      "project": "FrontPocket"
    }
  }'
