UPDATE cluster_connection_profiles SET ssl_mode = 'required' WHERE ssl_mode = 'require';
UPDATE cluster_connection_profiles SET ssl_mode = 'disabled' WHERE ssl_mode = 'disable';

ALTER TABLE cluster_connection_profiles ADD COLUMN tls_cert_file TEXT NOT NULL DEFAULT '';
ALTER TABLE cluster_connection_profiles ADD COLUMN tls_key_file TEXT NOT NULL DEFAULT '';
ALTER TABLE cluster_connection_profiles ADD COLUMN tls_ca_file TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS postgres_tls_apply_status (
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    command_id TEXT REFERENCES agent_commands(id) ON DELETE SET NULL,
    requested_tls_mode TEXT NOT NULL DEFAULT 'prefer' CHECK (requested_tls_mode IN ('disabled', 'prefer', 'required')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    error TEXT NOT NULL DEFAULT '',
    tls_active BOOLEAN NOT NULL DEFAULT FALSE,
    applied_at TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (cluster_id, node_id)
);

CREATE INDEX IF NOT EXISTS idx_postgres_tls_apply_status_cluster_id ON postgres_tls_apply_status(cluster_id);
CREATE INDEX IF NOT EXISTS idx_postgres_tls_apply_status_command_id ON postgres_tls_apply_status(command_id);
