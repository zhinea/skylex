package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type NodeService struct {
	skylexv1.UnimplementedNodeServiceServer
	nodes          *db.NodeRepository
	clusters       *db.ClusterRepository
	commands       *db.AgentCommandRepository
	commandSecrets *db.AgentCommandSecretRepository
	commandLogs    *db.CommandLogRepository
	statusTTL      time.Duration
	validate       *validator.Validate
	log            *slog.Logger
}

func NewNodeService(nodes *db.NodeRepository, clusters *db.ClusterRepository, commands *db.AgentCommandRepository, commandLogs *db.CommandLogRepository, statusTTL time.Duration, log *slog.Logger) *NodeService {
	validate := validator.New()
	_ = validate.RegisterValidation("pgrole", func(fl validator.FieldLevel) bool {
		return roleNamePattern.MatchString(fl.Field().String())
	})
	return &NodeService{nodes: nodes, clusters: clusters, commands: commands, commandLogs: commandLogs, statusTTL: statusTTL, validate: validate, log: log}
}

func (s *NodeService) SetCommandSecretRepository(repo *db.AgentCommandSecretRepository) {
	s.commandSecrets = repo
}

func (s *NodeService) ListNodes(ctx context.Context, req *skylexv1.ListNodesRequest) (*skylexv1.ListNodesResponse, error) {
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

	nodes, total, err := s.nodes.ListByCluster(ctx, req.GetClusterId(), offset, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list nodes: %v", err)
	}

	var protoNodes []*skylexv1.Node
	for _, n := range nodes {
		protoNodes = append(protoNodes, s.nodeToProto(n))
	}

	return &skylexv1.ListNodesResponse{
		Nodes: protoNodes,
		Pagination: &skylexv1.Pagination{
			Page:     int32(page),
			PageSize: int32(pageSize),
			Total:    int32(total),
		},
	}, nil
}

func (s *NodeService) GetNode(ctx context.Context, req *skylexv1.GetNodeRequest) (*skylexv1.GetNodeResponse, error) {
	node, err := s.nodes.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get node: %v", err)
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %q not found", req.GetId())
	}

	return &skylexv1.GetNodeResponse{
		Node: s.nodeToProto(node),
	}, nil
}

func (s *NodeService) ListNodeMetrics(ctx context.Context, req *skylexv1.ListNodeMetricsRequest) (*skylexv1.ListNodeMetricsResponse, error) {
	if req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 300
	}
	if limit > 1000 {
		limit = 1000
	}

	var since time.Time
	if req.GetSince() != nil {
		since = req.GetSince().AsTime()
	}
	metrics, err := s.nodes.ListMetrics(ctx, req.GetNodeId(), since, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list node metrics: %v", err)
	}
	protoMetrics := make([]*skylexv1.NodeMetric, 0, len(metrics))
	for _, m := range metrics {
		protoMetrics = append(protoMetrics, nodeMetricToProto(m))
	}
	return &skylexv1.ListNodeMetricsResponse{Metrics: protoMetrics}, nil
}

func (s *NodeService) DrainNode(ctx context.Context, req *skylexv1.DrainNodeRequest) (*skylexv1.DrainNodeResponse, error) {
	node, err := s.nodes.GetByID(ctx, req.GetNodeId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get node: %v", err)
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %q not found", req.GetNodeId())
	}

	// Drain stops the PostgreSQL workload and marks the node drained, but
	// always keeps the node registered. Deletion is a separate, explicit
	// action (DeleteNode), so the agent keeps a valid identity and does not
	// fall into an "unknown agent" retry loop. This is uniform for both
	// unassigned nodes and cluster members; the latter keeps cluster topology
	// intact.
	if err := s.nodes.UpdateStatus(ctx, node.ID, models.NodeStatusDrained); err != nil {
		return nil, status.Errorf(codes.Internal, "update node status: %v", err)
	}
	if err := s.nodes.UpdateStatusDetail(ctx, node.ID, "drained"); err != nil {
		s.log.Warn("update node status detail for drain", "error", err, "node_id", node.ID)
	}

	if node.AgentID != "" {
		if _, err := s.commands.Create(ctx, node.AgentID, node.ID, "pg_stop", ""); err != nil {
			s.log.Warn("queue stop command for drain", "error", err)
		}
	}

	node.Status = models.NodeStatusDrained
	node.StatusDetail = "drained"

	s.log.Info("node drained", "node_id", node.ID)
	return &skylexv1.DrainNodeResponse{Node: s.nodeToProto(node)}, nil
}

