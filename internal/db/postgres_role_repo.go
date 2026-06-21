package db

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/zhinea/skylex/internal/id"
)

// PostgresRole represents a managed PostgreSQL role for a cluster.
type PostgresRole struct {
	ID                string
	ClusterID         string
	RoleName          string
	RoleKind          string // admin | read_write | read_only | custom
	EncryptedPassword string
	PasswordVersion   int
	ExpiresAt         *time.Time
	Status            string // pending | ready | failed | deleting
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PostgresRoleRepository manages postgres_roles rows.
type PostgresRoleRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

// RedactStoredError removes secret-bearing fragments before persisting command errors.
func RedactStoredError(input string) string {
	lower := strings.ToLower(input)
	for _, marker := range []string{"password", "secret", "token"} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			return input[:idx] + marker + "=[REDACTED]"
		}
	}
	return input
}

// PostgresRoleTx bundles all writes needed to create/rotate/delete a managed role.
// Callers use it so role metadata, operation rows, queued commands, and command
// secrets are committed atomically.
type PostgresRoleTx struct {
	Role      *PostgresRole
	Operation *PostgresOperation
	Command   *AgentCommand
}

func NewPostgresRoleRepository(conn *sql.DB, log *slog.Logger) *PostgresRoleRepository {
	return &PostgresRoleRepository{conn: conn, log: log}
}

func (r *PostgresRoleRepository) Create(ctx context.Context, clusterID, roleName, roleKind, encryptedPassword string, expiresAt *time.Time) (*PostgresRole, error) {
	roleID := id.New()
	now := time.Now().UTC()

	_, err := r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO postgres_roles
		 (id, cluster_id, role_name, role_kind, encrypted_password, password_version, expires_at, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?, 'pending', ?, ?)`),
		roleID, clusterID, roleName, roleKind, encryptedPassword, expiresAt, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert postgres role: %w", err)
	}

	return &PostgresRole{
		ID:                roleID,
		ClusterID:         clusterID,
		RoleName:          roleName,
		RoleKind:          roleKind,
		EncryptedPassword: encryptedPassword,
		PasswordVersion:   1,
		ExpiresAt:         expiresAt,
		Status:            "pending",
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (r *PostgresRoleRepository) GetByID(ctx context.Context, roleID string) (*PostgresRole, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, role_name, role_kind, encrypted_password, password_version, expires_at, status, created_at, updated_at
		 FROM postgres_roles WHERE id = ?`), roleID)
	return scanPostgresRole(row)
}

func (r *PostgresRoleRepository) GetByClusterAndName(ctx context.Context, clusterID, roleName string) (*PostgresRole, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, role_name, role_kind, encrypted_password, password_version, expires_at, status, created_at, updated_at
		 FROM postgres_roles WHERE cluster_id = ? AND role_name = ?`), clusterID, roleName)
	return scanPostgresRole(row)
}

func (r *PostgresRoleRepository) ListByCluster(ctx context.Context, clusterID string) ([]*PostgresRole, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT id, cluster_id, role_name, role_kind, encrypted_password, password_version, expires_at, status, created_at, updated_at
		 FROM postgres_roles WHERE cluster_id = ? ORDER BY created_at ASC`), clusterID)
	if err != nil {
		return nil, fmt.Errorf("list postgres roles: %w", err)
	}
	defer rows.Close()

	var roles []*PostgresRole
	for rows.Next() {
		role, err := scanPostgresRoleRow(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *PostgresRoleRepository) UpdateStatus(ctx context.Context, roleID, status string) error {
	now := time.Now().UTC()
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE postgres_roles SET status = ?, updated_at = ? WHERE id = ?`),
		status, now, roleID)
	if err != nil {
		return fmt.Errorf("update postgres role status: %w", err)
	}
	return nil
}

// RotatePassword atomically updates the encrypted password and increments the version.
func (r *PostgresRoleRepository) RotatePassword(ctx context.Context, roleID, encryptedPassword string) (int, error) {
	now := time.Now().UTC()

	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin rotate password tx: %w", err)
	}
	defer tx.Rollback()

	var currentVersion int
	if err := tx.QueryRowContext(ctx,
		Rebind(`SELECT password_version FROM postgres_roles WHERE id = ?`), roleID,
	).Scan(&currentVersion); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("role %q not found", roleID)
		}
		return 0, fmt.Errorf("fetch password_version: %w", err)
	}

	newVersion := currentVersion + 1
	_, err = tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_roles SET encrypted_password = ?, password_version = ?, status = 'pending', updated_at = ? WHERE id = ?`),
		encryptedPassword, newVersion, now, roleID,
	)
	if err != nil {
		return 0, fmt.Errorf("update encrypted password: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit rotate password: %w", err)
	}
	return newVersion, nil
}

