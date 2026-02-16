CREATE TABLE IF NOT EXISTS agent_states (
  agent_id TEXT PRIMARY KEY,
  hp INTEGER NOT NULL,
  hunger INTEGER NOT NULL,
  energy INTEGER NOT NULL,
  x INTEGER NOT NULL,
  y INTEGER NOT NULL,
  version BIGINT NOT NULL,
  updated_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS action_executions (
  id BIGSERIAL PRIMARY KEY,
  agent_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  intent_type TEXT NOT NULL,
  dt INTEGER NOT NULL,
  result_code TEXT NOT NULL,
  updated_state BYTEA,
  events BYTEA,
  applied_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(agent_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS domain_events (
  id BIGSERIAL PRIMARY KEY,
  agent_id TEXT,
  type TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  payload BYTEA,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
