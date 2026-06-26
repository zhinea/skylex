package server

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
)

// countPendingByAction returns how many pending commands match the action.
func countPendingByAction(t *testing.T, ctx context.Context, database *db.DB, agentID, nodeID, action string) int {
	t.Helper()
	commands := db.NewAgentCommandRepository(database.Conn(), nil)
	pending, err := commands.ListPending(ctx, agentID, nodeID)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	n := 0
	for _, c := range pending {
		if c.Action == action {
			n++
		}
	}
	return n
}

// TestEnsureSkylexAdminProvisioned_AlreadyProvisionedIsNoop verifies that a
// cluster that already has skylex_admin_password is not re-provisioned: no new
// pg_ensure_admin_role command is queued and the stored secret is untouched.
func TestEnsureSkylexAdminProvisioned_AlreadyProvisionedIsNoop(t *testing.T) {
	database, svc, nodes, clusterSecrets, _ := newPostgresManagementServiceWithSecrets(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "backfill-noop")
	primary := newReadyPrimary(t, ctx, nodes, clusterID, "agent-noop")

	if err := clusterSecrets.StoreSecret(ctx, clusterID, "skylex_admin_password", "existing-pw"); err != nil {
		t.Fatalf("store cluster secret: %v", err)
	}

	newly, err := ensureSkylexAdminProvisioned(ctx, svc.clusterSecrets, svc.commandSecrets, svc.log, clusterID, primary)
	if err != nil {
		t.Fatalf("ensureSkylexAdminProvisioned: %v", err)
	}
	if newly {
		t.Fatal("expected newlyProvisioned=false for an already-provisioned cluster")
	}
	if n := countPendingByAction(t, ctx, database, "agent-noop", primary.ID, "pg_ensure_admin_role"); n != 0 {
		t.Fatalf("expected 0 pg_ensure_admin_role commands, got %d", n)
	}
	pw, err := clusterSecrets.ResolveSecret(ctx, clusterID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve secret: %v", err)
	}
	if pw != "existing-pw" {
		t.Fatalf("expected secret to be unchanged, got %q", pw)
	}
}

// TestEnsureSkylexAdminProvisioned_BackfillsMissingSecret verifies that a cluster
// without the secret gets one stored AND exactly one pg_ensure_admin_role queued
// on the primary, carrying a resolvable command secret matching the stored value.
func TestEnsureSkylexAdminProvisioned_BackfillsMissingSecret(t *testing.T) {
	database, svc, nodes, clusterSecrets, commandSecrets := newPostgresManagementServiceWithSecrets(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "backfill-missing")
	primary := newReadyPrimary(t, ctx, nodes, clusterID, "agent-missing")

	// No secret stored: this cluster predates Phase 2.
	newly, err := ensureSkylexAdminProvisioned(ctx, svc.clusterSecrets, svc.commandSecrets, svc.log, clusterID, primary)
	if err != nil {
		t.Fatalf("ensureSkylexAdminProvisioned: %v", err)
	}
	if !newly {
		t.Fatal("expected newlyProvisioned=true for a pre-Phase-2 cluster")
	}

	// The secret is now durably stored.
	storedPW, err := clusterSecrets.ResolveSecret(ctx, clusterID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve stored secret: %v", err)
	}
	if storedPW == "" {
		t.Fatal("expected skylex_admin_password to be stored after backfill")
	}

	// Exactly one pg_ensure_admin_role is queued on the primary.
	if n := countPendingByAction(t, ctx, database, "agent-missing", primary.ID, "pg_ensure_admin_role"); n != 1 {
		t.Fatalf("expected exactly 1 pg_ensure_admin_role command, got %d", n)
	}

	// The queued command carries the same password as a resolvable command secret.
	cmd := findPendingByAction(t, ctx, database, "", "agent-missing", primary.ID, "pg_ensure_admin_role")
	cmdSecret, err := commandSecrets.ResolveSecret(ctx, cmd.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve command secret: %v", err)
	}
	if cmdSecret != storedPW {
		t.Fatalf("expected command secret to match stored password %q, got %q", storedPW, cmdSecret)
	}
}

// TestEnsureSkylexAdminProvisioned_IdempotentSecondCall verifies that calling the
// helper twice in a row provisions only once (the second call is a no-op).
func TestEnsureSkylexAdminProvisioned_IdempotentSecondCall(t *testing.T) {
	database, svc, nodes, _, _ := newPostgresManagementServiceWithSecrets(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "backfill-idempotent")
	primary := newReadyPrimary(t, ctx, nodes, clusterID, "agent-idem")

	first, err := ensureSkylexAdminProvisioned(ctx, svc.clusterSecrets, svc.commandSecrets, svc.log, clusterID, primary)
	if err != nil {
		t.Fatalf("first ensureSkylexAdminProvisioned: %v", err)
	}
	if !first {
		t.Fatal("expected first call to provision the role")
	}

	second, err := ensureSkylexAdminProvisioned(ctx, svc.clusterSecrets, svc.commandSecrets, svc.log, clusterID, primary)
	if err != nil {
		t.Fatalf("second ensureSkylexAdminProvisioned: %v", err)
	}
	if second {
		t.Fatal("expected second call to be a no-op (already provisioned)")
	}

	if n := countPendingByAction(t, ctx, database, "agent-idem", primary.ID, "pg_ensure_admin_role"); n != 1 {
		t.Fatalf("expected exactly 1 pg_ensure_admin_role command after two calls, got %d", n)
	}
}