func (r *PostgresRoleRepository) Delete(ctx context.Context, roleID string) error {
	_, err := r.conn.ExecContext(ctx, Rebind(`DELETE FROM postgres_roles WHERE id = ?`), roleID)
	if err != nil {
		return fmt.Errorf("delete postgres role: %w", err)
	}
	return nil
}

// CreateWithCommand atomically creates a role, operation row, command row, and command secret.
func (r *PostgresRoleRepository) CreateWithCommand(ctx context.Context, input CreateRoleTxInput) (*PostgresRoleTx, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create role tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	roleID := input.RoleID
	if roleID == "" {
		roleID = id.New()
	}
	_, err = tx.ExecContext(ctx,
		Rebind(`INSERT INTO postgres_roles
		 (id, cluster_id, role_name, role_kind, encrypted_password, password_version, expires_at, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?, 'pending', ?, ?)`),
		roleID, input.ClusterID, input.RoleName, input.RoleKind, input.EncryptedPassword, input.ExpiresAt, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert postgres role: %w", err)
	}

	op, err := insertPostgresOperation(ctx, tx, input.OperationID, input.ClusterID, input.NodeID, "create_role", now)
	if err != nil {
		return nil, err
	}
	if input.BeforeAction != "" {
		if _, err := insertAgentCommand(ctx, tx, "", input.AgentID, input.NodeID, input.BeforeAction, input.BeforePayload, now.Add(-time.Millisecond)); err != nil {
			return nil, err
		}
	}
	cmd, err := insertAgentCommand(ctx, tx, input.CommandID, input.AgentID, input.NodeID, "pg_ensure_role", input.Payload, now)
	if err != nil {
		return nil, err
	}
	if err := insertCommandSecret(ctx, tx, cmd.ID, "password", input.EncryptedCommandSecret, input.SecretExpiresAt, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_operations SET status = 'running', updated_at = ? WHERE id = ?`), now, op.ID); err != nil {
		return nil, fmt.Errorf("mark operation running: %w", err)
	}
	op.Status = "running"

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create role tx: %w", err)
	}

	role := &PostgresRole{
		ID:                roleID,
		ClusterID:         input.ClusterID,
		RoleName:          input.RoleName,
		RoleKind:          input.RoleKind,
		EncryptedPassword: input.EncryptedPassword,
		PasswordVersion:   1,
		ExpiresAt:         input.ExpiresAt,
		Status:            "pending",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	return &PostgresRoleTx{Role: role, Operation: op, Command: cmd}, nil
}

// RetryCreateWithCommand reuses a failed role row and queues pg_ensure_role again.
func (r *PostgresRoleRepository) RetryCreateWithCommand(ctx context.Context, input CreateRoleTxInput) (*PostgresRoleTx, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin retry create role tx: %w", err)
	}
	defer tx.Rollback()

	var role PostgresRole
	var expiresAt sql.NullTime
	err = tx.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, role_name, role_kind, encrypted_password, password_version, expires_at, status, created_at, updated_at
		 FROM postgres_roles WHERE id = ?`), input.RoleID,
	).Scan(&role.ID, &role.ClusterID, &role.RoleName, &role.RoleKind, &role.EncryptedPassword, &role.PasswordVersion, &expiresAt, &role.Status, &role.CreatedAt, &role.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("role %q not found", input.RoleID)
	}
	if err != nil {
		return nil, fmt.Errorf("scan failed role for retry: %w", err)
	}
	if role.Status != "failed" {
		return nil, fmt.Errorf("role %q is not failed", input.RoleID)
	}
	if role.ClusterID != input.ClusterID || role.RoleName != input.RoleName {
		return nil, fmt.Errorf("role retry input does not match existing role")
	}
	if expiresAt.Valid {
		role.ExpiresAt = &expiresAt.Time
	}

	now := time.Now().UTC()
	role.PasswordVersion++
	role.RoleKind = input.RoleKind
	role.EncryptedPassword = input.EncryptedPassword
	role.ExpiresAt = input.ExpiresAt
	role.Status = "pending"
	role.UpdatedAt = now

	_, err = tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_roles
		 SET role_kind = ?, encrypted_password = ?, password_version = ?, expires_at = ?, status = 'pending', updated_at = ?
		 WHERE id = ?`),
		role.RoleKind, role.EncryptedPassword, role.PasswordVersion, role.ExpiresAt, now, role.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("update failed role for retry: %w", err)
	}

	op, err := insertPostgresOperation(ctx, tx, input.OperationID, input.ClusterID, input.NodeID, "create_role", now)
	if err != nil {
		return nil, err
	}
	if input.BeforeAction != "" {
		if _, err := insertAgentCommand(ctx, tx, "", input.AgentID, input.NodeID, input.BeforeAction, input.BeforePayload, now.Add(-time.Millisecond)); err != nil {
			return nil, err
		}
	}
	cmd, err := insertAgentCommand(ctx, tx, input.CommandID, input.AgentID, input.NodeID, "pg_ensure_role", input.Payload, now)
	if err != nil {
		return nil, err
	}
	if err := insertCommandSecret(ctx, tx, cmd.ID, "password", input.EncryptedCommandSecret, input.SecretExpiresAt, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_operations SET status = 'running', updated_at = ? WHERE id = ?`), now, op.ID); err != nil {
		return nil, fmt.Errorf("mark retry operation running: %w", err)
	}
	op.Status = "running"

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit retry create role tx: %w", err)
	}
	return &PostgresRoleTx{Role: &role, Operation: op, Command: cmd}, nil
}