func (s *NodeService) RejoinNode(ctx context.Context, req *skylexv1.RejoinNodeRequest) (*skylexv1.RejoinNodeResponse, error) {
	node, err := s.nodes.GetByID(ctx, req.GetNodeId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get node: %v", err)
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %q not found", req.GetNodeId())
	}

	primary, err := s.nodes.GetPrimary(ctx, node.ClusterID)
	if err != nil || primary == nil {
		return nil, status.Errorf(codes.FailedPrecondition, "no primary found for cluster %q", node.ClusterID)
	}

	if node.AgentID != "" {
		payload := fmt.Sprintf("%s:%d", primary.Address, primary.Port)
		if _, err := s.commands.Create(ctx, node.AgentID, node.ID, "repoint_replica", payload); err != nil {
			s.log.Warn("queue repoint command for rejoin", "error", err)
		}
		if _, err := s.commands.Create(ctx, node.AgentID, node.ID, "pg_start", ""); err != nil {
			s.log.Warn("queue start command for rejoin", "error", err)
		}
	}

	if err := s.nodes.UpdateStatus(ctx, node.ID, models.NodeStatusOnline); err != nil {
		s.log.Warn("update node status for rejoin", "error", err)
	}
	if err := s.nodes.UpdateStatusDetail(ctx, node.ID, "rejoining"); err != nil {
		s.log.Warn("update node status detail for rejoin", "error", err, "node_id", node.ID)
	}
	node.Status = models.NodeStatusOnline
	node.StatusDetail = "rejoining"

	s.log.Info("node rejoined", "node_id", node.ID)
	return &skylexv1.RejoinNodeResponse{Node: s.nodeToProto(node)}, nil
}

func (s *NodeService) DeleteNode(ctx context.Context, req *skylexv1.DeleteNodeRequest) (*skylexv1.DeleteNodeResponse, error) {
	node, err := s.nodes.GetByID(ctx, req.GetNodeId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get node: %v", err)
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %q not found", req.GetNodeId())
	}
	if node.ClusterID != "" {
		return nil, status.Errorf(codes.FailedPrecondition, "node %q belongs to a cluster; drain or remove it from the cluster before deleting", node.ID)
	}

	if node.AgentID != "" {
		if _, err := s.commands.Create(ctx, node.AgentID, node.ID, "agent_deactivate", ""); err != nil {
			return nil, status.Errorf(codes.Internal, "queue agent deactivation: %v", err)
		}
		if err := s.nodes.UpdateStatus(ctx, node.ID, models.NodeStatusDeleting); err != nil {
			return nil, status.Errorf(codes.Internal, "mark node deleting: %v", err)
		}
		if err := s.nodes.UpdateStatusDetail(ctx, node.ID, "deactivating_agent"); err != nil {
			s.log.Warn("update node status detail for delete", "error", err, "node_id", node.ID)
		}
		s.log.Info("node deletion pending agent deactivation", "node_id", node.ID)
		return &skylexv1.DeleteNodeResponse{}, nil
	}

	if err := s.nodes.Delete(ctx, node.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "delete node: %v", err)
	}

	s.log.Info("node deleted", "node_id", node.ID)
	return &skylexv1.DeleteNodeResponse{}, nil
}

