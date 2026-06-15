package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
)

type BackupRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

func NewBackupRepository(conn *sql.DB, log *slog.Logger) *BackupRepository {
	return &BackupRepository{conn: conn, log: log}
}

func (r *BackupRepository) CreateBackup(ctx context.Context, clusterID, nodeID, storageConfigID string, backupType models.BackupType, storagePath string) (*models.Backup, error) {
	backupID := id.New()
	now := time.Now().UTC()

	_, err := r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO backups (id, cluster_id, node_id, storage_config_id, type, storage_path, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		backupID, clusterID, nodeID, storageConfigID, backupType, storagePath, models.BackupStatusRunning, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert backup: %w", err)
	}

	return &models.Backup{
		ID:              backupID,
		ClusterID:       clusterID,
		NodeID:          nodeID,
		Type:            backupType,
		StoragePath:     storagePath,
		StorageConfigID: storageConfigID,
		Status:          models.BackupStatusRunning,
		CreatedAt:       now,
	}, nil
}

func (r *BackupRepository) GetBackup(ctx context.Context, id string) (*models.Backup, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, node_id, type, storage_path, storage_config_id, wal_start, wal_stop, lsn, size_bytes, status, created_at, completed_at
		 FROM backups WHERE id = ?`), id)
	return scanBackupRow(row)
}

func (r *BackupRepository) ListBackups(ctx context.Context, clusterID string, offset, limit int) ([]*models.Backup, int, error) {
	var total int
	if err := r.conn.QueryRowContext(ctx, Rebind(`SELECT COUNT(*) FROM backups WHERE cluster_id = ?`), clusterID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count backups: %w", err)
	}

	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT id, cluster_id, node_id, type, storage_path, storage_config_id, wal_start, wal_stop, lsn, size_bytes, status, created_at, completed_at
		 FROM backups WHERE cluster_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`),
		clusterID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query backups: %w", err)
	}
	defer rows.Close()

	var backups []*models.Backup
	for rows.Next() {
		b, err := scanBackupsRow(rows)
		if err != nil {
			return nil, 0, err
		}
		backups = append(backups, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate backups: %w", err)
	}

	return backups, total, nil
}

func (r *BackupRepository) CompleteBackup(ctx context.Context, id string, walStart, walStop, lsn string, sizeBytes int64) error {
	now := time.Now().UTC()
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE backups SET status = ?, wal_start = ?, wal_stop = ?, lsn = ?, size_bytes = ?, completed_at = ? WHERE id = ?`),
		models.BackupStatusCompleted, walStart, walStop, lsn, sizeBytes, now, id,
	)
	if err != nil {
		return fmt.Errorf("complete backup: %w", err)
	}
	return nil
}

func (r *BackupRepository) FailBackup(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE backups SET status = ?, completed_at = ? WHERE id = ?`),
		models.BackupStatusFailed, now, id,
	)
	if err != nil {
		return fmt.Errorf("fail backup: %w", err)
	}
	return nil
}

func (r *BackupRepository) DeleteBackup(ctx context.Context, id string) error {
	_, err := r.conn.ExecContext(ctx, Rebind(`DELETE FROM backups WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

func (r *BackupRepository) CreateRestoreJob(ctx context.Context, sourceClusterID string, backupID string, targetType models.RestoreTargetType, targetValue string, targetNodeID string) (*models.RestoreJob, error) {
	jobID := id.New()
	now := time.Now().UTC()

	_, err := r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO restore_jobs (id, cluster_id, backup_id, target_type, target_value, target_node, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		jobID, sourceClusterID, backupID, targetType, targetValue, targetNodeID, models.RestoreStatusRunning, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert restore job: %w", err)
	}

	return &models.RestoreJob{
		ID:               jobID,
		SourceClusterID:  sourceClusterID,
		TargetType:       targetType,
		TargetValue:      targetValue,
		TargetNodeID:     targetNodeID,
		Status:           models.RestoreStatusRunning,
		CreatedAt:        now,
	}, nil
}

func (r *BackupRepository) GetRestoreJob(ctx context.Context, id string) (*models.RestoreJob, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT id, cluster_id, target_type, target_value, target_node, status, created_at, completed_at
		 FROM restore_jobs WHERE id = ?`), id)
	return scanRestoreJobRow(row)
}

func (r *BackupRepository) ListRestoreJobs(ctx context.Context, clusterID string, offset, limit int) ([]*models.RestoreJob, int, error) {
	var total int
	if err := r.conn.QueryRowContext(ctx, Rebind(`SELECT COUNT(*) FROM restore_jobs WHERE cluster_id = ?`), clusterID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count restore jobs: %w", err)
	}

	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT id, cluster_id, target_type, target_value, target_node, status, created_at, completed_at
		 FROM restore_jobs WHERE cluster_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`),
		clusterID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query restore jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*models.RestoreJob
	for rows.Next() {
		j, err := scanRestoreJobsRow(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate restore jobs: %w", err)
	}

	return jobs, total, nil
}

func (r *BackupRepository) CompleteRestoreJob(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE restore_jobs SET status = ?, completed_at = ? WHERE id = ?`),
		models.RestoreStatusCompleted, now, id,
	)
	if err != nil {
		return fmt.Errorf("complete restore job: %w", err)
	}
	return nil
}

