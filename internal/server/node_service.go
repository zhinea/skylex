package server

import (
	"context"
	"fmt"
	"log/slog"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type NodeService struct {
	skylexv1.UnimplementedNodeServiceServer
	nodes       *db.NodeRepository
	commands    *db.AgentCommandRepository
	commandLogs *db.CommandLogRepository
	log         *slog.Logger
}

func NewNodeService(nodes *db.NodeRepository, commands *db.AgentCommandRepository, commandLogs *db.CommandLogRepository, log *slog.Logger) *NodeService {
	return &NodeService{nodes: nodes, commands: commands, commandLogs: commandLogs, log: log}
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
		protoNodes = append(protoNodes, nodeToProto(n))
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
		Node: nodeToProto(node),
	}, nil
}

func (s *NodeService) DrainNode(ctx context.Context, req *skylexv1.DrainNodeRequest) (*skylexv1.DrainNodeResponse, error) {
	node, err := s.nodes.GetByID(ctx, req.GetNodeId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get node: %v", err)
	}
	if node == nil {
		return nil, status.Errorf(codes.NotFound, "node %q not found", req.GetNodeId())
	}

	// Unassigned agents are removed from the UI completely. Cluster members
	// are only marked offline so the cluster topology stays intact.
	if node.ClusterID == "" {
		if node.AgentID != "" {
			if _, err := s.commands.Create(ctx, node.AgentID, node.ID, "pg_stop", ""); err != nil {
				s.log.Warn("queue stop command for drain", "error", err)
			}
		}
		if err := s.nodes.Delete(ctx, node.ID); err != nil {
			return nil, status.Errorf(codes.Internal, "delete node: %v", err)
		}
		s.log.Info("unassigned node drained and removed", "node_id", node.ID)
		return &skylexv1.DrainNodeResponse{}, nil
	}

	if err := s.nodes.UpdateStatus(ctx, node.ID, models.NodeStatusOffline); err != nil {
		return nil, status.Errorf(codes.Internal, "update node status: %v", err)
	}

	if node.AgentID != "" {
		if _, err := s.commands.Create(ctx, node.AgentID, node.ID, "pg_stop", ""); err != nil {
			s.log.Warn("queue stop command for drain", "error", err)
		}
	}

	s.log.Info("node drained", "node_id", node.ID)
	return &skylexv1.DrainNodeResponse{}, nil
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

	s.log.Info("node rejoined", "node_id", node.ID)
	return &skylexv1.RejoinNodeResponse{}, nil
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
				node, _ := s.nodes.GetByID(ctx, nodeID)
				if node != nil {
					hostname = node.Hostname
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

func nodeToProto(n *models.Node) *skylexv1.Node {
	var role skylexv1.NodeRole
	switch n.Role {
	case models.NodeRolePrimary:
		role = skylexv1.NodeRole_NODE_ROLE_PRIMARY
	case models.NodeRoleReplica:
		role = skylexv1.NodeRole_NODE_ROLE_REPLICA
	}

	return &skylexv1.Node{
		Id:           n.ID,
		ClusterId:    n.ClusterID,
		Hostname:     n.Hostname,
		Role:         role,
		Address:      n.Address,
		Port:         int32(n.Port),
		Labels:       n.Labels,
		AgentVersion: n.AgentVersion,
		LastSeen:     timestamppb.New(n.LastSeen),
		CreatedAt:    timestamppb.New(n.CreatedAt),
		UpdatedAt:    timestamppb.New(n.UpdatedAt),
	}
}