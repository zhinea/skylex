package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/engine"
	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ClusterService struct {
	skylexv1.UnimplementedClusterServiceServer
	conn           *sql.DB
	clusters       *db.ClusterRepository
	nodes          *db.NodeRepository
	commands       *db.AgentCommandRepository
	settings       *db.ClusterSettingsRepository
	audit          *db.AuditRepository
	failoverEngine *FailoverEngine
	log            *slog.Logger
}

const maxClusterLifecycleNodes = 1000

func NewClusterService(conn *sql.DB, clusters *db.ClusterRepository, nodes *db.NodeRepository, commands *db.AgentCommandRepository, settings *db.ClusterSettingsRepository, log *slog.Logger) *ClusterService {
	return &ClusterService{
		conn:     conn,
		clusters: clusters,
		nodes:    nodes,
		commands: commands,
		settings: settings,
		log:      log,
	}
}

func (s *ClusterService) SetFailoverEngine(e *FailoverEngine) {
	s.failoverEngine = e
}

func (s *ClusterService) SetAuditRepository(repo *db.AuditRepository) {
	s.audit = repo
}

// allowedClusterSettings is the curated set of PostgreSQL parameters that can
// be changed from the UI.  Restricting the surface area prevents typos and
// accidental outages from invalid configuration keys.
var allowedClusterSettings = map[string]struct{}{
	"max_connections": {},
	"shared_buffers":  {},
	"wal_level":       {},
	"max_wal_senders": {},
	"work_mem":        {},
}

var defaultClusterSettings = map[string]string{
	"max_connections": "200",
	"shared_buffers":  "128MB",
	"wal_level":       "replica",
	"max_wal_senders": "10",
	"work_mem":        "4MB",
}

func clusterSettingsWithDefaults(parameters map[string]string) map[string]string {
	merged := make(map[string]string, len(defaultClusterSettings)+len(parameters))
	for key, value := range defaultClusterSettings {
		merged[key] = value
	}
	for key, value := range parameters {
		merged[key] = value
	}
	return merged
}

// memoryUnitPattern accepts PostgreSQL memory units such as 128MB, 1GB, 256kB.
var memoryUnitPattern = regexp.MustCompile(`(?i)^\d+\s*(kB|MB|GB|TB|k|m|g|t)?$`)

// validateClusterSetting ensures a single key/value pair is safe to write into
// postgresql.conf.  Invalid values are rejected as gRPC InvalidArgument errors
// before anything is persisted or queued on an agent.
func validateClusterSetting(key, value string) error {
	if _, ok := allowedClusterSettings[key]; !ok {
		return fmt.Errorf("%q is not an editable PostgreSQL setting", key)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value for %q cannot be empty", key)
	}

	switch key {
	case "max_connections", "max_wal_senders":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("%q must be an integer", key)
		}
		if n <= 0 {
			return fmt.Errorf("%q must be greater than 0", key)
		}
	case "wal_level":
		switch strings.ToLower(value) {
		case "replica", "logical":
		default:
			return fmt.Errorf("%q must be replica or logical", key)
		}
	case "shared_buffers", "work_mem":
		if !memoryUnitPattern.MatchString(value) {
			return fmt.Errorf("%q must be a memory value such as 128MB", key)
		}
	}

	return nil
}

