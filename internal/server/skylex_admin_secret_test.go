package server

import (
	"context"
	"encoding/json"
	"log/slog"
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

// createReadyRole inserts a managed role directly via the repository and forces
// it to "ready". It bypasses CreateRole on purpose so it does not trigger the
// Phase 4 ensureSkylexAdminProvisioned backfill (which would store a cluster
// secret), letting the missing-secret test exercise a genuinely secret-less
// cluster.
func createReadyRole(t *testing.T, ctx context.Context, svc *PostgresManagementService, clusterID, roleName string) string {
	t.Helper()
	role, err := svc.roles.Create(ctx, clusterID, roleName, "read_write", "enc", nil)
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := svc.roles.UpdateStatus(ctx, role.ID, "ready"); err != nil {
		t.Fatalf("mark role ready: %v", err)
	}
	return role.ID
}

func TestDeleteRole_AttachesSkylexAdminSecret(t *testing.T) {
	database, svc, nodes, clusterSecrets, commandSecrets := newPostgresManagementServiceWithSecrets(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "drop-role-admin-secret")
	node := newReadyPrimary(t, ctx, nodes, clusterID, "agent-drop")

	if err := clusterSecrets.StoreSecret(ctx, clusterID, "skylex_admin_password", "admin-pw-4"); err != nil {
		t.Fatalf("store cluster secret: %v", err)
	}
	roleID := createReadyRole(t, ctx, svc, clusterID, "drop_me")

	if _, err := svc.DeleteRole(ctx, connect.NewRequest(&skylexv1.DeleteRoleRequest{RoleId: roleID})); err != nil {
		t.Fatalf("delete role: %v", err)
	}

	cmd := findPendingByAction(t, ctx, database, "", "agent-drop", node.ID, "pg_drop_role")
	secret, err := commandSecrets.ResolveSecret(ctx, cmd.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve command secret: %v", err)
	}
	if secret != "admin-pw-4" {
		t.Fatalf("expected skylex_admin_password %q on pg_drop_role, got %q", "admin-pw-4", secret)
	}
}

func TestDeleteRole_MissingClusterSecretQueuesWithoutAdminSecret(t *testing.T) {
	database, svc, nodes, _, commandSecrets := newPostgresManagementServiceWithSecrets(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "drop-role-no-secret")
	node := newReadyPrimary(t, ctx, nodes, clusterID, "agent-drop-2")

	// No cluster secret stored (cluster predates Phase 2). The drop must still be
	// queued, without an admin secret, and without panicking.
	roleID := createReadyRole(t, ctx, svc, clusterID, "drop_me")

	if _, err := svc.DeleteRole(ctx, connect.NewRequest(&skylexv1.DeleteRoleRequest{RoleId: roleID})); err != nil {
		t.Fatalf("delete role: %v", err)
	}

	cmd := findPendingByAction(t, ctx, database, "", "agent-drop-2", node.ID, "pg_drop_role")
	secret, err := commandSecrets.ResolveSecret(ctx, cmd.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve command secret: %v", err)
	}
	if secret != "" {
		t.Fatalf("expected no skylex_admin_password when cluster secret is missing, got %q", secret)
	}
}

// newAgentServiceWithSecrets wires an AgentService with the database, command
// and cluster secret stores plus the role encrypt key, so the server-side grant
// follow-up (handleDatabaseManagementCommandResult → QueueGrantCommand) attaches
// the durable skylex_admin secret exactly as production does. All repos share
// the same encrypt key, matching production wiring.
func newAgentServiceWithSecrets(t *testing.T, database *db.DB, log *slog.Logger) (*AgentService, *db.ClusterSecretRepository, *db.AgentCommandSecretRepository) {
	t.Helper()
	conn := database.Conn()
	key := []byte("12345678901234567890123456789012")
	clusterSecrets := db.NewClusterSecretRepository(conn, log, key)
	commandSecrets := db.NewAgentCommandSecretRepository(conn, log, key)
	svc := NewAgentService(&Config{Agent: AgentConfig{}},
		db.NewClusterRepository(conn, log), db.NewNodeRepository(conn, log),
		db.NewAgentCommandRepository(conn, log), db.NewCommandLogRepository(conn, log),
		db.NewAgentTokenRepository(conn, log), log)
	svc.SetCommandSecretRepository(commandSecrets)
	svc.SetClusterSecretRepository(clusterSecrets)
	svc.SetRoleEncryptKey(key)
	svc.SetPostgresDatabaseRepository(db.NewManagedDatabaseRepository(conn, log))
	return svc, clusterSecrets, commandSecrets
}

// queueEnsureDatabaseWithOwner inserts a managed database plus a pending
// pg_ensure_database command whose payload names an owner role, so that a
// successful result triggers the grant follow-up command.
func queueEnsureDatabaseWithOwner(t *testing.T, ctx context.Context, conn *db.ManagedDatabaseRepository, clusterID, nodeID, agentID string) (databaseID, operationID, commandID string) {
	t.Helper()
	databaseID = "db-" + agentID
	operationID = "op-" + agentID
	commandID = "cmd-" + agentID
	payload, err := json.Marshal(map[string]interface{}{
		"database_id":     databaseID,
		"operation_id":    operationID,
		"database_name":   "app_db",
		"owner_role_name": "app_owner",
		"owner_role_kind": "read_write",
		"allow_promote":   true,
	})
	if err != nil {
		t.Fatalf("marshal ensure payload: %v", err)
	}
	if _, err := conn.CreateWithCommand(ctx, db.CreateDatabaseTxInput{
		DatabaseID:   databaseID,
		OperationID:  operationID,
		CommandID:    commandID,
		ClusterID:    clusterID,
		NodeID:       nodeID,
		AgentID:      agentID,
		DatabaseName: "app_db",
		Payload:      string(payload),
		EnsureAction: "pg_ensure_database",
	}); err != nil {
		t.Fatalf("create database with command: %v", err)
	}
	return databaseID, operationID, commandID
}

func TestGrantFollowUp_AttachesSkylexAdminSecret(t *testing.T) {
	database, log := newTestDeps(t)
	svc, clusterSecrets, commandSecrets := newAgentServiceWithSecrets(t, database, log)
	ctx := context.Background()

	clusters := db.NewClusterRepository(database.Conn(), log)
	cluster, err := clusters.Create(ctx, "grant-secret", "", "/var/lib/postgresql/data", models.EnginePostgreSQL, "16", models.ReplicationAsync, 0, false, nil)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	nodes := db.NewNodeRepository(database.Conn(), log)
	node := newReadyPrimary(t, ctx, nodes, cluster.ID, "agent-grant")

	if err := clusterSecrets.StoreSecret(ctx, cluster.ID, "skylex_admin_password", "admin-pw-5"); err != nil {
		t.Fatalf("store cluster secret: %v", err)
	}
	_, _, ensureCmdID := queueEnsureDatabaseWithOwner(t, ctx, db.NewManagedDatabaseRepository(database.Conn(), log), cluster.ID, node.ID, "agent-grant")

	// Reporting a successful pg_ensure_database result triggers the grant
	// follow-up command, which must carry the skylex_admin secret.
	if _, err := svc.ReportCommandResult(ctx, &skylexv1.ReportCommandResultRequest{
		AgentId:   "agent-grant",
		CommandId: ensureCmdID,
		Success:   true,
	}); err != nil {
		t.Fatalf("report ensure database result: %v", err)
	}

	cmd := findPendingByAction(t, ctx, database, "", "agent-grant", node.ID, "pg_grant_database_privileges")
	secret, err := commandSecrets.ResolveSecret(ctx, cmd.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve command secret: %v", err)
	}
	if secret != "admin-pw-5" {
		t.Fatalf("expected skylex_admin_password %q on pg_grant_database_privileges, got %q", "admin-pw-5", secret)
	}
}

func TestGrantFollowUp_MissingClusterSecretQueuesWithoutAdminSecret(t *testing.T) {
	database, log := newTestDeps(t)
	svc, _, commandSecrets := newAgentServiceWithSecrets(t, database, log)
	ctx := context.Background()

	clusters := db.NewClusterRepository(database.Conn(), log)
	cluster, err := clusters.Create(ctx, "grant-no-secret", "", "/var/lib/postgresql/data", models.EnginePostgreSQL, "16", models.ReplicationAsync, 0, false, nil)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	nodes := db.NewNodeRepository(database.Conn(), log)
	node := newReadyPrimary(t, ctx, nodes, cluster.ID, "agent-grant-2")

	// No cluster secret stored (cluster predates Phase 2). The grant must still
	// be queued, without an admin secret, and without panicking.
	_, _, ensureCmdID := queueEnsureDatabaseWithOwner(t, ctx, db.NewManagedDatabaseRepository(database.Conn(), log), cluster.ID, node.ID, "agent-grant-2")

	if _, err := svc.ReportCommandResult(ctx, &skylexv1.ReportCommandResultRequest{
		AgentId:   "agent-grant-2",
		CommandId: ensureCmdID,
		Success:   true,
	}); err != nil {
		t.Fatalf("report ensure database result: %v", err)
	}

	cmd := findPendingByAction(t, ctx, database, "", "agent-grant-2", node.ID, "pg_grant_database_privileges")
	secret, err := commandSecrets.ResolveSecret(ctx, cmd.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve command secret: %v", err)
	}
	if secret != "" {
		t.Fatalf("expected no skylex_admin_password when cluster secret is missing, got %q", secret)
	}
}
