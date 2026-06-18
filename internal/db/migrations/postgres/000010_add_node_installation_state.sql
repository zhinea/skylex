ALTER TABLE nodes ADD COLUMN IF NOT EXISTS installation_state TEXT NOT NULL DEFAULT 'pending_preflight';
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS conflict_details TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_nodes_installation_state ON nodes(installation_state);
