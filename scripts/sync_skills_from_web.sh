#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$ROOT_DIR/apps/web/public/skills"
DST_DIR="$ROOT_DIR/skills"

if [[ ! -d "$SRC_DIR" ]]; then
  echo "source skills directory not found: $SRC_DIR" >&2
  exit 1
fi

rm -rf "$DST_DIR"
mkdir -p "$DST_DIR"
cp -R "$SRC_DIR"/. "$DST_DIR"/