// RotateWithCommand atomically updates a role password/version and queues the rotate command.
func (r *PostgresRoleRepository) RotateWithCommand(ctx context.Context, input RotateRoleTxInput) (*PostgresRoleTx, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin rotate role tx: %w", err)
	}
	defer tx.Rollback()

	var role PostgresRole
	var expiresAt sql.NullTime
	err = tx.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, role_name, role_kind, encrypted_password, password_version, expires_at, status, created_at, updated_at
		 FROM postgres_roles WHERE id = ?`), input.RoleID,
	).Scan(&role.ID, &role.ClusterID, &role.RoleName, &role.RoleKind, &role.EncryptedPassword, &role.PasswordVersion, &expiresAt, &role.Status, &role.CreatedAt, &role.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("role %q not found", input.RoleID)
	}
	if err != nil {
		return nil, fmt.Errorf("scan postgres role for rotate: %w", err)
	}
	if expiresAt.Valid {
		role.ExpiresAt = &expiresAt.Time
	}
	if role.Status == "deleting" {
		return nil, fmt.Errorf("role is being deleted")
	}

	now := time.Now().UTC()
	role.PasswordVersion++
	role.EncryptedPassword = input.EncryptedPassword
	role.Status = "pending"
	role.UpdatedAt = now

	_, err = tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_roles SET encrypted_password = ?, password_version = ?, status = 'pending', updated_at = ? WHERE id = ?`),
		input.EncryptedPassword, role.PasswordVersion, now, input.RoleID,
	)
	if err != nil {
		return nil, fmt.Errorf("update role password: %w", err)
	}

	op, err := insertPostgresOperation(ctx, tx, input.OperationID, role.ClusterID, input.NodeID, "rotate_role_password", now)
	if err != nil {
		return nil, err
	}
	if input.BeforeAction != "" {
		if _, err := insertAgentCommand(ctx, tx, "", input.AgentID, input.NodeID, input.BeforeAction, input.BeforePayload, now.Add(-time.Millisecond)); err != nil {
			return nil, err
		}
	}
	cmd, err := insertAgentCommand(ctx, tx, input.CommandID, input.AgentID, input.NodeID, "pg_rotate_role_password", input.Payload, now)
	if err != nil {
		return nil, err
	}
	if err := insertCommandSecret(ctx, tx, cmd.ID, "password", input.EncryptedCommandSecret, input.SecretExpiresAt, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_operations SET status = 'running', updated_at = ? WHERE id = ?`), now, op.ID); err != nil {
		return nil, fmt.Errorf("mark operation running: %w", err)
	}
	op.Status = "running"

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit rotate role tx: %w", err)
	}
	return &PostgresRoleTx{Role: &role, Operation: op, Command: cmd}, nil
}

// DeleteWithCommand atomically marks a role deleting and queues the drop command.
func (r *PostgresRoleRepository) DeleteWithCommand(ctx context.Context, input DeleteRoleTxInput) (*PostgresRoleTx, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin delete role tx: %w", err)
	}
	defer tx.Rollback()

	var role PostgresRole
	var expiresAt sql.NullTime
	err = tx.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, role_name, role_kind, encrypted_password, password_version, expires_at, status, created_at, updated_at
		 FROM postgres_roles WHERE id = ?`), input.RoleID,
	).Scan(&role.ID, &role.ClusterID, &role.RoleName, &role.RoleKind, &role.EncryptedPassword, &role.PasswordVersion, &expiresAt, &role.Status, &role.CreatedAt, &role.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("role %q not found", input.RoleID)
	}
	if err != nil {
		return nil, fmt.Errorf("scan postgres role for delete: %w", err)
	}
	if expiresAt.Valid {
		role.ExpiresAt = &expiresAt.Time
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_roles SET status = 'deleting', updated_at = ? WHERE id = ?`), now, input.RoleID); err != nil {
		return nil, fmt.Errorf("mark role deleting: %w", err)
	}
	role.Status = "deleting"
	role.UpdatedAt = now

	op, err := insertPostgresOperation(ctx, tx, input.OperationID, role.ClusterID, input.NodeID, "delete_role", now)
	if err != nil {
		return nil, err
	}
	if input.BeforeAction != "" {
		if _, err := insertAgentCommand(ctx, tx, "", input.AgentID, input.NodeID, input.BeforeAction, input.BeforePayload, now.Add(-time.Millisecond)); err != nil {
			return nil, err
		}
	}
	cmd, err := insertAgentCommand(ctx, tx, input.CommandID, input.AgentID, input.NodeID, "pg_drop_role", input.Payload, now)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_operations SET status = 'running', updated_at = ? WHERE id = ?`), now, op.ID); err != nil {
		return nil, fmt.Errorf("mark operation running: %w", err)
	}
	op.Status = "running"

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit delete role tx: %w", err)
	}
	return &PostgresRoleTx{Role: &role, Operation: op, Command: cmd}, nil
}

