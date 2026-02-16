#!/usr/bin/env bash
set -euo pipefail

SCHEMA_DIR="${SCHEMA_DIR:-db/schema}"
MIGRATIONS_TABLE="${MIGRATIONS_TABLE:-schema_migrations}"
SECRETS_FILE="${SECRETS_FILE:-.secrets}"
FLY_PROXY_LOCAL_PORT="${FLY_PROXY_LOCAL_PORT:-15432}"
FLY_PROXY_REMOTE="${FLY_PROXY_REMOTE:-5432}"
FLY_PROXY_TARGET="${FLY_PROXY_TARGET:-calendar-db.flycast}"

if [ ! -d "$SCHEMA_DIR" ]; then
  echo "schema dir not found: $SCHEMA_DIR" >&2
  exit 1
fi

if [ ! -f "$SECRETS_FILE" ]; then
  echo "secrets file not found: $SECRETS_FILE" >&2
  exit 1
fi

if ! command -v flyctl >/dev/null 2>&1; then
  echo "flyctl is required" >&2
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "psql is required" >&2
  exit 1
fi

echo "starting fly proxy: ${FLY_PROXY_LOCAL_PORT}:${FLY_PROXY_REMOTE} ${FLY_PROXY_TARGET}"
flyctl proxy "${FLY_PROXY_LOCAL_PORT}:${FLY_PROXY_REMOTE}" "${FLY_PROXY_TARGET}" >/tmp/clawverse-fly-proxy.log 2>&1 &
PROXY_PID=$!

cleanup() {
  if [ -n "${PROXY_PID:-}" ] && kill -0 "$PROXY_PID" >/dev/null 2>&1; then
    kill "$PROXY_PID" >/dev/null 2>&1 || true
    wait "$PROXY_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

for _ in $(seq 1 30); do
  if (echo >"/dev/tcp/127.0.0.1/${FLY_PROXY_LOCAL_PORT}") >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! (echo >"/dev/tcp/127.0.0.1/${FLY_PROXY_LOCAL_PORT}") >/dev/null 2>&1; then
  echo "fly proxy not ready on 127.0.0.1:${FLY_PROXY_LOCAL_PORT}" >&2
  echo "proxy log: /tmp/clawverse-fly-proxy.log" >&2
  exit 1
fi

echo "loading secrets from ${SECRETS_FILE}"
set -a
# shellcheck disable=SC1090
source "$SECRETS_FILE"
set +a

DSN="${CLAWVERSE_DB_DSN:-}"

if [ -z "$DSN" ]; then
  echo "CLAWVERSE_DB_DSN is required (from ${SECRETS_FILE})" >&2
  exit 1
fi

# Force migration traffic through local fly proxy.
if [[ "$DSN" == postgres://* || "$DSN" == postgresql://* ]]; then
  DSN="$(printf '%s' "$DSN" | sed -E "s#^((postgres(ql)?://([^/@]+@)?))[^/:?]+(:[0-9]+)?#\\1127.0.0.1:${FLY_PROXY_LOCAL_PORT}#")"
else
  # For key=value DSN style, append local host/port overrides.
  DSN="${DSN} host=127.0.0.1 port=${FLY_PROXY_LOCAL_PORT}"
fi

psql "$DSN" -v ON_ERROR_STOP=1 <<SQL
CREATE TABLE IF NOT EXISTS ${MIGRATIONS_TABLE} (
  version TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
SQL

found_any=0
while IFS= read -r file; do
  found_any=1
  filename="$(basename "$file")"
  version="${filename%.sql}"

  applied="$(psql "$DSN" -At -c "SELECT 1 FROM ${MIGRATIONS_TABLE} WHERE version='${version}' LIMIT 1;")"
  if [ "$applied" = "1" ]; then
    echo "skip $filename (already applied)"
    continue
  fi

  echo "apply $filename"
  tmp_sql="$(mktemp)"
  {
    echo "BEGIN;"
    cat "$file"
    echo
    echo "INSERT INTO ${MIGRATIONS_TABLE}(version, applied_at) VALUES ('${version}', NOW());"
    echo "COMMIT;"
  } > "$tmp_sql"

  if ! psql "$DSN" -v ON_ERROR_STOP=1 -f "$tmp_sql" >/dev/null; then
    rm -f "$tmp_sql"
    exit 1
  fi
  rm -f "$tmp_sql"
done < <(find "$SCHEMA_DIR" -maxdepth 1 -type f -name '*.sql' | sort)

if [ "$found_any" = "0" ]; then
  echo "no .sql files found in $SCHEMA_DIR" >&2
  exit 1
fi

echo "migrations complete"
