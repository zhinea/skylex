package server

import (
	"context"
	"testing"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/models"
)

func TestClusterService_GetClusterHealth_Validation(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()

	if _, err := svc.GetClusterHealth(ctx, &skylexv1.GetClusterHealthRequest{}); err == nil {
		t.Fatal("expected error for empty id")
	}
	if _, err := svc.GetClusterHealth(ctx, &skylexv1.GetClusterHealthRequest{Id: "missing"}); err == nil {
		t.Fatal("expected not-found error for unknown cluster")
	}
}

// TestClusterService_GetClusterHealth_Creating verifies that while provisioning
// is in flight the snapshot reports CREATING with an in-progress, partial
// progress derived from the queued preflight command (pending => not complete).
func TestClusterService_GetClusterHealth_Creating(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()
	nodeID := createIdleTestNode(t, ctx, svc)
	clusterID := createTestCluster(t, ctx, svc, nodeID)

	resp, err := svc.GetClusterHealth(ctx, &skylexv1.GetClusterHealthRequest{Id: clusterID})
	if err != nil {
		t.Fatalf("get cluster health: %v", err)
	}

	if resp.GetStatus() != skylexv1.ClusterStatus_CLUSTER_STATUS_CREATING {
		t.Fatalf("status = %v, want CREATING", resp.GetStatus())
	}
	if got := len(resp.GetNodes()); got != 1 {
		t.Fatalf("nodes = %d, want 1", got)
	}
	if resp.GetTotalNodes() != 1 {
		t.Fatalf("total_nodes = %d, want 1", resp.GetTotalNodes())
	}
	if resp.GetReadyNodes() != 0 {
		t.Fatalf("ready_nodes = %d, want 0", resp.GetReadyNodes())
	}

	progress := resp.GetProgress()
	if progress.GetOperation() != "create" {
		t.Fatalf("operation = %q, want create", progress.GetOperation())
	}
	if !progress.GetInProgress() {
		t.Fatal("expected in_progress=true while preflight is pending")
	}
	if progress.GetTotalSteps() < 1 {
		t.Fatalf("total_steps = %d, want >= 1", progress.GetTotalSteps())
	}
	if progress.GetPendingSteps() < 1 {
		t.Fatalf("pending_steps = %d, want >= 1", progress.GetPendingSteps())
	}
	if progress.GetPercent() != 0 {
		t.Fatalf("percent = %d, want 0 (nothing completed yet)", progress.GetPercent())
	}

	node := resp.GetNodes()[0]
	if node.GetReady() {
		t.Fatal("expected node not ready during provisioning")
	}
}

// TestClusterService_GetClusterHealth_Ready verifies that once provisioning
// completes the snapshot reflects the running status, a ready node, and 100%
// progress with no in-flight work.
func TestClusterService_GetClusterHealth_Ready(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()
	nodeID := createIdleTestNode(t, ctx, svc)
	clusterID := createTestCluster(t, ctx, svc, nodeID)

	// Simulate the agent completing provisioning: node installed + data ready,
	// the preflight command marked completed, and the cluster promoted.
	if err := svc.nodes.UpdateInstallationState(ctx, nodeID, models.InstallationStateInstalled, ""); err != nil {
		t.Fatalf("update installation state: %v", err)
	}
	if err := svc.nodes.UpdatePostgresStatus(ctx, nodeID, true, "16", true); err != nil {
		t.Fatalf("update postgres status: %v", err)
	}
	if err := svc.nodes.UpdateStatus(ctx, nodeID, models.NodeStatusOnline); err != nil {
		t.Fatalf("update node status: %v", err)
	}
	if err := svc.clusters.UpdateStatus(ctx, clusterID, models.ClusterStatusRunning); err != nil {
		t.Fatalf("update cluster status: %v", err)
	}
	// Mark every queued provisioning command for the node completed.
	if err := completeAllPendingCommands(ctx, svc, nodeID); err != nil {
		t.Fatalf("complete pending commands: %v", err)
	}

	resp, err := svc.GetClusterHealth(ctx, &skylexv1.GetClusterHealthRequest{Id: clusterID})
	if err != nil {
		t.Fatalf("get cluster health: %v", err)
	}

	if resp.GetStatus() != skylexv1.ClusterStatus_CLUSTER_STATUS_HEALTHY {
		t.Fatalf("status = %v, want HEALTHY", resp.GetStatus())
	}
	if resp.GetReadyNodes() != 1 {
		t.Fatalf("ready_nodes = %d, want 1", resp.GetReadyNodes())
	}

	progress := resp.GetProgress()
	if progress.GetInProgress() {
		t.Fatal("expected in_progress=false once provisioning is complete")
	}
	if progress.GetPendingSteps() != 0 {
		t.Fatalf("pending_steps = %d, want 0", progress.GetPendingSteps())
	}
	if progress.GetTotalSteps() > 0 && progress.GetPercent() != 100 {
		t.Fatalf("percent = %d, want 100", progress.GetPercent())
	}

	node := resp.GetNodes()[0]
	if !node.GetReady() {
		t.Fatal("expected node ready after provisioning")
	}
	if !node.GetPostgresInstalled() || !node.GetPostgresDataInitialized() {
		t.Fatal("expected postgres installed and data initialized")
	}
}

func completeAllPendingCommands(ctx context.Context, svc *ClusterService, nodeID string) error {
	cmds, err := svc.commands.ListByNodeIDs(ctx, []string{nodeID}, 1000)
	if err != nil {
		return err
	}
	for _, c := range cmds {
		if c.Status != models.CommandStatusPending {
			continue
		}
		if err := svc.commands.UpdateResult(ctx, c.ID, true, "", ""); err != nil {
			return err
		}
	}
	return nil
}
