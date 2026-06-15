package backup

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
)

type Worker struct {
	engine   *Engine
	backups  *db.BackupRepository
	schedules *db.BackupRepository
	clusters *db.ClusterRepository
	cron     *cron.Cron
	log      *slog.Logger
	mu       sync.Mutex
	jobs     map[string]cron.EntryID
}

func NewWorker(engine *Engine, backups *db.BackupRepository, clusters *db.ClusterRepository, log *slog.Logger) *Worker {
	return &Worker{
		engine:   engine,
		backups:  backups,
		clusters: clusters,
		cron:     cron.New(cron.WithSeconds()),
		log:      log,
		jobs:     make(map[string]cron.EntryID),
	}
}

func (w *Worker) Start(ctx context.Context) error {
	w.cron.Start()

	go w.processBackupQueue(ctx)
	go w.processRestoreQueue(ctx)

	go func() {
		<-ctx.Done()
		w.cron.Stop()
	}()

	w.log.Info("backup worker started")
	return nil
}

func (w *Worker) ScheduleBackup(schedule *models.BackupSchedule) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if entryID, exists := w.jobs[schedule.ID]; exists {
		w.cron.Remove(entryID)
		delete(w.jobs, schedule.ID)
	}

	if !schedule.Enabled {
		return nil
	}

	entryID, err := w.cron.AddFunc(schedule.Cron, func() {
		w.log.Info("scheduled backup triggered", "schedule_id", schedule.ID, "cluster_id", schedule.ClusterID)
		if err := w.createScheduledBackup(context.Background(), schedule); err != nil {
			w.log.Error("scheduled backup failed", "schedule_id", schedule.ID, "error", err)
		}
	})
	if err != nil {
		return err
	}

	w.jobs[schedule.ID] = entryID
	w.log.Info("scheduled backup", "schedule_id", schedule.ID, "cron", schedule.Cron)
	return nil
}

func (w *Worker) UnscheduleBackup(scheduleID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if entryID, exists := w.jobs[scheduleID]; exists {
		w.cron.Remove(entryID)
		delete(w.jobs, scheduleID)
	}
}

func (w *Worker) createScheduledBackup(ctx context.Context, schedule *models.BackupSchedule) error {
	cluster, err := w.clusters.GetByID(ctx, schedule.ClusterID)
	if err != nil || cluster == nil {
		return err
	}

	storagePath := w.engine.BuildStoragePath(schedule.ClusterID, schedule.Type, time.Now().UTC())

	backup, err := w.backups.CreateBackup(ctx, schedule.ClusterID, "", schedule.StorageConfigID, schedule.Type, storagePath)
	if err != nil {
		return err
	}

	return w.engine.ExecuteBackup(ctx, backup.ID)
}

func (w *Worker) processBackupQueue(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processPendingBackups(ctx)
		}
	}
}

func (w *Worker) processPendingBackups(ctx context.Context) {
	// poll for running backups that need resuming — in MVP, backups are executed synchronously via ExecuteBackup
}

func (w *Worker) processRestoreQueue(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processPendingRestores(ctx)
		}
	}
}

func (w *Worker) processPendingRestores(ctx context.Context) {
	// poll for running restore jobs that need resuming — in MVP, restores are executed synchronously via ExecuteRestore
}