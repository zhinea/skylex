package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/id"
)

// ServiceOperation records an async operation dispatched for a cluster.
type ServiceOperation struct {
	ID            string
	ClusterID     string
	NodeID        string // may be empty
	OperationType string
	Status        string // pending | running | succeeded | failed
	Error         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CompletedAt   *time.Time
}

// ServiceOperationRepository manages service_operations rows.
type ServiceOperationRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

func NewServiceOperationRepository(conn *sql.DB, log *slog.Logger) *ServiceOperationRepository {
	return &ServiceOperationRepository{conn: conn, log: log}
}

func (r *ServiceOperationRepository) Create(ctx context.Context, clusterID, nodeID, operationType string) (*ServiceOperation, error) {
	opID := id.New()
	now := time.Now().UTC()

	var nodeIDArg interface{}
	if nodeID != "" {
		nodeIDArg = nodeID
	}

	_, err := r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO service_operations
		 (id, cluster_id, node_id, operation_type, status, error, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'pending', '', ?, ?)`),
		opID, clusterID, nodeIDArg, operationType, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert postgres operation: %w", err)
	}

	return &ServiceOperation{
		ID:            opID,
		ClusterID:     clusterID,
		NodeID:        nodeID,
		OperationType: operationType,
		Status:        "pending",
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (r *ServiceOperationRepository) UpdateStatus(ctx context.Context, opID, status, errMsg string) error {
	now := time.Now().UTC()
	var completedAt interface{}
	if status == "succeeded" || status == "failed" {
		completedAt = now
	}

	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE service_operations SET status = ?, error = ?, updated_at = ?, completed_at = ? WHERE id = ?`),
		status, errMsg, now, completedAt, opID,
	)
	if err != nil {
		return fmt.Errorf("update postgres operation status: %w", err)
	}
	return nil
}

func (r *ServiceOperationRepository) ListByCluster(ctx context.Context, clusterID string, limit int) ([]*ServiceOperation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT id, cluster_id, node_id, operation_type, status, error, created_at, updated_at, completed_at
		 FROM service_operations WHERE cluster_id = ? ORDER BY created_at DESC LIMIT ?`), clusterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list postgres operations: %w", err)
	}
	defer rows.Close()

	var ops []*ServiceOperation
	for rows.Next() {
		op, err := scanServiceOperationRow(rows)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func scanServiceOperationRow(rows *sql.Rows) (*ServiceOperation, error) {
	var op ServiceOperation
	var nodeID sql.NullString
	var completedAt sql.NullTime
	err := rows.Scan(
		&op.ID, &op.ClusterID, &nodeID, &op.OperationType,
		&op.Status, &op.Error, &op.CreatedAt, &op.UpdatedAt, &completedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan postgres operation: %w", err)
	}
	if nodeID.Valid {
		op.NodeID = nodeID.String
	}
	if completedAt.Valid {
		op.CompletedAt = &completedAt.Time
	}
	return &op, nil
}
