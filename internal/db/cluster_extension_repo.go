package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/engine"
	"github.com/zhinea/skylex/internal/id"
)

// ClusterExtension is the per-cluster desired state of a single database
// extension. It is configuration state (one row per (cluster, extension)),
// engine-derived via cluster_id -> clusters.engine.
type ClusterExtension struct {
	ClusterID     string
	ExtensionName string
	Enabled       bool
	Status        string // off | pending | ready | failed
	Error         string
	CommandID     string
	AppliedAt     *time.Time
	UpdatedAt     time.Time
}

// ClusterExtensionRepository manages cluster_extensions rows and their queued
// apply command.
type ClusterExtensionRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

func NewClusterExtensionRepository(conn *sql.DB, log *slog.Logger) *ClusterExtensionRepository {
	return &ClusterExtensionRepository{conn: conn, log: log}
}

// ApplyExtensionsCommand carries the queued apply command for the primary node.
type ApplyExtensionsCommand struct {
	ClusterID string
	NodeID    string
	AgentID   string
	CommandID string
	Payload   string
	// EncryptedAdminSecret, when non-nil, is attached to the queued command under
	// the given key so the agent can authenticate to PostgreSQL as the durable
	// skylex_admin SUPERUSER. Omitted (nil) for clusters that predate Phase 2.
	EncryptedAdminSecret []byte
	AdminSecretKey       string
	SecretExpiresAt      *time.Time
}

func (r *ClusterExtensionRepository) ListByCluster(ctx context.Context, clusterID string) ([]*ClusterExtension, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT cluster_id, extension_name, enabled, status, error, command_id, applied_at, updated_at
		 FROM cluster_extensions WHERE cluster_id = ? ORDER BY extension_name ASC`), clusterID)
	if err != nil {
		return nil, fmt.Errorf("list cluster extensions: %w", err)
	}
	defer rows.Close()

	out := []*ClusterExtension{}
	for rows.Next() {
		ext, err := scanClusterExtension(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ext)
	}
	return out, rows.Err()
}

// SetEnabled upserts the desired enabled state for a single extension. Toggling
// does not change applied status; the change only takes effect on the next
// ApplyExtensions. Disabling resets status to 'off' once it is no longer ready.
func (r *ClusterExtensionRepository) SetEnabled(ctx context.Context, clusterID, extensionName string, enabled bool) (*ClusterExtension, error) {
	now := time.Now().UTC()
	// Insert or update. We keep the existing status so the UI can show that a
	// previously-applied extension is still active until the next Apply.
	if _, err := r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO cluster_extensions (cluster_id, extension_name, enabled, status, error, created_at, updated_at)
		 VALUES (?, ?, ?, 'off', '', ?, ?)
		 ON CONFLICT (cluster_id, extension_name)
		 DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`),
		clusterID, extensionName, enabled, now, now,
	); err != nil {
		return nil, fmt.Errorf("upsert cluster extension: %w", err)
	}
	return r.get(ctx, clusterID, extensionName)
}

