package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// ConnectionProfile holds the editable connection metadata for a cluster.
type ConnectionProfile struct {
	ClusterID               string
	EndpointMode            string
	PublicHost              string
	PublicPort              int
	SSLMode                 string
	AllowedCIDRs            []string
	AllowedAdminCIDRs       []string
	AllowedReplicationCIDRs []string
	TLSCertFile             string
	TLSKeyFile              string
	TLSCAFile               string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

const (
	DefaultEndpointMode = "direct_primary"
	DefaultPublicPort   = 5432
	DefaultSSLMode      = "prefer"
)

// ConnectionProfileRepository manages per-cluster connection profiles.
type ConnectionProfileRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

func NewConnectionProfileRepository(conn *sql.DB, log *slog.Logger) *ConnectionProfileRepository {
	return &ConnectionProfileRepository{conn: conn, log: log}
}

// GetByClusterID returns the connection profile for a cluster.
// If no profile has been saved yet, a profile populated with defaults is returned (no error).
func (r *ConnectionProfileRepository) GetByClusterID(ctx context.Context, clusterID string) (*ConnectionProfile, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT cluster_id, endpoint_mode, public_host, public_port, ssl_mode, allowed_cidrs, allowed_admin_cidrs, allowed_replication_cidrs, tls_cert_file, tls_key_file, tls_ca_file, created_at, updated_at
		 FROM cluster_connection_profiles WHERE cluster_id = ?`),
		clusterID,
	)

	var p ConnectionProfile
	var allowedCIDRsJSON, allowedAdminCIDRsJSON, allowedReplicationCIDRsJSON string

	err := row.Scan(&p.ClusterID, &p.EndpointMode, &p.PublicHost, &p.PublicPort,
		&p.SSLMode, &allowedCIDRsJSON, &allowedAdminCIDRsJSON, &allowedReplicationCIDRsJSON, &p.TLSCertFile, &p.TLSKeyFile, &p.TLSCAFile, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return &ConnectionProfile{
			ClusterID:               clusterID,
			EndpointMode:            DefaultEndpointMode,
			PublicHost:              "",
			PublicPort:              DefaultPublicPort,
			SSLMode:                 DefaultSSLMode,
			AllowedCIDRs:            []string{},
			AllowedAdminCIDRs:       []string{},
			AllowedReplicationCIDRs: []string{},
			TLSCertFile:             "",
			TLSKeyFile:              "",
			TLSCAFile:               "",
			CreatedAt:               time.Time{},
			UpdatedAt:               time.Time{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan connection profile: %w", err)
	}

	p.AllowedCIDRs = unmarshalCIDRs(allowedCIDRsJSON)
	p.AllowedAdminCIDRs = unmarshalCIDRs(allowedAdminCIDRsJSON)
	p.AllowedReplicationCIDRs = unmarshalCIDRs(allowedReplicationCIDRsJSON)
	return &p, nil
}

// Upsert creates or replaces the connection profile for the given cluster.
func (r *ConnectionProfileRepository) Upsert(ctx context.Context, p *ConnectionProfile) (*ConnectionProfile, error) {
	now := time.Now().UTC()

	cidrsJSON, err := json.Marshal(p.AllowedCIDRs)
	if err != nil {
		return nil, fmt.Errorf("marshal allowed_cidrs: %w", err)
	}
	adminCIDRsJSON, err := json.Marshal(p.AllowedAdminCIDRs)
	if err != nil {
		return nil, fmt.Errorf("marshal allowed_admin_cidrs: %w", err)
	}
	replicationCIDRsJSON, err := json.Marshal(p.AllowedReplicationCIDRs)
	if err != nil {
		return nil, fmt.Errorf("marshal allowed_replication_cidrs: %w", err)
	}

	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin upsert connection profile: %w", err)
	}
	defer tx.Rollback()

	// Check if a row already exists to preserve created_at.
	var createdAt time.Time
	err = tx.QueryRowContext(ctx,
		Rebind(`SELECT created_at FROM cluster_connection_profiles WHERE cluster_id = ?`),
		p.ClusterID,
	).Scan(&createdAt)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("check existing profile: %w", err)
	}
	isNew := (err == sql.ErrNoRows)
	if isNew {
		createdAt = now
	}

	if _, err := tx.ExecContext(ctx,
		Rebind(`DELETE FROM cluster_connection_profiles WHERE cluster_id = ?`),
		p.ClusterID,
	); err != nil {
		return nil, fmt.Errorf("delete old connection profile: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		Rebind(`INSERT INTO cluster_connection_profiles
		 (cluster_id, endpoint_mode, public_host, public_port, ssl_mode, allowed_cidrs, allowed_admin_cidrs, allowed_replication_cidrs, tls_cert_file, tls_key_file, tls_ca_file, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		p.ClusterID, p.EndpointMode, p.PublicHost, p.PublicPort,
		p.SSLMode, string(cidrsJSON), string(adminCIDRsJSON), string(replicationCIDRsJSON), p.TLSCertFile, p.TLSKeyFile, p.TLSCAFile, createdAt, now,
	); err != nil {
		return nil, fmt.Errorf("insert connection profile: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit upsert connection profile: %w", err)
	}

	out := *p
	out.CreatedAt = createdAt
	out.UpdatedAt = now
	return &out, nil
}

func unmarshalCIDRs(raw string) []string {
	var cidrs []string
	if raw != "" && raw != "null" {
		if err := json.Unmarshal([]byte(raw), &cidrs); err != nil {
			return []string{}
		}
	}
	if cidrs == nil {
		return []string{}
	}
	return cidrs
}
