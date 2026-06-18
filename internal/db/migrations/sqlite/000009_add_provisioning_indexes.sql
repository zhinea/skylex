CREATE INDEX IF NOT EXISTS idx_nodes_cluster_id_role ON nodes(cluster_id, role);
CREATE INDEX IF NOT EXISTS idx_agent_commands_node_id_status ON agent_commands(node_id, status);
CREATE INDEX IF NOT EXISTS idx_agent_commands_node_id_created_at ON agent_commands(node_id, created_at);
