package server

import (
	"context"
	"strings"
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
	access := db.NewPostgresAccessRepository(conn, log)
	tls := db.NewPostgresTLSRepository(conn, log, []byte("12345678901234567890123456789012"))
	tlsCA := db.NewPostgresTLSCARepository(conn, log, []byte("12345678901234567890123456789012"))
	svc := NewPostgresManagementService(profiles, nodes, clusters, roles, databases, access, tls, tlsCA, []byte("12345678901234567890123456789012"), log)
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

func TestPostgresManagementService_CreateDatabaseRejectsUnregisteredEngine(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)

	// Create a cluster whose engine has no registered provider. The management
	// service must refuse the operation at the boundary rather than queueing a
	// command with an empty/incorrect action string.
	cluster, err := svc.clusters.Create(ctx, "mysql-cluster", "", "/var/lib/mysql", models.EngineType("mysql"), "8", models.ReplicationAsync, 0, false, nil)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	_, err = svc.CreateDatabase(ctx, connect.NewRequest(&skylexv1.CreateDatabaseRequest{
		ClusterId:    cluster.ID,
		DatabaseName: "app_db",
	}))
	if err == nil {
		t.Fatal("expected create database to fail for unregistered engine")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v (%v)", connect.CodeOf(err), err)
	}
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

func TestPostgresManagementService_UpdateNetworkAccessRejectsViewerAndInvalidCIDR(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleViewer)

	_, err := svc.UpdateNetworkAccess(ctx, connect.NewRequest(&skylexv1.UpdateNetworkAccessRequest{
		ClusterId:                "cluster-1",
		AllowedApplicationCidrs:  []string{"10.0.0.0/8"},
		AllowedAdminCidrs:        []string{"10.1.0.0/16"},
		InternalReplicationCidrs: []string{"10.2.0.0/16"},
	}))
	if err == nil {
		t.Fatal("expected viewer update network access request to fail")
	}

	ctx = contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "network-access-invalid")
	_, err = svc.UpdateNetworkAccess(ctx, connect.NewRequest(&skylexv1.UpdateNetworkAccessRequest{
		ClusterId:               clusterID,
		AllowedApplicationCidrs: []string{"not-a-cidr"},
	}))
	if err == nil {
		t.Fatal("expected invalid CIDR to fail")
	}
}

func TestPostgresManagementService_ApplyHBAQueuesReadyNodes(t *testing.T) {
	database, svc, roles, nodes := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "network-access-apply")
	node, err := nodes.Create(ctx, clusterID, "primary", "10.0.0.10", 5432, models.NodeRolePrimary, "0.1.0", nil)
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
	if _, err := svc.UpdateNetworkAccess(ctx, connect.NewRequest(&skylexv1.UpdateNetworkAccessRequest{
		ClusterId:                clusterID,
		AllowedApplicationCidrs:  []string{"10.3.0.0/16"},
		AllowedAdminCidrs:        []string{"10.4.0.0/16"},
		InternalReplicationCidrs: []string{"10.5.0.0/16"},
	})); err != nil {
		t.Fatalf("update network access: %v", err)
	}

	resp, err := svc.ApplyHBA(ctx, connect.NewRequest(&skylexv1.ApplyHBARequest{ClusterId: clusterID}))
	if err != nil {
		t.Fatalf("apply hba: %v", err)
	}
	if len(resp.Msg.GetHbaStatuses()) != 1 {
		t.Fatalf("expected one HBA status, got %d", len(resp.Msg.GetHbaStatuses()))
	}

	commands := db.NewAgentCommandRepository(database.Conn(), svc.log)
	pending, err := commands.ListPending(ctx, "agent-1", node.ID)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	found := false
	for _, command := range pending {
		if command.Action == "pg_apply_hba" && strings.Contains(command.Payload, "10.3.0.0/16") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pg_apply_hba command with CIDR payload, got %#v", pending)
	}
}

func TestPostgresManagementService_UpdateTLSConfigRejectsViewerAndPartialManualCerts(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	viewerCtx := contextWithUserRole(models.RoleViewer)

	_, err := svc.UpdateTLSConfig(viewerCtx, connect.NewRequest(&skylexv1.UpdateTLSConfigRequest{
		ClusterId: "cluster-1",
		TlsMode:   "required",
		CertFile:  "/etc/skylex/postgres/server.crt",
		KeyFile:   "/etc/skylex/postgres/server.key",
	}))
	if err == nil {
		t.Fatal("expected viewer update TLS config request to fail")
	}

	operatorCtx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, operatorCtx, svc, "tls-invalid")
	_, err = svc.UpdateTLSConfig(operatorCtx, connect.NewRequest(&skylexv1.UpdateTLSConfigRequest{
		ClusterId: clusterID,
		TlsMode:   "required",
		CertFile:  "/etc/skylex/postgres/server.crt",
	}))
	if err == nil {
		t.Fatal("expected partial manual TLS cert paths to fail")
	}
}

