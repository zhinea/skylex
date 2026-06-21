ALTER TABLE cluster_connection_profiles ADD COLUMN allowed_admin_cidrs TEXT NOT NULL DEFAULT '[]';
ALTER TABLE cluster_connection_profiles ADD COLUMN allowed_replication_cidrs TEXT NOT NULL DEFAULT '[]';

CREATE TABLE IF NOT EXISTS postgres_hba_apply_status (
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    command_id TEXT REFERENCES agent_commands(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    error TEXT NOT NULL DEFAULT '',
    applied_at TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (cluster_id, node_id)
);

CREATE INDEX IF NOT EXISTS idx_postgres_hba_apply_status_cluster_id ON postgres_hba_apply_status(cluster_id);
CREATE INDEX IF NOT EXISTS idx_postgres_hba_apply_status_command_id ON postgres_hba_apply_status(command_id);
