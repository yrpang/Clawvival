#!/usr/bin/env bash
set -euo pipefail

# ---- Config (override via env) ----
CONTAINER_NAME="${CONTAINER_NAME:-clawvival-pg-gen}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:16-alpine}"
POSTGRES_PORT="${POSTGRES_PORT:-54329}"
POSTGRES_USER="${POSTGRES_USER:-clawvival}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-clawvival}"
POSTGRES_DB="${POSTGRES_DB:-clawvival}"
SCHEMA_DIR="${SCHEMA_DIR:-db/schema}"
MODEL_OUT="${MODEL_OUT:-../../internal/adapter/repo/gorm/model}"
KEEP_CONTAINER="${KEEP_CONTAINER:-1}" # 1=keep, 0=remove after generation

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

if [ ! -d "$SCHEMA_DIR" ]; then
  echo "schema dir not found: $SCHEMA_DIR" >&2
  exit 1
fi

# Start (or reuse) postgres container
if docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"; then
  if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME")" != "true" ]; then
    echo "starting existing container: $CONTAINER_NAME"
    docker start "$CONTAINER_NAME" >/dev/null
  else
    echo "reusing running container: $CONTAINER_NAME"
  fi
else
  echo "creating postgres container: $CONTAINER_NAME"
  docker run -d \
    --name "$CONTAINER_NAME" \
    -e POSTGRES_USER="$POSTGRES_USER" \
    -e POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
    -e POSTGRES_DB="$POSTGRES_DB" \
    -p "$POSTGRES_PORT":5432 \
    "$POSTGRES_IMAGE" >/dev/null
fi

# Wait until postgres is ready
for i in {1..60}; do
  if docker exec "$CONTAINER_NAME" pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
    break
  fi
  sleep 1
  if [ "$i" -eq 60 ]; then
    echo "postgres did not become ready in time" >&2
    exit 1
  fi
done

echo "postgres is ready on localhost:$POSTGRES_PORT"

# Apply schema migrations in lexical order
SQL_FILES="$(find "$SCHEMA_DIR" -maxdepth 1 -type f -name '*.sql' | sort)"
if [ -z "$SQL_FILES" ]; then
  echo "no .sql files found in $SCHEMA_DIR" >&2
  exit 1
fi

echo "$SQL_FILES" | while IFS= read -r f; do
  [ -z "$f" ] && continue
  echo "applying $(basename "$f")"
  docker exec -i "$CONTAINER_NAME" psql \
    -v ON_ERROR_STOP=1 \
    -U "$POSTGRES_USER" \
    -d "$POSTGRES_DB" < "$f" >/dev/null
done

export CLAWVIVAL_DB_DSN="host=127.0.0.1 port=$POSTGRES_PORT user=$POSTGRES_USER password=$POSTGRES_PASSWORD dbname=$POSTGRES_DB sslmode=disable"

echo "running gorm/gen"
(
  cd tools/modelgen
  if [ ! -f go.sum ]; then
    GOCACHE=/tmp/gocache go mod download
  fi
  GOCACHE=/tmp/gocache go run . --dsn "$CLAWVIVAL_DB_DSN" --out "$MODEL_OUT"
)

echo "done. generated models under internal/adapter/repo/gorm/model"

if [ "$KEEP_CONTAINER" = "0" ]; then
  echo "removing container $CONTAINER_NAME"
  docker rm -f "$CONTAINER_NAME" >/dev/null
fi
