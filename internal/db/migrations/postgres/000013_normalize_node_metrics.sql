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
    cpu_usage_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    load_average_1m BIGINT NOT NULL DEFAULT 0,
    load_average_5m BIGINT NOT NULL DEFAULT 0,
    load_average_15m BIGINT NOT NULL DEFAULT 0,
    memory_total_bytes BIGINT NOT NULL DEFAULT 0,
    memory_used_bytes BIGINT NOT NULL DEFAULT 0,
    memory_available_bytes BIGINT NOT NULL DEFAULT 0,
    memory_usage_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    disk_total_bytes BIGINT NOT NULL DEFAULT 0,
    disk_used_bytes BIGINT NOT NULL DEFAULT 0,
    disk_available_bytes BIGINT NOT NULL DEFAULT 0,
    disk_usage_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    uptime_seconds BIGINT NOT NULL DEFAULT 0
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
WHERE NOT EXISTS (SELECT 1 FROM node_metrics WHERE node_metrics.id = nodes.id || '-metric')
  AND (
    os <> '' OR platform <> '' OR platform_version <> '' OR kernel_version <> '' OR architecture <> ''
    OR cpu_cores <> 0 OR cpu_usage_percent <> 0 OR load_average_1m <> 0 OR load_average_5m <> 0 OR load_average_15m <> 0
    OR memory_total_bytes <> 0 OR memory_used_bytes <> 0 OR memory_available_bytes <> 0 OR memory_usage_percent <> 0
    OR disk_total_bytes <> 0 OR disk_used_bytes <> 0 OR disk_available_bytes <> 0 OR disk_usage_percent <> 0 OR uptime_seconds <> 0
  );

CREATE INDEX IF NOT EXISTS idx_node_metrics_node_recorded_at ON node_metrics(node_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_metrics_recorded_at ON node_metrics(recorded_at DESC);

ALTER TABLE nodes DROP COLUMN IF EXISTS os;
ALTER TABLE nodes DROP COLUMN IF EXISTS platform;
ALTER TABLE nodes DROP COLUMN IF EXISTS platform_version;
ALTER TABLE nodes DROP COLUMN IF EXISTS kernel_version;
ALTER TABLE nodes DROP COLUMN IF EXISTS architecture;
ALTER TABLE nodes DROP COLUMN IF EXISTS cpu_cores;
ALTER TABLE nodes DROP COLUMN IF EXISTS cpu_usage_percent;
ALTER TABLE nodes DROP COLUMN IF EXISTS load_average_1m;
ALTER TABLE nodes DROP COLUMN IF EXISTS load_average_5m;
ALTER TABLE nodes DROP COLUMN IF EXISTS load_average_15m;
ALTER TABLE nodes DROP COLUMN IF EXISTS memory_total_bytes;
ALTER TABLE nodes DROP COLUMN IF EXISTS memory_used_bytes;
ALTER TABLE nodes DROP COLUMN IF EXISTS memory_available_bytes;
ALTER TABLE nodes DROP COLUMN IF EXISTS memory_usage_percent;
ALTER TABLE nodes DROP COLUMN IF EXISTS disk_total_bytes;
ALTER TABLE nodes DROP COLUMN IF EXISTS disk_used_bytes;
ALTER TABLE nodes DROP COLUMN IF EXISTS disk_available_bytes;
ALTER TABLE nodes DROP COLUMN IF EXISTS disk_usage_percent;
ALTER TABLE nodes DROP COLUMN IF EXISTS uptime_seconds;
