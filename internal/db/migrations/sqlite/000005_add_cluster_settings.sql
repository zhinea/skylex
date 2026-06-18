CREATE TABLE IF NOT EXISTS cluster_settings (
    id TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(cluster_id, key)
);

CREATE INDEX IF NOT EXISTS idx_cluster_settings_cluster_id ON cluster_settings(cluster_id);
