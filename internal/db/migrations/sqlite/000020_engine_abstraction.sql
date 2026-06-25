-- Engine-neutral rename: drop the postgres_ prefix so a single set of tables
-- serves every service provider (PostgreSQL, MariaDB, ...). The engine is
-- derived from clusters.engine via cluster_id, so no per-row engine column is
-- added (keeps tables normalized; engine is static per cluster).
ALTER TABLE postgres_roles RENAME TO managed_roles;
ALTER TABLE postgres_databases RENAME TO managed_databases;
ALTER TABLE postgres_operations RENAME TO service_operations;
ALTER TABLE postgres_tls_certificate_authorities RENAME TO service_tls_authorities;

DROP INDEX IF EXISTS idx_postgres_roles_cluster_id;
DROP INDEX IF EXISTS idx_postgres_databases_cluster_id;
DROP INDEX IF EXISTS idx_postgres_databases_owner_role_id;
DROP INDEX IF EXISTS idx_postgres_operations_cluster_id;
DROP INDEX IF EXISTS idx_postgres_operations_node_id;

CREATE INDEX IF NOT EXISTS idx_managed_roles_cluster_id ON managed_roles(cluster_id);
CREATE INDEX IF NOT EXISTS idx_managed_databases_cluster_id ON managed_databases(cluster_id);
CREATE INDEX IF NOT EXISTS idx_managed_databases_owner_role_id ON managed_databases(owner_role_id);
CREATE INDEX IF NOT EXISTS idx_service_operations_cluster_id ON service_operations(cluster_id);
CREATE INDEX IF NOT EXISTS idx_service_operations_node_id ON service_operations(node_id);

-- Consolidate per-node feature apply status (HBA + TLS) into one table to remove
-- the redundant near-identical postgres_hba_apply_status / postgres_tls_apply_status
-- tables. Feature-specific fields (requested_tls_mode, tls_active) live in the
-- detail JSON column. This is ephemeral operational state, re-derived on the next
-- Apply*, so the old per-feature tables are dropped rather than data-migrated.
DROP TABLE IF EXISTS postgres_hba_apply_status;
DROP TABLE IF EXISTS postgres_tls_apply_status;

CREATE TABLE IF NOT EXISTS node_feature_apply_status (
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    feature TEXT NOT NULL CHECK (feature IN ('hba', 'tls')),
    command_id TEXT REFERENCES agent_commands(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    error TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '{}',
    applied_at TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (cluster_id, node_id, feature)
);

CREATE INDEX IF NOT EXISTS idx_node_feature_apply_status_cluster_id ON node_feature_apply_status(cluster_id);
CREATE INDEX IF NOT EXISTS idx_node_feature_apply_status_command_id ON node_feature_apply_status(command_id);
