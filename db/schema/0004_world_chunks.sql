CREATE TABLE IF NOT EXISTS world_chunks (
  id BIGSERIAL PRIMARY KEY,
  chunk_x INTEGER NOT NULL,
  chunk_y INTEGER NOT NULL,
  phase TEXT NOT NULL,
  tiles BYTEA NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(chunk_x, chunk_y, phase)
);

CREATE INDEX IF NOT EXISTS idx_world_chunks_phase ON world_chunks(phase);