// TestApplyExtensions_BackfillsAndUsesBootstrapOnce verifies the Option B timing
// path end-to-end: for a cluster without the secret, ApplyExtensions queues the
// pg_ensure_admin_role backfill AND issues pg_apply_extensions WITHOUT the admin
// secret (bootstrap fallback one final time). A subsequent ApplyExtensions then
// attaches the now-stored admin secret.
func TestApplyExtensions_BackfillsAndUsesBootstrapOnce(t *testing.T) {
	database, svc, nodes, _, commandSecrets := newPostgresManagementServiceWithSecrets(t)
	ctx := contextWithUserRole(models.RoleOperator)
	clusterID := createPostgresManagementTestCluster(t, ctx, svc, "ext-backfill")
	primary := newReadyPrimary(t, ctx, nodes, clusterID, "agent-ext-bf")

	if _, err := svc.SetExtension(ctx, connect.NewRequest(&skylexv1.SetExtensionRequest{
		ClusterId: clusterID, Name: "pg_trgm", Enabled: true,
	})); err != nil {
		t.Fatalf("set extension: %v", err)
	}

	// First apply: backfills skylex_admin and uses bootstrap identity (Option B).
	if _, err := svc.ApplyExtensions(ctx, connect.NewRequest(&skylexv1.ApplyExtensionsRequest{ClusterId: clusterID})); err != nil {
		t.Fatalf("first apply extensions: %v", err)
	}
	if n := countPendingByAction(t, ctx, database, "agent-ext-bf", primary.ID, "pg_ensure_admin_role"); n != 1 {
		t.Fatalf("expected 1 pg_ensure_admin_role queued by backfill, got %d", n)
	}
	firstApply := findPendingByAction(t, ctx, database, "", "agent-ext-bf", primary.ID, "pg_apply_extensions")
	firstSecret, err := commandSecrets.ResolveSecret(ctx, firstApply.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve first apply secret: %v", err)
	}
	if firstSecret != "" {
		t.Fatalf("expected no admin secret on the first apply (bootstrap fallback), got %q", firstSecret)
	}

	// Second apply: the secret now exists, so it is attached to the command.
	if _, err := svc.SetExtension(ctx, connect.NewRequest(&skylexv1.SetExtensionRequest{
		ClusterId: clusterID, Name: "hstore", Enabled: true,
	})); err != nil {
		t.Fatalf("set extension (2): %v", err)
	}
	if _, err := svc.ApplyExtensions(ctx, connect.NewRequest(&skylexv1.ApplyExtensionsRequest{ClusterId: clusterID})); err != nil {
		t.Fatalf("second apply extensions: %v", err)
	}
	// Still only one backfill command (idempotent).
	if n := countPendingByAction(t, ctx, database, "agent-ext-bf", primary.ID, "pg_ensure_admin_role"); n != 1 {
		t.Fatalf("expected backfill to remain idempotent (1 command), got %d", n)
	}
	// The newest pg_apply_extensions carries the stored admin secret.
	applies := pendingByAction(t, ctx, database, "agent-ext-bf", primary.ID, "pg_apply_extensions")
	if len(applies) < 2 {
		t.Fatalf("expected at least 2 pg_apply_extensions commands, got %d", len(applies))
	}
	latest := applies[len(applies)-1]
	secondSecret, err := commandSecrets.ResolveSecret(ctx, latest.ID, "skylex_admin_password")
	if err != nil {
		t.Fatalf("resolve second apply secret: %v", err)
	}
	if secondSecret == "" {
		t.Fatal("expected the second apply to carry the backfilled admin secret")
	}
}

// pendingByAction returns all pending commands for the given action, preserving
// insertion order (ListPending is ordered by created_at).
func pendingByAction(t *testing.T, ctx context.Context, database *db.DB, agentID, nodeID, action string) []*db.AgentCommand {
	t.Helper()
	commands := db.NewAgentCommandRepository(database.Conn(), nil)
	pending, err := commands.ListPending(ctx, agentID, nodeID)
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	out := make([]*db.AgentCommand, 0, len(pending))
	for _, c := range pending {
		if c.Action == action {
			out = append(out, c)
		}
	}
	return out
}
