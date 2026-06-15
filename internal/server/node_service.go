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
	nodes    *db.NodeRepository
	commands *db.AgentCommandRepository
	log      *slog.Logger
}

func NewNodeService(nodes *db.NodeRepository, commands *db.AgentCommandRepository, log *slog.Logger) *NodeService {
	return &NodeService{nodes: nodes, commands: commands, log: log}
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