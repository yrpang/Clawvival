#!/usr/bin/env bash
# Usage:
#   source scripts/setup_test_env.sh            # export env only (default gorm)
#   source scripts/setup_test_env.sh --prepare  # export env + prepare docker pg + schema + gorm/gen

# IMPORTANT:
# This script is intended to be sourced by an interactive shell.
# Do NOT change shell options here (set -e/-u/pipefail), otherwise caller shell
# behavior may be altered and cause unexpected terminal exits.

SCRIPT_REF="${BASH_SOURCE[0]:-$0}"
ROOT_DIR="$(cd "$(dirname "$SCRIPT_REF")/.." && pwd)"

# Keep the exact docker postgres defaults used by gen_models_with_docker_postgres.sh
export CONTAINER_NAME="${CONTAINER_NAME:-clawverse-pg-gen}"
export POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:16-alpine}"
export POSTGRES_PORT="${POSTGRES_PORT:-54329}"
export POSTGRES_USER="${POSTGRES_USER:-clawverse}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-clawverse}"
export POSTGRES_DB="${POSTGRES_DB:-clawverse}"

export CLAWVERSE_DB_DSN="${CLAWVERSE_DB_DSN:-host=127.0.0.1 port=${POSTGRES_PORT} user=${POSTGRES_USER} password=${POSTGRES_PASSWORD} dbname=${POSTGRES_DB} sslmode=disable}"

if [[ "${1:-}" == "--prepare" ]]; then
  (
    cd "$ROOT_DIR"
    ./scripts/gen_models_with_docker_postgres.sh
  )
fi

echo "[clawverse] env ready"
echo "  CLAWVERSE_DB_DSN=$CLAWVERSE_DB_DSN"
echo "  docker container=$CONTAINER_NAME (port=$POSTGRES_PORT)"
echo ""
echo "next:"
echo "  go run ./cmd/server"
