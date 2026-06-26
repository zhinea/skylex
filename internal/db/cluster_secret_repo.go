package db

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/id"
)

// ClusterSecretRepository manages durable encrypted secrets scoped to a cluster.
// One row per (cluster_id, key); upsert semantics keep the latest value.
type ClusterSecretRepository struct {
	conn       *sql.DB
	log        *slog.Logger
	encryptKey []byte
}

func NewClusterSecretRepository(conn *sql.DB, log *slog.Logger, encryptKey []byte) *ClusterSecretRepository {
	return &ClusterSecretRepository{conn: conn, log: log, encryptKey: encryptKey}
}

// StoreSecret encrypts plaintext and upserts it under (clusterID, key).
func (r *ClusterSecretRepository) StoreSecret(ctx context.Context, clusterID, key, plaintext string) error {
	secretID := id.New()
	now := time.Now().UTC()

	ciphertext, err := crypto.EncryptAES256GCM([]byte(plaintext), r.encryptKey)
	if err != nil {
		return fmt.Errorf("encrypt cluster secret: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	_, err = r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO cluster_secrets (id, cluster_id, key, ciphertext, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (cluster_id, key) DO UPDATE SET ciphertext = excluded.ciphertext, updated_at = excluded.updated_at`),
		secretID, clusterID, key, encoded, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert cluster secret: %w", err)
	}
	return nil
}

// ResolveSecret fetches and decrypts the secret for (clusterID, key).
// Returns ("", nil) when no matching row exists.
func (r *ClusterSecretRepository) ResolveSecret(ctx context.Context, clusterID, key string) (string, error) {
	var encoded string

	err := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT ciphertext FROM cluster_secrets WHERE cluster_id = ? AND key = ?`),
		clusterID, key,
	).Scan(&encoded)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("fetch cluster secret: %w", err)
	}

	ciphertextBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode cluster secret: %w", err)
	}

	plaintext, err := crypto.DecryptAES256GCM(ciphertextBytes, r.encryptKey)
	if err != nil {
		return "", fmt.Errorf("decrypt cluster secret: %w", err)
	}
	return string(plaintext), nil
}

// DeleteForCluster removes all secrets for a cluster.
// The ON DELETE CASCADE on cluster_id already covers this, but mirrors
// the agent_command pattern for explicit cleanup.
func (r *ClusterSecretRepository) DeleteForCluster(ctx context.Context, clusterID string) error {
	_, err := r.conn.ExecContext(ctx,
		Rebind(`DELETE FROM cluster_secrets WHERE cluster_id = ?`), clusterID)
	if err != nil {
		return fmt.Errorf("delete cluster secrets: %w", err)
	}
	return nil
}
