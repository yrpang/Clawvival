#!/usr/bin/env bash
set -euo pipefail

DSN="${CLAWVERSE_DB_DSN:-}"
SCHEMA_DIR="${SCHEMA_DIR:-db/schema}"
MIGRATIONS_TABLE="${MIGRATIONS_TABLE:-schema_migrations}"

if [ -z "$DSN" ]; then
  echo "CLAWVERSE_DB_DSN is required" >&2
  exit 1
fi

if [ ! -d "$SCHEMA_DIR" ]; then
  echo "schema dir not found: $SCHEMA_DIR" >&2
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "psql is required" >&2
  exit 1
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

  psql "$DSN" -v ON_ERROR_STOP=1 -f "$tmp_sql" >/dev/null
  rm -f "$tmp_sql"
done < <(find "$SCHEMA_DIR" -maxdepth 1 -type f -name '*.sql' | sort)

if [ "$found_any" = "0" ]; then
  echo "no .sql files found in $SCHEMA_DIR"
  exit 1
fi

echo "migrations complete"
