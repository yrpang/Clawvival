# Clawvival Agent Engineering Constraints

## DDD Layering
- `domain` only contains business rules and pure domain models; it must not import GORM/DB or adapter implementations.
- `app` orchestrates use cases and depends on `domain` + `app/ports`.
- `adapter` implements ports (HTTP/DB/runtime integrations) and is the only layer allowed to use framework or persistence details.

## Persistence Style (GORM + Schema-First)
- Database schema is source of truth (`db/schema/*`); models are generated from schema via `gorm/gen`.
- In adapter persistence code, prefer generated models under `internal/adapter/repo/gorm/model/*`.
- Do not keep parallel table structs (`xxxRow`) for the same table in production code unless there is a documented technical reason.
- If a temporary custom struct is unavoidable, add a comment with reason, scope, and planned removal.

## Query Style
- Use GORM object-style conditions (`Where(&Model{...})`, `Assign`, `FirstOrCreate`, etc.) as default.
- Avoid string SQL conditions in normal repository/runtime paths unless required for complex queries.

## TDD Workflow (Required)
- Follow strict TDD for feature and bugfix work:
1. Add/adjust tests first to express expected behavior.
2. Run tests and confirm the new/changed test fails for the expected reason.
3. Implement the minimal code change to make tests pass.
4. Re-run targeted tests, then full regression (`go test ./...`).
- Every completed task should include at least one meaningful test (unit/integration/e2e as appropriate).
- Do not merge code changes without matching test coverage for the changed behavior.

## Integration Test Environment (Local)
- Use the project script to prepare local integration-test dependencies and env vars:
  - `source scripts/setup_test_env.sh --prepare`
- This step should be run before any DB-backed integration/e2e test.
- After prepare, tests should use exported DSN env vars from the script (for example `CLAWVIVAL_DB_DSN`).
- Recommended execution order:
1. Run unit tests for the changed package(s).
2. Prepare local integration env (`source scripts/setup_test_env.sh --prepare`).
3. Run targeted integration/e2e tests.
4. Run full regression (`go test ./...`).

## Branching and Merge Policy
- Each task must be implemented on its own dedicated branch and then merged.
- Do not stack unrelated tasks in the same branch.
- Branch naming should use the task intent clearly (recommended prefix: `codex/`).
- Before merge, ensure:
1. Task-specific tests pass.
2. Full regression passes (`go test ./...`).
3. Changes are limited to the task scope.

## Deployment Runbook
- Clawvival production deploy is triggered by pushing to `main` (GitHub Action `Fly Deploy`).
- Before deploy, if schema changed:
1. Apply DB migration manually first (schema-first).
2. Use `scripts/migrate_postgres.sh` following the current project procedure (fly proxy + secrets/env + migrate).
- Recommended release sequence:
1. Run full tests: `go test ./...`.
2. If migration exists, execute migration and verify success.
3. Commit changes to `main`.
4. Push: `git push origin main`.
5. Check workflow status: `gh run list --branch main --limit 5` and `gh run watch <run_id> --exit-status`.
- Do not push code that depends on unapplied schema changes.
