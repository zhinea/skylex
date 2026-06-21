CREATE TABLE IF NOT EXISTS postgres_roles (
    id TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    role_name TEXT NOT NULL,
    role_kind TEXT NOT NULL CHECK (role_kind IN ('admin', 'read_write', 'read_only', 'custom')),
    encrypted_password TEXT NOT NULL,
    password_version INTEGER NOT NULL DEFAULT 1,
    expires_at TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ready', 'failed', 'deleting')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (cluster_id, role_name)
);

CREATE INDEX IF NOT EXISTS idx_postgres_roles_cluster_id ON postgres_roles(cluster_id);

CREATE TABLE IF NOT EXISTS postgres_operations (
    id TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    node_id TEXT REFERENCES nodes(id) ON DELETE SET NULL,
    operation_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_postgres_operations_cluster_id ON postgres_operations(cluster_id);
CREATE INDEX IF NOT EXISTS idx_postgres_operations_node_id ON postgres_operations(node_id);

CREATE TABLE IF NOT EXISTS agent_command_secrets (
    id TEXT PRIMARY KEY,
    command_id TEXT NOT NULL REFERENCES agent_commands(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    ciphertext TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    UNIQUE (command_id, key)
);

CREATE INDEX IF NOT EXISTS idx_agent_command_secrets_command_id ON agent_command_secrets(command_id);
