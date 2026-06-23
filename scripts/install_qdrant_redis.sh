#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
QDRANT_URL="${QDRANT_URL:-http://localhost:6333}"
REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

is_qdrant_ready() {
  if command_exists curl; then
    if curl -fsS "$QDRANT_URL/collections" >/dev/null 2>&1; then
      return 0
    fi
  fi

  if command_exists docker; then
    if docker ps --format '{{.Names}}' | grep -qx 'frontpocket-qdrant'; then
      return 0
    fi
  fi

  if command_exists qdrant; then
    return 0
  fi

  return 1
}

is_redis_ready() {
  if command_exists redis-cli; then
    if [ "$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ping 2>/dev/null || true)" = "PONG" ]; then
      return 0
    fi
  fi

  if command_exists docker; then
    if docker ps --format '{{.Names}}' | grep -qx 'frontpocket-redis'; then
      return 0
    fi
  fi

  if command_exists redis-server; then
    return 0
  fi

  return 1
}

need_qdrant=1
if is_qdrant_ready; then
  need_qdrant=0
fi

need_redis=1
if is_redis_ready; then
  need_redis=0
fi

if [ "$need_qdrant" -eq 0 ] && [ "$need_redis" -eq 0 ]; then
  echo "Qdrant and Redis are already present."
  exit 0
fi

if ! command_exists docker; then
  echo "Docker is required to install missing dependencies automatically."
  echo "Install Docker and rerun this script, or install Qdrant/Redis manually."
  exit 1
fi

if docker compose version >/dev/null 2>&1; then
  compose() {
    docker compose "$@"
  }
elif command_exists docker-compose; then
  compose() {
    docker-compose "$@"
  }
else
  echo "Docker Compose is required to install missing dependencies automatically."
  echo "Install Docker Compose and rerun this script, or install Qdrant/Redis manually."
  exit 1
fi

services=""
if [ "$need_qdrant" -eq 1 ]; then
  services="$services qdrant"
fi
if [ "$need_redis" -eq 1 ]; then
  services="$services redis"
fi

echo "Installing/start missing services:$services"
compose -f "$ROOT_DIR/docker-compose.yml" up -d $services

attempt=0
max_attempts=30
while [ "$attempt" -lt "$max_attempts" ]; do
  qdrant_ok=1
  redis_ok=1

  if [ "$need_qdrant" -eq 1 ] && ! is_qdrant_ready; then
    qdrant_ok=0
  fi
  if [ "$need_redis" -eq 1 ] && ! is_redis_ready; then
    redis_ok=0
  fi

  if [ "$qdrant_ok" -eq 1 ] && [ "$redis_ok" -eq 1 ]; then
    echo "Qdrant and Redis are ready."
    exit 0
  fi

  attempt=$((attempt + 1))
  sleep 1
done

echo "Timed out waiting for services to become ready."
exit 1
