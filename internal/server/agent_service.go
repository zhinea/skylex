package server

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AgentService struct {
	skylexv1.UnimplementedAgentServiceServer
	cfg            *Config
	clusters       *db.ClusterRepository
	nodes          *db.NodeRepository
	commands       *db.AgentCommandRepository
	commandLogs    *db.CommandLogRepository
	agentTokenRepo *db.AgentTokenRepository
	commandSecrets *db.AgentCommandSecretRepository
	postgresRoles  *db.PostgresRoleRepository
	postgresDBs    *db.PostgresDatabaseRepository
	postgresAccess *db.PostgresAccessRepository
	log            *slog.Logger
}

func NewAgentService(cfg *Config, clusters *db.ClusterRepository, nodes *db.NodeRepository, commands *db.AgentCommandRepository, commandLogs *db.CommandLogRepository, agentTokenRepo *db.AgentTokenRepository, log *slog.Logger) *AgentService {
	return &AgentService{
		cfg:            cfg,
		clusters:       clusters,
		nodes:          nodes,
		commands:       commands,
		commandLogs:    commandLogs,
		agentTokenRepo: agentTokenRepo,
		log:            log,
	}
}

func (s *AgentService) SetCommandSecretRepository(repo *db.AgentCommandSecretRepository) {
	s.commandSecrets = repo
}

func (s *AgentService) SetPostgresRoleRepository(repo *db.PostgresRoleRepository) {
	s.postgresRoles = repo
}

func (s *AgentService) SetPostgresDatabaseRepository(repo *db.PostgresDatabaseRepository) {
	s.postgresDBs = repo
}

func (s *AgentService) SetPostgresAccessRepository(repo *db.PostgresAccessRepository) {
	s.postgresAccess = repo
}

func (s *AgentService) validateAgentToken(token string) (bool, error) {
	if token == "" {
		return false, nil
	}

	hash := crypto.HashToken(token)
	storedToken, err := s.agentTokenRepo.GetByTokenHash(hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if storedToken != nil {
		if storedToken.ExpiresAt == nil || storedToken.ExpiresAt.After(time.Now()) {
			return true, nil
		}
	}

	// Optional dev-token fallback when no stored token matched. This is only
	// enabled when agent_token is explicitly configured in the server config.
	if s.cfg.Agent.AgentToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.Agent.AgentToken)) == 1 {
		return true, nil
	}

	return false, nil
}

func (s *AgentService) RegisterAgent(ctx context.Context, req *skylexv1.RegisterAgentRequest) (*skylexv1.RegisterAgentResponse, error) {
	if req.AgentToken == "" {
		return nil, status.Error(codes.Unauthenticated, "agent token is required")
	}

	valid, err := s.validateAgentToken(req.AgentToken)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "validate agent token: %v", err)
	}
	if !valid {
		return nil, status.Error(codes.Unauthenticated, "invalid or revoked agent token")
	}

	agentID, err := crypto.GenerateToken(16)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate agent id: %v", err)
	}

	caps := req.GetCapabilities()

	node, _ := s.nodes.GetByHostname(ctx, req.GetHostname())
	if node != nil {
		if err := s.nodes.UpdateAgentID(ctx, node.ID, agentID); err != nil {
			s.log.Warn("update node agent_id", "error", err, "node_id", node.ID)
		}
		if err := s.nodes.UpdateHeartbeat(ctx, node.ID, models.NodeStatusOnline, 0); err != nil {
			s.log.Warn("update node heartbeat (register)", "error", err, "node_id", node.ID)
		}
		// Persist capabilities reported at registration time.
		if caps != nil {
			if err := s.nodes.UpdatePostgresStatus(ctx, node.ID,
				caps.GetPostgresAvailable(),
				caps.GetPostgresVersion(),
				false, // data-dir initialization is reported via ReportStatus
			); err != nil {
				s.log.Warn("update node postgres status (register)", "error", err, "node_id", node.ID)
			}
			if err := s.nodes.UpdateDockerAvailable(ctx, node.ID, caps.GetDockerAvailable()); err != nil {
				s.log.Warn("update node docker_available (register)", "error", err, "node_id", node.ID)
			}
		}
		s.log.Info("agent linked to existing node",
			"agent_id", agentID,
			"node_id", node.ID,
			"hostname", req.GetHostname(),
		)
	} else {
		node, err := s.nodes.Create(ctx, "", req.GetHostname(), req.GetAddress(), int(req.GetPort()),
			models.NodeRoleReplica, req.GetAgentVersion(), req.GetLabels())
		if err != nil {
			s.log.Warn("create node for agent", "error", err)
		} else {
			if err := s.nodes.UpdateAgentID(ctx, node.ID, agentID); err != nil {
				s.log.Warn("set agent_id on new node", "error", err)
			}
			if err := s.nodes.UpdateHeartbeat(ctx, node.ID, models.NodeStatusOnline, 0); err != nil {
				s.log.Warn("update node heartbeat (register new)", "error", err, "node_id", node.ID)
			}
			if caps != nil {
				if err := s.nodes.UpdatePostgresStatus(ctx, node.ID,
					caps.GetPostgresAvailable(),
					caps.GetPostgresVersion(),
					false,
				); err != nil {
					s.log.Warn("update node postgres status (register new)", "error", err, "node_id", node.ID)
				}
				if err := s.nodes.UpdateDockerAvailable(ctx, node.ID, caps.GetDockerAvailable()); err != nil {
					s.log.Warn("update node docker_available (register new)", "error", err, "node_id", node.ID)
				}
			}
			s.log.Info("agent registered with new node",
				"agent_id", agentID,
				"node_id", node.ID,
				"hostname", req.GetHostname(),
			)
		}
	}

	return &skylexv1.RegisterAgentResponse{
		AgentId: agentID,
	}, nil
}

