ALTER TABLE agent_states
  ADD COLUMN IF NOT EXISTS ongoing_action_type TEXT,
  ADD COLUMN IF NOT EXISTS ongoing_action_end_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS ongoing_action_minutes INTEGER NOT NULL DEFAULT 0;
