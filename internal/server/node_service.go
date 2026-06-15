package server

import (
	"context"
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
	nodes *db.NodeRepository
	log   *slog.Logger
}

func NewNodeService(nodes *db.NodeRepository, log *slog.Logger) *NodeService {
	return &NodeService{nodes: nodes, log: log}
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
	return nil, status.Error(codes.Unimplemented, "DrainNode not implemented")
}

func (s *NodeService) RejoinNode(ctx context.Context, req *skylexv1.RejoinNodeRequest) (*skylexv1.RejoinNodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "RejoinNode not implemented")
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