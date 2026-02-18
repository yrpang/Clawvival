CREATE TABLE IF NOT EXISTS agent_resource_nodes (
  id BIGSERIAL PRIMARY KEY,
  agent_id TEXT NOT NULL,
  target_id TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  x INTEGER NOT NULL,
  y INTEGER NOT NULL,
  depleted_until TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(agent_id, target_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_resource_nodes_agent_id ON agent_resource_nodes(agent_id);
