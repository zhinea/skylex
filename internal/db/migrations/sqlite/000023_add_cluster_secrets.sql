-- Durable cluster-scoped secret store. One row per (cluster_id, key);
-- upsert semantics keep the latest value. Cascade delete removes rows
-- when the parent cluster is dropped.
CREATE TABLE IF NOT EXISTS cluster_secrets (
    id TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    ciphertext TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cluster_secrets_cluster_key ON cluster_secrets(cluster_id, key);
