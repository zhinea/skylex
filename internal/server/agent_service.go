package server

import (
	"context"
	"log/slog"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AgentService struct {
	skylexv1.UnimplementedAgentServiceServer
	nodes    *db.NodeRepository
	commands *db.AgentCommandRepository
	log      *slog.Logger
}

func NewAgentService(nodes *db.NodeRepository, commands *db.AgentCommandRepository, log *slog.Logger) *AgentService {
	return &AgentService{
		nodes:    nodes,
		commands: commands,
		log:      log,
	}
}

func (s *AgentService) RegisterAgent(ctx context.Context, req *skylexv1.RegisterAgentRequest) (*skylexv1.RegisterAgentResponse, error) {
	agentID, err := crypto.GenerateToken(16)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate agent id: %v", err)
	}

	s.log.Info("agent registered",
		"agent_id", agentID,
		"hostname", req.GetHostname(),
		"address", req.GetAddress(),
	)

	return &skylexv1.RegisterAgentResponse{
		AgentId: agentID,
	}, nil
}

func (s *AgentService) Heartbeat(ctx context.Context, req *skylexv1.HeartbeatRequest) (*skylexv1.HeartbeatResponse, error) {
	if req.GetNodeId() != "" {
		if err := s.nodes.UpdateHeartbeat(ctx, req.GetNodeId()); err != nil {
			s.log.Warn("heartbeat update failed", "node_id", req.GetNodeId(), "error", err)
		}
	}

	return &skylexv1.HeartbeatResponse{}, nil
}

func (s *AgentService) ReportStatus(ctx context.Context, req *skylexv1.ReportStatusRequest) (*skylexv1.ReportStatusResponse, error) {
	for _, status := range req.GetNodeStatuses() {
		if status.GetNodeId() == "" {
			continue
		}
		node, err := s.nodes.GetByID(ctx, status.GetNodeId())
		if err != nil || node == nil {
			s.log.Warn("node not found for status report", "node_id", status.GetNodeId())
			continue
		}

		if status.GetPostgresRunning() {
			_ = s.nodes.UpdateStatus(ctx, status.GetNodeId(), models.NodeStatusOnline)
		} else {
			_ = s.nodes.UpdateStatus(ctx, status.GetNodeId(), models.NodeStatusOffline)
		}
	}

	return &skylexv1.ReportStatusResponse{}, nil
}

func (s *AgentService) FetchCommand(ctx context.Context, req *skylexv1.FetchCommandRequest) (*skylexv1.FetchCommandResponse, error) {
	cmds, err := s.commands.ListPending(ctx, req.GetAgentId())
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