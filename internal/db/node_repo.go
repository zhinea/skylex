package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
)

type NodeRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

func NewNodeRepository(conn *sql.DB, log *slog.Logger) *NodeRepository {
	return &NodeRepository{conn: conn, log: log}
}

func (r *NodeRepository) Create(ctx context.Context, clusterID, hostname, address string, port int, role models.NodeRole, agentVersion string, labels map[string]string) (*models.Node, error) {
	nodeID := id.New()
	now := time.Now().UTC()

	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, fmt.Errorf("marshal labels: %w", err)
	}

	_, err = r.conn.ExecContext(ctx,
		`INSERT INTO nodes (id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nodeID, clusterID, hostname, address, port, role, models.NodeStatusOnline, agentVersion, "", string(labelsJSON), now, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert node: %w", err)
	}

	return &models.Node{
		ID:           nodeID,
		ClusterID:    clusterID,
		Hostname:     hostname,
		Address:      address,
		Port:         port,
		Role:         role,
		Status:       models.NodeStatusOnline,
		AgentVersion: agentVersion,
		Labels:       labels,
		LastSeen:     now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (r *NodeRepository) GetByID(ctx context.Context, id string) (*models.Node, error) {
	row := r.conn.QueryRowContext(ctx,
		`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at
		 FROM nodes WHERE id = ?`, id)
	return scanNodeRow(row)
}

func (r *NodeRepository) ListByCluster(ctx context.Context, clusterID string, offset, limit int) ([]*models.Node, int, error) {
	var total int
	if err := r.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE cluster_id = ?`, clusterID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count nodes: %w", err)
	}

	rows, err := r.conn.QueryContext(ctx,
		`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at
		 FROM nodes WHERE cluster_id = ? ORDER BY role ASC, created_at ASC LIMIT ? OFFSET ?`,
		clusterID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*models.Node
	for rows.Next() {
		n, err := scanNodesRow(rows)
		if err != nil {
			return nil, 0, err
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate nodes: %w", err)
	}

	return nodes, total, nil
}

func (r *NodeRepository) UpdateStatus(ctx context.Context, id string, status models.NodeStatus) error {
	_, err := r.conn.ExecContext(ctx,
		`UPDATE nodes SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node status: %w", err)
	}
	return nil
}

func (r *NodeRepository) UpdateRole(ctx context.Context, id string, role models.NodeRole) error {
	_, err := r.conn.ExecContext(ctx,
		`UPDATE nodes SET role = ?, updated_at = ? WHERE id = ?`,
		role, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node role: %w", err)
	}
	return nil
}

func (r *NodeRepository) UpdateHeartbeat(ctx context.Context, id string) error {
	_, err := r.conn.ExecContext(ctx,
		`UPDATE nodes SET last_seen = ?, updated_at = ? WHERE id = ?`,
		time.Now().UTC(), time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node heartbeat: %w", err)
	}
	return nil
}

func (r *NodeRepository) GetPrimary(ctx context.Context, clusterID string) (*models.Node, error) {
	row := r.conn.QueryRowContext(ctx,
		`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at
		 FROM nodes WHERE cluster_id = ? AND role = ? LIMIT 1`,
		clusterID, models.NodeRolePrimary)
	return scanNodeRow(row)
}

func (r *NodeRepository) GetReplicas(ctx context.Context, clusterID string) ([]*models.Node, error) {
	rows, err := r.conn.QueryContext(ctx,
		`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at
		 FROM nodes WHERE cluster_id = ? AND role = ? ORDER BY created_at ASC`,
		clusterID, models.NodeRoleReplica)
	if err != nil {
		return nil, fmt.Errorf("query replicas: %w", err)
	}
	defer rows.Close()

	var nodes []*models.Node
	for rows.Next() {
		n, err := scanNodesRow(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (r *NodeRepository) Delete(ctx context.Context, id string) error {
	_, err := r.conn.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	return nil
}

func (r *NodeRepository) UpdateAgentID(ctx context.Context, id, agentID string) error {
	_, err := r.conn.ExecContext(ctx,
		`UPDATE nodes SET agent_id = ?, updated_at = ? WHERE id = ?`,
		agentID, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node agent_id: %w", err)
	}
	return nil
}

func (r *NodeRepository) GetByHostname(ctx context.Context, hostname string) (*models.Node, error) {
	row := r.conn.QueryRowContext(ctx,
		`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at
		 FROM nodes WHERE hostname = ? LIMIT 1`, hostname)
	return scanNodeRow(row)
}

func (r *NodeRepository) GetByAgentID(ctx context.Context, agentID string) (*models.Node, error) {
	row := r.conn.QueryRowContext(ctx,
		`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at
		 FROM nodes WHERE agent_id = ? LIMIT 1`, agentID)
	return scanNodeRow(row)
}

func scanNodeRow(row *sql.Row) (*models.Node, error) {
	var n models.Node
	var labelsJSON string

	err := row.Scan(&n.ID, &n.ClusterID, &n.Hostname, &n.Address, &n.Port,
		&n.Role, &n.Status, &n.AgentVersion, &n.AgentID, &labelsJSON, &n.LastSeen, &n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan node: %w", err)
	}

	n.Labels = unmarshalLabels(labelsJSON)
	return &n, nil
}

func scanNodesRow(rows *sql.Rows) (*models.Node, error) {
	var n models.Node
	var labelsJSON string

	if err := rows.Scan(&n.ID, &n.ClusterID, &n.Hostname, &n.Address, &n.Port,
		&n.Role, &n.Status, &n.AgentVersion, &n.AgentID, &labelsJSON, &n.LastSeen, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan node row: %w", err)
	}

	n.Labels = unmarshalLabels(labelsJSON)
	return &n, nil
}