CREATE TABLE IF NOT EXISTS cluster_connection_profiles (
    cluster_id TEXT PRIMARY KEY REFERENCES clusters(id) ON DELETE CASCADE,
    endpoint_mode TEXT NOT NULL DEFAULT 'direct_primary',
    public_host TEXT NOT NULL DEFAULT '',
    public_port INTEGER NOT NULL DEFAULT 5432,
    ssl_mode TEXT NOT NULL DEFAULT 'prefer',
    allowed_cidrs TEXT NOT NULL DEFAULT '[]',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_cluster_connection_profiles_cluster_id ON cluster_connection_profiles(cluster_id);
