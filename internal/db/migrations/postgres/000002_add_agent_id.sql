ALTER TABLE nodes ADD COLUMN IF NOT EXISTS agent_id TEXT DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_nodes_agent_id ON nodes(agent_id);