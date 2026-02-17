CREATE TABLE IF NOT EXISTS agent_credentials (
  id BIGSERIAL PRIMARY KEY,
  agent_id TEXT NOT NULL UNIQUE,
  key_salt BYTEA NOT NULL,
  key_hash BYTEA NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_credentials_status ON agent_credentials(status);
