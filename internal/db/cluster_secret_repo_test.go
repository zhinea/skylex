package db

import (
	"context"
	"testing"
)

func newClusterSecretRepo(t *testing.T) (*ClusterSecretRepository, *DB, context.Context) {
	t.Helper()
	database, log := newTestDB(t)
	encryptKey := make([]byte, 32) // 32 zero bytes — valid AES-256 key for tests
	repo := NewClusterSecretRepository(database.Conn(), log, encryptKey)
	return repo, database, context.Background()
}

// TestClusterSecretRepo_StoreAndResolveRoundTrip verifies that storing then
// resolving a secret returns the original plaintext (encryption round-trip).
func TestClusterSecretRepo_StoreAndResolveRoundTrip(t *testing.T) {
	repo, database, ctx := newClusterSecretRepo(t)
	clusterID := insertTestCluster(t, database, "secret-roundtrip")

	if err := repo.StoreSecret(ctx, clusterID, "admin_password", "s3cr3t!"); err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	got, err := repo.ResolveSecret(ctx, clusterID, "admin_password")
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if got != "s3cr3t!" {
		t.Fatalf("expected %q, got %q", "s3cr3t!", got)
	}
}

// TestClusterSecretRepo_StoreUpserts verifies that storing twice with the same
// (cluster_id, key) overwrites rather than inserting a duplicate row.
func TestClusterSecretRepo_StoreUpserts(t *testing.T) {
	repo, database, ctx := newClusterSecretRepo(t)
	clusterID := insertTestCluster(t, database, "secret-upsert")

	if err := repo.StoreSecret(ctx, clusterID, "pw", "first"); err != nil {
		t.Fatalf("first store: %v", err)
	}
	if err := repo.StoreSecret(ctx, clusterID, "pw", "second"); err != nil {
		t.Fatalf("second store: %v", err)
	}

	// Exactly one row in the table.
	var count int
	if err := database.Conn().QueryRowContext(ctx,
		Rebind(`SELECT COUNT(*) FROM cluster_secrets WHERE cluster_id = ? AND key = ?`),
		clusterID, "pw",
	).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row after upsert, got %d", count)
	}

	// Value is the latest one.
	got, err := repo.ResolveSecret(ctx, clusterID, "pw")
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if got != "second" {
		t.Fatalf("expected %q after upsert, got %q", "second", got)
	}
}

// TestClusterSecretRepo_ResolveMissingKeyReturnsEmpty verifies that resolving a
// key that was never stored returns ("", nil) — not an error.
func TestClusterSecretRepo_ResolveMissingKeyReturnsEmpty(t *testing.T) {
	repo, database, ctx := newClusterSecretRepo(t)
	clusterID := insertTestCluster(t, database, "secret-missing")

	got, err := repo.ResolveSecret(ctx, clusterID, "does_not_exist")
	if err != nil {
		t.Fatalf("expected no error for missing key, got: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string for missing key, got %q", got)
	}
}

// TestClusterSecretRepo_DeleteForCluster verifies that DeleteForCluster removes
// all secrets so subsequent Resolve returns ("", nil).
func TestClusterSecretRepo_DeleteForCluster(t *testing.T) {
	repo, database, ctx := newClusterSecretRepo(t)
	clusterID := insertTestCluster(t, database, "secret-delete")

	if err := repo.StoreSecret(ctx, clusterID, "k1", "v1"); err != nil {
		t.Fatalf("store k1: %v", err)
	}
	if err := repo.StoreSecret(ctx, clusterID, "k2", "v2"); err != nil {
		t.Fatalf("store k2: %v", err)
	}

	if err := repo.DeleteForCluster(ctx, clusterID); err != nil {
		t.Fatalf("DeleteForCluster: %v", err)
	}

	got, err := repo.ResolveSecret(ctx, clusterID, "k1")
	if err != nil {
		t.Fatalf("ResolveSecret after delete: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty after delete, got %q", got)
	}
}

// TestClusterSecretRepo_CiphertextDiffersFromPlaintext verifies that the value
// stored in the DB column is NOT the plaintext (i.e. encryption is actually applied).
func TestClusterSecretRepo_CiphertextDiffersFromPlaintext(t *testing.T) {
	repo, database, ctx := newClusterSecretRepo(t)
	clusterID := insertTestCluster(t, database, "secret-ciphertext")

	plaintext := "super_secret_value"
	if err := repo.StoreSecret(ctx, clusterID, "pw", plaintext); err != nil {
		t.Fatalf("store: %v", err)
	}

	var stored string
	if err := database.Conn().QueryRowContext(ctx,
		Rebind(`SELECT ciphertext FROM cluster_secrets WHERE cluster_id = ? AND key = ?`),
		clusterID, "pw",
	).Scan(&stored); err != nil {
		t.Fatalf("read raw ciphertext: %v", err)
	}

	if stored == plaintext {
		t.Fatal("ciphertext column must not equal plaintext — encryption was not applied")
	}
	if stored == "" {
		t.Fatal("ciphertext column must not be empty")
	}
}
