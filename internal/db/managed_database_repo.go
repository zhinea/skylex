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

// PostgresDatabase represents a Skylex-managed application database.
type PostgresDatabase struct {
	ID           string
	ClusterID    string
	DatabaseName string
	OwnerRoleID  *string
	Status       string // pending | ready | failed | deleting
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PostgresDatabaseRepository manages managed_databases rows and their queued commands.
type PostgresDatabaseRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

// PostgresDatabaseTx bundles rows created/changed with the queued command.
type PostgresDatabaseTx struct {
	Database  *PostgresDatabase
	Operation *PostgresOperation
	Command   *AgentCommand
}

type DatabaseGrantRequest struct {
	ClusterID     string
	DatabaseID    string
	DatabaseName  string
	OperationID   string
	GrantRoleName string
	GrantRoleKind string
}

func NewPostgresDatabaseRepository(conn *sql.DB, log *slog.Logger) *PostgresDatabaseRepository {
	return &PostgresDatabaseRepository{conn: conn, log: log}
}

func (r *PostgresDatabaseRepository) GetByID(ctx context.Context, databaseID string) (*PostgresDatabase, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, database_name, owner_role_id, status, created_at, updated_at
		 FROM managed_databases WHERE id = ?`), databaseID)
	return scanPostgresDatabase(row)
}

func (r *PostgresDatabaseRepository) GetByClusterAndName(ctx context.Context, clusterID, databaseName string) (*PostgresDatabase, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, database_name, owner_role_id, status, created_at, updated_at
		 FROM managed_databases WHERE cluster_id = ? AND database_name = ?`), clusterID, databaseName)
	return scanPostgresDatabase(row)
}

func (r *PostgresDatabaseRepository) ListByCluster(ctx context.Context, clusterID string) ([]*PostgresDatabase, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT id, cluster_id, database_name, owner_role_id, status, created_at, updated_at
		 FROM managed_databases WHERE cluster_id = ? ORDER BY created_at ASC`), clusterID)
	if err != nil {
		return nil, fmt.Errorf("list postgres databases: %w", err)
	}
	defer rows.Close()

	databases := []*PostgresDatabase{}
	for rows.Next() {
		database, err := scanPostgresDatabaseRow(rows)
		if err != nil {
			return nil, err
		}
		databases = append(databases, database)
	}
	return databases, rows.Err()
}

func (r *PostgresDatabaseRepository) HasByOwnerRole(ctx context.Context, roleID string) (bool, error) {
	var exists bool
	if err := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT EXISTS(SELECT 1 FROM managed_databases WHERE owner_role_id = ? AND status != 'deleting')`), roleID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check databases by owner role: %w", err)
	}
	return exists, nil
}

