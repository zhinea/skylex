package server

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

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

func TestNodeService_ResolveInstallationConflictAdoptInitializedNativeSkipsInitAndStart(t *testing.T) {
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
		Name: "adopt-initialized-native",
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
	if err := svc.commands.MarkPendingFailedByNodeIDs(ctx, []string{node.ID}, []string{"pg_preflight"}, "test setup"); err != nil {
		t.Fatalf("clear pending preflight: %v", err)
	}
	conflictDetails := "existing PostgreSQL/data detected: version=PostgreSQL 16.14 data_dir=/var/lib/postgresql/data data_present=true data_initialized=true"
	if err := svc.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateConflict, conflictDetails); err != nil {
		t.Fatalf("mark conflict: %v", err)
	}

	nodeSvc := NewNodeService(svc.nodes, svc.clusters, svc.commands, db.NewCommandLogRepository(svc.conn, svc.log), 30*time.Second, svc.log)
	secrets := db.NewAgentCommandSecretRepository(svc.conn, svc.log, []byte("12345678901234567890123456789012"))
	nodeSvc.SetCommandSecretRepository(secrets)
	_, err = nodeSvc.ResolveInstallationConflict(ctx, &skylexv1.ResolveInstallationConflictRequest{
		NodeId:                node.ID,
		Action:                skylexv1.ResolveInstallationConflictAction_RESOLVE_INSTALLATION_CONFLICT_ACTION_ADOPT,
		PostgresAdminUser:     "postgres",
		PostgresAdminPassword: "admin-secret",
	})
	if err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}

	got := queuedActions(t, ctx, svc, "agent-native", node.ID)
	want := []string{"pg_adopt_native", "pg_create_repl_user"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("adopt actions = %v, want %v", got, want)
	}
	pending, err := svc.commands.ListPending(ctx, "agent-native", node.ID)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	for _, cmd := range pending {
		if cmd.Action != "pg_adopt_native" && cmd.Action != "pg_create_repl_user" {
			continue
		}
		if cmd.Payload == "" {
			t.Fatalf("expected credential payload for %s", cmd.Action)
		}
		var payload map[string]string
		if err := json.Unmarshal([]byte(cmd.Payload), &payload); err != nil {
			t.Fatalf("unmarshal %s payload: %v", cmd.Action, err)
		}
		if payload["postgres_admin_user"] != "postgres" || payload["password_secret_key"] != "postgres_admin_password" {
			t.Fatalf("unexpected credential payload for %s: %#v", cmd.Action, payload)
		}
		secret, err := secrets.ResolveSecret(ctx, cmd.ID, "postgres_admin_password")
		if err != nil {
			t.Fatalf("resolve command secret for %s: %v", cmd.Action, err)
		}
		if secret != "admin-secret" {
			t.Fatalf("unexpected command secret for %s: %q", cmd.Action, secret)
		}
	}
}

