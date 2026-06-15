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

type ClusterService struct {
	skylexv1.UnimplementedClusterServiceServer
	clusters       *db.ClusterRepository
	nodes          *db.NodeRepository
	commands       *db.AgentCommandRepository
	failoverEngine *FailoverEngine
	log            *slog.Logger
}

func NewClusterService(clusters *db.ClusterRepository, nodes *db.NodeRepository, commands *db.AgentCommandRepository, log *slog.Logger) *ClusterService {
	return &ClusterService{
		clusters: clusters,
		nodes:    nodes,
		commands: commands,
		log:      log,
	}
}

func (s *ClusterService) SetFailoverEngine(e *FailoverEngine) {
	s.failoverEngine = e
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

	engine := models.EngineType(cfg.GetEngine().String())
	version := cfg.GetVersion()
	if version == "" {
		version = "16"
	}

	mode := convertReplicationMode(cfg.GetReplicationMode())

	cluster, err := s.clusters.Create(ctx, req.GetName(),
		cfg.GetStorageConfigId(), "", engine, version, mode,
		int(cfg.GetReplicaCount()), cfg.GetPitrEnabled(), cfg.GetLabels())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create cluster: %v", err)
	}

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
	}, nil
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

	if err := s.clusters.UpdateStatus(ctx, req.GetId(), models.ClusterStatusDeleting); err != nil {
		return nil, status.Errorf(codes.Internal, "mark deleting: %v", err)
	}

	if err := s.clusters.Delete(ctx, req.GetId()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete cluster: %v", err)
	}

	return &skylexv1.DeleteClusterResponse{}, nil
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

	var status skylexv1.ClusterStatus
	switch c.Status {
	case models.ClusterStatusCreating:
		status = skylexv1.ClusterStatus_CLUSTER_STATUS_CREATING
	case models.ClusterStatusRunning:
		status = skylexv1.ClusterStatus_CLUSTER_STATUS_HEALTHY
	case models.ClusterStatusDegraded:
		status = skylexv1.ClusterStatus_CLUSTER_STATUS_DEGRADED
	case models.ClusterStatusDeleting:
		status = skylexv1.ClusterStatus_CLUSTER_STATUS_DELETING
	case models.ClusterStatusStopped:
		status = skylexv1.ClusterStatus_CLUSTER_STATUS_FAILED
	}

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
		},
		Status:    status,
		CreatedAt: timestamppb.New(c.CreatedAt),
		UpdatedAt: timestamppb.New(c.UpdatedAt),
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