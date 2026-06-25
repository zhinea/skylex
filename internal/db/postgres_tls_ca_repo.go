package db

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/crypto"
)

type PostgresTLSCA struct {
	ClusterID         string
	CACertPEM         string
	EncryptedCAKeyPEM string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PostgresTLSCARepository struct {
	conn       *sql.DB
	log        *slog.Logger
	encryptKey []byte
}

func NewPostgresTLSCARepository(conn *sql.DB, log *slog.Logger, encryptKey []byte) *PostgresTLSCARepository {
	return &PostgresTLSCARepository{conn: conn, log: log, encryptKey: encryptKey}
}

func (r *PostgresTLSCARepository) GetByClusterID(ctx context.Context, clusterID string) (*PostgresTLSCA, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT cluster_id, ca_cert_pem, encrypted_ca_key_pem, created_at, updated_at
		 FROM service_tls_authorities WHERE cluster_id = ?`), clusterID)
	var ca PostgresTLSCA
	if err := row.Scan(&ca.ClusterID, &ca.CACertPEM, &ca.EncryptedCAKeyPEM, &ca.CreatedAt, &ca.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan postgres tls ca: %w", err)
	}
	return &ca, nil
}

func (r *PostgresTLSCARepository) Upsert(ctx context.Context, clusterID, caCertPEM, caKeyPEM string) (*PostgresTLSCA, error) {
	ciphertext, err := crypto.EncryptAES256GCM([]byte(caKeyPEM), r.encryptKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt ca key: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	now := time.Now().UTC()

	_, err = r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO service_tls_authorities (cluster_id, ca_cert_pem, encrypted_ca_key_pem, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (cluster_id) DO UPDATE SET ca_cert_pem = excluded.ca_cert_pem, encrypted_ca_key_pem = excluded.encrypted_ca_key_pem, updated_at = excluded.updated_at`),
		clusterID, caCertPEM, encoded, now, now)
	if err != nil {
		return nil, fmt.Errorf("upsert postgres tls ca: %w", err)
	}
	return r.GetByClusterID(ctx, clusterID)
}

func (r *PostgresTLSCARepository) DecryptCAKey(ca *PostgresTLSCA) (string, error) {
	if ca == nil {
		return "", fmt.Errorf("ca is nil")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ca.EncryptedCAKeyPEM)
	if err != nil {
		return "", fmt.Errorf("decode ca key: %w", err)
	}
	plaintext, err := crypto.DecryptAES256GCM(ciphertext, r.encryptKey)
	if err != nil {
		return "", fmt.Errorf("decrypt ca key: %w", err)
	}
	return string(plaintext), nil
}