func (s *AgentService) Heartbeat(ctx context.Context, req *skylexv1.HeartbeatRequest) (*skylexv1.HeartbeatResponse, error) {
	node, err := s.nodeForAgent(ctx, req.GetAgentId())
	if err != nil {
		return nil, err
	}
	if node.Status == models.NodeStatusDrained || node.Status == models.NodeStatusDeleting {
		return &skylexv1.HeartbeatResponse{}, nil
	}
	if err := s.nodes.UpdateHeartbeat(ctx, node.ID, models.NodeStatusOnline, normalizeLatencyMS(req.GetObservedLatencyMs())); err != nil {
		s.log.Warn("heartbeat update failed", "node_id", node.ID, "error", err)
	}

	return &skylexv1.HeartbeatResponse{}, nil
}

func normalizeLatencyMS(latency int64) int64 {
	if latency < 0 {
		return 0
	}
	return latency
}

func (s *AgentService) ReportStatus(ctx context.Context, req *skylexv1.ReportStatusRequest) (*skylexv1.ReportStatusResponse, error) {
	agentNode, err := s.nodeForAgent(ctx, req.GetAgentId())
	if err != nil {
		return nil, err
	}

	for _, nodeStatus := range req.GetNodeStatuses() {
		if nodeStatus.GetNodeId() != "" && nodeStatus.GetNodeId() != agentNode.ID {
			s.log.Warn("agent attempted to report status for another node", "agent_id", req.GetAgentId(), "node_id", nodeStatus.GetNodeId())
			continue
		}
		node := agentNode
		if node.Status == models.NodeStatusDrained || node.Status == models.NodeStatusDeleting {
			continue
		}

		if nodeStatus.GetPostgresRunning() {
			_ = s.nodes.UpdateStatus(ctx, node.ID, models.NodeStatusOnline)
		} else {
			_ = s.nodes.UpdateStatus(ctx, node.ID, models.NodeStatusOffline)
		}

		// Store PostgreSQL installation/health state whenever the agent reports it.
		if nodeStatus.GetPostgresInstalled() || nodeStatus.GetPostgresBinVersion() != "" || dockerProvisioningInstalled(node) {
			pgInstalled := nodeStatus.GetPostgresInstalled()
			pgDataInitialized := nodeStatus.GetPostgresDataInitialized()
			pgVersion := nodeStatus.GetPostgresBinVersion()
			if pgVersion == "" {
				pgVersion = nodeStatus.GetPostgresVersion()
			}
			if dockerProvisioningInstalled(node) {
				pgInstalled = true
				pgDataInitialized = pgDataInitialized || node.PostgresDataInitialized
				if pgVersion == "" {
					pgVersion = node.PostgresVersion
				}
			}
			if err := s.nodes.UpdatePostgresStatus(ctx, node.ID,
				pgInstalled,
				pgVersion,
				pgDataInitialized,
			); err != nil {
				s.log.Warn("update node postgres status (report)", "error", err, "node_id", node.ID)
			}
		}

		// Phase 4: compute and store human-readable status detail.
		detail := nodeStatus.GetNodeStatusDetail()
		if detail == "" {
			detail = computeNodeStatusDetail(node, nodeStatus)
		}
		if detail != "" {
			_ = s.nodes.UpdateStatusDetail(ctx, node.ID, detail)
		}

		installationState := modelInstallationState(nodeStatus.GetInstallationState())
		if installationState != models.InstallationStateUnspecified {
			if err := s.nodes.UpdateInstallationState(ctx, node.ID, installationState, nodeStatus.GetConflictDetails()); err != nil {
				s.log.Warn("update node installation state (report)", "error", err, "node_id", node.ID)
			}
		}

		if metrics := nodeStatus.GetSystemMetrics(); metrics != nil {
			metric := nodeMetricsToModel(node.ID, metrics)
			if err := s.nodes.InsertMetric(ctx, metric); err != nil {
				s.log.Warn("insert node system metrics (report)", "error", err, "node_id", node.ID)
			}
		}
	}

	return &skylexv1.ReportStatusResponse{}, nil
}