func (s *NodeService) ResolveInstallationConflict(ctx context.Context, req *skylexv1.ResolveInstallationConflictRequest) (*skylexv1.ResolveInstallationConflictResponse, error) {
	if req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}
	node, err := s.nodes.GetByID(ctx, req.GetNodeId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get node: %v", err)
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %q not found", req.GetNodeId())
	}
	if node.ClusterID == "" {
		return nil, status.Error(codes.FailedPrecondition, "node is not assigned to a cluster")
	}
	if node.ServiceLocation != models.ServiceLocationNative {
		return nil, status.Error(codes.FailedPrecondition, "installation conflicts are only resolved for native nodes")
	}
	if node.InstallationState != models.InstallationStateConflict {
		return nil, status.Errorf(codes.FailedPrecondition, "node is not in installation conflict state")
	}

	cluster, err := s.clusters.GetByID(ctx, node.ClusterID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	if cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", node.ClusterID)
	}
	if cluster.Status != models.ClusterStatusCreating {
		return nil, status.Errorf(codes.FailedPrecondition, "cluster is not creating")
	}
	if node.AgentID == "" {
		return nil, status.Errorf(codes.FailedPrecondition, "node %q has no agent_id assigned", node.ID)
	}

	var adoptCreds *nativeAdoptCredentials
	switch req.GetAction() {
	case skylexv1.ResolveInstallationConflictAction_RESOLVE_INSTALLATION_CONFLICT_ACTION_ADOPT:
		creds, err := s.nativeAdoptCredentials(req)
		if err != nil {
			return nil, err
		}
		adoptCreds = creds
		if err := s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateInstalling, ""); err != nil {
			return nil, status.Errorf(codes.Internal, "update installation state: %v", err)
		}
		if err := s.queueAdoptCommands(ctx, node, adoptCreds); err != nil {
			return nil, status.Errorf(codes.Internal, "queue adopt commands: %v", err)
		}
	case skylexv1.ResolveInstallationConflictAction_RESOLVE_INSTALLATION_CONFLICT_ACTION_PURGE:
		if err := s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateInstalling, ""); err != nil {
			return nil, status.Errorf(codes.Internal, "update installation state: %v", err)
		}
		if err := s.queuePurgeCommands(ctx, node, cluster.Version); err != nil {
			return nil, status.Errorf(codes.Internal, "queue purge commands: %v", err)
		}
	case skylexv1.ResolveInstallationConflictAction_RESOLVE_INSTALLATION_CONFLICT_ACTION_ABORT:
		if err := s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateFailed, "cluster creation aborted by user"); err != nil {
			return nil, status.Errorf(codes.Internal, "update installation state: %v", err)
		}
		if err := s.clusters.UpdateStatus(ctx, node.ClusterID, models.ClusterStatusStopped); err != nil {
			return nil, status.Errorf(codes.Internal, "mark cluster failed: %v", err)
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "action must be ADOPT, PURGE, or ABORT")
	}

	updated, err := s.nodes.GetByID(ctx, node.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get updated node: %v", err)
	}
	s.log.Info("installation conflict resolved", "node_id", node.ID, "action", req.GetAction().String())
	return &skylexv1.ResolveInstallationConflictResponse{Node: s.nodeToProto(updated)}, nil
}

type nativeAdoptCredentials struct {
	AdminUser     string `validate:"required,pgrole"`
	AdminPassword string `validate:"required"`
}

func (s *NodeService) nativeAdoptCredentials(req *skylexv1.ResolveInstallationConflictRequest) (*nativeAdoptCredentials, error) {
	creds := &nativeAdoptCredentials{
		AdminUser:     strings.TrimSpace(req.GetPostgresAdminUser()),
		AdminPassword: req.GetPostgresAdminPassword(),
	}
	if err := s.validate.Struct(creds); err != nil {
		return nil, status.Error(codes.InvalidArgument, "postgres_admin_user and postgres_admin_password are required for ADOPT")
	}
	return creds, nil
}

func (s *NodeService) nativeAdoptCredentialPayload(creds *nativeAdoptCredentials) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"postgres_admin_user": creds.AdminUser,
		"password_secret_key": "postgres_admin_password",
	})
	if err != nil {
		return "", fmt.Errorf("marshal adopt credentials payload: %w", err)
	}
	return string(payload), nil
}

