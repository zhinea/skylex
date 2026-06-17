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
    agent_id TEXT DEFAULT ''
);

INSERT INTO nodes_new SELECT id, cluster_id, hostname, address, port, role, status, agent_version, labels, last_seen, created_at, updated_at, agent_id FROM nodes;

DROP TABLE nodes;
ALTER TABLE nodes_new RENAME TO nodes;

CREATE INDEX idx_nodes_cluster_id ON nodes(cluster_id);
CREATE INDEX idx_nodes_agent_id ON nodes(agent_id);

PRAGMA foreign_keys = ON;
