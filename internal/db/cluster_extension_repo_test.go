package db

import (
	"context"
	"testing"

	_ "github.com/zhinea/skylex/internal/engine/postgres"
	"github.com/zhinea/skylex/internal/models"
)

// seedExtensionCluster creates a cluster + primary node + linked agent so the
// extension repo's foreign keys resolve.
func seedExtensionCluster(t *testing.T, ctx context.Context, conn *clusterExtensionTestDeps) (clusterID, nodeID, agentID string) {
	t.Helper()
	cluster, err := conn.clusters.Create(ctx, "ext-cluster", "", "/var/lib/postgresql/data", models.EnginePostgreSQL, "16", models.ReplicationAsync, 0, false, nil)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node, err := conn.nodes.Create(ctx, cluster.ID, "primary", "10.0.0.1", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := conn.nodes.UpdateAgentID(ctx, node.ID, "agent-ext-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	return cluster.ID, node.ID, "agent-ext-1"
}

type clusterExtensionTestDeps struct {
	clusters *ClusterRepository
	nodes    *NodeRepository
	ext      *ClusterExtensionRepository
}

func newClusterExtensionDeps(t *testing.T) (*clusterExtensionTestDeps, context.Context) {
	t.Helper()
	database, log := newTestDB(t)
	conn := database.Conn()
	return &clusterExtensionTestDeps{
		clusters: NewClusterRepository(conn, log),
		nodes:    NewNodeRepository(conn, log),
		ext:      NewClusterExtensionRepository(conn, log),
	}, context.Background()
}

func TestClusterExtensions_SetEnabledAndList(t *testing.T) {
	deps, ctx := newClusterExtensionDeps(t)
	clusterID, _, _ := seedExtensionCluster(t, ctx, deps)

	// Default: no rows.
	exts, err := deps.ext.ListByCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(exts) != 0 {
		t.Fatalf("expected no extensions by default, got %d", len(exts))
	}

	// Toggle on.
	got, err := deps.ext.SetEnabled(ctx, clusterID, "pg_trgm", true)
	if err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	if !got.Enabled || got.Status != "off" {
		t.Fatalf("expected enabled=true status=off, got %+v", got)
	}

	// Toggling again (idempotent upsert) flips state without duplicating rows.
	if _, err := deps.ext.SetEnabled(ctx, clusterID, "pg_trgm", false); err != nil {
		t.Fatalf("set disabled: %v", err)
	}
	exts, err = deps.ext.ListByCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(exts) != 1 {
		t.Fatalf("expected exactly 1 row after re-toggle, got %d", len(exts))
	}
	if exts[0].Enabled {
		t.Fatalf("expected pg_trgm disabled after re-toggle")
	}
}

func TestClusterExtensions_QueueApplyMarksPending(t *testing.T) {
	deps, ctx := newClusterExtensionDeps(t)
	clusterID, nodeID, agentID := seedExtensionCluster(t, ctx, deps)

	if _, err := deps.ext.SetEnabled(ctx, clusterID, "pg_trgm", true); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	if _, err := deps.ext.SetEnabled(ctx, clusterID, "citext", false); err != nil {
		t.Fatalf("set disabled: %v", err)
	}

	err := deps.ext.QueueApplyCommand(ctx, ApplyExtensionsCommand{
		ClusterID: clusterID,
		NodeID:    nodeID,
		AgentID:   agentID,
		CommandID: "cmd-ext-1",
		Payload:   `{"cluster_id":"` + clusterID + `"}`,
	})
	if err != nil {
		t.Fatalf("queue apply: %v", err)
	}

	exts, err := deps.ext.ListByCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range exts {
		if e.Status != "pending" {
			t.Fatalf("expected %q pending, got %q", e.ExtensionName, e.Status)
		}
		if e.CommandID != "cmd-ext-1" {
			t.Fatalf("expected command_id tied, got %q", e.CommandID)
		}
	}
}

func TestClusterExtensions_HandleResultSuccess(t *testing.T) {
	deps, ctx := newClusterExtensionDeps(t)
	clusterID, nodeID, agentID := seedExtensionCluster(t, ctx, deps)

	if _, err := deps.ext.SetEnabled(ctx, clusterID, "pg_trgm", true); err != nil {
		t.Fatalf("enable pg_trgm: %v", err)
	}
	if _, err := deps.ext.SetEnabled(ctx, clusterID, "citext", false); err != nil {
		t.Fatalf("disable citext: %v", err)
	}
	if err := deps.ext.QueueApplyCommand(ctx, ApplyExtensionsCommand{
		ClusterID: clusterID, NodeID: nodeID, AgentID: agentID,
		CommandID: "cmd-ext-2", Payload: `{"cluster_id":"` + clusterID + `"}`,
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}

	handled, err := deps.ext.HandleCommandResult(ctx, "cmd-ext-2", true, "")
	if err != nil || !handled {
		t.Fatalf("handle result: handled=%v err=%v", handled, err)
	}

	exts, err := deps.ext.ListByCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Disabled citext row removed; enabled pg_trgm row ready.
	if len(exts) != 1 {
		t.Fatalf("expected 1 row (disabled removed), got %d", len(exts))
	}
	if exts[0].ExtensionName != "pg_trgm" || exts[0].Status != "ready" || exts[0].AppliedAt == nil {
		t.Fatalf("expected pg_trgm ready with applied_at, got %+v", exts[0])
	}
}

func TestClusterExtensions_HandleResultFailure(t *testing.T) {
	deps, ctx := newClusterExtensionDeps(t)
	clusterID, nodeID, agentID := seedExtensionCluster(t, ctx, deps)

	if _, err := deps.ext.SetEnabled(ctx, clusterID, "pg_trgm", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := deps.ext.QueueApplyCommand(ctx, ApplyExtensionsCommand{
		ClusterID: clusterID, NodeID: nodeID, AgentID: agentID,
		CommandID: "cmd-ext-3", Payload: `{"cluster_id":"` + clusterID + `"}`,
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}

	handled, err := deps.ext.HandleCommandResult(ctx, "cmd-ext-3", false, "contrib package missing")
	if err != nil || !handled {
		t.Fatalf("handle: handled=%v err=%v", handled, err)
	}
	exts, _ := deps.ext.ListByCluster(ctx, clusterID)
	if len(exts) != 1 || exts[0].Status != "failed" || exts[0].Error == "" {
		t.Fatalf("expected failed status with error, got %+v", exts)
	}
}

func TestClusterExtensions_HandleResultIgnoresUnknownCommand(t *testing.T) {
	deps, ctx := newClusterExtensionDeps(t)
	handled, err := deps.ext.HandleCommandResult(ctx, "does-not-exist", true, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if handled {
		t.Fatal("expected handled=false for unknown command")
	}
}
