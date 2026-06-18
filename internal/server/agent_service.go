package server

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
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
	nodes          *db.NodeRepository
	commands       *db.AgentCommandRepository
	agentTokenRepo *db.AgentTokenRepository
	log            *slog.Logger
}

func NewAgentService(cfg *Config, nodes *db.NodeRepository, commands *db.AgentCommandRepository, agentTokenRepo *db.AgentTokenRepository, log *slog.Logger) *AgentService {
	return &AgentService{
		cfg:            cfg,
		nodes:          nodes,
		commands:       commands,
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

	node, _ := s.nodes.GetByHostname(ctx, req.GetHostname())
	if node != nil {
		if err := s.nodes.UpdateAgentID(ctx, node.ID, agentID); err != nil {
			s.log.Warn("update node agent_id", "error", err, "node_id", node.ID)
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
	}

	return &skylexv1.ReportStatusResponse{}, nil
}

func (s *AgentService) FetchCommand(ctx context.Context, req *skylexv1.FetchCommandRequest) (*skylexv1.FetchCommandResponse, error) {
	node, _ := s.nodes.GetByAgentID(ctx, req.GetAgentId())
	nodeID := ""
	if node != nil {
		nodeID = node.ID
	}

	cmds, err := s.commands.ListPending(ctx, req.GetAgentId(), nodeID)
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

	return &skylexv1.ReportCommandResultResponse{}, nil
}