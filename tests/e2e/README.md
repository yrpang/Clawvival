# Remote E2E API Tests

Run against deployed service:

```bash
go test -tags=e2e ./tests/e2e -v
```

Optional env vars:

- `E2E_BASE_URL` (default: `https://clawvival.app`)
- `E2E_AGENT_ID` (optional; when empty, test will call `/api/agent/register`)
- `E2E_AGENT_KEY` (optional; must be provided together with `E2E_AGENT_ID`, otherwise test will auto-register)

Example:

```bash
E2E_BASE_URL=https://clawvival.app \
E2E_AGENT_ID=your-agent-id \
E2E_AGENT_KEY=your-agent-key \
go test -tags=e2e ./tests/e2e -v
```

The suite validates main endpoints:
- `/api/agent/observe`
- `/api/agent/action`
- `/api/agent/status`
- `/api/agent/replay`
- `/skills/index.json`
- `/skills/survival/skill.md`
- `/ops/kpi`

Coverage focus (fast acceptance against MVP design + engineering contracts):
- Auth and header contract (`register` + protected endpoint checks)
- `observe/status` schema contract (`view=11x11`, `session_id`, `world.rules`, `action_costs` intent set)
- Entity visibility contract (`objects/resources/threats` only on visible tiles)
- Action intent contract (all MVP intents covered via success/rejection paths)
- Unified rejection envelope (`result_code=REJECTED` + `error/action_error`)
- `terminate` contract (interrupt ongoing `rest`, proportional settle, continue actions after terminate)
- Replay contract (`session_id` filter, event payload essentials) and `/ops/kpi` availability
