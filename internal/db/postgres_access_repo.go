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

// PostgresHBAApplyStatus records the latest Skylex-managed pg_hba.conf apply
// attempt per cluster node. Status is operation state, so it lives outside
// cluster/node entity tables.
type PostgresHBAApplyStatus struct {
	ClusterID string
	NodeID    string
	CommandID string
	Status    string
	Error     string
	AppliedAt *time.Time
	UpdatedAt time.Time
}

// PostgresAccessRepository manages cluster network allowlists and HBA apply state.
type PostgresAccessRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

type ApplyHBANodeCommand struct {
	NodeID    string
	AgentID   string
	CommandID string
	Payload   string
}

func NewPostgresAccessRepository(conn *sql.DB, log *slog.Logger) *PostgresAccessRepository {
	return &PostgresAccessRepository{conn: conn, log: log}
}

func (r *PostgresAccessRepository) ListHBAStatusByCluster(ctx context.Context, clusterID string) ([]*PostgresHBAApplyStatus, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT cluster_id, node_id, command_id, status, error, applied_at, updated_at
		 FROM postgres_hba_apply_status WHERE cluster_id = ? ORDER BY updated_at DESC`), clusterID)
	if err != nil {
		return nil, fmt.Errorf("list hba apply status: %w", err)
	}
	defer rows.Close()

	statuses := []*PostgresHBAApplyStatus{}
	for rows.Next() {
		status, err := scanPostgresHBAApplyStatus(rows)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, rows.Err()
}

func (r *PostgresAccessRepository) QueueApplyHBACommands(ctx context.Context, clusterID string, commands []ApplyHBANodeCommand) ([]*PostgresHBAApplyStatus, error) {
	if len(commands) == 0 {
		return []*PostgresHBAApplyStatus{}, nil
	}

	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin queue hba apply tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	statuses := make([]*PostgresHBAApplyStatus, 0, len(commands))
	for _, cmd := range commands {
		commandID := cmd.CommandID
		if commandID == "" {
			commandID = id.New()
		}
		if _, err := insertAgentCommand(ctx, tx, commandID, cmd.AgentID, cmd.NodeID, "pg_apply_hba", cmd.Payload, now); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx,
			Rebind(`DELETE FROM postgres_hba_apply_status WHERE cluster_id = ? AND node_id = ?`),
			clusterID, cmd.NodeID,
		); err != nil {
			return nil, fmt.Errorf("delete old hba apply status: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			Rebind(`INSERT INTO postgres_hba_apply_status
			 (cluster_id, node_id, command_id, status, error, applied_at, updated_at)
			 VALUES (?, ?, ?, 'pending', '', NULL, ?)`),
			clusterID, cmd.NodeID, commandID, now,
		); err != nil {
			return nil, fmt.Errorf("insert hba apply status: %w", err)
		}
		statuses = append(statuses, &PostgresHBAApplyStatus{
			ClusterID: clusterID,
			NodeID:    cmd.NodeID,
			CommandID: commandID,
			Status:    "pending",
			UpdatedAt: now,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit queue hba apply tx: %w", err)
	}
	return statuses, nil
}

func (r *PostgresAccessRepository) HandleHBACommandResult(ctx context.Context, commandID string, success bool, errMsg string) (bool, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin hba command result tx: %w", err)
	}
	defer tx.Rollback()

	var action, payload string
	if err := tx.QueryRowContext(ctx,
		Rebind(`SELECT action, payload FROM agent_commands WHERE id = ?`), commandID,
	).Scan(&action, &payload); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("get command for hba result: %w", err)
	}
	if action != "pg_apply_hba" {
		return false, nil
	}

	var p struct {
		ClusterID string `json:"cluster_id"`
		NodeID    string `json:"node_id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return true, fmt.Errorf("parse hba command payload: %w", err)
	}
	if p.ClusterID == "" || p.NodeID == "" {
		return true, fmt.Errorf("hba command payload missing cluster_id or node_id")
	}

	now := time.Now().UTC()
	status := "succeeded"
	var appliedAt interface{} = now
	if !success {
		status = "failed"
		appliedAt = nil
	}
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_hba_apply_status
		 SET status = ?, error = ?, applied_at = ?, updated_at = ?
		 WHERE cluster_id = ? AND node_id = ? AND command_id = ?`),
		status, RedactStoredError(errMsg), appliedAt, now, p.ClusterID, p.NodeID, commandID,
	); err != nil {
		return true, fmt.Errorf("update hba apply status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return true, fmt.Errorf("commit hba command result tx: %w", err)
	}
	return true, nil
}

func scanPostgresHBAApplyStatus(rows *sql.Rows) (*PostgresHBAApplyStatus, error) {
	var status PostgresHBAApplyStatus
	var commandID sql.NullString
	var appliedAt sql.NullTime
	if err := rows.Scan(&status.ClusterID, &status.NodeID, &commandID, &status.Status, &status.Error, &appliedAt, &status.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan hba apply status: %w", err)
	}
	if commandID.Valid {
		status.CommandID = commandID.String
	}
	if appliedAt.Valid {
		status.AppliedAt = &appliedAt.Time
	}
	return &status, nil
}
