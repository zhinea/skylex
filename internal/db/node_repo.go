package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
)

type NodeRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

const nodeSelectColumns = `id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at, postgres_installed, postgres_version, postgres_data_initialized, status_detail, service_location, docker_available, installation_state, conflict_details, agent_latency_ms`

const nodeMetricSelectColumns = `id, node_id, recorded_at, os, platform, platform_version, kernel_version, architecture, cpu_cores, cpu_usage_percent, load_average_1m, load_average_5m, load_average_15m, memory_total_bytes, memory_used_bytes, memory_available_bytes, memory_usage_percent, disk_total_bytes, disk_used_bytes, disk_available_bytes, disk_usage_percent, uptime_seconds`

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
		nodeID, clusterIDNull, hostname, address, port, role, models.NodeStatusOffline, agentVersion, "", string(labelsJSON), now, now, now,
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
		Status:       models.NodeStatusOffline,
		AgentVersion: agentVersion,
		AgentID:      "",
		Labels:       labels,
		LastSeen:     now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (r *NodeRepository) GetByID(ctx context.Context, id string) (*models.Node, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT `+nodeSelectColumns+` FROM nodes WHERE id = ?`), id)
	node, err := scanNodeRow(row)
	if err != nil || node == nil {
		return node, err
	}
	return node, r.attachLatestMetrics(ctx, []*models.Node{node})
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

	listQuery := Rebind(fmt.Sprintf(`SELECT %s
		 FROM nodes %s ORDER BY role ASC, created_at ASC LIMIT ? OFFSET ?`, nodeSelectColumns, where))
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
	if err := r.attachLatestMetrics(ctx, nodes); err != nil {
		return nil, 0, err
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

func (r *NodeRepository) UpdateHeartbeat(ctx context.Context, id string, status models.NodeStatus, latencyMS int64) error {
	if latencyMS < 0 {
		latencyMS = 0
	}
	now := time.Now().UTC()
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE nodes SET status = ?, last_seen = ?, agent_latency_ms = ?, updated_at = ? WHERE id = ?`),
		status, now, latencyMS, now, id)
	if err != nil {
		return fmt.Errorf("update node heartbeat: %w", err)
	}
	return nil
}

// GetByIDs returns nodes matching the given IDs, preserving input order.
// Returns an error if any ID is not found.
func (r *NodeRepository) GetByIDs(ctx context.Context, ids []string) ([]*models.Node, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT %s
		 FROM nodes WHERE id IN (%s)`, nodeSelectColumns, strings.Join(placeholders, ", "))

	rows, err := r.conn.QueryContext(ctx, Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("query nodes by ids: %w", err)
	}
	defer rows.Close()

	byID := make(map[string]*models.Node, len(ids))
	for rows.Next() {
		n, err := scanNodesRow(rows)
		if err != nil {
			return nil, err
		}
		byID[n.ID] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nodes: %w", err)
	}

	// Validate all requested IDs were found, preserving input order.
	result := make([]*models.Node, 0, len(ids))
	var missing []string
	for _, id := range ids {
		n, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		result = append(result, n)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("node(s) not found: %s", strings.Join(missing, ", "))
	}

	return result, r.attachLatestMetrics(ctx, result)
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
		Rebind(`SELECT `+nodeSelectColumns+` FROM nodes WHERE cluster_id = ? AND role = ? LIMIT 1`),
		clusterID, models.NodeRolePrimary)
	node, err := scanNodeRow(row)
	if err != nil || node == nil {
		return node, err
	}
	return node, r.attachLatestMetrics(ctx, []*models.Node{node})
}

func (r *NodeRepository) GetReplicas(ctx context.Context, clusterID string) ([]*models.Node, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT `+nodeSelectColumns+` FROM nodes WHERE cluster_id = ? AND role = ? ORDER BY created_at ASC`),
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, r.attachLatestMetrics(ctx, nodes)
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
		Rebind(`SELECT `+nodeSelectColumns+` FROM nodes WHERE hostname = ? LIMIT 1`), hostname)
	node, err := scanNodeRow(row)
	if err != nil || node == nil {
		return node, err
	}
	return node, r.attachLatestMetrics(ctx, []*models.Node{node})
}

