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

	var clusterIDNull sql.NullString
	if clusterID != "" {
		clusterIDNull = sql.NullString{String: clusterID, Valid: true}
	}

	_, err = r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO nodes (id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		nodeID, clusterIDNull, hostname, address, port, role, models.NodeStatusOnline, agentVersion, "", string(labelsJSON), now, now, now,
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
		Rebind(`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at, postgres_installed, postgres_version, postgres_data_initialized
		 FROM nodes WHERE id = ?`), id)
	return scanNodeRow(row)
}

func (r *NodeRepository) ListByCluster(ctx context.Context, clusterID string, offset, limit int) ([]*models.Node, int, error) {
	var (
		total int
		where string
		args  []interface{}
	)
	if clusterID != "" {
		where = "WHERE cluster_id = ?"
		args = append(args, clusterID)
	}

	countQuery := Rebind(fmt.Sprintf(`SELECT COUNT(*) FROM nodes %s`, where))
	if err := r.conn.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count nodes: %w", err)
	}

	listQuery := Rebind(fmt.Sprintf(`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at, postgres_installed, postgres_version, postgres_data_initialized
		 FROM nodes %s ORDER BY role ASC, created_at ASC LIMIT ? OFFSET ?`, where))
	queryArgs := append(args, limit, offset)

	rows, err := r.conn.QueryContext(ctx, listQuery, queryArgs...)
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
		Rebind(`UPDATE nodes SET status = ?, updated_at = ? WHERE id = ?`),
		status, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node status: %w", err)
	}
	return nil
}

func (r *NodeRepository) UpdateRole(ctx context.Context, id string, role models.NodeRole) error {
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE nodes SET role = ?, updated_at = ? WHERE id = ?`),
		role, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node role: %w", err)
	}
	return nil
}

func (r *NodeRepository) UpdateHeartbeat(ctx context.Context, id string) error {
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE nodes SET last_seen = ?, updated_at = ? WHERE id = ?`),
		time.Now().UTC(), time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node heartbeat: %w", err)
	}
	return nil
}

// ListUnassigned returns up to limit nodes that are not part of any cluster.
func (r *NodeRepository) ListUnassigned(ctx context.Context, limit int) ([]*models.Node, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at
		 FROM nodes WHERE (cluster_id IS NULL OR cluster_id = '') ORDER BY created_at ASC LIMIT ?`),
		limit)
	if err != nil {
		return nil, fmt.Errorf("query unassigned nodes: %w", err)
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

// AssignToCluster puts an idle node into a cluster with the given role.
func (r *NodeRepository) AssignToCluster(ctx context.Context, nodeID, clusterID string, role models.NodeRole) error {
	var clusterIDNull sql.NullString
	if clusterID != "" {
		clusterIDNull = sql.NullString{String: clusterID, Valid: true}
	}

	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE nodes SET cluster_id = ?, role = ?, updated_at = ? WHERE id = ?`),
		clusterIDNull, role, time.Now().UTC(), nodeID)
	if err != nil {
		return fmt.Errorf("assign node to cluster: %w", err)
	}
	return nil
}

func (r *NodeRepository) GetPrimary(ctx context.Context, clusterID string) (*models.Node, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at, postgres_installed, postgres_version, postgres_data_initialized
		 FROM nodes WHERE cluster_id = ? AND role = ? LIMIT 1`),
		clusterID, models.NodeRolePrimary)
	return scanNodeRow(row)
}

func (r *NodeRepository) GetReplicas(ctx context.Context, clusterID string) ([]*models.Node, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at, postgres_installed, postgres_version, postgres_data_initialized
		 FROM nodes WHERE cluster_id = ? AND role = ? ORDER BY created_at ASC`),
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
	_, err := r.conn.ExecContext(ctx, Rebind(`DELETE FROM nodes WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	return nil
}

func (r *NodeRepository) UpdateAgentID(ctx context.Context, id, agentID string) error {
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE nodes SET agent_id = ?, updated_at = ? WHERE id = ?`),
		agentID, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node agent_id: %w", err)
	}
	return nil
}

func (r *NodeRepository) GetByHostname(ctx context.Context, hostname string) (*models.Node, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at, postgres_installed, postgres_version, postgres_data_initialized
		 FROM nodes WHERE hostname = ? LIMIT 1`), hostname)
	return scanNodeRow(row)
}

func (r *NodeRepository) GetByAgentID(ctx context.Context, agentID string) (*models.Node, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at, postgres_installed, postgres_version, postgres_data_initialized
		 FROM nodes WHERE agent_id = ? LIMIT 1`), agentID)
	return scanNodeRow(row)
}

func scanNodeRow(row *sql.Row) (*models.Node, error) {
	var n models.Node
	var clusterID sql.NullString
	var labelsJSON string

	err := row.Scan(&n.ID, &n.ClusterID, &n.Hostname, &n.Address, &n.Port,
		&n.Role, &n.Status, &n.AgentVersion, &n.AgentID, &labelsJSON, &n.LastSeen, &n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan node: %w", err)
	}

	n.ClusterID = clusterID.String
	n.Labels = unmarshalLabels(labelsJSON)
	return &n, nil
}

func scanNodesRow(rows *sql.Rows) (*models.Node, error) {
	var n models.Node
	var clusterID sql.NullString
	var labelsJSON string

	if err := rows.Scan(&n.ID, &n.ClusterID, &n.Hostname, &n.Address, &n.Port,
		&n.Role, &n.Status, &n.AgentVersion, &n.AgentID, &labelsJSON, &n.LastSeen, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan node row: %w", err)
	}

	n.ClusterID = clusterID.String
	n.Labels = unmarshalLabels(labelsJSON)
	return &n, nil
}

// UpdatePostgresStatus stores the PostgreSQL installation and data-directory
// state for a node as reported by the agent.
func (r *NodeRepository) UpdatePostgresStatus(ctx context.Context, id string, installed bool, version string, dataInitialized bool) error {
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE nodes SET postgres_installed = ?, postgres_version = ?, postgres_data_initialized = ?, updated_at = ? WHERE id = ?`),
		installed, version, dataInitialized, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node postgres status: %w", err)
	}
	return nil
}
