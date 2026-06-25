package server

import (
	"testing"

	connect "connectrpc.com/connect"
	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
)

func TestExtensions_GetReturnsCatalogAllOff(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "ext-get")

	resp, err := svc.GetExtensions(ctx, connect.NewRequest(&skylexv1.GetExtensionsRequest{ClusterId: clusterID}))
	if err != nil {
		t.Fatalf("get extensions: %v", err)
	}
	exts := resp.Msg.GetExtensions()
	if len(exts) == 0 {
		t.Fatal("expected catalog to be non-empty")
	}
	for _, e := range exts {
		if e.GetEnabled() || e.GetStatus() != "off" {
			t.Fatalf("expected %q off by default, got enabled=%v status=%q", e.GetName(), e.GetEnabled(), e.GetStatus())
		}
	}
}

func TestExtensions_SetRejectsViewer(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	_, err := svc.SetExtension(contextWithUserRole(models.RoleViewer), connect.NewRequest(&skylexv1.SetExtensionRequest{
		ClusterId: "cluster-1", Name: "pg_trgm", Enabled: true,
	}))
	if err == nil {
		t.Fatal("expected viewer set extension to be rejected")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", connect.CodeOf(err))
	}
}

func TestExtensions_SetRejectsUnknownName(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "ext-bad-name")

	_, err := svc.SetExtension(ctx, connect.NewRequest(&skylexv1.SetExtensionRequest{
		ClusterId: clusterID, Name: "definitely_not_real", Enabled: true,
	}))
	if err == nil {
		t.Fatal("expected unknown extension name to be rejected")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestExtensions_RejectsUnregisteredEngine(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	cluster, err := svc.clusters.Create(ctx, "mysql-ext", "", "/var/lib/mysql", models.EngineType("mysql"), "8", models.ReplicationAsync, 0, false, nil)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	_, err = svc.GetExtensions(ctx, connect.NewRequest(&skylexv1.GetExtensionsRequest{ClusterId: cluster.ID}))
	if err == nil {
		t.Fatal("expected unregistered engine to be rejected")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", connect.CodeOf(err))
	}
}

func TestExtensions_SetPersistsAndApplyQueuesCommand(t *testing.T) {
	database, svc, _, nodes := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "ext-apply")
	node, err := nodes.Create(ctx, clusterID, "primary", "10.0.0.1", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(ctx, node.ID, "agent-ext"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	if err := nodes.UpdatePostgresStatus(ctx, node.ID, true, "16", true); err != nil {
		t.Fatalf("update pg status: %v", err)
	}

	// Toggle one extension on; it must persist as enabled with status off.
	setResp, err := svc.SetExtension(ctx, connect.NewRequest(&skylexv1.SetExtensionRequest{
		ClusterId: clusterID, Name: "pg_trgm", Enabled: true,
	}))
	if err != nil {
		t.Fatalf("set extension: %v", err)
	}
	if !setResp.Msg.GetExtension().GetEnabled() || setResp.Msg.GetExtension().GetStatus() != "off" {
		t.Fatalf("expected pg_trgm enabled+off, got %+v", setResp.Msg.GetExtension())
	}

	// Apply queues exactly one pg_apply_extensions command on the primary.
	if _, err := svc.ApplyExtensions(ctx, connect.NewRequest(&skylexv1.ApplyExtensionsRequest{ClusterId: clusterID})); err != nil {
		t.Fatalf("apply extensions: %v", err)
	}
	commands := db.NewAgentCommandRepository(database.Conn(), svc.log)
	pending, err := commands.ListPending(ctx, "agent-ext", node.ID)
	if err != nil {
		t.Fatalf("get pending: %v", err)
	}
	found := false
	for _, c := range pending {
		if c.Action == "pg_apply_extensions" {
			found = true
			if c.NodeID != node.ID {
				t.Fatalf("expected command on primary node %q, got %q", node.ID, c.NodeID)
			}
		}
	}
	if !found {
		t.Fatal("expected a pg_apply_extensions command to be queued")
	}
}

func TestExtensions_ApplyWithoutTogglesFails(t *testing.T) {
	_, svc, _, nodes := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "ext-empty")
	node, err := nodes.Create(ctx, clusterID, "primary", "10.0.0.1", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(ctx, node.ID, "agent-empty"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	if err := nodes.UpdatePostgresStatus(ctx, node.ID, true, "16", true); err != nil {
		t.Fatalf("update pg status: %v", err)
	}

	_, err = svc.ApplyExtensions(ctx, connect.NewRequest(&skylexv1.ApplyExtensionsRequest{ClusterId: clusterID}))
	if err == nil {
		t.Fatal("expected apply with no toggles to fail")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", connect.CodeOf(err))
	}
}