func (r *PostgresDatabaseRepository) CreateWithCommand(ctx context.Context, input CreateDatabaseTxInput) (*PostgresDatabaseTx, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create database tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	databaseID := input.DatabaseID
	if databaseID == "" {
		databaseID = id.New()
	}

	var ownerRoleIDArg interface{}
	if input.OwnerRoleID != "" {
		ownerRoleIDArg = input.OwnerRoleID
	}

	_, err = tx.ExecContext(ctx,
		Rebind(`INSERT INTO managed_databases
		 (id, cluster_id, database_name, owner_role_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'pending', ?, ?)`),
		databaseID, input.ClusterID, input.DatabaseName, ownerRoleIDArg, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert postgres database: %w", err)
	}

	op, err := insertPostgresOperation(ctx, tx, input.OperationID, input.ClusterID, input.NodeID, "create_database", now)
	if err != nil {
		return nil, err
	}
	if input.BeforeAction != "" {
		if _, err := insertAgentCommand(ctx, tx, "", input.AgentID, input.NodeID, input.BeforeAction, input.BeforePayload, now.Add(-time.Millisecond)); err != nil {
			return nil, err
		}
	}
	cmd, err := insertAgentCommand(ctx, tx, input.CommandID, input.AgentID, input.NodeID, input.EnsureAction, input.Payload, now)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE service_operations SET status = 'running', updated_at = ? WHERE id = ?`), now, op.ID); err != nil {
		return nil, fmt.Errorf("mark operation running: %w", err)
	}
	op.Status = "running"

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create database tx: %w", err)
	}

	database := &PostgresDatabase{
		ID:           databaseID,
		ClusterID:    input.ClusterID,
		DatabaseName: input.DatabaseName,
		Status:       "pending",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if input.OwnerRoleID != "" {
		database.OwnerRoleID = &input.OwnerRoleID
	}
	return &PostgresDatabaseTx{Database: database, Operation: op, Command: cmd}, nil
}

func (r *PostgresDatabaseRepository) DeleteWithCommand(ctx context.Context, input DeleteDatabaseTxInput) (*PostgresDatabaseTx, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin delete database tx: %w", err)
	}
	defer tx.Rollback()

	var database PostgresDatabase
	var ownerRoleID sql.NullString
	err = tx.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, database_name, owner_role_id, status, created_at, updated_at
		 FROM managed_databases WHERE id = ?`), input.DatabaseID,
	).Scan(&database.ID, &database.ClusterID, &database.DatabaseName, &ownerRoleID, &database.Status, &database.CreatedAt, &database.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("database %q not found", input.DatabaseID)
	}
	if err != nil {
		return nil, fmt.Errorf("scan postgres database for delete: %w", err)
	}
	if ownerRoleID.Valid {
		database.OwnerRoleID = &ownerRoleID.String
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE managed_databases SET status = 'deleting', updated_at = ? WHERE id = ?`), now, input.DatabaseID); err != nil {
		return nil, fmt.Errorf("mark database deleting: %w", err)
	}
	database.Status = "deleting"
	database.UpdatedAt = now

	op, err := insertPostgresOperation(ctx, tx, input.OperationID, database.ClusterID, input.NodeID, "delete_database", now)
	if err != nil {
		return nil, err
	}
	if input.BeforeAction != "" {
		if _, err := insertAgentCommand(ctx, tx, "", input.AgentID, input.NodeID, input.BeforeAction, input.BeforePayload, now.Add(-time.Millisecond)); err != nil {
			return nil, err
		}
	}
	cmd, err := insertAgentCommand(ctx, tx, input.CommandID, input.AgentID, input.NodeID, input.DropAction, input.Payload, now)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE service_operations SET status = 'running', updated_at = ? WHERE id = ?`), now, op.ID); err != nil {
		return nil, fmt.Errorf("mark operation running: %w", err)
	}
	op.Status = "running"

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit delete database tx: %w", err)
	}
	return &PostgresDatabaseTx{Database: &database, Operation: op, Command: cmd}, nil
}

func (r *PostgresDatabaseRepository) QueueGrantCommand(ctx context.Context, input GrantDatabaseTxInput) (*AgentCommand, error) {
	now := time.Now().UTC()
	commandID := input.CommandID
	if commandID == "" {
		commandID = id.New()
	}
	_, err := r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO agent_commands (id, agent_id, node_id, action, payload, status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', ?)`),
		commandID, input.AgentID, input.NodeID, input.GrantAction, input.Payload, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert grant database command: %w", err)
	}
	return &AgentCommand{ID: commandID, AgentID: input.AgentID, NodeID: input.NodeID, Action: input.GrantAction, Payload: input.Payload, Status: "pending", CreatedAt: now}, nil
}

func (r *PostgresDatabaseRepository) MarkCreateFailed(ctx context.Context, databaseID, operationID, errMsg string) error {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mark database create failed tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	if err := markDatabaseStatus(ctx, tx, databaseID, "failed", now); err != nil {
		return err
	}
	if err := markDatabaseOperation(ctx, tx, operationID, "create_database", "failed", RedactStoredError(errMsg), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mark database create failed tx: %w", err)
	}
	return nil
}

