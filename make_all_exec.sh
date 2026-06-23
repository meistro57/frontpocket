#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

find "$ROOT_DIR" -type f -name '*.sh' -exec chmod +x {} +
