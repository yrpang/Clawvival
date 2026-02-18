# modelgen

Generate GORM models from a PostgreSQL database (schema-first flow).

Recommended (one command from repo root):

```bash
./scripts/gen_models_with_docker_postgres.sh
```

This script will:
- start/reuse a Docker PostgreSQL container
- apply all `db/schema/*.sql`
- run `gorm/gen` and write models to `internal/adapter/repo/gorm/model`

Usage:

```bash
cd tools/modelgen
go mod tidy
go run . --dsn "$DATABASE_URL" --out ../../internal/adapter/repo/gorm/model
```

Prerequisites:
- PostgreSQL is reachable via DSN
- Migrations in `db/schema` have already been applied to that database