// HandleCommandResult applies role and operation state transitions for completed role commands.
func (r *PostgresRoleRepository) HandleCommandResult(ctx context.Context, commandID string, success bool, errMsg string) (bool, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin role command result tx: %w", err)
	}
	defer tx.Rollback()

	var action, payload string
	err = tx.QueryRowContext(ctx,
		Rebind(`SELECT action, payload FROM agent_commands WHERE id = ?`), commandID,
	).Scan(&action, &payload)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get command for role result: %w", err)
	}

	operationType := operationTypeForRoleAction(action)
	if operationType == "" {
		return false, nil
	}

	var rolePayload struct {
		RoleID      string `json:"role_id"`
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal([]byte(payload), &rolePayload); err != nil {
		return true, fmt.Errorf("parse role command payload: %w", err)
	}
	if rolePayload.RoleID == "" {
		return true, fmt.Errorf("role command payload missing role_id")
	}

	now := time.Now().UTC()
	if success {
		switch action {
		case "pg_drop_role":
			if _, err := tx.ExecContext(ctx, Rebind(`DELETE FROM postgres_roles WHERE id = ?`), rolePayload.RoleID); err != nil {
				return true, fmt.Errorf("delete role after drop: %w", err)
			}
		default:
			if _, err := tx.ExecContext(ctx, Rebind(`UPDATE postgres_roles SET status = 'ready', updated_at = ? WHERE id = ?`), now, rolePayload.RoleID); err != nil {
				return true, fmt.Errorf("mark role ready: %w", err)
			}
		}
	} else {
		if _, err := tx.ExecContext(ctx, Rebind(`UPDATE postgres_roles SET status = 'failed', updated_at = ? WHERE id = ?`), now, rolePayload.RoleID); err != nil {
			return true, fmt.Errorf("mark role failed: %w", err)
		}
	}

	opStatus := "succeeded"
	if !success {
		opStatus = "failed"
	}
	if rolePayload.OperationID == "" {
		return true, fmt.Errorf("role command payload missing operation_id")
	}
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE postgres_operations SET status = ?, error = ?, updated_at = ?, completed_at = ? WHERE id = ? AND operation_type = ?`),
		opStatus, errMsg, now, now, rolePayload.OperationID, operationType,
	); err != nil {
		return true, fmt.Errorf("update role operation: %w", err)
	}

	if _, err := tx.ExecContext(ctx, Rebind(`DELETE FROM agent_command_secrets WHERE command_id = ?`), commandID); err != nil {
		return true, fmt.Errorf("delete command secrets: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return true, fmt.Errorf("commit role command result tx: %w", err)
	}
	return true, nil
}

type CreateRoleTxInput struct {
	RoleID                 string
	OperationID            string
	CommandID              string
	ClusterID              string
	NodeID                 string
	AgentID                string
	RoleName               string
	RoleKind               string
	EncryptedPassword      string
	Payload                string
	BeforeAction           string
	BeforePayload          string
	EncryptedCommandSecret []byte
	SecretExpiresAt        *time.Time
	ExpiresAt              *time.Time
}

type RotateRoleTxInput struct {
	RoleID                 string
	OperationID            string
	CommandID              string
	NodeID                 string
	AgentID                string
	EncryptedPassword      string
	Payload                string
	BeforeAction           string
	BeforePayload          string
	EncryptedCommandSecret []byte
	SecretExpiresAt        *time.Time
}

type DeleteRoleTxInput struct {
	RoleID        string
	OperationID   string
	CommandID     string
	NodeID        string
	AgentID       string
	Payload       string
	BeforeAction  string
	BeforePayload string
}

func insertPostgresOperation(ctx context.Context, tx *sql.Tx, opID, clusterID, nodeID, operationType string, now time.Time) (*PostgresOperation, error) {
	if opID == "" {
		opID = id.New()
	}
	var nodeIDArg interface{}
	if nodeID != "" {
		nodeIDArg = nodeID
	}
	_, err := tx.ExecContext(ctx,
		Rebind(`INSERT INTO postgres_operations
		 (id, cluster_id, node_id, operation_type, status, error, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'pending', '', ?, ?)`),
		opID, clusterID, nodeIDArg, operationType, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert postgres operation: %w", err)
	}
	return &PostgresOperation{ID: opID, ClusterID: clusterID, NodeID: nodeID, OperationType: operationType, Status: "pending", CreatedAt: now, UpdatedAt: now}, nil
}

func insertAgentCommand(ctx context.Context, tx *sql.Tx, cmdID, agentID, nodeID, action, payload string, now time.Time) (*AgentCommand, error) {
	if cmdID == "" {
		cmdID = id.New()
	}
	_, err := tx.ExecContext(ctx,
		Rebind(`INSERT INTO agent_commands (id, agent_id, node_id, action, payload, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		cmdID, agentID, nodeID, action, payload, "pending", now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert agent command: %w", err)
	}
	return &AgentCommand{ID: cmdID, AgentID: agentID, NodeID: nodeID, Action: action, Payload: payload, Status: "pending", CreatedAt: now}, nil
}

