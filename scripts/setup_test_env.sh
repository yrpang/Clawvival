#!/usr/bin/env bash
# Usage:
#   source scripts/setup_test_env.sh            # export env only (default gorm)
#   source scripts/setup_test_env.sh --prepare  # export env + prepare docker pg + apply schema + gorm/gen
#
# Responsibility split:
# - this script exports the local DATABASE_URL contract for tests/dev
# - --prepare delegates DB bootstrap/model generation to
#   scripts/run_modelgen_with_local_postgres.sh

# IMPORTANT:
# This script is intended to be sourced by an interactive shell.
# Do NOT change shell options here (set -e/-u/pipefail), otherwise caller shell
# behavior may be altered and cause unexpected terminal exits.

SCRIPT_REF="${BASH_SOURCE[0]:-$0}"
ROOT_DIR="$(cd "$(dirname "$SCRIPT_REF")/.." && pwd)"

# Keep the exact docker postgres defaults used by run_modelgen_with_local_postgres.sh
export CONTAINER_NAME="${CONTAINER_NAME:-clawvival-pg-gen}"
export POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:16-alpine}"
export POSTGRES_PORT="${POSTGRES_PORT:-54329}"
export POSTGRES_USER="${POSTGRES_USER:-clawvival}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-clawvival}"
export POSTGRES_DB="${POSTGRES_DB:-clawvival}"

export DATABASE_URL="${DATABASE_URL:-host=127.0.0.1 port=${POSTGRES_PORT} user=${POSTGRES_USER} password=${POSTGRES_PASSWORD} dbname=${POSTGRES_DB} sslmode=disable}"

if [[ "${1:-}" == "--prepare" ]]; then
  (
    cd "$ROOT_DIR"
    ./scripts/run_modelgen_with_local_postgres.sh
  )
fi

echo "[clawvival] env ready"
echo "  DATABASE_URL=$DATABASE_URL"
echo "  docker container=$CONTAINER_NAME (port=$POSTGRES_PORT)"
echo ""
echo "next:"
echo "  go run ./cmd/server"