func (s *NodeService) queueAdoptCommands(ctx context.Context, node *models.Node, creds *nativeAdoptCredentials) error {
	payload, err := s.nativeAdoptCredentialPayload(creds)
	if err != nil {
		return err
	}
	commands := []provisioningCommand{{"pg_adopt_native", payload}}
	if nativeConflictDataInitialized(node.ConflictDetails) {
		if node.Role == models.NodeRolePrimary {
			commands = append(commands, provisioningCommand{"pg_create_repl_user", payload})
		}
		return s.queueNodeCommands(ctx, node, commands, creds)
	}
	if node.Role == models.NodeRolePrimary {
		commands = append(commands, primaryCommands()...)
	} else {
		primary, err := s.nodes.GetPrimary(ctx, node.ClusterID)
		if err != nil {
			return fmt.Errorf("get primary: %w", err)
		}
		if primary == nil {
			return fmt.Errorf("no primary found for cluster %q", node.ClusterID)
		}
		commands = append(commands, replicaCommands(primary)...)
	}
	return s.queueNodeCommands(ctx, node, commands, creds)
}

func (s *NodeService) queuePurgeCommands(ctx context.Context, node *models.Node, version string) error {
	commands := []provisioningCommand{{"pg_purge_native", ""}}
	installNode := *node
	installNode.PostgresInstalled = false
	commands = append(commands, installCommands(&installNode, version, models.ServiceLocationNative, true, node.ClusterID)...)
	if node.Role == models.NodeRolePrimary {
		commands = append(commands, primaryCommands()...)
	} else {
		primary, err := s.nodes.GetPrimary(ctx, node.ClusterID)
		if err != nil {
			return fmt.Errorf("get primary: %w", err)
		}
		if primary == nil {
			return fmt.Errorf("no primary found for cluster %q", node.ClusterID)
		}
		commands = append(commands, replicaCommands(primary)...)
	}
	return s.queueNodeCommands(ctx, node, commands, nil)
}

func (s *NodeService) queueNodeCommands(ctx context.Context, node *models.Node, commands []provisioningCommand, adoptCreds *nativeAdoptCredentials) error {
	for _, c := range commands {
		if adoptCreds != nil && (c.action == "pg_adopt_native" || c.action == "pg_create_repl_user") {
			if s.commandSecrets == nil {
				return fmt.Errorf("command secret repository is not configured")
			}
			secretExpiresAt := time.Now().UTC().Add(24 * time.Hour)
			if _, err := s.commandSecrets.CreateCommandWithSecrets(ctx, node.AgentID, node.ID, c.action, c.payload, map[string]string{
				"postgres_admin_password": adoptCreds.AdminPassword,
			}, &secretExpiresAt); err != nil {
				return fmt.Errorf("queue %s with credentials: %w", c.action, err)
			}
			continue
		}
		if _, err := s.commands.Create(ctx, node.AgentID, node.ID, c.action, c.payload); err != nil {
			return fmt.Errorf("queue %s: %w", c.action, err)
		}
	}
	return nil
}

func (s *NodeService) ListNodeCommandLogs(ctx context.Context, req *skylexv1.ListNodeCommandLogsRequest) (*skylexv1.ListNodeCommandLogsResponse, error) {
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	var (
		logs []*db.CommandLog
		err  error
	)

	switch {
	case req.GetCommandId() != "":
		logs, err = s.commandLogs.ListByCommandID(ctx, req.GetCommandId(), pageSize, offset)
	case req.GetNodeId() != "":
		logs, err = s.commandLogs.ListByNodeID(ctx, req.GetNodeId(), pageSize, offset)
	case req.GetClusterId() != "":
		logs, err = s.commandLogs.ListByClusterID(ctx, req.GetClusterId(), pageSize, offset)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "cluster_id, node_id, or command_id is required")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list command logs: %v", err)
	}

	hostnameMap := make(map[string]string)
	protoLogs := make([]*skylexv1.CommandLog, 0, len(logs))
	for _, l := range logs {
		nodeID := ""
		cmd, _ := s.commands.GetByID(ctx, l.CommandID)
		if cmd != nil {
			nodeID = cmd.NodeID
		}

		hostname := ""
		if nodeID != "" {
			if h, ok := hostnameMap[nodeID]; ok {
				hostname = h
			} else {
				if h, err := s.nodes.GetHostnameByID(ctx, nodeID); err == nil && h != "" {
					hostname = h
					hostnameMap[nodeID] = hostname
				}
			}
		}

		protoLogs = append(protoLogs, &skylexv1.CommandLog{
			Id:          l.ID,
			CommandId:   l.CommandID,
			NodeId:      nodeID,
			Hostname:    hostname,
			Level:       l.Level,
			Message:     l.Message,
			TimestampMs: l.CreatedAt.UnixMilli(),
		})
	}

	return &skylexv1.ListNodeCommandLogsResponse{
		Logs: protoLogs,
		Pagination: &skylexv1.Pagination{
			Page:     int32(page),
			PageSize: int32(pageSize),
			Total:    int32(len(protoLogs)),
		},
	}, nil
}