func TestPostgresManagementService_UpdateTLSConfigAllowsManagedCerts(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "tls-managed")

	resp, err := svc.UpdateTLSConfig(ctx, connect.NewRequest(&skylexv1.UpdateTLSConfigRequest{
		ClusterId: clusterID,
		TlsMode:   "required",
	}))
	if err != nil {
		t.Fatalf("expected managed TLS config to save without paths: %v", err)
	}
	if resp.Msg.GetConfig().GetTlsMode() != "required" {
		t.Fatalf("expected required TLS mode, got %q", resp.Msg.GetConfig().GetTlsMode())
	}
}

func TestPostgresManagementService_GetConnectionProfileDefaultsTLSDisabled(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "tls-default-disabled")

	resp, err := svc.GetConnectionProfile(ctx, connect.NewRequest(&skylexv1.GetConnectionProfileRequest{ClusterId: clusterID}))
	if err != nil {
		t.Fatalf("get connection profile: %v", err)
	}
	if got := resp.Msg.GetProfile().GetSslMode(); got != db.DefaultSSLMode {
		t.Fatalf("expected profile SSL mode %q, got %q", db.DefaultSSLMode, got)
	}
	if got := resp.Msg.GetTlsConfig().GetTlsMode(); got != db.DefaultSSLMode {
		t.Fatalf("expected TLS config mode %q, got %q", db.DefaultSSLMode, got)
	}
}

func TestPostgresManagementService_GenerateTLSCAStoresPublicCertOnlyInResponse(t *testing.T) {
	_, svc, _, _ := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "tls-ca")

	resp, err := svc.GenerateTLSCA(ctx, connect.NewRequest(&skylexv1.GenerateTLSCARequest{ClusterId: clusterID}))
	if err != nil {
		t.Fatalf("generate tls ca: %v", err)
	}
	if !resp.Msg.GetConfig().GetCaGenerated() {
		t.Fatal("expected ca_generated true")
	}
	if !strings.Contains(resp.Msg.GetCaCertPem(), "BEGIN CERTIFICATE") {
		t.Fatalf("expected public CA certificate PEM, got %q", resp.Msg.GetCaCertPem())
	}
	if strings.Contains(resp.Msg.GetCaCertPem(), "PRIVATE KEY") {
		t.Fatal("CA response must not include private key material")
	}
}

func TestPostgresManagementService_ApplyTLSQueuesReadyNodes(t *testing.T) {
	database, svc, _, nodes := newPostgresManagementServiceTestDeps(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "tls-apply")
	node, err := nodes.Create(ctx, clusterID, "primary", "10.0.0.10", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create primary node: %v", err)
	}
	if err := nodes.UpdateAgentID(ctx, node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	if err := nodes.UpdatePostgresStatus(ctx, node.ID, true, "16", true); err != nil {
		t.Fatalf("update postgres status: %v", err)
	}
	if _, err := svc.UpdateTLSConfig(ctx, connect.NewRequest(&skylexv1.UpdateTLSConfigRequest{
		ClusterId: clusterID,
		TlsMode:   "required",
	})); err != nil {
		t.Fatalf("update tls config: %v", err)
	}
	if _, err := svc.GenerateTLSCA(ctx, connect.NewRequest(&skylexv1.GenerateTLSCARequest{ClusterId: clusterID})); err != nil {
		t.Fatalf("generate tls ca: %v", err)
	}

	resp, err := svc.ApplyTLS(ctx, connect.NewRequest(&skylexv1.ApplyTLSRequest{ClusterId: clusterID}))
	if err != nil {
		t.Fatalf("apply tls: %v", err)
	}
	if len(resp.Msg.GetStatuses()) != 1 {
		t.Fatalf("expected one TLS status, got %d", len(resp.Msg.GetStatuses()))
	}

	commands := db.NewAgentCommandRepository(database.Conn(), svc.log)
	pending, err := commands.ListPending(ctx, "agent-1", node.ID)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	found := false
	for _, command := range pending {
		if command.Action == "pg_apply_tls" && strings.Contains(command.Payload, "server_cert_pem") && !strings.Contains(command.Payload, "BEGIN PRIVATE KEY") && !strings.Contains(command.Payload, "BEGIN RSA PRIVATE KEY") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pg_apply_tls command with certificate payload, got %#v", pending)
	}
}
