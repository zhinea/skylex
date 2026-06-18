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
	if req.GetAgentId() != "" {
		node, err := s.nodes.GetByAgentID(ctx, req.GetAgentId())
		if err != nil {
			s.log.Warn("lookup node for heartbeat failed", "agent_id", req.GetAgentId(), "error", err)
		} else if node == nil {
			// Unknown agent_ids are usually stale/orphaned agents from a
			// previous registration. Log at debug to avoid log spam.
			s.log.Debug("node not found for heartbeat", "agent_id", req.GetAgentId())
		} else {
			if err := s.nodes.UpdateHeartbeat(ctx, node.ID); err != nil {
				s.log.Warn("heartbeat update failed", "node_id", node.ID, "error", err)
			}
		}
	}

	return &skylexv1.HeartbeatResponse{}, nil
}

func (s *AgentService) ReportStatus(ctx context.Context, req *skylexv1.ReportStatusRequest) (*skylexv1.ReportStatusResponse, error) {
	for _, nodeStatus := range req.GetNodeStatuses() {
		var node *models.Node
		var err error

		if nodeStatus.GetNodeId() != "" {
			node, err = s.nodes.GetByID(ctx, nodeStatus.GetNodeId())
		} else {
			node, err = s.nodes.GetByAgentID(ctx, req.GetAgentId())
		}

		if err != nil {
			s.log.Warn("lookup node for status report failed",
				"node_id", nodeStatus.GetNodeId(),
				"agent_id", req.GetAgentId(),
				"error", err,
			)
			continue
		}
		if node == nil {
			s.log.Debug("node not found for status report",
				"node_id", nodeStatus.GetNodeId(),
				"agent_id", req.GetAgentId(),
			)
			continue
		}

		if nodeStatus.GetPostgresRunning() {
			_ = s.nodes.UpdateStatus(ctx, node.ID, models.NodeStatusOnline)
		} else {
			_ = s.nodes.UpdateStatus(ctx, node.ID, models.NodeStatusOffline)
		}

		// Store PostgreSQL installation/health state whenever the agent reports it.
		if nodeStatus.GetPostgresInstalled() || nodeStatus.GetPostgresBinVersion() != "" {
			pgVersion := nodeStatus.GetPostgresBinVersion()
			if pgVersion == "" {
				pgVersion = nodeStatus.GetPostgresVersion()
			}
			if err := s.nodes.UpdatePostgresStatus(ctx, node.ID,
				nodeStatus.GetPostgresInstalled(),
				pgVersion,
				nodeStatus.GetPostgresDataInitialized(),
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
	}

	return &skylexv1.ReportStatusResponse{}, nil
}

func (s *AgentService) FetchCommand(ctx context.Context, req *skylexv1.FetchCommandRequest) (*skylexv1.FetchCommandResponse, error) {
	node, _ := s.nodes.GetByAgentID(ctx, req.GetAgentId())
	nodeID := ""
	if node != nil {
		nodeID = node.ID
	}

	cmds, err := s.commands.ListPendingLimit(ctx, req.GetAgentId(), nodeID, 1)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "fetch commands: %v", err)
	}

	var protoCmds []*skylexv1.AgentCommand
	for _, c := range cmds {
		protoCmds = append(protoCmds, &skylexv1.AgentCommand{
			Id:      c.ID,
			NodeId:  c.NodeID,
			Action:  c.Action,
			Payload: c.Payload,
		})
	}

	return &skylexv1.FetchCommandResponse{
		Commands: protoCmds,
	}, nil
}

func (s *AgentService) ReportCommandResult(ctx context.Context, req *skylexv1.ReportCommandResultRequest) (*skylexv1.ReportCommandResultResponse, error) {
	if err := s.commands.UpdateResult(ctx, req.GetCommandId(), req.GetSuccess(), req.GetOutput(), req.GetError()); err != nil {
		return nil, status.Errorf(codes.Internal, "update command result: %v", err)
	}

	if err := s.handleProvisioningCommandResult(ctx, req.GetCommandId(), req.GetSuccess(), req.GetOutput(), req.GetError()); err != nil {
		s.log.Warn("handle provisioning command result failed", "command_id", req.GetCommandId(), "error", err)
	}

	if err := s.updateClusterProvisioningStatus(ctx, req.GetCommandId()); err != nil {
		s.log.Warn("update cluster provisioning status failed", "command_id", req.GetCommandId(), "error", err)
	}

	return &skylexv1.ReportCommandResultResponse{}, nil
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
		return s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateInstalled, "")
	case "pg_purge_native":
		return s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateInstalling, "")
	case "pg_adopt_native":
		return s.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateAdopted, "")
	}
	return nil
}

func (s *AgentService) queueNativeProvisioningContinuation(ctx context.Context, node *models.Node, version string, skipInstall bool) error {
	commands := installCommands(node, version, models.ServiceLocationNative, true)
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
	node, _ := s.nodes.GetByAgentID(ctx, req.GetAgentId())
	nodeID := ""
	if node != nil {
		nodeID = node.ID
	}

	entries := req.GetEntries()
	if len(entries) == 0 {
		return &skylexv1.ReportCommandLogResponse{}, nil
	}

	logs := make([]*db.CommandLog, 0, len(entries))
	for _, e := range entries {
		level := e.GetLevel()
		if level == "" {
			level = "info"
		}
		logs = append(logs, &db.CommandLog{
			CommandID: e.GetCommandId(),
			AgentID:   req.GetAgentId(),
			Level:     level,
			Message:   e.GetMessage(),
			CreatedAt: timeFromMillis(e.GetTimestampMs()),
		})
	}

	if err := s.commandLogs.CreateBatch(ctx, logs); err != nil {
		s.log.Warn("failed to store command logs", "agent_id", req.GetAgentId(), "node_id", nodeID, "count", len(logs), "error", err)
		return nil, status.Errorf(codes.Internal, "store command logs: %v", err)
	}

	return &skylexv1.ReportCommandLogResponse{}, nil
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