// HandleCommandResult applies state transitions for database management commands.
// A successful ensure with an owner role returns a follow-up grant request; the
// caller resolves the current primary before queueing that grant command.
func (r *PostgresDatabaseRepository) HandleCommandResult(ctx context.Context, commandID string, success bool, errMsg string) (bool, *DatabaseGrantRequest, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, nil, fmt.Errorf("begin database command result tx: %w", err)
	}
	defer tx.Rollback()

	var action, payload string
	err = tx.QueryRowContext(ctx,
		Rebind(`SELECT action, payload FROM agent_commands WHERE id = ?`), commandID,
	).Scan(&action, &payload)
	if err == sql.ErrNoRows {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("get command for database result: %w", err)
	}

	operationType := operationTypeForDatabaseAction(action)
	if operationType == "" {
		return false, nil, nil
	}

	var p databaseCommandPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return true, nil, fmt.Errorf("parse database command payload: %w", err)
	}
	if p.DatabaseID == "" {
		return true, nil, fmt.Errorf("database command payload missing database_id")
	}
	if p.OperationID == "" {
		return true, nil, fmt.Errorf("database command payload missing operation_id")
	}

	now := time.Now().UTC()
	storedErr := RedactStoredError(errMsg)
	var followUp *DatabaseGrantRequest

	switch action {
	case "pg_ensure_database":
		if success && p.OwnerRoleName != "" {
			clusterID, err := clusterIDForDatabase(ctx, tx, p.DatabaseID)
			if err != nil {
				return true, nil, err
			}
			followUp = &DatabaseGrantRequest{
				ClusterID:     clusterID,
				DatabaseID:    p.DatabaseID,
				DatabaseName:  p.DatabaseName,
				OperationID:   p.OperationID,
				GrantRoleName: p.OwnerRoleName,
				GrantRoleKind: p.OwnerRoleKind,
			}
		} else if success {
			if err := markDatabaseStatus(ctx, tx, p.DatabaseID, "ready", now); err != nil {
				return true, nil, err
			}
			if err := markDatabaseOperation(ctx, tx, p.OperationID, operationType, "succeeded", "", now); err != nil {
				return true, nil, err
			}
		} else {
			if err := markDatabaseStatus(ctx, tx, p.DatabaseID, "failed", now); err != nil {
				return true, nil, err
			}
			if err := markDatabaseOperation(ctx, tx, p.OperationID, operationType, "failed", storedErr, now); err != nil {
				return true, nil, err
			}
		}
	case "pg_grant_database_privileges":
		if success {
			if err := markDatabaseStatus(ctx, tx, p.DatabaseID, "ready", now); err != nil {
				return true, nil, err
			}
			if err := markDatabaseOperation(ctx, tx, p.OperationID, "create_database", "succeeded", "", now); err != nil {
				return true, nil, err
			}
		} else {
			if err := markDatabaseStatus(ctx, tx, p.DatabaseID, "failed", now); err != nil {
				return true, nil, err
			}
			if err := markDatabaseOperation(ctx, tx, p.OperationID, "create_database", "failed", storedErr, now); err != nil {
				return true, nil, err
			}
		}
	case "pg_drop_database":
		if success {
			if _, err := tx.ExecContext(ctx, Rebind(`DELETE FROM managed_databases WHERE id = ?`), p.DatabaseID); err != nil {
				return true, nil, fmt.Errorf("delete database after drop: %w", err)
			}
			if err := markDatabaseOperation(ctx, tx, p.OperationID, operationType, "succeeded", "", now); err != nil {
				return true, nil, err
			}
		} else {
			if err := markDatabaseStatus(ctx, tx, p.DatabaseID, "failed", now); err != nil {
				return true, nil, err
			}
			if err := markDatabaseOperation(ctx, tx, p.OperationID, operationType, "failed", storedErr, now); err != nil {
				return true, nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return true, nil, fmt.Errorf("commit database command result tx: %w", err)
	}
	return true, followUp, nil
}

type CreateDatabaseTxInput struct {
	DatabaseID    string
	OperationID   string
	CommandID     string
	ClusterID     string
	NodeID        string
	AgentID       string
	DatabaseName  string
	OwnerRoleID   string
	Payload       string
	BeforeAction  string
	BeforePayload string
	// EnsureAction is the engine-specific agent command action for ensuring a
	// database (e.g. "pg_ensure_database"). Resolved by the caller from the
	// engine provider.
	EnsureAction string
}

type DeleteDatabaseTxInput struct {
	DatabaseID    string
	OperationID   string
	CommandID     string
	NodeID        string
	AgentID       string
	Payload       string
	BeforeAction  string
	BeforePayload string
	// DropAction is the engine-specific agent command action for dropping a
	// database (e.g. "pg_drop_database").
	DropAction string
}

type GrantDatabaseTxInput struct {
	CommandID string
	NodeID    string
	AgentID   string
	Payload   string
	// GrantAction is the engine-specific agent command action for granting
	// database privileges (e.g. "pg_grant_database_privileges").
	GrantAction string
}

type databaseCommandPayload struct {
	DatabaseID    string `json:"database_id"`
	OperationID   string `json:"operation_id"`
	DatabaseName  string `json:"database_name"`
	OwnerRoleName string `json:"owner_role_name"`
	OwnerRoleKind string `json:"owner_role_kind"`
	GrantRoleName string `json:"grant_role_name"`
	GrantRoleKind string `json:"grant_role_kind"`
}

func operationTypeForDatabaseAction(action string) string {
	switch action {
	case "pg_ensure_database", "pg_grant_database_privileges":
		return "create_database"
	case "pg_drop_database":
		return "delete_database"
	default:
		return ""
	}
}

func clusterIDForDatabase(ctx context.Context, tx *sql.Tx, databaseID string) (string, error) {
	var clusterID string
	if err := tx.QueryRowContext(ctx,
		Rebind(`SELECT cluster_id FROM managed_databases WHERE id = ?`), databaseID,
	).Scan(&clusterID); err != nil {
		return "", fmt.Errorf("get database cluster: %w", err)
	}
	return clusterID, nil
}

func markDatabaseStatus(ctx context.Context, tx *sql.Tx, databaseID, status string, now time.Time) error {
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE managed_databases SET status = ?, updated_at = ? WHERE id = ?`), status, now, databaseID,
	); err != nil {
		return fmt.Errorf("mark database %s: %w", status, err)
	}
	return nil
}

func markDatabaseOperation(ctx context.Context, tx *sql.Tx, operationID, operationType, status, errMsg string, now time.Time) error {
	var completedAt interface{}
	if status == "succeeded" || status == "failed" {
		completedAt = now
	}
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE service_operations SET status = ?, error = ?, updated_at = ?, completed_at = ? WHERE id = ? AND operation_type = ?`),
		status, errMsg, now, completedAt, operationID, operationType,
	); err != nil {
		return fmt.Errorf("update database operation: %w", err)
	}
	return nil
}

func scanPostgresDatabase(row *sql.Row) (*PostgresDatabase, error) {
	var database PostgresDatabase
	var ownerRoleID sql.NullString
	err := row.Scan(&database.ID, &database.ClusterID, &database.DatabaseName, &ownerRoleID, &database.Status, &database.CreatedAt, &database.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan postgres database: %w", err)
	}
	if ownerRoleID.Valid {
		database.OwnerRoleID = &ownerRoleID.String
	}
	return &database, nil
}

func scanPostgresDatabaseRow(rows *sql.Rows) (*PostgresDatabase, error) {
	var database PostgresDatabase
	var ownerRoleID sql.NullString
	err := rows.Scan(&database.ID, &database.ClusterID, &database.DatabaseName, &ownerRoleID, &database.Status, &database.CreatedAt, &database.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan postgres database row: %w", err)
	}
	if ownerRoleID.Valid {
		database.OwnerRoleID = &ownerRoleID.String
	}
	return &database, nil
}
