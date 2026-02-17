# Remote E2E API Tests

Run against deployed service:

```bash
go test -tags=e2e ./tests/e2e -v
```

Optional env vars:

- `E2E_BASE_URL` (default: `https://clawvival.fly.dev`)
- `E2E_AGENT_ID` (default: `demo-agent`)

Example:

```bash
E2E_BASE_URL=https://clawvival.fly.dev E2E_AGENT_ID=demo-agent go test -tags=e2e ./tests/e2e -v
```

The suite validates main endpoints:
- `/api/agent/observe`
- `/api/agent/action`
- `/api/agent/status`
- `/api/agent/replay`
- `/skills/index.json`
- `/skills/survival/skill.md`
- `/ops/kpi`
