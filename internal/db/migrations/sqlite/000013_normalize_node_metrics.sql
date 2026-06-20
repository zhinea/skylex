CREATE TABLE IF NOT EXISTS node_metrics (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    recorded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    os TEXT NOT NULL DEFAULT '',
    platform TEXT NOT NULL DEFAULT '',
    platform_version TEXT NOT NULL DEFAULT '',
    kernel_version TEXT NOT NULL DEFAULT '',
    architecture TEXT NOT NULL DEFAULT '',
    cpu_cores INTEGER NOT NULL DEFAULT 0,
    cpu_usage_percent REAL NOT NULL DEFAULT 0,
    load_average_1m INTEGER NOT NULL DEFAULT 0,
    load_average_5m INTEGER NOT NULL DEFAULT 0,
    load_average_15m INTEGER NOT NULL DEFAULT 0,
    memory_total_bytes INTEGER NOT NULL DEFAULT 0,
    memory_used_bytes INTEGER NOT NULL DEFAULT 0,
    memory_available_bytes INTEGER NOT NULL DEFAULT 0,
    memory_usage_percent REAL NOT NULL DEFAULT 0,
    disk_total_bytes INTEGER NOT NULL DEFAULT 0,
    disk_used_bytes INTEGER NOT NULL DEFAULT 0,
    disk_available_bytes INTEGER NOT NULL DEFAULT 0,
    disk_usage_percent REAL NOT NULL DEFAULT 0,
    uptime_seconds INTEGER NOT NULL DEFAULT 0
);

INSERT INTO node_metrics (
    id, node_id, recorded_at, os, platform, platform_version, kernel_version, architecture,
    cpu_cores, cpu_usage_percent, load_average_1m, load_average_5m, load_average_15m,
    memory_total_bytes, memory_used_bytes, memory_available_bytes, memory_usage_percent,
    disk_total_bytes, disk_used_bytes, disk_available_bytes, disk_usage_percent, uptime_seconds
)
SELECT
    id || '-metric', id, updated_at, os, platform, platform_version, kernel_version, architecture,
    cpu_cores, cpu_usage_percent, load_average_1m, load_average_5m, load_average_15m,
    memory_total_bytes, memory_used_bytes, memory_available_bytes, memory_usage_percent,
    disk_total_bytes, disk_used_bytes, disk_available_bytes, disk_usage_percent, uptime_seconds
FROM nodes
WHERE os <> '' OR platform <> '' OR platform_version <> '' OR kernel_version <> '' OR architecture <> ''
   OR cpu_cores <> 0 OR cpu_usage_percent <> 0 OR load_average_1m <> 0 OR load_average_5m <> 0 OR load_average_15m <> 0
   OR memory_total_bytes <> 0 OR memory_used_bytes <> 0 OR memory_available_bytes <> 0 OR memory_usage_percent <> 0
   OR disk_total_bytes <> 0 OR disk_used_bytes <> 0 OR disk_available_bytes <> 0 OR disk_usage_percent <> 0 OR uptime_seconds <> 0;

CREATE INDEX IF NOT EXISTS idx_node_metrics_node_recorded_at ON node_metrics(node_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_metrics_recorded_at ON node_metrics(recorded_at DESC);

PRAGMA foreign_keys = OFF;

CREATE TABLE nodes_new (
    id TEXT PRIMARY KEY,
    cluster_id TEXT REFERENCES clusters(id) ON DELETE CASCADE,
    hostname TEXT NOT NULL,
    address TEXT NOT NULL,
    port INTEGER NOT NULL DEFAULT 5432,
    role TEXT NOT NULL DEFAULT 'replica',
    status TEXT NOT NULL DEFAULT 'offline',
    agent_version TEXT NOT NULL DEFAULT '',
    labels TEXT DEFAULT '{}',
    last_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    agent_id TEXT DEFAULT '',
    postgres_installed INTEGER NOT NULL DEFAULT 0,
    postgres_version TEXT NOT NULL DEFAULT '',
    postgres_data_initialized INTEGER NOT NULL DEFAULT 0,
    status_detail TEXT NOT NULL DEFAULT '',
    service_location TEXT NOT NULL DEFAULT 'native',
    docker_available INTEGER NOT NULL DEFAULT 0,
    installation_state TEXT NOT NULL DEFAULT 'pending_preflight',
    conflict_details TEXT NOT NULL DEFAULT '',
    agent_latency_ms INTEGER NOT NULL DEFAULT 0
);

INSERT INTO nodes_new (
    id, cluster_id, hostname, address, port, role, status, agent_version, labels, last_seen,
    created_at, updated_at, agent_id, postgres_installed, postgres_version,
    postgres_data_initialized, status_detail, service_location, docker_available,
    installation_state, conflict_details, agent_latency_ms
)
SELECT
    id, cluster_id, hostname, address, port, role, status, agent_version, labels, last_seen,
    created_at, updated_at, agent_id, postgres_installed, postgres_version,
    postgres_data_initialized, status_detail, service_location, docker_available,
    installation_state, conflict_details, agent_latency_ms
FROM nodes;

DROP TABLE nodes;
ALTER TABLE nodes_new RENAME TO nodes;

CREATE INDEX IF NOT EXISTS idx_nodes_cluster_id ON nodes(cluster_id);
CREATE INDEX IF NOT EXISTS idx_nodes_agent_id ON nodes(agent_id);
CREATE INDEX IF NOT EXISTS idx_nodes_installation_state ON nodes(installation_state);
CREATE INDEX IF NOT EXISTS idx_nodes_cluster_id_role ON nodes(cluster_id, role);

PRAGMA foreign_keys = ON;
