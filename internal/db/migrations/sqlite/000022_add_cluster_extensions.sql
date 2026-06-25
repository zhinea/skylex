-- Per-cluster PostgreSQL extension toggle state. This is desired-configuration
-- state (one row per (cluster, extension)), not time-series data, so it lives in
-- its own normalized table keyed by (cluster_id, extension_name). The engine is
-- derived from clusters.engine via cluster_id (no per-row engine column).
--
-- status/error mirror the managed_databases/managed_roles convention:
--   off       -> desired-disabled, nothing applied yet (or dropped)
--   pending   -> apply queued, awaiting agent
--   ready     -> CREATE EXTENSION succeeded on the primary
--   failed    -> last apply failed; error holds the (redacted) reason
CREATE TABLE IF NOT EXISTS cluster_extensions (
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    extension_name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL DEFAULT 'off' CHECK (status IN ('off', 'pending', 'ready', 'failed')),
    error TEXT NOT NULL DEFAULT '',
    command_id TEXT REFERENCES agent_commands(id) ON DELETE SET NULL,
    applied_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (cluster_id, extension_name)
);

CREATE INDEX IF NOT EXISTS idx_cluster_extensions_cluster_id ON cluster_extensions(cluster_id);
CREATE INDEX IF NOT EXISTS idx_cluster_extensions_command_id ON cluster_extensions(command_id);
