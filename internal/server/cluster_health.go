package server

import (
	"context"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetClusterHealth returns a poll-friendly snapshot of a cluster: its current
// status, a compact per-node health view, and the progress of any in-flight
// provisioning/lifecycle action. It deliberately excludes log lines and raw
// metrics (those are served by ListNodeCommandLogs / ListNodeMetrics) so the
// UI can poll it cheaply while a cluster is being created or edited.
func (s *ClusterService) GetClusterHealth(ctx context.Context, req *skylexv1.GetClusterHealthRequest) (*skylexv1.GetClusterHealthResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	cluster, err := s.clusters.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	if cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.GetId())
	}

	nodes, _, err := s.nodes.ListByCluster(ctx, cluster.ID, 0, maxClusterLifecycleNodes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list cluster nodes: %v", err)
	}

	nodeHealth := make([]*skylexv1.NodeHealth, 0, len(nodes))
	nodeIDs := make([]string, 0, len(nodes))
	readyNodes := 0
	for _, n := range nodes {
		nodeIDs = append(nodeIDs, n.ID)
		// Pass 0 so applyAgentConnectionStatus uses its 30s default freshness
		// window; ClusterService does not carry the configured heartbeat timeout.
		applyAgentConnectionStatus(n, 0)
		installed := effectivePostgresInstalled(n)
		dataInitialized := effectivePostgresDataInitialized(n)
		ready := nodeEligibleForLifecycle(n)
		if ready {
			readyNodes++
		}
		nodeHealth = append(nodeHealth, &skylexv1.NodeHealth{
			NodeId:                  n.ID,
			Hostname:                n.Hostname,
			Role:                    nodeRoleToProto(n.Role),
			Status:                  string(n.Status),
			StatusDetail:            effectiveStatusDetail(n, installed, dataInitialized),
			InstallationState:       protoInstallationState(n.InstallationState),
			PostgresInstalled:       installed,
			PostgresDataInitialized: dataInitialized,
			AgentConnected:          n.AgentConnected,
			Ready:                   ready,
			ConflictDetails:         n.ConflictDetails,
		})
	}

	progress, err := s.computeActionProgress(ctx, cluster, nodes, nodeIDs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "compute action progress: %v", err)
	}

	return &skylexv1.GetClusterHealthResponse{
		ClusterId:  cluster.ID,
		Status:     clusterStatusToProto(cluster.Status),
		Nodes:      nodeHealth,
		Progress:   progress,
		ReadyNodes: int32(readyNodes),
		TotalNodes: int32(len(nodes)),
	}, nil
}

// computeActionProgress derives step counts for the in-flight operation from
// queued agent_commands. Progress is intentionally based on command rows (a
// stable, countable source of truth) rather than parsing log output.
func (s *ClusterService) computeActionProgress(ctx context.Context, cluster *models.Cluster, nodes []*models.Node, nodeIDs []string) (*skylexv1.ClusterActionProgress, error) {
	if len(nodeIDs) == 0 {
		return &skylexv1.ClusterActionProgress{}, nil
	}

	cmds, err := s.commands.ListByNodeIDs(ctx, nodeIDs, maxClusterLifecycleNodes)
	if err != nil {
		return nil, err
	}

	creating := cluster.Status == models.ClusterStatusCreating
	var total, completed, failed, pending int
	for _, c := range cmds {
		// During creation only count provisioning commands queued for this
		// cluster generation; for lifecycle operations count start/stop work.
		if creating {
			if c.CreatedAt.Before(cluster.CreatedAt) || !isProvisioningAction(c.Action) {
				continue
			}
		} else if !isLifecycleOnlyAction(c.Action) {
			continue
		}
		total++
		switch c.Status {
		case models.CommandStatusCompleted:
			completed++
		case models.CommandStatusFailed:
			failed++
		case models.CommandStatusPending:
			pending++
		}
	}

	operation := ""
	if creating {
		operation = "create"
	} else if pending > 0 {
		operation = "lifecycle"
	}

	percent := 0
	if total > 0 {
		percent = (completed * 100) / total
	}

	return &skylexv1.ClusterActionProgress{
		Operation:      operation,
		TotalSteps:     int32(total),
		CompletedSteps: int32(completed),
		FailedSteps:    int32(failed),
		PendingSteps:   int32(pending),
		Percent:        int32(percent),
		InProgress:     pending > 0,
	}, nil
}

func nodeRoleToProto(role models.NodeRole) skylexv1.NodeRole {
	switch role {
	case models.NodeRolePrimary:
		return skylexv1.NodeRole_NODE_ROLE_PRIMARY
	case models.NodeRoleReplica:
		return skylexv1.NodeRole_NODE_ROLE_REPLICA
	default:
		return skylexv1.NodeRole_NODE_ROLE_UNSPECIFIED
	}
}

func clusterStatusToProto(s models.ClusterStatus) skylexv1.ClusterStatus {
	switch s {
	case models.ClusterStatusCreating:
		return skylexv1.ClusterStatus_CLUSTER_STATUS_CREATING
	case models.ClusterStatusRunning:
		return skylexv1.ClusterStatus_CLUSTER_STATUS_HEALTHY
	case models.ClusterStatusDegraded:
		return skylexv1.ClusterStatus_CLUSTER_STATUS_DEGRADED
	case models.ClusterStatusDeleting:
		return skylexv1.ClusterStatus_CLUSTER_STATUS_DELETING
	case models.ClusterStatusStopped:
		return skylexv1.ClusterStatus_CLUSTER_STATUS_STOPPED
	default:
		return skylexv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED
	}
}