func nodeMetricsToModel(nodeID string, metrics *skylexv1.NodeSystemMetrics) *models.NodeMetric {
	return &models.NodeMetric{
		NodeID:               nodeID,
		RecordedAt:           time.Now().UTC(),
		OS:                   metrics.GetOs(),
		Platform:             metrics.GetPlatform(),
		PlatformVersion:      metrics.GetPlatformVersion(),
		KernelVersion:        metrics.GetKernelVersion(),
		Architecture:         metrics.GetArchitecture(),
		CPUCores:             int(metrics.GetCpuCores()),
		CPUUsagePercent:      metrics.GetCpuUsagePercent(),
		LoadAverage1M:        metrics.GetLoadAverage_1M(),
		LoadAverage5M:        metrics.GetLoadAverage_5M(),
		LoadAverage15M:       metrics.GetLoadAverage_15M(),
		MemoryTotalBytes:     metrics.GetMemoryTotalBytes(),
		MemoryUsedBytes:      metrics.GetMemoryUsedBytes(),
		MemoryAvailableBytes: metrics.GetMemoryAvailableBytes(),
		MemoryUsagePercent:   metrics.GetMemoryUsagePercent(),
		DiskTotalBytes:       metrics.GetDiskTotalBytes(),
		DiskUsedBytes:        metrics.GetDiskUsedBytes(),
		DiskAvailableBytes:   metrics.GetDiskAvailableBytes(),
		DiskUsagePercent:     metrics.GetDiskUsagePercent(),
		UptimeSeconds:        metrics.GetUptimeSeconds(),
	}
}

func (s *AgentService) FetchCommand(ctx context.Context, req *skylexv1.FetchCommandRequest) (*skylexv1.FetchCommandResponse, error) {
	node, err := s.nodeForAgent(ctx, req.GetAgentId())
	if err != nil {
		return nil, err
	}

	cmds, err := s.commands.ListPendingLimit(ctx, req.GetAgentId(), node.ID, 1)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "fetch commands: %v", err)
	}

	var protoCmds []*skylexv1.AgentCommand
	for _, c := range cmds {
		protoCmd := &skylexv1.AgentCommand{
			Id:      c.ID,
			NodeId:  c.NodeID,
			Action:  c.Action,
			Payload: c.Payload,
		}

		// Resolve command secrets for role management commands.
		if s.commandSecrets != nil && isRoleManagementAction(c.Action) {
			secrets, err := s.commandSecrets.ResolveAllForCommand(ctx, c.ID)
			if err != nil {
				s.log.Warn("resolve command secrets failed", "command_id", c.ID, "error", err)
			} else if len(secrets) > 0 {
				protoCmd.Secrets = secrets
			}
		}

		protoCmds = append(protoCmds, protoCmd)
	}

	return &skylexv1.FetchCommandResponse{
		Commands: protoCmds,
	}, nil
}