// validateClusterSettingsParameters validates the whole map and returns keys
// sorted lexicographically so callers can produce deterministic payloads.
func validateClusterSettingsParameters(parameters map[string]string) ([]string, error) {
	if len(parameters) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(parameters))
	for k := range parameters {
		if err := validateClusterSetting(k, parameters[k]); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *ClusterService) CreateCluster(ctx context.Context, req *skylexv1.CreateClusterRequest) (*skylexv1.CreateClusterResponse, error) {
	cfg := req.GetConfig()
	if cfg == nil {
		return nil, status.Error(codes.InvalidArgument, "config is required")
	}

	existing, err := s.clusters.GetByName(ctx, req.GetName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check cluster name: %v", err)
	}
	if existing != nil {
		return nil, status.Errorf(codes.AlreadyExists, "cluster %q already exists", req.GetName())
	}

	engine := convertEngine(cfg.GetEngine())
	version := cfg.GetVersion()
	if version == "" {
		version = "16"
	}

	mode := convertReplicationMode(cfg.GetReplicationMode())
	replicaCount := int(cfg.GetReplicaCount())
	neededNodes := replicaCount + 1

	nodeIDs := req.GetNodeIds()
	if len(nodeIDs) != neededNodes {
		return nil, status.Errorf(codes.InvalidArgument,
			"node_ids must have exactly %d entries (1 primary + %d replica(s)), got %d",
			neededNodes, replicaCount, len(nodeIDs))
	}

	// Fetch and validate nodes exist (GetByIDs returns them in input order).
	selectedNodes, err := s.nodes.GetByIDs(ctx, nodeIDs)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "resolve node_ids: %v", err)
	}

	// Verify all nodes are unassigned.
	var alreadyAssigned []string
	var missingAgent []string
	for _, n := range selectedNodes {
		if n.ClusterID != "" {
			alreadyAssigned = append(alreadyAssigned, n.Hostname)
		}
		if n.AgentID == "" {
			missingAgent = append(missingAgent, n.Hostname)
		}
	}
	if len(alreadyAssigned) > 0 {
		return nil, status.Errorf(codes.FailedPrecondition,
			"the following node(s) are already assigned to a cluster: %s",
			strings.Join(alreadyAssigned, ", "))
	}
	if len(missingAgent) > 0 {
		return nil, status.Errorf(codes.FailedPrecondition,
			"the following node(s) do not have a linked agent: %s",
			strings.Join(missingAgent, ", "))
	}

	// Preflight for Docker locations: if a node reports Docker as unavailable,
	// the agent will attempt to install/enable Docker Engine automatically when
	// it executes pg_install_docker. Log the situation so operators know why a
	// dependency is being pulled in.
	serviceLocation := convertServiceLocation(cfg.GetServiceLocation())
	if serviceLocation == models.ServiceLocationDocker {
		var noDocker []string
		for _, n := range selectedNodes {
			if !n.DockerAvailable {
				noDocker = append(noDocker, n.Hostname)
			}
		}
		if len(noDocker) > 0 {
			s.log.Warn("docker not available on selected node(s); agent will attempt to install/enable it",
				"nodes", strings.Join(noDocker, ", "))
		}
	}
	if serviceLocation == models.ServiceLocationUnspecified {
		serviceLocation = models.ServiceLocationNative
	}

	// Wrap cluster creation and node assignment in a transaction.
	clusterID := id.New()
	now := time.Now().UTC()

	labelsJSON, err := json.Marshal(cfg.GetLabels())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal labels: %v", err)
	}

	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "begin transaction: %v", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		db.Rebind(`INSERT INTO clusters (id, name, engine, version, replication_mode, replica_count, storage_config_id, data_dir, pitr_enabled, status, labels, service_location, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		clusterID, req.GetName(), engine, version, mode, replicaCount,
		cfg.GetStorageConfigId(), "", boolToInt(cfg.GetPitrEnabled()),
		models.ClusterStatusCreating, string(labelsJSON), serviceLocation, now, now,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create cluster: %v", err)
	}

	roles := make([]models.NodeRole, len(selectedNodes))
	roles[0] = models.NodeRolePrimary
	for i := 1; i < len(selectedNodes); i++ {
		roles[i] = models.NodeRoleReplica
	}

	for i, n := range selectedNodes {
		_, err = tx.ExecContext(ctx,
			db.Rebind(`UPDATE nodes SET cluster_id = ?, role = ?, service_location = ?, installation_state = ?, conflict_details = ?, updated_at = ? WHERE id = ?`),
			clusterID, roles[i], serviceLocation, models.InstallationStatePendingPreflight, "", now, n.ID,
		)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "assign node %s: %v", n.ID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "commit transaction: %v", err)
	}

	cluster := &models.Cluster{
		ID:              clusterID,
		Name:            req.GetName(),
		Engine:          engine,
		Version:         version,
		ReplicationMode: mode,
		Replicas:        replicaCount,
		StorageConfigID: cfg.GetStorageConfigId(),
		PITREnabled:     cfg.GetPitrEnabled(),
		Status:          models.ClusterStatusCreating,
		ServiceLocation: serviceLocation,
		Tags:            cfg.GetLabels(),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	assignedNodes := make([]*models.Node, len(selectedNodes))
	for i, n := range selectedNodes {
		copyNode := *n
		copyNode.ClusterID = clusterID
		copyNode.Role = roles[i]
		copyNode.ServiceLocation = serviceLocation
		copyNode.InstallationState = models.InstallationStatePendingPreflight
		copyNode.ConflictDetails = ""
		assignedNodes[i] = &copyNode
	}

	primary := assignedNodes[0]
	if err = s.queuePrimaryCommands(ctx, primary, version, serviceLocation); err != nil {
		return nil, status.Errorf(codes.Internal, "queue primary commands: %v", err)
	}

	for i := 1; i < len(assignedNodes); i++ {
		replica := assignedNodes[i]
		if err = s.queueReplicaCommands(ctx, replica, primary, version, serviceLocation); err != nil {
			return nil, status.Errorf(codes.Internal, "queue replica commands: %v", err)
		}
	}

	s.log.Info("cluster provisioning queued",
		"cluster_id", cluster.ID,
		"cluster_name", cluster.Name,
		"primary", primary.Hostname,
		"replicas", replicaCount,
	)

	return &skylexv1.CreateClusterResponse{
		Cluster: clusterToProto(cluster),
	}, nil
}

func (s *ClusterService) GetCluster(ctx context.Context, req *skylexv1.GetClusterRequest) (*skylexv1.GetClusterResponse, error) {
	cluster, err := s.clusters.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	if cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.GetId())
	}

	return &skylexv1.GetClusterResponse{
		Cluster: clusterToProto(cluster),
		Modules: engineModulesProto(cluster.Engine),
	}, nil
}

// engineModulesProto returns the UI module list advertised by the cluster's
// engine provider. It returns nil when no provider is registered for the engine
// (the UI then falls back to its built-in defaults).
func engineModulesProto(e models.EngineType) []*skylexv1.EngineModule {
	provider, err := engine.For(e)
	if err != nil {
		return nil
	}
	modules := provider.Modules()
	out := make([]*skylexv1.EngineModule, 0, len(modules))
	for _, m := range modules {
		out = append(out, &skylexv1.EngineModule{Id: string(m.ID), Label: m.Label})
	}
	return out
}

func (s *ClusterService) ListClusters(ctx context.Context, req *skylexv1.ListClustersRequest) (*skylexv1.ListClustersResponse, error) {
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	clusters, total, err := s.clusters.List(ctx, offset, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list clusters: %v", err)
	}

	var protoClusters []*skylexv1.Cluster
	for _, c := range clusters {
		protoClusters = append(protoClusters, clusterToProto(c))
	}

	return &skylexv1.ListClustersResponse{
		Clusters: protoClusters,
		Pagination: &skylexv1.Pagination{
			Page:     int32(page),
			PageSize: int32(pageSize),
			Total:    int32(total),
		},
	}, nil
}

func (s *ClusterService) UpdateCluster(ctx context.Context, req *skylexv1.UpdateClusterRequest) (*skylexv1.UpdateClusterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "UpdateCluster not implemented")
}

func (s *ClusterService) DeleteCluster(ctx context.Context, req *skylexv1.DeleteClusterRequest) (*skylexv1.DeleteClusterResponse, error) {
	cluster, err := s.clusters.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	if cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.GetId())
	}
	if err := s.requireClusterStoppedForDelete(ctx, cluster); err != nil {
		return nil, err
	}

	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "begin transaction: %v", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Cancel any pending commands for nodes that belong to this cluster so they
	// are not executed after the cluster no longer exists.
	_, err = tx.ExecContext(ctx,
		db.Rebind(`UPDATE agent_commands SET status = ?, error = ?, completed_at = ?
			 WHERE status = ? AND node_id IN (SELECT id FROM nodes WHERE cluster_id = ?)`),
		models.CommandStatusFailed, "cluster deleted", time.Now().UTC(),
		models.CommandStatusPending, cluster.ID,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cancel pending commands: %v", err)
	}

	// Return the cluster's nodes to the idle pool so they can be reused.
	_, err = tx.ExecContext(ctx,
		db.Rebind(`UPDATE nodes SET cluster_id = NULL, role = ?, service_location = ?, installation_state = ?, conflict_details = ?, status_detail = ?, updated_at = ?
			 WHERE cluster_id = ?`),
		"", "", "", "", "", time.Now().UTC(), cluster.ID,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "unassign cluster nodes: %v", err)
	}

	_, err = tx.ExecContext(ctx,
		db.Rebind(`DELETE FROM cluster_settings WHERE cluster_id = ?`),
		cluster.ID,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete cluster settings: %v", err)
	}

	_, err = tx.ExecContext(ctx,
		db.Rebind(`DELETE FROM clusters WHERE id = ?`),
		cluster.ID,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete cluster: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "commit transaction: %v", err)
	}

	return &skylexv1.DeleteClusterResponse{}, nil
}

func (s *ClusterService) StartCluster(ctx context.Context, req *skylexv1.StartClusterRequest) (*skylexv1.StartClusterResponse, error) {
	cluster, err := s.queueClusterLifecycle(ctx, req.GetClusterId(), "start", []provisioningCommand{{"pg_start", ""}}, models.ClusterStatusRunning, models.NodeStatusOnline, "starting")
	if err != nil {
		return nil, err
	}
	return &skylexv1.StartClusterResponse{Cluster: clusterToProto(cluster)}, nil
}

func (s *ClusterService) PauseCluster(ctx context.Context, req *skylexv1.PauseClusterRequest) (*skylexv1.PauseClusterResponse, error) {
	cluster, err := s.queueClusterLifecycle(ctx, req.GetClusterId(), "pause", []provisioningCommand{{"pg_stop", ""}}, models.ClusterStatusStopped, models.NodeStatusOffline, "stopping")
	if err != nil {
		return nil, err
	}
	return &skylexv1.PauseClusterResponse{Cluster: clusterToProto(cluster)}, nil
}

func (s *ClusterService) RestartCluster(ctx context.Context, req *skylexv1.RestartClusterRequest) (*skylexv1.RestartClusterResponse, error) {
	cluster, err := s.queueClusterLifecycle(ctx, req.GetClusterId(), "restart", []provisioningCommand{{"pg_stop", ""}, {"pg_start", ""}}, models.ClusterStatusRunning, models.NodeStatusOnline, "restarting")
	if err != nil {
		return nil, err
	}
	return &skylexv1.RestartClusterResponse{Cluster: clusterToProto(cluster)}, nil
}

func (s *ClusterService) queueClusterLifecycle(ctx context.Context, clusterID, operation string, commands []provisioningCommand, clusterStatus models.ClusterStatus, nodeStatus models.NodeStatus, nodeDetail string) (*models.Cluster, error) {
	if clusterID == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}
	if UserRoleFromContext(ctx) == models.RoleViewer {
		return nil, status.Errorf(codes.PermissionDenied, "viewer role cannot %s clusters", operation)
	}

	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "begin transaction: %v", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var cluster models.Cluster
	var labelsJSON string
	var pitrInt int
	if err = tx.QueryRowContext(ctx,
		db.Rebind(`SELECT id, name, engine, version, replication_mode, replica_count, storage_config_id, data_dir, pitr_enabled, status, labels, created_at, updated_at, service_location
			 FROM clusters WHERE id = ?`), clusterID).
		Scan(&cluster.ID, &cluster.Name, &cluster.Engine, &cluster.Version, &cluster.ReplicationMode, &cluster.Replicas,
			&cluster.StorageConfigID, &cluster.DataDir, &pitrInt, &cluster.Status, &labelsJSON, &cluster.CreatedAt, &cluster.UpdatedAt,
			&cluster.ServiceLocation); err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "cluster %q not found", clusterID)
		}
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	cluster.PITREnabled = pitrInt == 1
	cluster.Tags = unmarshalClusterLabels(labelsJSON)

	if cluster.Status == models.ClusterStatusCreating || cluster.Status == models.ClusterStatusDeleting {
		return nil, status.Errorf(codes.FailedPrecondition, "cluster %q is %s; wait for provisioning/deletion before lifecycle operations", cluster.ID, cluster.Status)
	}

	nodes, err := clusterNodesForLifecycle(ctx, tx, cluster.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list cluster nodes: %v", err)
	}
	if len(nodes) == 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "cluster %q has no assigned nodes", cluster.ID)
	}

	eligible := make([]*models.Node, 0, len(nodes))
	for _, node := range nodes {
		if !nodeEligibleForLifecycle(node) {
			continue
		}
		eligible = append(eligible, node)
	}
	if len(eligible) == 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "cluster %q has no ready nodes with connected agents for lifecycle operation", cluster.ID)
	}

	nodeIDs := make([]string, 0, len(eligible))
	for _, node := range eligible {
		nodeIDs = append(nodeIDs, node.ID)
	}
	if hasPending, err := hasPendingClusterCommands(ctx, tx, nodeIDs); err != nil {
		return nil, status.Errorf(codes.Internal, "check pending commands: %v", err)
	} else if hasPending {
		return nil, status.Error(codes.FailedPrecondition, "an agent command is already pending for this cluster; wait for it to finish before lifecycle operations")
	}

	now := time.Now().UTC()
	for _, node := range eligible {
		for _, c := range commands {
			cmdID := id.New()
			if _, err = tx.ExecContext(ctx,
				db.Rebind(`INSERT INTO agent_commands (id, agent_id, node_id, action, payload, status, created_at)
					 VALUES (?, ?, ?, ?, ?, ?, ?)`),
				cmdID, node.AgentID, node.ID, c.action, c.payload, models.CommandStatusPending, now); err != nil {
				return nil, status.Errorf(codes.Internal, "queue %s for node %s: %v", c.action, node.ID, err)
			}
		}
		if _, err = tx.ExecContext(ctx,
			db.Rebind(`UPDATE nodes SET status = ?, status_detail = ?, updated_at = ? WHERE id = ?`),
			nodeStatus, nodeDetail, now, node.ID); err != nil {
			return nil, status.Errorf(codes.Internal, "mark node lifecycle pending: %v", err)
		}
	}

	if _, err = tx.ExecContext(ctx,
		db.Rebind(`UPDATE clusters SET status = ?, updated_at = ? WHERE id = ?`),
		clusterStatus, now, cluster.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "update cluster status: %v", err)
	}
	cluster.Status = clusterStatus
	cluster.UpdatedAt = now

	if err = tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "commit lifecycle operation: %v", err)
	}
	err = nil

	s.log.Info("cluster lifecycle commands queued", "cluster_id", cluster.ID, "operation", operation, "nodes", len(eligible))
	s.auditClusterLifecycle(ctx, cluster.ID, operation, len(eligible))
	return &cluster, nil
}

func clusterNodesForLifecycle(ctx context.Context, tx *sql.Tx, clusterID string) ([]*models.Node, error) {
	rows, err := tx.QueryContext(ctx, db.Rebind(`SELECT id, cluster_id, hostname, address, port, role, status, agent_version, agent_id, labels, last_seen, created_at, updated_at, postgres_installed, postgres_version, postgres_data_initialized, status_detail, service_location, docker_available, installation_state, conflict_details, agent_latency_ms
		 FROM nodes WHERE cluster_id = ? ORDER BY role ASC, created_at ASC LIMIT ?`), clusterID, maxClusterLifecycleNodes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := make([]*models.Node, 0)
	for rows.Next() {
		node, err := scanLifecycleNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func scanLifecycleNode(rows *sql.Rows) (*models.Node, error) {
	var n models.Node
	var clusterID sql.NullString
	var labelsJSON string
	if err := rows.Scan(&n.ID, &clusterID, &n.Hostname, &n.Address, &n.Port,
		&n.Role, &n.Status, &n.AgentVersion, &n.AgentID, &labelsJSON, &n.LastSeen, &n.CreatedAt, &n.UpdatedAt,
		&n.PostgresInstalled, &n.PostgresVersion, &n.PostgresDataInitialized, &n.StatusDetail,
		&n.ServiceLocation, &n.DockerAvailable, &n.InstallationState, &n.ConflictDetails, &n.AgentLatencyMS); err != nil {
		return nil, fmt.Errorf("scan lifecycle node: %w", err)
	}
	n.ClusterID = clusterID.String
	n.Labels = unmarshalClusterLabels(labelsJSON)
	return &n, nil
}

func nodeEligibleForLifecycle(node *models.Node) bool {
	return node.AgentID != "" && node.PostgresInstalled && node.PostgresDataInitialized &&
		(node.InstallationState == models.InstallationStateInstalled || node.InstallationState == models.InstallationStateAdopted)
}

func hasPendingClusterCommands(ctx context.Context, tx *sql.Tx, nodeIDs []string) (bool, error) {
	if len(nodeIDs) == 0 {
		return false, nil
	}
	placeholders := make([]string, len(nodeIDs))
	args := make([]interface{}, 0, len(nodeIDs)+1)
	for i, id := range nodeIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, models.CommandStatusPending)
	query := fmt.Sprintf(`SELECT COUNT(*) FROM agent_commands WHERE node_id IN (%s) AND status = ?`, strings.Join(placeholders, ", "))
	var count int
	if err := tx.QueryRowContext(ctx, db.Rebind(query), args...).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *ClusterService) auditClusterLifecycle(ctx context.Context, clusterID, operation string, nodeCount int) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Log(&models.AuditLog{
		UserID:   UserIDFromContext(ctx),
		Action:   models.AuditActionLifecycleCluster,
		Resource: clusterID,
		Detail:   fmt.Sprintf("operation=%s cluster_id=%s nodes=%d", operation, clusterID, nodeCount),
	}); err != nil {
		s.log.Warn("audit cluster lifecycle failed", "cluster_id", clusterID, "operation", operation, "error", err)
	}
}

func unmarshalClusterLabels(raw string) map[string]string {
	labels := make(map[string]string)
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &labels); err != nil {
			return make(map[string]string)
		}
	}
	return labels
}

func (s *ClusterService) requireClusterStoppedForDelete(ctx context.Context, cluster *models.Cluster) error {
	nodes, _, err := s.nodes.ListByCluster(ctx, cluster.ID, 0, maxClusterLifecycleNodes)
	if err != nil {
		return status.Errorf(codes.Internal, "list cluster nodes: %v", err)
	}
	nodeIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}
	cmds, err := s.commands.ListByNodeIDs(ctx, nodeIDs, maxClusterLifecycleNodes)
	if err != nil {
		return status.Errorf(codes.Internal, "list cluster commands: %v", err)
	}
	for _, cmd := range cmds {
		if cmd.Status == models.CommandStatusPending && isLifecycleOnlyAction(cmd.Action) {
			return status.Error(codes.FailedPrecondition, "PostgreSQL lifecycle command is still pending; wait for the service to stop before deleting it")
		}
	}
	for _, node := range nodes {
		if node.Status == models.NodeStatusOnline || node.StatusDetail == "healthy" || node.StatusDetail == "running" || node.StatusDetail == "syncing_replica" {
			return status.Error(codes.FailedPrecondition, "PostgreSQL service is still running; pause/stop the cluster before deleting it")
		}
	}
	return nil
}

func (s *ClusterService) FailoverCluster(ctx context.Context, req *skylexv1.FailoverClusterRequest) (*skylexv1.FailoverClusterResponse, error) {
	if req.GetClusterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}

	cluster, err := s.clusters.GetByID(ctx, req.GetClusterId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	if cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.GetClusterId())
	}

	primary, err := s.nodes.GetPrimary(ctx, cluster.ID)
	if err != nil || primary == nil {
		return nil, status.Errorf(codes.FailedPrecondition, "no primary found for cluster %q", cluster.ID)
	}

	if s.failoverEngine == nil {
		return nil, status.Errorf(codes.Unavailable, "failover engine is not available")
	}

	go func() {
		s.failoverEngine.executeFailover(context.Background(), cluster, primary)
	}()

	s.log.Info("manual failover initiated",
		"cluster_id", cluster.ID,
		"primary_id", primary.ID,
	)

	return &skylexv1.FailoverClusterResponse{
		Cluster: clusterToProto(cluster),
	}, nil
}

func (s *ClusterService) RestartNode(ctx context.Context, req *skylexv1.RestartNodeRequest) (*skylexv1.RestartNodeResponse, error) {
	if req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}
	if UserRoleFromContext(ctx) == models.RoleViewer {
		return nil, status.Error(codes.PermissionDenied, "viewer role cannot restart nodes")
	}

	node, err := s.nodes.GetByID(ctx, req.GetNodeId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get node: %v", err)
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %q not found", req.GetNodeId())
	}

	agentID := node.AgentID
	if agentID == "" {
		return nil, status.Errorf(codes.FailedPrecondition, "node %q has no agent_id assigned", node.ID)
	}
	if !nodeEligibleForLifecycle(node) {
		return nil, status.Errorf(codes.FailedPrecondition, "node %q is not ready for PostgreSQL lifecycle operations", node.ID)
	}

	if _, err := s.commands.Create(ctx, agentID, node.ID, "pg_stop", ""); err != nil {
		return nil, status.Errorf(codes.Internal, "queue stop command: %v", err)
	}
	if _, err := s.commands.Create(ctx, agentID, node.ID, "pg_start", ""); err != nil {
		return nil, status.Errorf(codes.Internal, "queue start command: %v", err)
	}

	s.log.Info("restart node commands queued", "node_id", node.ID)

	return &skylexv1.RestartNodeResponse{}, nil
}

func (s *ClusterService) ScaleCluster(ctx context.Context, req *skylexv1.ScaleClusterRequest) (*skylexv1.ScaleClusterResponse, error) {
	if req.GetClusterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}

	cluster, err := s.clusters.GetByID(ctx, req.GetClusterId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	if cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.GetClusterId())
	}

	s.log.Info("scale cluster requested",
		"cluster_id", cluster.ID,
		"current_replicas", cluster.Replicas,
		"requested_replicas", req.GetReplicaCount(),
	)

	return &skylexv1.ScaleClusterResponse{
		Cluster: clusterToProto(cluster),
	}, nil
}

func (s *ClusterService) GetClusterSettings(ctx context.Context, req *skylexv1.GetClusterSettingsRequest) (*skylexv1.GetClusterSettingsResponse, error) {
	if req.GetClusterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}

	cluster, err := s.clusters.GetByID(ctx, req.GetClusterId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	if cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.GetClusterId())
	}

	parameters, err := s.settings.GetByClusterID(ctx, req.GetClusterId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster settings: %v", err)
	}

	return &skylexv1.GetClusterSettingsResponse{
		Settings: &skylexv1.ClusterSettings{Parameters: clusterSettingsWithDefaults(parameters)},
	}, nil
}

func (s *ClusterService) UpdateClusterSettings(ctx context.Context, req *skylexv1.UpdateClusterSettingsRequest) (*skylexv1.UpdateClusterSettingsResponse, error) {
	if req.GetClusterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}

	cluster, err := s.clusters.GetByID(ctx, req.GetClusterId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	if cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.GetClusterId())
	}

	parameters := req.GetSettings().GetParameters()
	if _, err := validateClusterSettingsParameters(parameters); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid settings: %v", err)
	}

	if err := s.settings.ReplaceAll(ctx, cluster.ID, parameters); err != nil {
		return nil, status.Errorf(codes.Internal, "persist cluster settings: %v", err)
	}

	if err := s.queueApplySettingsCommands(ctx, cluster.ID, parameters); err != nil {
		return nil, status.Errorf(codes.Internal, "queue apply settings commands: %v", err)
	}

	s.log.Info("cluster settings updated",
		"cluster_id", cluster.ID,
		"keys", len(parameters),
	)

	return &skylexv1.UpdateClusterSettingsResponse{
		Cluster: clusterToProto(cluster),
	}, nil
}

func (s *ClusterService) queueApplySettingsCommands(ctx context.Context, clusterID string, parameters map[string]string) error {
	nodes, _, err := s.nodes.ListByCluster(ctx, clusterID, 0, 1000)
	if err != nil {
		return fmt.Errorf("list cluster nodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil
	}

	payload, err := json.Marshal(parameters)
	if err != nil {
		return fmt.Errorf("marshal settings payload: %w", err)
	}

	for _, node := range nodes {
		if node.AgentID == "" {
			continue
		}
		if _, err := s.commands.Create(ctx, node.AgentID, node.ID, "pg_apply_settings", string(payload)); err != nil {
			return fmt.Errorf("queue apply settings for node %s: %w", node.ID, err)
		}
	}
	return nil
}

func clusterToProto(c *models.Cluster) *skylexv1.Cluster {
	var engine skylexv1.Engine
	switch c.Engine {
	case models.EnginePostgreSQL:
		engine = skylexv1.Engine_ENGINE_POSTGRESQL
	}

	var mode skylexv1.ReplicationMode
	switch c.ReplicationMode {
	case models.ReplicationSync:
		mode = skylexv1.ReplicationMode_REPLICATION_MODE_SYNC
	case models.ReplicationAsync:
		mode = skylexv1.ReplicationMode_REPLICATION_MODE_ASYNC
	}

	var clusterStatus skylexv1.ClusterStatus
	switch c.Status {
	case models.ClusterStatusCreating:
		clusterStatus = skylexv1.ClusterStatus_CLUSTER_STATUS_CREATING
	case models.ClusterStatusRunning:
		clusterStatus = skylexv1.ClusterStatus_CLUSTER_STATUS_HEALTHY
	case models.ClusterStatusDegraded:
		clusterStatus = skylexv1.ClusterStatus_CLUSTER_STATUS_DEGRADED
	case models.ClusterStatusDeleting:
		clusterStatus = skylexv1.ClusterStatus_CLUSTER_STATUS_DELETING
	case models.ClusterStatusStopped:
		clusterStatus = skylexv1.ClusterStatus_CLUSTER_STATUS_STOPPED
	}

	serviceLocation := protoServiceLocation(c.ServiceLocation)

	return &skylexv1.Cluster{
		Id:   c.ID,
		Name: c.Name,
		Config: &skylexv1.ClusterConfig{
			Engine:          engine,
			Version:         c.Version,
			ReplicationMode: mode,
			ReplicaCount:    int32(c.Replicas),
			StorageConfigId: c.StorageConfigID,
			PitrEnabled:     c.PITREnabled,
			Labels:          c.Tags,
			ServiceLocation: serviceLocation,
		},
		Status:          clusterStatus,
		ServiceLocation: serviceLocation,
		CreatedAt:       timestamppb.New(c.CreatedAt),
		UpdatedAt:       timestamppb.New(c.UpdatedAt),
	}
}

func convertEngine(e skylexv1.Engine) models.EngineType {
	switch e {
	case skylexv1.Engine_ENGINE_POSTGRESQL:
		return models.EnginePostgreSQL
	default:
		return models.EngineType("")
	}
}

func convertReplicationMode(mode skylexv1.ReplicationMode) models.ReplicationMode {
	switch mode {
	case skylexv1.ReplicationMode_REPLICATION_MODE_SYNC:
		return models.ReplicationSync
	case skylexv1.ReplicationMode_REPLICATION_MODE_ASYNC:
		fallthrough
	default:
		return models.ReplicationAsync
	}
}

func convertServiceLocation(loc skylexv1.ServiceLocation) models.ServiceLocation {
	switch loc {
	case skylexv1.ServiceLocation_SERVICE_LOCATION_DOCKER:
		return models.ServiceLocationDocker
	case skylexv1.ServiceLocation_SERVICE_LOCATION_NATIVE:
		return models.ServiceLocationNative
	default:
		return models.ServiceLocationUnspecified
	}
}

func protoServiceLocation(loc models.ServiceLocation) skylexv1.ServiceLocation {
	switch loc {
	case models.ServiceLocationDocker:
		return skylexv1.ServiceLocation_SERVICE_LOCATION_DOCKER
	case models.ServiceLocationNative:
		return skylexv1.ServiceLocation_SERVICE_LOCATION_NATIVE
	default:
		return skylexv1.ServiceLocation_SERVICE_LOCATION_UNSPECIFIED
	}
}

type provisioningCommand struct{ action, payload string }

func (s *ClusterService) queuePrimaryCommands(ctx context.Context, node *models.Node, version string, serviceLocation models.ServiceLocation) error {
	commands := installCommands(node, version, serviceLocation, false, node.ClusterID)
	if serviceLocation != models.ServiceLocationNative {
		commands = append(commands, primaryCommands()...)
	}
	return s.queueNodeCommands(ctx, node, commands)
}

func (s *ClusterService) queueReplicaCommands(ctx context.Context, replica, primary *models.Node, version string, serviceLocation models.ServiceLocation) error {
	commands := installCommands(replica, version, serviceLocation, false, replica.ClusterID)
	if serviceLocation != models.ServiceLocationNative {
		commands = append(commands, replicaCommands(primary)...)
	}
	return s.queueNodeCommands(ctx, replica, commands)
}

func (s *ClusterService) queueNodeCommands(ctx context.Context, node *models.Node, commands []provisioningCommand) error {
	for _, c := range commands {
		if _, err := s.commands.Create(ctx, node.AgentID, node.ID, c.action, c.payload); err != nil {
			return fmt.Errorf("queue %s: %w", c.action, err)
		}
	}
	return nil
}

func primaryCommands() []provisioningCommand {
	return []provisioningCommand{
		{"pg_init", ""},
		{"pg_start", ""},
		{"pg_create_repl_user", ""},
	}
}

func replicaCommands(primary *models.Node) []provisioningCommand {
	payload := fmt.Sprintf("%s:%d", nodeAddress(primary), primary.Port)
	return []provisioningCommand{
		{"pg_basebackup", payload},
		{"repoint_replica", payload},
		{"pg_start", ""},
	}
}

func installCommands(node *models.Node, version string, serviceLocation models.ServiceLocation, resolvedNativeConflict bool, clusterID string) []provisioningCommand {
	if serviceLocation == models.ServiceLocationNative && !resolvedNativeConflict {
		return []provisioningCommand{{"pg_preflight", ""}}
	}
	commands := []provisioningCommand{}
	if serviceLocation == models.ServiceLocationDocker {
		payload, _ := json.Marshal(map[string]string{
			"cluster_id": clusterID,
			"version":    version,
		})
		commands = append(commands, provisioningCommand{"pg_install_docker", string(payload)})
		return commands
	}
	if !node.PostgresInstalled {
		commands = append(commands, provisioningCommand{"pg_install_native", version})
	}
	return commands
}

func nodeAddress(n *models.Node) string {
	if n.Address != "" {
		return n.Address
	}
	return n.Hostname
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
