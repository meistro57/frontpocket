#!/usr/bin/env sh
set -eu

QDRANT_URL="${QDRANT_URL:-http://localhost:6333}"
COLLECTION="${QDRANT_COLLECTION:-frontpocket_memory}"

curl -sS -X DELETE "$QDRANT_URL/collections/$COLLECTION" || true
curl -sS -X PUT "$QDRANT_URL/collections/$COLLECTION" \
  -H 'Content-Type: application/json' \
  -d '{
    "vectors": {
      "size": 768,
      "distance": "Cosine"
    }
  }'