// isRoleManagementAction reports whether an action carries command secrets.
func isRoleManagementAction(action string) bool {
	switch action {
	case "pg_ensure_role", "pg_rotate_role_password", "pg_drop_role":
		return true
	default:
		return false
	}
}

func (s *AgentService) handleRoleManagementCommandResult(ctx context.Context, commandID string, success bool, errMsg string) (bool, error) {
	if s.postgresRoles == nil {
		return false, nil
	}
	return s.postgresRoles.HandleCommandResult(ctx, commandID, success, db.RedactStoredError(errMsg))
}

func (s *AgentService) handleDatabaseManagementCommandResult(ctx context.Context, commandID string, success bool, errMsg string) (bool, error) {
	if s.postgresDBs == nil {
		return false, nil
	}
	handled, grant, err := s.postgresDBs.HandleCommandResult(ctx, commandID, success, db.RedactStoredError(errMsg))
	if err != nil || !handled || grant == nil {
		return handled, err
	}

	primary, resolveErr := s.nodes.GetPrimary(ctx, grant.ClusterID)
	if resolveErr != nil {
		if markErr := s.postgresDBs.MarkCreateFailed(ctx, grant.DatabaseID, grant.OperationID, "resolve current primary for grant: "+resolveErr.Error()); markErr != nil {
			return true, fmt.Errorf("resolve grant primary: %w; mark failed: %v", resolveErr, markErr)
		}
		return true, fmt.Errorf("resolve grant primary: %w", resolveErr)
	}
	if primary == nil || primary.AgentID == "" || !primary.PostgresInstalled || !primary.PostgresDataInitialized {
		msg := "current primary is not ready for database grant"
		if markErr := s.postgresDBs.MarkCreateFailed(ctx, grant.DatabaseID, grant.OperationID, msg); markErr != nil {
			return true, fmt.Errorf("mark database create failed after grant primary check: %w", markErr)
		}
		return true, nil
	}
	allowPromote, err := s.allowPromotionForCluster(ctx, grant.ClusterID)
	if err != nil {
		if markErr := s.postgresDBs.MarkCreateFailed(ctx, grant.DatabaseID, grant.OperationID, err.Error()); markErr != nil {
			return true, fmt.Errorf("allow promotion check: %w; mark failed: %v", err, markErr)
		}
		return true, err
	}

	payload, err := json.Marshal(map[string]interface{}{
		"database_id":     grant.DatabaseID,
		"operation_id":    grant.OperationID,
		"database_name":   grant.DatabaseName,
		"grant_role_name": grant.GrantRoleName,
		"grant_role_kind": grant.GrantRoleKind,
		"allow_promote":   allowPromote,
	})
	if err != nil {
		if markErr := s.postgresDBs.MarkCreateFailed(ctx, grant.DatabaseID, grant.OperationID, "marshal database grant payload"); markErr != nil {
			return true, fmt.Errorf("marshal grant payload: %w; mark failed: %v", err, markErr)
		}
		return true, fmt.Errorf("marshal grant payload: %w", err)
	}
	cmd, err := s.postgresDBs.QueueGrantCommand(ctx, db.GrantDatabaseTxInput{
		NodeID:  primary.ID,
		AgentID: primary.AgentID,
		Payload: string(payload),
	})
	if err != nil {
		if markErr := s.postgresDBs.MarkCreateFailed(ctx, grant.DatabaseID, grant.OperationID, err.Error()); markErr != nil {
			return true, fmt.Errorf("queue database grant: %w; mark failed: %v", err, markErr)
		}
		return true, err
	}
	s.log.Info("queued pg_grant_database_privileges", "database_id", grant.DatabaseID, "command_id", cmd.ID)
	return true, nil
}

func (s *AgentService) handlePostgresAccessCommandResult(ctx context.Context, commandID string, success bool, errMsg string) (bool, error) {
	if s.postgresAccess == nil {
		return false, nil
	}
	return s.postgresAccess.HandleHBACommandResult(ctx, commandID, success, db.RedactStoredError(errMsg))
}