func insertCommandSecret(ctx context.Context, tx *sql.Tx, commandID, key string, ciphertext []byte, expiresAt *time.Time, now time.Time) error {
	secretID := id.New()
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	_, err := tx.ExecContext(ctx,
		Rebind(`INSERT INTO agent_command_secrets (id, command_id, key, ciphertext, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`),
		secretID, commandID, key, encoded, now, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert command secret: %w", err)
	}
	return nil
}

func operationTypeForRoleAction(action string) string {
	switch action {
	case "pg_ensure_role":
		return "create_role"
	case "pg_rotate_role_password":
		return "rotate_role_password"
	case "pg_drop_role":
		return "delete_role"
	default:
		return ""
	}
}

// scanPostgresRole scans a single *sql.Row.
func scanPostgresRole(row *sql.Row) (*PostgresRole, error) {
	var role PostgresRole
	var expiresAt sql.NullTime
	err := row.Scan(
		&role.ID, &role.ClusterID, &role.RoleName, &role.RoleKind,
		&role.EncryptedPassword, &role.PasswordVersion, &expiresAt,
		&role.Status, &role.CreatedAt, &role.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan postgres role: %w", err)
	}
	if expiresAt.Valid {
		role.ExpiresAt = &expiresAt.Time
	}
	return &role, nil
}

// scanPostgresRoleRow scans from *sql.Rows.
func scanPostgresRoleRow(rows *sql.Rows) (*PostgresRole, error) {
	var role PostgresRole
	var expiresAt sql.NullTime
	err := rows.Scan(
		&role.ID, &role.ClusterID, &role.RoleName, &role.RoleKind,
		&role.EncryptedPassword, &role.PasswordVersion, &expiresAt,
		&role.Status, &role.CreatedAt, &role.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan postgres role row: %w", err)
	}
	if expiresAt.Valid {
		role.ExpiresAt = &expiresAt.Time
	}
	return &role, nil
}
