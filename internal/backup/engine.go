package backup

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
)

type Engine struct {
	backups       *db.BackupRepository
	storageConfig *db.StorageConfigRepository
	pgBackRest    *PgBackRest
	log           *slog.Logger
}

func NewEngine(backups *db.BackupRepository, storageConfig *db.StorageConfigRepository, pgBackRest *PgBackRest, log *slog.Logger) *Engine {
	return &Engine{
		backups:       backups,
		storageConfig: storageConfig,
		pgBackRest:    pgBackRest,
		log:           log,
	}
}

func (e *Engine) ExecuteBackup(ctx context.Context, backupID string) error {
	backup, err := e.backups.GetBackup(ctx, backupID)
	if err != nil {
		return fmt.Errorf("get backup: %w", err)
	}
	if backup == nil {
		return fmt.Errorf("backup %s not found", backupID)
	}

	stanza := "skylex-" + backup.ClusterID

	if err := e.pgBackRest.StanzaCheck(ctx, stanza, "", ""); err != nil {
		if createErr := e.pgBackRest.StanzaCreate(ctx, stanza, "", ""); createErr != nil {
			e.backups.FailBackup(ctx, backupID)
			return fmt.Errorf("stanza create: %w", createErr)
		}
	}

	e.log.Info("starting backup", "backup_id", backupID, "cluster_id", backup.ClusterID, "type", backup.Type)

	output, err := e.pgBackRest.Backup(ctx, stanza, string(backup.Type), "", "")
	if err != nil {
		e.backups.FailBackup(ctx, backupID)
		return fmt.Errorf("backup execution: %w", err)
	}

	if err := e.backups.CompleteBackup(ctx, backupID, "wal_start", "wal_stop", "lsn", int64(len(output))); err != nil {
		return fmt.Errorf("complete backup record: %w", err)
	}

	e.log.Info("backup completed", "backup_id", backupID)
	return nil
}

func (e *Engine) ExecuteRestore(ctx context.Context, restoreJobID string) error {
	job, err := e.backups.GetRestoreJob(ctx, restoreJobID)
	if err != nil {
		return fmt.Errorf("get restore job: %w", err)
	}
	if job == nil {
		return fmt.Errorf("restore job %s not found", restoreJobID)
	}

	stanza := "skylex-" + job.SourceClusterID

	var targetTime, targetLSN string
	switch job.TargetType {
	case models.RestoreTargetTime:
		targetTime = job.TargetValue
	case models.RestoreTargetLSN:
		targetLSN = job.TargetValue
	case models.RestoreTargetLatest:
		// no target = restore to latest
	}

	e.log.Info("starting restore", "restore_job_id", restoreJobID, "cluster_id", job.SourceClusterID)

	if _, err := e.pgBackRest.Restore(ctx, stanza, targetTime, targetLSN, "", ""); err != nil {
		e.backups.FailRestoreJob(ctx, restoreJobID)
		return fmt.Errorf("restore execution: %w", err)
	}

	if err := e.backups.CompleteRestoreJob(ctx, restoreJobID); err != nil {
		return fmt.Errorf("complete restore record: %w", err)
	}

	e.log.Info("restore completed", "restore_job_id", restoreJobID)
	return nil
}

func (e *Engine) CleanupExpired(ctx context.Context, clusterID string, retentionCount, retentionDays int) error {
	stanza := "skylex-" + clusterID
	return e.pgBackRest.Expire(ctx, stanza, retentionCount, retentionDays, "")
}

func (e *Engine) BuildStoragePath(clusterID string, backupType models.BackupType, timestamp time.Time) string {
	return fmt.Sprintf("backups/%s/%s/%s", clusterID, backupType, timestamp.Format("20060102T150405Z"))
}