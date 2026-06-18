package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"

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
	settings       *db.ClusterSettingsRepository
	failoverEngine *FailoverEngine
	log            *slog.Logger
}

func NewClusterService(clusters *db.ClusterRepository, nodes *db.NodeRepository, commands *db.AgentCommandRepository, settings *db.ClusterSettingsRepository, log *slog.Logger) *ClusterService {
	return &ClusterService{
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

	engine := models.EngineType(cfg.GetEngine().String())
	version := cfg.GetVersion()
	if version == "" {
		version = "16"
	}

	mode := convertReplicationMode(cfg.GetReplicationMode())

	replicaCount := int(cfg.GetReplicaCount())
	neededNodes := replicaCount + 1 // primary + replicas
	idleNodes, err := s.nodes.ListUnassigned(ctx, neededNodes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list idle nodes: %v", err)
	}
	if len(idleNodes) < neededNodes {
		return nil, status.Errorf(codes.FailedPrecondition,
			"need %d idle node(s) for this cluster, found %d. Register more agents or set replicas to %d",
			neededNodes, len(idleNodes), max(len(idleNodes)-1, 0))
	}

	cluster, err := s.clusters.Create(ctx, req.GetName(),
		cfg.GetStorageConfigId(), "", engine, version, mode,
		replicaCount, cfg.GetPitrEnabled(), cfg.GetLabels())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create cluster: %v", err)
	}

	primary := idleNodes[0]
	if err := s.nodes.AssignToCluster(ctx, primary.ID, cluster.ID, models.NodeRolePrimary); err != nil {
		return nil, status.Errorf(codes.Internal, "assign primary node: %v", err)
	}
	if err := s.queuePrimaryCommands(ctx, primary); err != nil {
		return nil, status.Errorf(codes.Internal, "queue primary commands: %v", err)
	}

	for i := 1; i <= replicaCount; i++ {
		replica := idleNodes[i]
		if err := s.nodes.AssignToCluster(ctx, replica.ID, cluster.ID, models.NodeRoleReplica); err != nil {
			return nil, status.Errorf(codes.Internal, "assign replica node: %v", err)
		}
		if err := s.queueReplicaCommands(ctx, replica, primary); err != nil {
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
		Settings: &skylexv1.ClusterSettings{Parameters: parameters},
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

func (s *ClusterService) queuePrimaryCommands(ctx context.Context, node *models.Node) error {
	commands := []struct{ action, payload string }{
		{"pg_init", ""},
		{"pg_start", ""},
		{"pg_create_repl_user", ""},
	}
	for _, c := range commands {
		if _, err := s.commands.Create(ctx, node.AgentID, node.ID, c.action, c.payload); err != nil {
			return fmt.Errorf("queue %s: %w", c.action, err)
		}
	}
	return nil
}

func (s *ClusterService) queueReplicaCommands(ctx context.Context, replica, primary *models.Node) error {
	payload := fmt.Sprintf("%s:%d", nodeAddress(primary), primary.Port)
	commands := []struct{ action, payload string }{
		{"pg_basebackup", payload},
		{"repoint_replica", payload},
		{"pg_start", ""},
	}
	for _, c := range commands {
		if _, err := s.commands.Create(ctx, replica.AgentID, replica.ID, c.action, c.payload); err != nil {
			return fmt.Errorf("queue %s: %w", c.action, err)
		}
	}
	return nil
}

func nodeAddress(n *models.Node) string {
	if n.Address != "" {
		return n.Address
	}
	return n.Hostname
}