func (r *NodeRepository) GetByAgentID(ctx context.Context, agentID string) (*models.Node, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT `+nodeSelectColumns+` FROM nodes WHERE agent_id = ? LIMIT 1`), agentID)
	node, err := scanNodeRow(row)
	if err != nil || node == nil {
		return node, err
	}
	return node, r.attachLatestMetrics(ctx, []*models.Node{node})
}

func (r *NodeRepository) GetHostnameByID(ctx context.Context, id string) (string, error) {
	var hostname string
	err := r.conn.QueryRowContext(ctx, Rebind(`SELECT hostname FROM nodes WHERE id = ?`), id).Scan(&hostname)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get node hostname: %w", err)
	}
	return hostname, nil
}

func scanNodeRow(row *sql.Row) (*models.Node, error) {
	var n models.Node
	var clusterID sql.NullString
	var labelsJSON string

	err := row.Scan(&n.ID, &clusterID, &n.Hostname, &n.Address, &n.Port,
		&n.Role, &n.Status, &n.AgentVersion, &n.AgentID, &labelsJSON, &n.LastSeen, &n.CreatedAt, &n.UpdatedAt,
		&n.PostgresInstalled, &n.PostgresVersion, &n.PostgresDataInitialized, &n.StatusDetail,
		&n.ServiceLocation, &n.DockerAvailable, &n.InstallationState, &n.ConflictDetails, &n.AgentLatencyMS)
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

	if err := rows.Scan(&n.ID, &clusterID, &n.Hostname, &n.Address, &n.Port,
		&n.Role, &n.Status, &n.AgentVersion, &n.AgentID, &labelsJSON, &n.LastSeen, &n.CreatedAt, &n.UpdatedAt,
		&n.PostgresInstalled, &n.PostgresVersion, &n.PostgresDataInitialized, &n.StatusDetail,
		&n.ServiceLocation, &n.DockerAvailable, &n.InstallationState, &n.ConflictDetails, &n.AgentLatencyMS); err != nil {
		return nil, fmt.Errorf("scan node row: %w", err)
	}

	n.ClusterID = clusterID.String
	n.Labels = unmarshalLabels(labelsJSON)
	return &n, nil
}

func (r *NodeRepository) InsertMetric(ctx context.Context, metric *models.NodeMetric) error {
	if metric.ID == "" {
		metric.ID = id.New()
	}
	if metric.RecordedAt.IsZero() {
		metric.RecordedAt = time.Now().UTC()
	}
	_, err := r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO node_metrics (`+nodeMetricSelectColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		metric.ID, metric.NodeID, metric.RecordedAt, metric.OS, metric.Platform, metric.PlatformVersion, metric.KernelVersion, metric.Architecture,
		metric.CPUCores, metric.CPUUsagePercent, metric.LoadAverage1M, metric.LoadAverage5M, metric.LoadAverage15M,
		metric.MemoryTotalBytes, metric.MemoryUsedBytes, metric.MemoryAvailableBytes, metric.MemoryUsagePercent,
		metric.DiskTotalBytes, metric.DiskUsedBytes, metric.DiskAvailableBytes, metric.DiskUsagePercent, metric.UptimeSeconds)
	if err != nil {
		return fmt.Errorf("insert node metric: %w", err)
	}
	return nil
}

func (r *NodeRepository) ListMetrics(ctx context.Context, nodeID string, since time.Time, limit int) ([]*models.NodeMetric, error) {
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	query := `SELECT ` + nodeMetricSelectColumns + ` FROM (SELECT ` + nodeMetricSelectColumns + ` FROM node_metrics WHERE node_id = ?`
	args := []interface{}{nodeID}
	if !since.IsZero() {
		query += ` AND recorded_at >= ?`
		args = append(args, since)
	}
	query += ` ORDER BY recorded_at DESC LIMIT ?) ORDER BY recorded_at ASC`
	args = append(args, limit)

	rows, err := r.conn.QueryContext(ctx, Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("query node metrics: %w", err)
	}
	defer rows.Close()

	metrics := make([]*models.NodeMetric, 0, limit)
	for rows.Next() {
		m, err := scanNodeMetricRow(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate node metrics: %w", err)
	}
	return metrics, nil
}

func (r *NodeRepository) attachLatestMetrics(ctx context.Context, nodes []*models.Node) error {
	if len(nodes) == 0 {
		return nil
	}
	ids := make([]string, 0, len(nodes))
	byID := make(map[string]*models.Node, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
		byID[n.ID] = n
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT %s FROM node_metrics nm
		WHERE nm.node_id IN (%s)
		AND nm.id = (SELECT id FROM node_metrics WHERE node_id = nm.node_id ORDER BY recorded_at DESC, id DESC LIMIT 1)`, nodeMetricSelectColumns, strings.Join(placeholders, ", "))
	rows, err := r.conn.QueryContext(ctx, Rebind(query), args...)
	if err != nil {
		return fmt.Errorf("query latest node metrics: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		m, err := scanNodeMetricRow(rows)
		if err != nil {
			return err
		}
		if n := byID[m.NodeID]; n != nil {
			n.LatestMetrics = m
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate latest node metrics: %w", err)
	}
	return nil
}

func scanNodeMetricRow(rows *sql.Rows) (*models.NodeMetric, error) {
	var m models.NodeMetric
	if err := rows.Scan(&m.ID, &m.NodeID, &m.RecordedAt, &m.OS, &m.Platform, &m.PlatformVersion, &m.KernelVersion, &m.Architecture,
		&m.CPUCores, &m.CPUUsagePercent, &m.LoadAverage1M, &m.LoadAverage5M, &m.LoadAverage15M,
		&m.MemoryTotalBytes, &m.MemoryUsedBytes, &m.MemoryAvailableBytes, &m.MemoryUsagePercent,
		&m.DiskTotalBytes, &m.DiskUsedBytes, &m.DiskAvailableBytes, &m.DiskUsagePercent, &m.UptimeSeconds); err != nil {
		return nil, fmt.Errorf("scan node metric row: %w", err)
	}
	return &m, nil
}

func (r *NodeRepository) UpdateInstallationState(ctx context.Context, id string, state models.InstallationState, conflictDetails string) error {
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE nodes SET installation_state = ?, conflict_details = ?, updated_at = ? WHERE id = ?`),
		state, conflictDetails, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node installation state: %w", err)
	}
	return nil
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

// UpdateStatusDetail sets the human-readable status detail for a node.
func (r *NodeRepository) UpdateStatusDetail(ctx context.Context, id string, detail string) error {
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE nodes SET status_detail = ?, updated_at = ? WHERE id = ?`),
		detail, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node status detail: %w", err)
	}
	return nil
}

// UpdateDockerAvailable stores the Docker availability flag for a node as
// reported by the agent at registration/heartbeat time.
func (r *NodeRepository) UpdateDockerAvailable(ctx context.Context, id string, available bool) error {
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE nodes SET docker_available = ?, updated_at = ? WHERE id = ?`),
		available, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update node docker_available: %w", err)
	}
	return nil
}
