ALTER TABLE nodes ADD COLUMN agent_id TEXT DEFAULT '';

CREATE INDEX idx_nodes_agent_id ON nodes(agent_id);