package server

import (
	"context"
	"encoding/json"
	"testing"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
)

func newClusterServiceTestDeps(t *testing.T) (*db.DB, *ClusterService) {
	t.Helper()
	database, log := newTestDeps(t)
	conn := database.Conn()

	clusters := db.NewClusterRepository(conn, log)
	nodes := db.NewNodeRepository(conn, log)
	commands := db.NewAgentCommandRepository(conn, log)
	settings := db.NewClusterSettingsRepository(conn, log)

	svc := NewClusterService(conn, clusters, nodes, commands, settings, log)
	return database, svc
}

func createTestCluster(t *testing.T, ctx context.Context, svc *ClusterService, nodeID string) string {
	t.Helper()
	resp, err := svc.CreateCluster(ctx, &skylexv1.CreateClusterRequest{
		Name: "test-settings-cluster",
		Config: &skylexv1.ClusterConfig{
			Engine:          skylexv1.Engine_ENGINE_POSTGRESQL,
			Version:         "16",
			ReplicationMode: skylexv1.ReplicationMode_REPLICATION_MODE_ASYNC,
			ReplicaCount:    0,
			PitrEnabled:     false,
		},
		NodeIds: []string{nodeID},
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	return resp.GetCluster().GetId()
}

func createIdleTestNode(t *testing.T, ctx context.Context, svc *ClusterService) string {
	t.Helper()
	node, err := svc.nodes.Create(ctx, "", "node-1", "10.0.0.1", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create idle node: %v", err)
	}
	if err := svc.nodes.UpdateAgentID(ctx, node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	// Phase 4: test nodes must have PostgreSQL installed to pass preflight.
	if err := svc.nodes.UpdatePostgresStatus(ctx, node.ID, true, "16", false); err != nil {
		t.Fatalf("update postgres status: %v", err)
	}
	return node.ID
}

func queuedActions(t *testing.T, ctx context.Context, svc *ClusterService, agentID, nodeID string) []string {
	t.Helper()
	pending, err := svc.commands.ListPending(ctx, agentID, nodeID)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	actions := make([]string, 0, len(pending))
	for _, cmd := range pending {
		actions = append(actions, cmd.Action)
	}
	return actions
}

func TestClusterService_CreateCluster_QueuesNativePreflightOnly(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()
	node, err := svc.nodes.Create(ctx, "", "native-node", "10.0.0.2", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := svc.nodes.UpdateAgentID(ctx, node.ID, "agent-native"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}

	_, err = svc.CreateCluster(ctx, &skylexv1.CreateClusterRequest{
		Name: "native-install-cluster",
		Config: &skylexv1.ClusterConfig{
			Engine:          skylexv1.Engine_ENGINE_POSTGRESQL,
			Version:         "16",
			ReplicationMode: skylexv1.ReplicationMode_REPLICATION_MODE_ASYNC,
			ReplicaCount:    0,
			ServiceLocation: skylexv1.ServiceLocation_SERVICE_LOCATION_NATIVE,
		},
		NodeIds: []string{node.ID},
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	want := []string{"pg_preflight"}
	got := queuedActions(t, ctx, svc, "agent-native", node.ID)
	if len(got) != len(want) {
		t.Fatalf("expected actions %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected actions %v, got %v", want, got)
		}
	}
}

func TestClusterService_CreateCluster_QueuesDockerInstallWithoutNativePreflight(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()
	node, err := svc.nodes.Create(ctx, "", "docker-node", "10.0.0.3", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := svc.nodes.UpdateAgentID(ctx, node.ID, "agent-docker"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}

	resp, err := svc.CreateCluster(ctx, &skylexv1.CreateClusterRequest{
		Name: "docker-install-cluster",
		Config: &skylexv1.ClusterConfig{
			Engine:          skylexv1.Engine_ENGINE_POSTGRESQL,
			Version:         "16",
			ReplicationMode: skylexv1.ReplicationMode_REPLICATION_MODE_ASYNC,
			ReplicaCount:    0,
			ServiceLocation: skylexv1.ServiceLocation_SERVICE_LOCATION_DOCKER,
		},
		NodeIds: []string{node.ID},
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	want := []string{"pg_install_docker", "pg_init", "pg_start", "pg_create_repl_user"}
	got := queuedActions(t, ctx, svc, "agent-docker", node.ID)
	if len(got) != len(want) {
		t.Fatalf("expected actions %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected actions %v, got %v", want, got)
		}
	}

	// Verify JSON payload contains cluster_id and version.
	pending, err := svc.commands.ListPending(ctx, "agent-docker", node.ID)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) > 0 && pending[0].Action == "pg_install_docker" {
		var payload map[string]string
		if err := json.Unmarshal([]byte(pending[0].Payload), &payload); err != nil {
			t.Fatalf("expected JSON payload, got %q: %v", pending[0].Payload, err)
		}
		if payload["cluster_id"] != resp.GetCluster().GetId() {
			t.Fatalf("expected cluster_id %q, got %q", resp.GetCluster().GetId(), payload["cluster_id"])
		}
		if payload["version"] != "16" {
			t.Fatalf("expected version '16', got %q", payload["version"])
		}
	}
}

func TestClusterService_UpdateClusterSettings_RejectInvalidKey(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()
	nodeID := createIdleTestNode(t, ctx, svc)
	clusterID := createTestCluster(t, ctx, svc, nodeID)

	_, err := svc.UpdateClusterSettings(ctx, &skylexv1.UpdateClusterSettingsRequest{
		ClusterId: clusterID,
		Settings: &skylexv1.ClusterSettings{
			Parameters: map[string]string{"invalid_random_param": "123"},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid setting key")
	}
}

func TestClusterService_UpdateClusterSettings_RejectInvalidValue(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()
	nodeID := createIdleTestNode(t, ctx, svc)
	clusterID := createTestCluster(t, ctx, svc, nodeID)

	_, err := svc.UpdateClusterSettings(ctx, &skylexv1.UpdateClusterSettingsRequest{
		ClusterId: clusterID,
		Settings: &skylexv1.ClusterSettings{
			Parameters: map[string]string{"max_connections": "not-a-number"},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid max_connections value")
	}
}

func TestClusterService_UpdateClusterSettings_PersistsAndQueuesApply(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()
	nodeID := createIdleTestNode(t, ctx, svc)
	clusterID := createTestCluster(t, ctx, svc, nodeID)

	params := map[string]string{
		"max_connections": "250",
		"shared_buffers":  "256MB",
		"work_mem":        "8MB",
	}

	_, err := svc.UpdateClusterSettings(ctx, &skylexv1.UpdateClusterSettingsRequest{
		ClusterId: clusterID,
		Settings:  &skylexv1.ClusterSettings{Parameters: params},
	})
	if err != nil {
		t.Fatalf("update cluster settings: %v", err)
	}

	settings, err := svc.GetClusterSettings(ctx, &skylexv1.GetClusterSettingsRequest{ClusterId: clusterID})
	if err != nil {
		t.Fatalf("get cluster settings: %v", err)
	}
	got := settings.GetSettings().GetParameters()
	for k, v := range params {
		if got[k] != v {
			t.Fatalf("expected %s=%q, got %q", k, v, got[k])
		}
	}

	// Find the assigned node to verify the apply command was queued.
	nodes, _, err := svc.nodes.ListByCluster(ctx, clusterID, 0, 10)
	if err != nil {
		t.Fatalf("list cluster nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 cluster node, got %d", len(nodes))
	}
	assignedNodeID := nodes[0].ID

	pending, err := svc.commands.ListPending(ctx, "agent-1", assignedNodeID)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}

	var applyCmd *db.AgentCommand
	for _, c := range pending {
		if c.Action == "pg_apply_settings" {
			applyCmd = c
			break
		}
	}
	if applyCmd == nil {
		t.Fatalf("expected pg_apply_settings command among pending: %+v", pending)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(applyCmd.Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload) != len(params) {
		t.Fatalf("expected payload length %d, got %d", len(params), len(payload))
	}
}