func (s *NodeService) nodeToProto(n *models.Node) *skylexv1.Node {
	applyAgentConnectionStatus(n, s.statusTTL)

	var role skylexv1.NodeRole
	switch n.Role {
	case models.NodeRolePrimary:
		role = skylexv1.NodeRole_NODE_ROLE_PRIMARY
	case models.NodeRoleReplica:
		role = skylexv1.NodeRole_NODE_ROLE_REPLICA
	}

	var serviceLocation skylexv1.ServiceLocation
	switch n.ServiceLocation {
	case models.ServiceLocationDocker:
		serviceLocation = skylexv1.ServiceLocation_SERVICE_LOCATION_DOCKER
	case models.ServiceLocationNative:
		serviceLocation = skylexv1.ServiceLocation_SERVICE_LOCATION_NATIVE
	default:
		serviceLocation = skylexv1.ServiceLocation_SERVICE_LOCATION_UNSPECIFIED
	}

	metric := n.LatestMetrics
	if metric == nil {
		metric = &models.NodeMetric{}
	}
	postgresInstalled := effectivePostgresInstalled(n)
	postgresDataInitialized := effectivePostgresDataInitialized(n)
	statusDetail := effectiveStatusDetail(n, postgresInstalled, postgresDataInitialized)
	return &skylexv1.Node{
		Id:                      n.ID,
		ClusterId:               n.ClusterID,
		Hostname:                n.Hostname,
		Role:                    role,
		Address:                 n.Address,
		Port:                    int32(n.Port),
		Labels:                  n.Labels,
		AgentVersion:            n.AgentVersion,
		LastSeen:                timestamppb.New(n.LastSeen),
		CreatedAt:               timestamppb.New(n.CreatedAt),
		UpdatedAt:               timestamppb.New(n.UpdatedAt),
		Status:                  string(n.Status),
		PostgresInstalled:       postgresInstalled,
		PostgresVersion:         n.PostgresVersion,
		PostgresDataInitialized: postgresDataInitialized,
		StatusDetail:            statusDetail,
		ServiceLocation:         serviceLocation,
		DockerAvailable:         n.DockerAvailable,
		InstallationState:       protoInstallationState(n.InstallationState),
		ConflictDetails:         n.ConflictDetails,
		AgentConnected:          n.AgentConnected,
		AgentLatencyMs:          n.AgentLatencyMS,
		Os:                      metric.OS,
		Platform:                metric.Platform,
		PlatformVersion:         metric.PlatformVersion,
		KernelVersion:           metric.KernelVersion,
		Architecture:            metric.Architecture,
		CpuCores:                int32(metric.CPUCores),
		CpuUsagePercent:         metric.CPUUsagePercent,
		LoadAverage_1M:          metric.LoadAverage1M,
		LoadAverage_5M:          metric.LoadAverage5M,
		LoadAverage_15M:         metric.LoadAverage15M,
		MemoryTotalBytes:        metric.MemoryTotalBytes,
		MemoryUsedBytes:         metric.MemoryUsedBytes,
		MemoryAvailableBytes:    metric.MemoryAvailableBytes,
		MemoryUsagePercent:      metric.MemoryUsagePercent,
		DiskTotalBytes:          metric.DiskTotalBytes,
		DiskUsedBytes:           metric.DiskUsedBytes,
		DiskAvailableBytes:      metric.DiskAvailableBytes,
		DiskUsagePercent:        metric.DiskUsagePercent,
		UptimeSeconds:           metric.UptimeSeconds,
		LatestMetric:            nodeMetricToProto(n.LatestMetrics),
	}
}