func TestNodeService_ResolveInstallationConflictAdoptRequiresCredentials(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()
	node, err := svc.nodes.Create(ctx, "", "native-node", "10.0.0.2", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := svc.nodes.UpdateAgentID(ctx, node.ID, "agent-native"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	if _, err := svc.CreateCluster(ctx, &skylexv1.CreateClusterRequest{
		Name: "adopt-requires-creds",
		Config: &skylexv1.ClusterConfig{
			Engine:          skylexv1.Engine_ENGINE_POSTGRESQL,
			Version:         "16",
			ReplicationMode: skylexv1.ReplicationMode_REPLICATION_MODE_ASYNC,
			ServiceLocation: skylexv1.ServiceLocation_SERVICE_LOCATION_NATIVE,
		},
		NodeIds: []string{node.ID},
	}); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := svc.commands.MarkPendingFailedByNodeIDs(ctx, []string{node.ID}, []string{"pg_preflight"}, "test setup"); err != nil {
		t.Fatalf("clear pending preflight: %v", err)
	}
	if err := svc.nodes.UpdateInstallationState(ctx, node.ID, models.InstallationStateConflict, "data_initialized=true"); err != nil {
		t.Fatalf("mark conflict: %v", err)
	}

	nodeSvc := NewNodeService(svc.nodes, svc.clusters, svc.commands, db.NewCommandLogRepository(svc.conn, svc.log), 30*time.Second, svc.log)
	_, err = nodeSvc.ResolveInstallationConflict(ctx, &skylexv1.ResolveInstallationConflictRequest{
		NodeId: node.ID,
		Action: skylexv1.ResolveInstallationConflictAction_RESOLVE_INSTALLATION_CONFLICT_ACTION_ADOPT,
	})
	if err == nil {
		t.Fatal("expected missing credential error")
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

func TestClusterService_GetClusterSettings_ReturnsDefaults(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()
	nodeID := createIdleTestNode(t, ctx, svc)
	clusterID := createTestCluster(t, ctx, svc, nodeID)

	settings, err := svc.GetClusterSettings(ctx, &skylexv1.GetClusterSettingsRequest{ClusterId: clusterID})
	if err != nil {
		t.Fatalf("get cluster settings: %v", err)
	}
	got := settings.GetSettings().GetParameters()
	for key, want := range defaultClusterSettings {
		if got[key] != want {
			t.Fatalf("expected default %s=%q, got %q", key, want, got[key])
		}
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

func TestClusterService_DeleteCluster_UnassignsNodes(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()

	nodeID := createIdleTestNode(t, ctx, svc)
	clusterID := createTestCluster(t, ctx, svc, nodeID)

	if _, err := svc.DeleteCluster(ctx, &skylexv1.DeleteClusterRequest{Id: clusterID}); err != nil {
		t.Fatalf("delete cluster: %v", err)
	}

	node, err := svc.nodes.GetByID(ctx, nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if node == nil {
		t.Fatal("expected node to still exist after cluster deletion")
	}
	if node.ClusterID != "" {
		t.Fatalf("expected node to be unassigned, got cluster_id %q", node.ClusterID)
	}
	if node.Role != "" {
		t.Fatalf("expected node role to be reset, got %q", node.Role)
	}
	if node.InstallationState != "" {
		t.Fatalf("expected installation_state to be reset, got %q", node.InstallationState)
	}
	if node.ServiceLocation != "" {
		t.Fatalf("expected service_location to be reset, got %q", node.ServiceLocation)
	}

	// The unassigned node should be selectable for a new cluster.
	_, err = svc.CreateCluster(ctx, &skylexv1.CreateClusterRequest{
		Name: "reused-node-cluster",
		Config: &skylexv1.ClusterConfig{
			Engine:          skylexv1.Engine_ENGINE_POSTGRESQL,
			Version:         "16",
			ReplicationMode: skylexv1.ReplicationMode_REPLICATION_MODE_ASYNC,
			ReplicaCount:    0,
		},
		NodeIds: []string{nodeID},
	})
	if err != nil {
		t.Fatalf("recreate cluster with reused node: %v", err)
	}
}

func createReadyLifecycleCluster(t *testing.T, ctx context.Context, svc *ClusterService) (string, string) {
	t.Helper()
	nodeID := createIdleTestNode(t, ctx, svc)
	clusterID := createTestCluster(t, ctx, svc, nodeID)
	if err := svc.nodes.UpdateInstallationState(ctx, nodeID, models.InstallationStateInstalled, ""); err != nil {
		t.Fatalf("update installation state: %v", err)
	}
	if err := svc.nodes.UpdatePostgresStatus(ctx, nodeID, true, "16", true); err != nil {
		t.Fatalf("update postgres status: %v", err)
	}
	if err := svc.nodes.UpdateStatus(ctx, nodeID, models.NodeStatusOffline); err != nil {
		t.Fatalf("update node status: %v", err)
	}
	if err := svc.nodes.UpdateStatusDetail(ctx, nodeID, "stopped"); err != nil {
		t.Fatalf("update status detail: %v", err)
	}
	if err := svc.clusters.UpdateStatus(ctx, clusterID, models.ClusterStatusStopped); err != nil {
		t.Fatalf("update cluster status: %v", err)
	}
	if err := svc.commands.MarkPendingFailedByNodeIDs(ctx, []string{nodeID}, []string{"pg_preflight"}, "test setup"); err != nil {
		t.Fatalf("clear preflight command: %v", err)
	}
	return clusterID, nodeID
}

func TestClusterService_LifecycleRejectsViewer(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleViewer)
	clusterID, _ := createReadyLifecycleCluster(t, ctx, svc)

	if _, err := svc.StartCluster(ctx, &skylexv1.StartClusterRequest{ClusterId: clusterID}); err == nil {
		t.Fatal("expected viewer start cluster request to fail")
	}
	if _, err := svc.PauseCluster(ctx, &skylexv1.PauseClusterRequest{ClusterId: clusterID}); err == nil {
		t.Fatal("expected viewer pause cluster request to fail")
	}
	if _, err := svc.RestartCluster(ctx, &skylexv1.RestartClusterRequest{ClusterId: clusterID}); err == nil {
		t.Fatal("expected viewer restart cluster request to fail")
	}
}

func TestClusterService_LifecycleQueuesCommands(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID, nodeID := createReadyLifecycleCluster(t, ctx, svc)

	if _, err := svc.StartCluster(ctx, &skylexv1.StartClusterRequest{ClusterId: clusterID}); err != nil {
		t.Fatalf("start cluster: %v", err)
	}
	if got, want := queuedActions(t, ctx, svc, "agent-1", nodeID), []string{"pg_start"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("start actions = %v, want %v", got, want)
	}

	for _, cmd := range []string{"pg_start"} {
		if err := svc.commands.MarkPendingFailedByNodeIDs(ctx, []string{nodeID}, []string{cmd}, "test cleanup"); err != nil {
			t.Fatalf("clear pending %s: %v", cmd, err)
		}
	}
	if _, err := svc.PauseCluster(ctx, &skylexv1.PauseClusterRequest{ClusterId: clusterID}); err != nil {
		t.Fatalf("pause cluster: %v", err)
	}
	if got, want := queuedActions(t, ctx, svc, "agent-1", nodeID), []string{"pg_stop"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pause actions = %v, want %v", got, want)
	}

	if err := svc.commands.MarkPendingFailedByNodeIDs(ctx, []string{nodeID}, []string{"pg_stop"}, "test cleanup"); err != nil {
		t.Fatalf("clear pending stop: %v", err)
	}
	if _, err := svc.RestartCluster(ctx, &skylexv1.RestartClusterRequest{ClusterId: clusterID}); err != nil {
		t.Fatalf("restart cluster: %v", err)
	}
	if got, want := queuedActions(t, ctx, svc, "agent-1", nodeID), []string{"pg_stop", "pg_start"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restart actions = %v, want %v", got, want)
	}
}

func TestClusterService_DeleteClusterRequiresStoppedService(t *testing.T) {
	_, svc := newClusterServiceTestDeps(t)
	ctx := context.Background()
	clusterID, nodeID := createReadyLifecycleCluster(t, ctx, svc)
	if err := svc.nodes.UpdateStatus(ctx, nodeID, models.NodeStatusOnline); err != nil {
		t.Fatalf("mark node online: %v", err)
	}
	if err := svc.nodes.UpdateStatusDetail(ctx, nodeID, "healthy"); err != nil {
		t.Fatalf("mark node healthy: %v", err)
	}
	if err := svc.clusters.UpdateStatus(ctx, clusterID, models.ClusterStatusRunning); err != nil {
		t.Fatalf("mark cluster running: %v", err)
	}

	if _, err := svc.DeleteCluster(ctx, &skylexv1.DeleteClusterRequest{Id: clusterID}); err == nil {
		t.Fatal("expected running cluster deletion to fail")
	}

	if err := svc.nodes.UpdateStatus(ctx, nodeID, models.NodeStatusOffline); err != nil {
		t.Fatalf("mark node offline: %v", err)
	}
	if err := svc.nodes.UpdateStatusDetail(ctx, nodeID, "stopped"); err != nil {
		t.Fatalf("mark node stopped: %v", err)
	}
	if err := svc.clusters.UpdateStatus(ctx, clusterID, models.ClusterStatusStopped); err != nil {
		t.Fatalf("mark cluster stopped: %v", err)
	}
	if _, err := svc.DeleteCluster(ctx, &skylexv1.DeleteClusterRequest{Id: clusterID}); err != nil {
		t.Fatalf("delete stopped cluster: %v", err)
	}
}
