#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

"$ROOT_DIR/make_all_exec.sh"

cd "$ROOT_DIR"
mkdir -p "$ROOT_DIR/bin"
go build ./...
go build -o "$ROOT_DIR/bin/frontpocket" ./cmd/frontpocket
go build -o "$ROOT_DIR/bin/minddrill"   ./cmd/minddrill

echo ""
echo "Built: bin/frontpocket and bin/minddrill"
echo "Run with: ./bin/frontpocket  and  ./bin/minddrill"
