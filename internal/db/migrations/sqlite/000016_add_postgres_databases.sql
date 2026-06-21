CREATE TABLE IF NOT EXISTS postgres_databases (
    id TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    database_name TEXT NOT NULL,
    owner_role_id TEXT REFERENCES postgres_roles(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ready', 'failed', 'deleting')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (cluster_id, database_name)
);

CREATE INDEX IF NOT EXISTS idx_postgres_databases_cluster_id ON postgres_databases(cluster_id);
CREATE INDEX IF NOT EXISTS idx_postgres_databases_owner_role_id ON postgres_databases(owner_role_id);