func (s *AgentService) allowPromotionForCluster(ctx context.Context, clusterID string) (bool, error) {
	nodes, _, err := s.nodes.ListByCluster(ctx, clusterID, 0, 1000)
	if err != nil {
		return false, fmt.Errorf("list cluster nodes: %w", err)
	}
	return len(nodes) == 1, nil
}

func (s *AgentService) ReportCommandResult(ctx context.Context, req *skylexv1.ReportCommandResultRequest) (*skylexv1.ReportCommandResultResponse, error) {
	node, err := s.nodeForAgent(ctx, req.GetAgentId())
	if err != nil {
		return nil, err
	}
	if err := s.requireCommandForAgent(ctx, req.GetAgentId(), node.ID, req.GetCommandId()); err != nil {
		return nil, err
	}

	if err := s.commands.UpdateResult(ctx, req.GetCommandId(), req.GetSuccess(), req.GetOutput(), req.GetError()); err != nil {
		return nil, status.Errorf(codes.Internal, "update command result: %v", err)
	}
	if handled, err := s.handleRoleManagementCommandResult(ctx, req.GetCommandId(), req.GetSuccess(), req.GetError()); err != nil {
		s.log.Warn("handle role management command result failed", "command_id", req.GetCommandId(), "error", err)
	} else if handled {
		return &skylexv1.ReportCommandResultResponse{}, nil
	}
	if handled, err := s.handleDatabaseManagementCommandResult(ctx, req.GetCommandId(), req.GetSuccess(), req.GetError()); err != nil {
		s.log.Warn("handle database management command result failed", "command_id", req.GetCommandId(), "error", err)
	} else if handled {
		return &skylexv1.ReportCommandResultResponse{}, nil
	}
	if handled, err := s.handlePostgresAccessCommandResult(ctx, req.GetCommandId(), req.GetSuccess(), req.GetError()); err != nil {
		s.log.Warn("handle postgres access command result failed", "command_id", req.GetCommandId(), "error", err)
	} else if handled {
		return &skylexv1.ReportCommandResultResponse{}, nil
	}
	if s.commandSecrets != nil {
		if err := s.commandSecrets.DeleteForCommand(ctx, req.GetCommandId()); err != nil {
			s.log.Warn("delete command secrets failed", "command_id", req.GetCommandId(), "error", err)
		}
	}

	if handled, err := s.handleAgentLifecycleCommandResult(ctx, req.GetCommandId(), req.GetSuccess()); err != nil {
		s.log.Warn("handle agent lifecycle command result failed", "command_id", req.GetCommandId(), "error", err)
	} else if handled {
		return &skylexv1.ReportCommandResultResponse{}, nil
	}

	if err := s.handleProvisioningCommandResult(ctx, req.GetCommandId(), req.GetSuccess(), req.GetOutput(), req.GetError()); err != nil {
		s.log.Warn("handle provisioning command result failed", "command_id", req.GetCommandId(), "error", err)
	}

	if err := s.updateClusterProvisioningStatus(ctx, req.GetCommandId()); err != nil {
		s.log.Warn("update cluster provisioning status failed", "command_id", req.GetCommandId(), "error", err)
	}

	return &skylexv1.ReportCommandResultResponse{}, nil
}

func (s *AgentService) handleAgentLifecycleCommandResult(ctx context.Context, commandID string, success bool) (bool, error) {
	cmd, err := s.commands.GetByID(ctx, commandID)
	if err != nil || cmd == nil || cmd.Action != "agent_deactivate" {
		return false, err
	}

	if success {
		return true, s.nodes.Delete(ctx, cmd.NodeID)
	}

	if err := s.nodes.UpdateStatus(ctx, cmd.NodeID, models.NodeStatusOffline); err != nil {
		return true, err
	}
	return true, s.nodes.UpdateStatusDetail(ctx, cmd.NodeID, "agent_deactivation_failed")
}

type preflightResult struct {
	State           string `json:"state"`
	Version         string `json:"version"`
	DataDir         string `json:"data_dir"`
	DataPresent     bool   `json:"data_present"`
	DataInitialized bool   `json:"data_initialized"`
}

