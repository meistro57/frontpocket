#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

"$ROOT_DIR/make_all_exec.sh"

cd "$ROOT_DIR"
go build ./...
