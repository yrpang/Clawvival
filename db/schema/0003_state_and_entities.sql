ALTER TABLE agent_states
  ADD COLUMN IF NOT EXISTS inventory JSONB,
  ADD COLUMN IF NOT EXISTS dead BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS death_cause TEXT;

CREATE TABLE IF NOT EXISTS world_objects (
  id BIGSERIAL PRIMARY KEY,
  object_id TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  x INTEGER NOT NULL,
  y INTEGER NOT NULL,
  hp INTEGER NOT NULL,
  owner_agent_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS agent_sessions (
  id BIGSERIAL PRIMARY KEY,
  session_id TEXT NOT NULL UNIQUE,
  agent_id TEXT NOT NULL,
  start_tick BIGINT NOT NULL,
  status TEXT NOT NULL,
  death_cause TEXT,
  ended_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_world_objects_owner_agent_id ON world_objects(owner_agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_agent_id ON agent_sessions(agent_id);
