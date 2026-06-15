CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'viewer',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_tokens (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    role TEXT NOT NULL DEFAULT 'operator',
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS clusters (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    engine TEXT NOT NULL DEFAULT 'postgresql',
    version TEXT NOT NULL DEFAULT '16',
    replication_mode TEXT NOT NULL DEFAULT 'asynchronous',
    replica_count INTEGER NOT NULL DEFAULT 0,
    storage_config_id TEXT,
    data_dir TEXT NOT NULL DEFAULT '/var/lib/postgresql/data',
    pitr_enabled INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'creating',
    labels TEXT DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    hostname TEXT NOT NULL,
    address TEXT NOT NULL,
    port INTEGER NOT NULL DEFAULT 5432,
    role TEXT NOT NULL DEFAULT 'replica',
    status TEXT NOT NULL DEFAULT 'offline',
    agent_version TEXT NOT NULL DEFAULT '',
    labels TEXT DEFAULT '{}',
    last_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS storage_configs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL DEFAULT 's3',
    endpoint TEXT NOT NULL,
    bucket TEXT NOT NULL,
    region TEXT NOT NULL DEFAULT '',
    access_key_id TEXT NOT NULL,
    secret_key TEXT NOT NULL,
    use_ssl INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS backups (
    id TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL,
    storage_config_id TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'full',
    storage_path TEXT NOT NULL,
    wal_start TEXT,
    wal_stop TEXT,
    lsn TEXT,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'running',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS restore_jobs (
    id TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    backup_id TEXT,
    target_type TEXT NOT NULL DEFAULT 'latest',
    target_value TEXT,
    target_node TEXT,
    status TEXT NOT NULL DEFAULT 'running',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS backup_schedules (
    id TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    cron TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'full',
    retention_count INTEGER NOT NULL DEFAULT 7,
    retention_days INTEGER NOT NULL DEFAULT 30,
    storage_config_id TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE SEQUENCE IF NOT EXISTS audit_logs_id_seq;
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGINT PRIMARY KEY DEFAULT nextval('audit_logs_id_seq'),
    user_id TEXT,
    action TEXT NOT NULL,
    resource TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_commands (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    action TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    result TEXT,
    error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_nodes_cluster_id ON nodes(cluster_id);
CREATE INDEX IF NOT EXISTS idx_backups_cluster_id ON backups(cluster_id);
CREATE INDEX IF NOT EXISTS idx_backups_status ON backups(status);
CREATE INDEX IF NOT EXISTS idx_restore_jobs_cluster_id ON restore_jobs(cluster_id);
CREATE INDEX IF NOT EXISTS idx_backup_schedules_cluster_id ON backup_schedules(cluster_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_agent_commands_agent_id ON agent_commands(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_commands_status ON agent_commands(status);