func effectivePostgresInstalled(n *models.Node) bool {
	return n.PostgresInstalled || n.InstallationState == models.InstallationStateInstalled || n.InstallationState == models.InstallationStateAdopted
}

func effectivePostgresDataInitialized(n *models.Node) bool {
	return n.PostgresDataInitialized
}

func effectiveStatusDetail(n *models.Node, postgresInstalled, postgresDataInitialized bool) string {
	if n.StatusDetail != "waiting_for_postgres" && n.StatusDetail != "initializing_data_directory" {
		return n.StatusDetail
	}
	if !postgresInstalled {
		return n.StatusDetail
	}
	if !postgresDataInitialized {
		return "initializing_data_directory"
	}
	if n.Status == models.NodeStatusOnline {
		if n.Role == models.NodeRoleReplica {
			return "syncing_replica"
		}
		return "healthy"
	}
	return "stopped"
}

func dockerProvisioningInstalled(n *models.Node) bool {
	return n.ServiceLocation == models.ServiceLocationDocker && (n.InstallationState == models.InstallationStateInstalled || n.InstallationState == models.InstallationStateAdopted)
}

func nodeMetricToProto(m *models.NodeMetric) *skylexv1.NodeMetric {
	if m == nil {
		return nil
	}
	return &skylexv1.NodeMetric{
		Id:                   m.ID,
		NodeId:               m.NodeID,
		RecordedAt:           timestamppb.New(m.RecordedAt),
		Os:                   m.OS,
		Platform:             m.Platform,
		PlatformVersion:      m.PlatformVersion,
		KernelVersion:        m.KernelVersion,
		Architecture:         m.Architecture,
		CpuCores:             int32(m.CPUCores),
		CpuUsagePercent:      m.CPUUsagePercent,
		LoadAverage_1M:       m.LoadAverage1M,
		LoadAverage_5M:       m.LoadAverage5M,
		LoadAverage_15M:      m.LoadAverage15M,
		MemoryTotalBytes:     m.MemoryTotalBytes,
		MemoryUsedBytes:      m.MemoryUsedBytes,
		MemoryAvailableBytes: m.MemoryAvailableBytes,
		MemoryUsagePercent:   m.MemoryUsagePercent,
		DiskTotalBytes:       m.DiskTotalBytes,
		DiskUsedBytes:        m.DiskUsedBytes,
		DiskAvailableBytes:   m.DiskAvailableBytes,
		DiskUsagePercent:     m.DiskUsagePercent,
		UptimeSeconds:        m.UptimeSeconds,
	}
}

func applyAgentConnectionStatus(n *models.Node, statusTTL time.Duration) {
	if statusTTL <= 0 {
		statusTTL = 30 * time.Second
	}
	if n.LastSeen.IsZero() {
		n.AgentConnected = false
		n.AgentLatencyMS = 0
		return
	}

	latency := time.Since(n.LastSeen)
	if latency < 0 {
		latency = 0
	}
	n.AgentConnected = latency <= statusTTL
	if !n.AgentConnected || n.AgentLatencyMS < 0 {
		n.AgentLatencyMS = 0
	}
}

func protoInstallationState(state models.InstallationState) skylexv1.InstallationState {
	switch state {
	case models.InstallationStatePendingPreflight:
		return skylexv1.InstallationState_INSTALLATION_STATE_PENDING_PREFLIGHT
	case models.InstallationStateNothingFound:
		return skylexv1.InstallationState_INSTALLATION_STATE_NOTHING_FOUND
	case models.InstallationStateConflict:
		return skylexv1.InstallationState_INSTALLATION_STATE_CONFLICT
	case models.InstallationStateInstalling:
		return skylexv1.InstallationState_INSTALLATION_STATE_INSTALLING
	case models.InstallationStateInstalled:
		return skylexv1.InstallationState_INSTALLATION_STATE_INSTALLED
	case models.InstallationStateFailed:
		return skylexv1.InstallationState_INSTALLATION_STATE_FAILED
	case models.InstallationStateAdopted:
		return skylexv1.InstallationState_INSTALLATION_STATE_ADOPTED
	default:
		return skylexv1.InstallationState_INSTALLATION_STATE_UNSPECIFIED
	}
}
