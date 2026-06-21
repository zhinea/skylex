package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/id"
)

// PostgresTLSApplyStatus records latest TLS config application state per node.
type PostgresTLSApplyStatus struct {
	ClusterID        string
	NodeID           string
	CommandID        string
	RequestedTLSMode string
	Status           string
	Error            string
	TLSActive        bool
	AppliedAt        *time.Time
	UpdatedAt        time.Time
}

type ApplyTLSNodeCommand struct {
	NodeID    string
	AgentID   string
	CommandID string
	Payload   string
}

// PostgresTLSRepository manages per-node TLS apply status.
type PostgresTLSRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

func NewPostgresTLSRepository(conn *sql.DB, log *slog.Logger) *PostgresTLSRepository {
	return &PostgresTLSRepository{conn: conn, log: log}
}

func (r *PostgresTLSRepository) ListStatusByCluster(ctx context.Context, clusterID string) ([]*PostgresTLSApplyStatus, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT cluster_id, node_id, command_id, requested_tls_mode, status, error, tls_active, applied_at, updated_at
		 FROM postgres_tls_apply_status WHERE cluster_id = ? ORDER BY updated_at DESC`), clusterID)
	if err != nil {
		return nil, fmt.Errorf("list tls apply status: %w", err)
	}
	defer rows.Close()

	statuses := []*PostgresTLSApplyStatus{}
	for rows.Next() {
		status, err := scanPostgresTLSApplyStatus(rows)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, rows.Err()
}

func (r *PostgresTLSRepository) QueueApplyTLSCommands(ctx context.Context, clusterID, requestedTLSMode string, commands []ApplyTLSNodeCommand) ([]*PostgresTLSApplyStatus, error) {
	if len(commands) == 0 {
		return []*PostgresTLSApplyStatus{}, nil
	}

	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin queue tls apply tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	statuses := make([]*PostgresTLSApplyStatus, 0, len(commands))
	for _, cmd := range commands {
		commandID := cmd.CommandID
		if commandID == "" {
			commandID = id.New()
		}
		if _, err := insertAgentCommand(ctx, tx, commandID, cmd.AgentID, cmd.NodeID, "pg_apply_tls", cmd.Payload, now); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx,
			Rebind(`DELETE FROM postgres_tls_apply_status WHERE cluster_id = ? AND node_id = ?`),
			clusterID, cmd.NodeID,
		); err != nil {
			return nil, fmt.Errorf("delete old tls apply status: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			Rebind(`INSERT INTO postgres_tls_apply_status
			 (cluster_id, node_id, command_id, requested_tls_mode, status, error, tls_active, applied_at, updated_at)
			 VALUES (?, ?, ?, ?, 'pending', '', FALSE, NULL, ?)`),
			clusterID, cmd.NodeID, commandID, requestedTLSMode, now,
		); err != nil {
			return nil, fmt.Errorf("insert tls apply status: %w", err)
		}
		statuses = append(statuses, &PostgresTLSApplyStatus{
			ClusterID:        clusterID,
			NodeID:           cmd.NodeID,
			CommandID:        commandID,
			RequestedTLSMode: requestedTLSMode,
			Status:           "pending",
			UpdatedAt:        now,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit queue tls apply tx: %w", err)
	}
	return statuses, nil
}

func (r *PostgresTLSRepository) HandleCommandResult(ctx context.Context, commandID string, success bool, errMsg string) (bool, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tls command result tx: %w", err)
	}
	defer tx.Rollback()

	var action, payload string
	if err := tx.QueryRowContext(ctx,
		Rebind(`SELECT action, payload FROM agent_commands WHERE id = ?`), commandID,
	).Scan(&action, &payload); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("get command for tls result: %w", err)
	}
	if action != "pg_apply_tls" {
		return false, nil
	}

	var p struct {
		ClusterID string `json:"cluster_id"`
		NodeID    string `json:"node_id"`
		TLSMode   string `json:"tls_mode"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return true, fmt.Errorf("parse tls command payload: %w", err)
	}
	if p.ClusterID == "" || p.NodeID == "" {
		return true, fmt.Errorf("tls command payload missing cluster_id or node_id")
	}

	now := time.Now().UTC()
	status := "succeeded"
	var appliedAt interface{} = now
	if !success {
		status = "failed"
		appliedAt = nil
	}
	tlsActive := success && p.TLSMode != "disabled"
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_tls_apply_status
		 SET status = ?, error = ?, tls_active = ?, applied_at = ?, updated_at = ?
		 WHERE cluster_id = ? AND node_id = ? AND command_id = ?`),
		status, RedactStoredError(errMsg), tlsActive, appliedAt, now, p.ClusterID, p.NodeID, commandID,
	); err != nil {
		return true, fmt.Errorf("update tls apply status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return true, fmt.Errorf("commit tls command result tx: %w", err)
	}
	return true, nil
}

func scanPostgresTLSApplyStatus(rows *sql.Rows) (*PostgresTLSApplyStatus, error) {
	var status PostgresTLSApplyStatus
	var commandID sql.NullString
	var appliedAt sql.NullTime
	if err := rows.Scan(&status.ClusterID, &status.NodeID, &commandID, &status.RequestedTLSMode, &status.Status, &status.Error, &status.TLSActive, &appliedAt, &status.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan tls apply status: %w", err)
	}
	if commandID.Valid {
		status.CommandID = commandID.String
	}
	if appliedAt.Valid {
		status.AppliedAt = &appliedAt.Time
	}
	return &status, nil
}
