CREATE TABLE IF NOT EXISTS agent_command_logs (
    id TEXT PRIMARY KEY,
    command_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_command_logs_command_id_created_at ON agent_command_logs(command_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_command_logs_agent_id_created_at ON agent_command_logs(agent_id, created_at DESC);