func (r *ClusterExtensionRepository) get(ctx context.Context, clusterID, extensionName string) (*ClusterExtension, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT cluster_id, extension_name, enabled, status, error, command_id, applied_at, updated_at
		 FROM cluster_extensions WHERE cluster_id = ? AND extension_name = ?`), clusterID, extensionName)
	return scanClusterExtensionRow(row)
}

// QueueApplyCommand inserts a single agent command (run on the primary) and
// marks every toggled extension row 'pending', tying it to the command. A single
// command applies the full desired state, avoiding per-extension command fan-out.
func (r *ClusterExtensionRepository) QueueApplyCommand(ctx context.Context, cmd ApplyExtensionsCommand) error {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin queue apply extensions tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	commandID := cmd.CommandID
	if commandID == "" {
		commandID = id.New()
	}
	if _, err := insertAgentCommand(ctx, tx, commandID, cmd.AgentID, cmd.NodeID, "pg_apply_extensions", cmd.Payload, now); err != nil {
		return err
	}
	if len(cmd.EncryptedAdminSecret) > 0 && cmd.AdminSecretKey != "" {
		if err := insertCommandSecret(ctx, tx, commandID, cmd.AdminSecretKey, cmd.EncryptedAdminSecret, cmd.SecretExpiresAt, now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		Rebind(`UPDATE cluster_extensions SET status = 'pending', error = '', command_id = ?, updated_at = ? WHERE cluster_id = ?`),
		commandID, now, cmd.ClusterID,
	); err != nil {
		return fmt.Errorf("mark extensions pending: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit queue apply extensions tx: %w", err)
	}
	return nil
}

// HandleCommandResult applies the apply-extensions command result. On success,
// enabled rows become 'ready' and disabled rows are removed (their extension was
// dropped). On failure all pending rows are marked 'failed'. Returns handled=true
// only when the command is an apply_extensions command for this engine.
func (r *ClusterExtensionRepository) HandleCommandResult(ctx context.Context, commandID string, success bool, errMsg string) (bool, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin extensions command result tx: %w", err)
	}
	defer tx.Rollback()

	var action, payload string
	if err := tx.QueryRowContext(ctx,
		Rebind(`SELECT action, payload FROM agent_commands WHERE id = ?`), commandID,
	).Scan(&action, &payload); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("get command for extensions result: %w", err)
	}
	if op, ok := engine.LogicalOpForAction(action); !ok || op != engine.OpApplyExtensions {
		return false, nil
	}

	var p struct {
		ClusterID string `json:"cluster_id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return true, fmt.Errorf("parse extensions command payload: %w", err)
	}
	if p.ClusterID == "" {
		return true, fmt.Errorf("extensions command payload missing cluster_id")
	}

	now := time.Now().UTC()
	if success {
		// Disabled extensions were dropped on the node; remove their rows.
		if _, err := tx.ExecContext(ctx,
			Rebind(`DELETE FROM cluster_extensions WHERE cluster_id = ? AND command_id = ? AND enabled = ?`),
			p.ClusterID, commandID, false,
		); err != nil {
			return true, fmt.Errorf("remove disabled extensions: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			Rebind(`UPDATE cluster_extensions SET status = 'ready', error = '', applied_at = ?, updated_at = ?
			 WHERE cluster_id = ? AND command_id = ? AND enabled = ?`),
			now, now, p.ClusterID, commandID, true,
		); err != nil {
			return true, fmt.Errorf("mark extensions ready: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			Rebind(`UPDATE cluster_extensions SET status = 'failed', error = ?, updated_at = ?
			 WHERE cluster_id = ? AND command_id = ?`),
			RedactStoredError(errMsg), now, p.ClusterID, commandID,
		); err != nil {
			return true, fmt.Errorf("mark extensions failed: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return true, fmt.Errorf("commit extensions command result tx: %w", err)
	}
	return true, nil
}

func scanClusterExtension(rows *sql.Rows) (*ClusterExtension, error) {
	var ext ClusterExtension
	var commandID sql.NullString
	var appliedAt sql.NullTime
	if err := rows.Scan(&ext.ClusterID, &ext.ExtensionName, &ext.Enabled, &ext.Status, &ext.Error, &commandID, &appliedAt, &ext.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan cluster extension: %w", err)
	}
	if commandID.Valid {
		ext.CommandID = commandID.String
	}
	if appliedAt.Valid {
		ext.AppliedAt = &appliedAt.Time
	}
	return &ext, nil
}

func scanClusterExtensionRow(row *sql.Row) (*ClusterExtension, error) {
	var ext ClusterExtension
	var commandID sql.NullString
	var appliedAt sql.NullTime
	err := row.Scan(&ext.ClusterID, &ext.ExtensionName, &ext.Enabled, &ext.Status, &ext.Error, &commandID, &appliedAt, &ext.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan cluster extension row: %w", err)
	}
	if commandID.Valid {
		ext.CommandID = commandID.String
	}
	if appliedAt.Valid {
		ext.AppliedAt = &appliedAt.Time
	}
	return &ext, nil
}