func (r preflightResult) details() string {
	if r.State == "NOTHING_FOUND" {
		return "no native PostgreSQL installation or data directory content found"
	}
	version := r.Version
	if version == "" {
		version = "unknown"
	}
	return fmt.Sprintf("existing PostgreSQL/data detected: version=%s data_dir=%s data_present=%v data_initialized=%v", version, r.DataDir, r.DataPresent, r.DataInitialized)
}

func (s *AgentService) handleProvisioningCommandResult(ctx context.Context, commandID string, success bool, output, errMsg string) error {
	cmd, err := s.commands.GetByID(ctx, commandID)
	if err != nil || cmd == nil || cmd.NodeID == "" {
		return err
	}
	node, err := s.nodes.GetByID(ctx, cmd.NodeID)
	if err != nil || node == nil {
		return err
	}

	if !success {
		if isProvisioningAction(cmd.Action) {
			return s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateFailed, errMsg)
		}
		return nil
	}

	switch cmd.Action {
	case "pg_preflight":
		var result preflightResult
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			return fmt.Errorf("parse preflight result: %w", err)
		}
		switch result.State {
		case "PG_EXISTS":
			return s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateConflict, result.details())
		case "NOTHING_FOUND":
			if err := s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateInstalling, ""); err != nil {
				return err
			}
			cluster, err := s.clusters.GetByID(ctx, node.ClusterID)
			if err != nil || cluster == nil {
				return err
			}
			return s.queueNativeProvisioningContinuation(ctx, node, cluster.Version, false)
		default:
			return fmt.Errorf("unknown preflight state %q", result.State)
		}
	case "pg_install_native", "pg_install_docker":
		if err := s.markPostgresInstalled(ctx, node); err != nil {
			return err
		}
		return s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateInstalled, "")
	case "pg_init", "pg_basebackup":
		return s.markPostgresDataInitialized(ctx, node)
	case "pg_start":
		if err := s.markPostgresDataInitialized(ctx, node); err != nil {
			return err
		}
		if err := s.nodes.UpdateStatus(ctx, node.ID, models.NodeStatusOnline); err != nil {
			return err
		}
		return s.nodes.UpdateStatusDetail(ctx, node.ID, computeNodeStatusDetail(node, &skylexv1.NodeStatusReport{
			PostgresInstalled:       true,
			PostgresDataInitialized: true,
			PostgresRunning:         true,
		}))
	case "pg_purge_native":
		return s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateInstalling, "")
	case "pg_adopt_native":
		if err := s.markPostgresInstalled(ctx, node); err != nil {
			return err
		}
		return s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateAdopted, "")
	}
	return nil
}

func (s *AgentService) markPostgresInstalled(ctx context.Context, node *models.Node) error {
	version := node.PostgresVersion
	if version == "" && node.ClusterID != "" && s.clusters != nil {
		cluster, err := s.clusters.GetByID(ctx, node.ClusterID)
		if err != nil {
			return fmt.Errorf("get cluster version: %w", err)
		}
		if cluster != nil {
			version = cluster.Version
		}
	}
	return s.nodes.UpdatePostgresStatus(ctx, node.ID, true, version, node.PostgresDataInitialized)
}

func (s *AgentService) markPostgresDataInitialized(ctx context.Context, node *models.Node) error {
	version := node.PostgresVersion
	if version == "" && node.ClusterID != "" && s.clusters != nil {
		cluster, err := s.clusters.GetByID(ctx, node.ClusterID)
		if err != nil {
			return fmt.Errorf("get cluster version: %w", err)
		}
		if cluster != nil {
			version = cluster.Version
		}
	}
	return s.nodes.UpdatePostgresStatus(ctx, node.ID, true, version, true)
}

