package server

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
)

// newPostgresManagementServiceWithSecrets wires the cluster and command secret
// repositories into the management service so management commands attach the
// durable skylex_admin password. All repos share one encrypt key, matching the
// production wiring where roleEncryptKey is used uniformly.
func newPostgresManagementServiceWithSecrets(t *testing.T) (*db.DB, *PostgresManagementService, *db.NodeRepository, *db.ClusterSecretRepository, *db.AgentCommandSecretRepository) {
	t.Helper()
	database, svc, _, nodes := newPostgresManagementServiceTestDeps(t)
	key := []byte("12345678901234567890123456789012")
	clusterSecrets := db.NewClusterSecretRepository(database.Conn(), svc.log, key)
	commandSecrets := db.NewAgentCommandSecretRepository(database.Conn(), svc.log, key)
	svc.SetClusterSecretRepository(clusterSecrets)
	svc.SetCommandSecretRepository(commandSecrets)
	return database, svc, nodes, clusterSecrets, commandSecrets
}

func newReadyPrimary(t *testing.T, ctx context.Context, nodes *db.NodeRepository, clusterID, agentID string) *models.Node {
	t.Helper()
	node, err := nodes.Create(ctx, clusterID, "primary", "10.0.0.1", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create primary node: %v", err)
	}
	if err := nodes.UpdateAgentID(ctx, node.ID, agentID); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	if err := nodes.UpdatePostgresStatus(ctx, node.ID, true, "16", true); err != nil {
		t.Fatalf("update postgres status: %v", err)
	}
	return node
}

// findPendingByAction returns the first pending command for the given action.
func findPendingByAction(t *testing.T, ctx context.Context, database *db.DB, log, agentID, nodeID, action string) *db.AgentCommand {
	t.Helper()
	commands := db.NewAgentCommandRepository(database.Conn(), nil)
	pending, err := commands.ListPending(ctx, agentID, nodeID)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	for _, c := range pending {
		if c.Action == action {
			return c
		}
	}
	t.Fatalf("expected a %q command to be queued, got %#v", action, pending)
	return nil
}

func TestApplyExtensions_AttachesSkylexAdminSecret(t *testing.T) {
	database, svc, nodes, clusterSecrets, commandSecrets := newPostgresManagementServiceWithSecrets(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "ext-admin-secret")
	node := newReadyPrimary(t, ctx, nodes, clusterID, "agent-ext")

	if err := clusterSecrets.StoreSecret(ctx, clusterID, "skylex_admin_password", "admin-pw-1"); err != nil {
		t.Fatalf("store cluster secret: %v", err)
	}
	if _, err := svc.SetExtension(ctx, connect.NewRequest(&skylexv1.SetExtensionRequest{
		ClusterId: clusterID, Name: "pg_trgm", Enabled: true,
	})); err != nil {
		t.Fatalf("set extension: %v", err)
	}
	if _, err := svc.ApplyExtensions(ctx, connect.NewRequest(&skylexv1.ApplyExtensionsRequest{ClusterId: clusterID})); err != nil {
		t.Fatalf("apply extensions: %v", err)
	}

	cmd := findPendingByAction(t, ctx, database, "", "agent-ext", node.ID, "pg_apply_extensions")
	secret, err := commandSecrets.ResolveSecret(ctx, cmd.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve command secret: %v", err)
	}
	if secret != "admin-pw-1" {
		t.Fatalf("expected skylex_admin_password %q on pg_apply_extensions, got %q", "admin-pw-1", secret)
	}
}

func TestApplyExtensions_MissingClusterSecretQueuesWithoutAdminSecret(t *testing.T) {
	database, svc, nodes, _, commandSecrets := newPostgresManagementServiceWithSecrets(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "ext-no-secret")
	node := newReadyPrimary(t, ctx, nodes, clusterID, "agent-ext-2")

	// No cluster secret stored (cluster predates Phase 2).
	if _, err := svc.SetExtension(ctx, connect.NewRequest(&skylexv1.SetExtensionRequest{
		ClusterId: clusterID, Name: "pg_trgm", Enabled: true,
	})); err != nil {
		t.Fatalf("set extension: %v", err)
	}
	if _, err := svc.ApplyExtensions(ctx, connect.NewRequest(&skylexv1.ApplyExtensionsRequest{ClusterId: clusterID})); err != nil {
		t.Fatalf("apply extensions: %v", err)
	}

	cmd := findPendingByAction(t, ctx, database, "", "agent-ext-2", node.ID, "pg_apply_extensions")
	secret, err := commandSecrets.ResolveSecret(ctx, cmd.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve command secret: %v", err)
	}
	if secret != "" {
		t.Fatalf("expected no skylex_admin_password when cluster secret is missing, got %q", secret)
	}
}

func TestCreateRole_AttachesBothPasswordAndSkylexAdminSecret(t *testing.T) {
	database, svc, nodes, clusterSecrets, commandSecrets := newPostgresManagementServiceWithSecrets(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "role-admin-secret")
	node := newReadyPrimary(t, ctx, nodes, clusterID, "agent-role")

	if err := clusterSecrets.StoreSecret(ctx, clusterID, "skylex_admin_password", "admin-pw-2"); err != nil {
		t.Fatalf("store cluster secret: %v", err)
	}
	resp, err := svc.CreateRole(ctx, connect.NewRequest(&skylexv1.CreateRoleRequest{
		ClusterId: clusterID,
		RoleName:  "app_user",
		RoleKind:  "read_write",
	}))
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	cmd := findPendingByAction(t, ctx, database, "", "agent-role", node.ID, "pg_ensure_role")

	// The managed role's one-time password is carried under "password".
	rolePW, err := commandSecrets.ResolveSecret(ctx, cmd.ID, "password")
	if err != nil {
		t.Fatalf("resolve role password secret: %v", err)
	}
	if rolePW == "" || rolePW != resp.Msg.GetOneTimePassword() {
		t.Fatalf("expected role password secret to match the one-time password")
	}

	// The connection identity is carried separately under skylex_admin_password.
	adminPW, err := commandSecrets.ResolveSecret(ctx, cmd.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve admin secret: %v", err)
	}
	if adminPW != "admin-pw-2" {
		t.Fatalf("expected skylex_admin_password %q on pg_ensure_role, got %q", "admin-pw-2", adminPW)
	}
}

func TestCreateDatabase_AttachesSkylexAdminSecret(t *testing.T) {
	database, svc, nodes, clusterSecrets, commandSecrets := newPostgresManagementServiceWithSecrets(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "db-admin-secret")
	node := newReadyPrimary(t, ctx, nodes, clusterID, "agent-db")

	if err := clusterSecrets.StoreSecret(ctx, clusterID, "skylex_admin_password", "admin-pw-3"); err != nil {
		t.Fatalf("store cluster secret: %v", err)
	}
	if _, err := svc.CreateDatabase(ctx, connect.NewRequest(&skylexv1.CreateDatabaseRequest{
		ClusterId:    clusterID,
		DatabaseName: "app_db",
	})); err != nil {
		t.Fatalf("create database: %v", err)
	}

	cmd := findPendingByAction(t, ctx, database, "", "agent-db", node.ID, "pg_ensure_database")
	secret, err := commandSecrets.ResolveSecret(ctx, cmd.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve command secret: %v", err)
	}
	if secret != "admin-pw-3" {
		t.Fatalf("expected skylex_admin_password %q on pg_ensure_database, got %q", "admin-pw-3", secret)
	}
}
