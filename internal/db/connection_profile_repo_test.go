package db

import (
	"context"
	"testing"
)

// TestConnectionProfileRepo_DefaultsOnMissingRow verifies that GetByClusterID
// returns a zeroed default profile when no row has been saved yet.
func TestConnectionProfileRepo_DefaultsOnMissingRow(t *testing.T) {
	database, log := newTestDB(t)
	repo := NewConnectionProfileRepository(database.Conn(), log)
	ctx := context.Background()

	p, err := repo.GetByClusterID(ctx, "nonexistent-cluster")
	if err != nil {
		t.Fatalf("get missing profile: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil default profile")
	}
	if p.EndpointMode != DefaultEndpointMode {
		t.Errorf("expected default endpoint_mode %q, got %q", DefaultEndpointMode, p.EndpointMode)
	}
	if p.PublicPort != DefaultPublicPort {
		t.Errorf("expected default public_port %d, got %d", DefaultPublicPort, p.PublicPort)
	}
	if p.SSLMode != DefaultSSLMode {
		t.Errorf("expected default ssl_mode %q, got %q", DefaultSSLMode, p.SSLMode)
	}
	if p.AllowedCIDRs == nil {
		t.Error("expected non-nil AllowedCIDRs slice")
	}
	if len(p.AllowedCIDRs) != 0 {
		t.Errorf("expected empty AllowedCIDRs, got %v", p.AllowedCIDRs)
	}
	if p.AllowedAdminCIDRs == nil || p.AllowedReplicationCIDRs == nil {
		t.Error("expected non-nil network access CIDR slices")
	}
}