func (s *AgentService) queueNativeProvisioningContinuation(ctx context.Context, node *models.Node, version string, skipInstall bool) error {
	commands := installCommands(node, version, models.ServiceLocationNative, true, node.ClusterID)
	if skipInstall {
		commands = nil
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
	for _, c := range commands {
		if _, err := s.commands.Create(ctx, node.AgentID, node.ID, c.action, c.payload); err != nil {
			return fmt.Errorf("queue %s: %w", c.action, err)
		}
	}
	return nil
}

func (s *AgentService) updateClusterProvisioningStatus(ctx context.Context, commandID string) error {
	cmd, err := s.commands.GetByID(ctx, commandID)
	if err != nil || cmd == nil || cmd.NodeID == "" {
		return err
	}

	node, err := s.nodes.GetByID(ctx, cmd.NodeID)
	if err != nil || node == nil || node.ClusterID == "" {
		return err
	}

	cluster, err := s.clusters.GetByID(ctx, node.ClusterID)
	if err != nil || cluster == nil || cluster.Status != models.ClusterStatusCreating {
		return err
	}

	nodes, _, err := s.nodes.ListByCluster(ctx, node.ClusterID, 0, 1000)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}
	for _, n := range nodes {
		switch n.InstallationState {
		case models.InstallationStateConflict, models.InstallationStatePendingPreflight, models.InstallationStateInstalling:
			return nil
		case models.InstallationStateFailed:
			nodeIDs := make([]string, 0, len(nodes))
			for _, n := range nodes {
				nodeIDs = append(nodeIDs, n.ID)
			}
			if err := s.commands.MarkPendingFailedByNodeIDs(ctx, nodeIDs, provisioningActions(), "skipped after provisioning failure"); err != nil {
				s.log.Warn("mark pending provisioning commands failed", "cluster_id", node.ClusterID, "error", err)
			}
			return s.updateClusterStatus(ctx, node.ClusterID, models.ClusterStatusStopped)
		}
	}

	nodeIDs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		nodeIDs = append(nodeIDs, n.ID)
	}
	cmds, err := s.commands.ListByNodeIDs(ctx, nodeIDs, 1000)
	if err != nil {
		return err
	}

	hasPending := false
	for _, c := range cmds {
		if c.CreatedAt.Before(cluster.CreatedAt) || !isProvisioningAction(c.Action) {
			continue
		}
		switch c.Status {
		case models.CommandStatusFailed:
			if err := s.commands.MarkPendingFailedByNodeIDs(ctx, nodeIDs, provisioningActions(), "skipped after provisioning failure"); err != nil {
				s.log.Warn("mark pending provisioning commands failed", "cluster_id", node.ClusterID, "error", err)
			}
			return s.updateClusterStatus(ctx, node.ClusterID, models.ClusterStatusStopped)
		case models.CommandStatusPending:
			hasPending = true
		}
	}
	if !hasPending {
		return s.updateClusterStatus(ctx, node.ClusterID, models.ClusterStatusRunning)
	}
	return nil
}

func isProvisioningAction(action string) bool {
	switch action {
	case "pg_preflight", "pg_install_native", "pg_install_docker", "pg_purge_native", "pg_adopt_native", "pg_init", "pg_start", "pg_create_repl_user", "pg_basebackup", "repoint_replica":
		return true
	default:
		return false
	}
}

func provisioningActions() []string {
	return []string{"pg_preflight", "pg_install_native", "pg_install_docker", "pg_purge_native", "pg_adopt_native", "pg_init", "pg_start", "pg_create_repl_user", "pg_basebackup", "repoint_replica"}
}

func (s *AgentService) updateClusterStatus(ctx context.Context, clusterID string, clusterStatus models.ClusterStatus) error {
	if s.clusters == nil {
		return nil
	}
	return s.clusters.UpdateStatus(ctx, clusterID, clusterStatus)
}

