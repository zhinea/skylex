package server

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
)

func newPostgresManagementServiceTestDeps(t *testing.T) (*db.DB, *PostgresManagementService, *db.PostgresRoleRepository, *db.NodeRepository) {
	t.Helper()
	database, log := newTestDeps(t)
	conn := database.Conn()
	profiles := db.NewConnectionProfileRepository(conn, log)
	clusters := db.NewClusterRepository(conn, log)
	nodes := db.NewNodeRepository(conn, log)
	roles := db.NewPostgresRoleRepository(conn, log)
	databases := db.NewPostgresDatabaseRepository(conn, log)
	svc := NewPostgresManagementService(profiles, nodes, clusters, roles, databases, []byte("12345678901234567890123456789012"), log)
	return database, svc, roles, nodes
}

func contextWithUserRole(role models.Role) context.Context {
	return context.WithValue(context.Background(), ctxKeyUserRole, role)
}

func createPostgresManagementTestCluster(t *testing.T, ctx context.Context, svc *PostgresManagementService, name string) string {
	t.Helper()
	cluster, err := svc.clusters.Create(ctx, name, "", "/var/lib/postgresql/data", models.EnginePostgreSQL, "16", models.ReplicationAsync, 0, false, nil)
	if err != nil {
		t.Fatalf("create test cluster: %v", err)
	}
	return cluster.ID
}

func TestPostgresManagementService_CreateDatabaseRejectsViewer(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)

	_, err := svc.CreateDatabase(contextWithUserRole(models.RoleViewer), connect.NewRequest(&skylexv1.CreateDatabaseRequest{
		ClusterId:    "cluster-1",
		DatabaseName: "app_db",
	}))
	if err == nil {
		t.Fatal("expected viewer create database request to fail")
	}
}

func TestPostgresManagementService_CreateDatabaseRejectsInvalidAndReservedNames(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)

	for _, name := range []string{"postgres", "template1", "bad-name"} {
		_, err := svc.CreateDatabase(ctx, connect.NewRequest(&skylexv1.CreateDatabaseRequest{
			ClusterId:    "cluster-1",
			DatabaseName: name,
		}))
		if err == nil {
			t.Fatalf("expected database name %q to be rejected", name)
		}
	}
}

func TestPostgresManagementService_CreateDatabaseQueuesEnsureCommand(t *testing.T) {
	database, svc, roles, nodes := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "managed-db-service")
	node, err := nodes.Create(ctx, clusterID, "primary", "10.0.0.1", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create primary node: %v", err)
	}
	if err := nodes.UpdateAgentID(ctx, node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	if err := nodes.UpdatePostgresStatus(ctx, node.ID, true, "16", true); err != nil {
		t.Fatalf("update postgres status: %v", err)
	}
	role, err := roles.Create(ctx, clusterID, "app_user", "read_write", "ciphertext", nil)
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := roles.UpdateStatus(ctx, role.ID, "ready"); err != nil {
		t.Fatalf("mark role ready: %v", err)
	}

	resp, err := svc.CreateDatabase(ctx, connect.NewRequest(&skylexv1.CreateDatabaseRequest{
		ClusterId:    clusterID,
		DatabaseName: "app_db",
		OwnerRoleId:  role.ID,
	}))
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	if resp.Msg.GetDatabase().GetOwnerRoleName() != "app_user" {
		t.Fatalf("expected owner role name app_user, got %q", resp.Msg.GetDatabase().GetOwnerRoleName())
	}

	commands := db.NewAgentCommandRepository(database.Conn(), svc.log)
	pending, err := commands.ListPending(ctx, "agent-1", node.ID)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	found := false
	for _, command := range pending {
		if command.Action == "pg_ensure_database" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pg_ensure_database command, got %#v", pending)
	}
}
