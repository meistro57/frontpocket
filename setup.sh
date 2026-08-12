#!/usr/bin/env sh
# One-command setup for a fresh clone: configure, provision Qdrant + Redis
# (only if not already running), then build. Safe to rerun — never
# overwrites an existing .env, and install_qdrant_redis.sh only starts
# whatever's actually missing.
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$ROOT_DIR"

echo "=== FrontPocket setup ==="

echo ""
echo "--- 1/3: configuration ---"
if [ -f .env ]; then
  echo ".env already exists — leaving it alone."
else
  cp .env.example .env
  echo "Created .env from .env.example (local-first defaults: Ollama embeddings, no chat model)."
  echo "Edit .env any time to switch providers, then rerun this script."
fi

echo ""
echo "--- 2/3: qdrant + redis ---"
"$ROOT_DIR/scripts/install_qdrant_redis.sh"

echo ""
echo "--- 3/3: build ---"
"$ROOT_DIR/make_all.sh"

echo ""
echo "=== Setup complete ==="
echo ""
echo "Start the API:   ./bin/frontpocket"
echo "Start MindDrill: ./bin/minddrill    (http://localhost:8089)"
echo "Health check:    curl http://localhost:8088/health"