func (s *AgentService) ReportCommandLog(ctx context.Context, req *skylexv1.ReportCommandLogRequest) (*skylexv1.ReportCommandLogResponse, error) {
	node, err := s.nodeForAgent(ctx, req.GetAgentId())
	if err != nil {
		return nil, err
	}

	entries := req.GetEntries()
	if len(entries) == 0 {
		return &skylexv1.ReportCommandLogResponse{}, nil
	}

	logs := make([]*db.CommandLog, 0, len(entries))
	validatedCommands := make(map[string]bool)
	for _, e := range entries {
		commandID := e.GetCommandId()
		if commandID == "" {
			return nil, status.Error(codes.InvalidArgument, "command_id is required")
		}
		if !validatedCommands[commandID] {
			if err := s.requireCommandForAgent(ctx, req.GetAgentId(), node.ID, commandID); err != nil {
				return nil, err
			}
			validatedCommands[commandID] = true
		}

		level := e.GetLevel()
		if level == "" {
			level = "info"
		}
		logs = append(logs, &db.CommandLog{
			CommandID: commandID,
			AgentID:   req.GetAgentId(),
			Level:     level,
			Message:   e.GetMessage(),
			CreatedAt: timeFromMillis(e.GetTimestampMs()),
		})
	}

	if err := s.commandLogs.CreateBatch(ctx, logs); err != nil {
		s.log.Warn("failed to store command logs", "agent_id", req.GetAgentId(), "node_id", node.ID, "count", len(logs), "error", err)
		return nil, status.Errorf(codes.Internal, "store command logs: %v", err)
	}

	return &skylexv1.ReportCommandLogResponse{}, nil
}

func (s *AgentService) nodeForAgent(ctx context.Context, agentID string) (*models.Node, error) {
	if agentID == "" {
		return nil, status.Error(codes.Unauthenticated, "agent_id is required")
	}
	node, err := s.nodes.GetByAgentID(ctx, agentID)
	if err != nil {
		s.log.Warn("lookup node for agent failed", "agent_id", agentID, "error", err)
		return nil, status.Errorf(codes.Internal, "lookup agent node: %v", err)
	}
	if node == nil {
		return nil, status.Error(codes.Unauthenticated, "unknown agent")
	}
	return node, nil
}

func (s *AgentService) requireCommandForAgent(ctx context.Context, agentID, nodeID, commandID string) error {
	if commandID == "" {
		return status.Error(codes.InvalidArgument, "command_id is required")
	}
	cmd, err := s.commands.GetByID(ctx, commandID)
	if err != nil {
		return status.Errorf(codes.Internal, "get command: %v", err)
	}
	if cmd == nil {
		return status.Error(codes.NotFound, "command not found")
	}
	if cmd.AgentID != agentID && cmd.NodeID != nodeID {
		return status.Error(codes.PermissionDenied, "command does not belong to agent")
	}
	return nil
}

func timeFromMillis(ms int64) time.Time {
	if ms <= 0 {
		return time.Now().UTC()
	}
	return time.UnixMilli(ms).UTC()
}

// computeNodeStatusDetail derives a human-readable status detail from the
// node's current state and the agent's latest status report.
func computeNodeStatusDetail(node *models.Node, report *skylexv1.NodeStatusReport) string {
	if report.GetInstallationState() == skylexv1.InstallationState_INSTALLATION_STATE_CONFLICT {
		return "installation_conflict"
	}
	if !report.GetPostgresInstalled() {
		return "waiting_for_postgres"
	}
	if !report.GetPostgresDataInitialized() {
		return "initializing_data_directory"
	}
	if !report.GetPostgresRunning() {
		return "stopped"
	}
	if node.Role == models.NodeRoleReplica {
		return "syncing_replica"
	}
	return "healthy"
}

func modelInstallationState(state skylexv1.InstallationState) models.InstallationState {
	switch state {
	case skylexv1.InstallationState_INSTALLATION_STATE_PENDING_PREFLIGHT:
		return models.InstallationStatePendingPreflight
	case skylexv1.InstallationState_INSTALLATION_STATE_NOTHING_FOUND:
		return models.InstallationStateNothingFound
	case skylexv1.InstallationState_INSTALLATION_STATE_CONFLICT:
		return models.InstallationStateConflict
	case skylexv1.InstallationState_INSTALLATION_STATE_INSTALLING:
		return models.InstallationStateInstalling
	case skylexv1.InstallationState_INSTALLATION_STATE_INSTALLED:
		return models.InstallationStateInstalled
	case skylexv1.InstallationState_INSTALLATION_STATE_FAILED:
		return models.InstallationStateFailed
	case skylexv1.InstallationState_INSTALLATION_STATE_ADOPTED:
		return models.InstallationStateAdopted
	default:
		return models.InstallationStateUnspecified
	}
}