// TestConnectionProfileRepo_UpsertAndGet verifies create-then-read round trip.
func TestConnectionProfileRepo_UpsertAndGet(t *testing.T) {
	database, log := newTestDB(t)
	repo := NewConnectionProfileRepository(database.Conn(), log)
	ctx := context.Background()

	// Insert a cluster row so the FK constraint is satisfied.
	clusterID := insertTestCluster(t, database, "test-cluster-profile")

	in := &ConnectionProfile{
		ClusterID:               clusterID,
		EndpointMode:            "manual_stable_endpoint",
		PublicHost:              "pg.example.com",
		PublicPort:              5433,
		SSLMode:                 "require",
		AllowedCIDRs:            []string{"10.0.0.0/8", "192.168.1.0/24"},
		AllowedAdminCIDRs:       []string{"10.1.0.0/16"},
		AllowedReplicationCIDRs: []string{"10.2.0.0/16"},
	}

	out, err := repo.Upsert(ctx, in)
	if err != nil {
		t.Fatalf("upsert profile: %v", err)
	}
	if out.ClusterID != clusterID {
		t.Errorf("cluster_id mismatch: want %q got %q", clusterID, out.ClusterID)
	}
	if out.EndpointMode != "manual_stable_endpoint" {
		t.Errorf("endpoint_mode mismatch: got %q", out.EndpointMode)
	}
	if out.PublicHost != "pg.example.com" {
		t.Errorf("public_host mismatch: got %q", out.PublicHost)
	}
	if out.PublicPort != 5433 {
		t.Errorf("public_port mismatch: got %d", out.PublicPort)
	}
	if out.SSLMode != "require" {
		t.Errorf("ssl_mode mismatch: got %q", out.SSLMode)
	}
	if len(out.AllowedCIDRs) != 2 {
		t.Errorf("expected 2 allowed_cidrs, got %d", len(out.AllowedCIDRs))
	}
	if len(out.AllowedAdminCIDRs) != 1 || out.AllowedAdminCIDRs[0] != "10.1.0.0/16" {
		t.Errorf("allowed_admin_cidrs mismatch: got %v", out.AllowedAdminCIDRs)
	}
	if len(out.AllowedReplicationCIDRs) != 1 || out.AllowedReplicationCIDRs[0] != "10.2.0.0/16" {
		t.Errorf("allowed_replication_cidrs mismatch: got %v", out.AllowedReplicationCIDRs)
	}
	if out.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
	if out.UpdatedAt.IsZero() {
		t.Error("expected non-zero updated_at")
	}

	// GetByClusterID should return the persisted values.
	got, err := repo.GetByClusterID(ctx, clusterID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.EndpointMode != "manual_stable_endpoint" {
		t.Errorf("get: endpoint_mode mismatch: got %q", got.EndpointMode)
	}
	if got.PublicHost != "pg.example.com" {
		t.Errorf("get: public_host mismatch: got %q", got.PublicHost)
	}
}

// TestConnectionProfileRepo_UpsertUpdatesExisting verifies that a second upsert
// updates the row while preserving created_at and setting a new updated_at.
func TestConnectionProfileRepo_UpsertUpdatesExisting(t *testing.T) {
	database, log := newTestDB(t)
	repo := NewConnectionProfileRepository(database.Conn(), log)
	ctx := context.Background()

	clusterID := insertTestCluster(t, database, "test-cluster-update")

	first, err := repo.Upsert(ctx, &ConnectionProfile{
		ClusterID:               clusterID,
		EndpointMode:            "direct_primary",
		PublicHost:              "",
		PublicPort:              5432,
		SSLMode:                 "prefer",
		AllowedCIDRs:            []string{},
		AllowedAdminCIDRs:       []string{"10.10.0.0/16"},
		AllowedReplicationCIDRs: []string{"10.20.0.0/16"},
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second, err := repo.Upsert(ctx, &ConnectionProfile{
		ClusterID:               clusterID,
		EndpointMode:            "manual_stable_endpoint",
		PublicHost:              "stable.example.com",
		PublicPort:              5432,
		SSLMode:                 "require",
		AllowedCIDRs:            []string{"0.0.0.0/0"},
		AllowedAdminCIDRs:       []string{"10.11.0.0/16"},
		AllowedReplicationCIDRs: []string{"10.21.0.0/16"},
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	// created_at should be preserved from first upsert.
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("created_at changed on update: first=%v second=%v", first.CreatedAt, second.CreatedAt)
	}

	// The stored values should reflect the second upsert.
	got, err := repo.GetByClusterID(ctx, clusterID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.EndpointMode != "manual_stable_endpoint" {
		t.Errorf("endpoint_mode not updated: got %q", got.EndpointMode)
	}
	if got.PublicHost != "stable.example.com" {
		t.Errorf("public_host not updated: got %q", got.PublicHost)
	}
	if got.SSLMode != "require" {
		t.Errorf("ssl_mode not updated: got %q", got.SSLMode)
	}
	if len(got.AllowedCIDRs) != 1 || got.AllowedCIDRs[0] != "0.0.0.0/0" {
		t.Errorf("allowed_cidrs not updated: got %v", got.AllowedCIDRs)
	}
	if len(got.AllowedAdminCIDRs) != 1 || got.AllowedAdminCIDRs[0] != "10.11.0.0/16" {
		t.Errorf("allowed_admin_cidrs not updated: got %v", got.AllowedAdminCIDRs)
	}
	if len(got.AllowedReplicationCIDRs) != 1 || got.AllowedReplicationCIDRs[0] != "10.21.0.0/16" {
		t.Errorf("allowed_replication_cidrs not updated: got %v", got.AllowedReplicationCIDRs)
	}
}

// TestMigrations_SQLite_ConnectionProfileTable verifies that the migration
// creates the cluster_connection_profiles table with expected columns.
func TestMigrations_SQLite_ConnectionProfileTable(t *testing.T) {
	database, _ := newTestDB(t)
	conn := database.Conn()
	ctx := context.Background()

	_, err := conn.ExecContext(ctx,
		`SELECT cluster_id, endpoint_mode, public_host, public_port, ssl_mode, allowed_cidrs, allowed_admin_cidrs, allowed_replication_cidrs, created_at, updated_at
		 FROM cluster_connection_profiles LIMIT 0`)
	if err != nil {
		t.Fatalf("cluster_connection_profiles table missing expected columns: %v", err)
	}
}

// insertTestCluster inserts a minimal cluster row and returns its ID.
// This satisfies the FK constraint on cluster_connection_profiles.
func insertTestCluster(t *testing.T, database *DB, name string) string {
	t.Helper()
	clusterID := "test-" + name
	_, err := database.Conn().Exec(
		Rebind(`INSERT INTO clusters (id, name, engine, version, replication_mode, replica_count,
			data_dir, pitr_enabled, status, labels, created_at, updated_at)
			VALUES (?, ?, 'postgresql', '16', 'asynchronous', 0, '/data', 0, 'creating', '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`),
		clusterID, name,
	)
	if err != nil {
		t.Fatalf("insert test cluster: %v", err)
	}
	return clusterID
}