func (r *BackupRepository) FailRestoreJob(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE restore_jobs SET status = ?, completed_at = ? WHERE id = ?`),
		models.RestoreStatusFailed, now, id,
	)
	if err != nil {
		return fmt.Errorf("fail restore job: %w", err)
	}
	return nil
}

func (r *BackupRepository) CreateSchedule(ctx context.Context, clusterID, cron string, backupType models.BackupType, retentionCount, retentionDays int, storageConfigID string) (*models.BackupSchedule, error) {
	scheduleID := id.New()
	now := time.Now().UTC()

	_, err := r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO backup_schedules (id, cluster_id, cron, type, retention_count, retention_days, storage_config_id, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`),
		scheduleID, clusterID, cron, backupType, retentionCount, retentionDays, storageConfigID, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert schedule: %w", err)
	}

	return &models.BackupSchedule{
		ID:              scheduleID,
		ClusterID:       clusterID,
		Cron:            cron,
		Type:            backupType,
		RetentionCount:  retentionCount,
		RetentionDays:   retentionDays,
		StorageConfigID: storageConfigID,
		Enabled:         true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func (r *BackupRepository) ListSchedules(ctx context.Context, clusterID string) ([]*models.BackupSchedule, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT id, cluster_id, cron, type, retention_count, retention_days, storage_config_id, enabled, created_at, updated_at
		 FROM backup_schedules WHERE cluster_id = ? ORDER BY created_at ASC`), clusterID)
	if err != nil {
		return nil, fmt.Errorf("query schedules: %w", err)
	}
	defer rows.Close()

	var schedules []*models.BackupSchedule
	for rows.Next() {
		var s models.BackupSchedule
		var enabledInt int
		if err := rows.Scan(&s.ID, &s.ClusterID, &s.Cron, &s.Type, &s.RetentionCount, &s.RetentionDays,
			&s.StorageConfigID, &enabledInt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		s.Enabled = intToBool(enabledInt)
		schedules = append(schedules, &s)
	}
	return schedules, rows.Err()
}

func (r *BackupRepository) UpdateSchedule(ctx context.Context, id string, cron string, backupType models.BackupType, retentionCount, retentionDays int, storageConfigID string, enabled bool) (*models.BackupSchedule, error) {
	now := time.Now().UTC()
	enabledInt := boolToInt(enabled)

	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE backup_schedules SET cron = ?, type = ?, retention_count = ?, retention_days = ?, storage_config_id = ?, enabled = ?, updated_at = ? WHERE id = ?`),
		cron, backupType, retentionCount, retentionDays, storageConfigID, enabledInt, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}

	return &models.BackupSchedule{
		ID:              id,
		Cron:            cron,
		Type:            backupType,
		RetentionCount:  retentionCount,
		RetentionDays:   retentionDays,
		StorageConfigID: storageConfigID,
		Enabled:         enabled,
		UpdatedAt:       now,
	}, nil
}

func (r *BackupRepository) DeleteSchedule(ctx context.Context, id string) error {
	_, err := r.conn.ExecContext(ctx, Rebind(`DELETE FROM backup_schedules WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	return nil
}

func scanBackupRow(row *sql.Row) (*models.Backup, error) {
	var b models.Backup
	var walStart, walStop, lsn sql.NullString
	var completedAt sql.NullTime

	err := row.Scan(&b.ID, &b.ClusterID, &b.NodeID, &b.Type, &b.StoragePath, &b.StorageConfigID,
		&walStart, &walStop, &lsn, &b.SizeBytes, &b.Status, &b.CreatedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan backup: %w", err)
	}

	b.WALStart = walStart.String
	b.WALStop = walStop.String
	b.LSN = lsn.String
	if completedAt.Valid {
		b.CompletedAt = &completedAt.Time
	}
	return &b, nil
}

func scanBackupsRow(rows *sql.Rows) (*models.Backup, error) {
	var b models.Backup
	var walStart, walStop, lsn sql.NullString
	var completedAt sql.NullTime

	if err := rows.Scan(&b.ID, &b.ClusterID, &b.NodeID, &b.Type, &b.StoragePath, &b.StorageConfigID,
		&walStart, &walStop, &lsn, &b.SizeBytes, &b.Status, &b.CreatedAt, &completedAt); err != nil {
		return nil, fmt.Errorf("scan backup row: %w", err)
	}

	b.WALStart = walStart.String
	b.WALStop = walStop.String
	b.LSN = lsn.String
	if completedAt.Valid {
		b.CompletedAt = &completedAt.Time
	}
	return &b, nil
}

func scanRestoreJobRow(row *sql.Row) (*models.RestoreJob, error) {
	var j models.RestoreJob
	var targetValue, targetNode sql.NullString
	var completedAt sql.NullTime

	err := row.Scan(&j.ID, &j.SourceClusterID, &j.TargetType, &targetValue, &targetNode, &j.Status, &j.CreatedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan restore job: %w", err)
	}

	j.TargetValue = targetValue.String
	j.TargetNodeID = targetNode.String
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}
	return &j, nil
}

func scanRestoreJobsRow(rows *sql.Rows) (*models.RestoreJob, error) {
	var j models.RestoreJob
	var targetValue, targetNode sql.NullString
	var completedAt sql.NullTime

	if err := rows.Scan(&j.ID, &j.SourceClusterID, &j.TargetType, &targetValue, &targetNode, &j.Status, &j.CreatedAt, &completedAt); err != nil {
		return nil, fmt.Errorf("scan restore job row: %w", err)
	}

	j.TargetValue = targetValue.String
	j.TargetNodeID = targetNode.String
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}
	return &j, nil
}