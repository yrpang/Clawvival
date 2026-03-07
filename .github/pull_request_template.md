## Checklist

- [ ] Production migration applied

Notes:
- Check this only when the PR changes `db/schema/*` and you have already run `scripts/migrate_postgres.sh` against the target environment.
- Leave it unchecked for PRs without schema changes.
