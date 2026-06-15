package server

import (
	"context"
	"log/slog"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/backup"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type BackupService struct {
	skylexv1.UnimplementedBackupServiceServer
	skylexv1.UnimplementedScheduleServiceServer
	backups   *db.BackupRepository
	clusters  *db.ClusterRepository
	engine    *backup.Engine
	worker    *backup.Worker
	log       *slog.Logger
}

func NewBackupService(backups *db.BackupRepository, clusters *db.ClusterRepository, engine *backup.Engine, worker *backup.Worker, log *slog.Logger) *BackupService {
	return &BackupService{
		backups:  backups,
		clusters: clusters,
		engine:   engine,
		worker:   worker,
		log:      log,
	}
}

func (s *BackupService) CreateBackup(ctx context.Context, req *skylexv1.CreateBackupRequest) (*skylexv1.CreateBackupResponse, error) {
	cluster, err := s.clusters.GetByID(ctx, req.GetClusterId())
	if err != nil || cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.GetClusterId())
	}

	backupType := convertBackupType(req.GetType())
	storagePath := s.engine.BuildStoragePath(req.GetClusterId(), backupType, time.Now().UTC())

	backupRecord, err := s.backups.CreateBackup(ctx, req.GetClusterId(), "", cluster.StorageConfigID, backupType, storagePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create backup: %v", err)
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := s.engine.ExecuteBackup(bgCtx, backupRecord.ID); err != nil {
			s.log.Error("backup failed", "backup_id", backupRecord.ID, "error", err)
		}
	}()

	return &skylexv1.CreateBackupResponse{
		Backup: backupToProto(backupRecord),
	}, nil
}

func (s *BackupService) ListBackups(ctx context.Context, req *skylexv1.ListBackupsRequest) (*skylexv1.ListBackupsResponse, error) {
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	backups, total, err := s.backups.ListBackups(ctx, req.GetClusterId(), offset, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list backups: %v", err)
	}

	var protoBackups []*skylexv1.Backup
	for _, b := range backups {
		protoBackups = append(protoBackups, backupToProto(b))
	}

	return &skylexv1.ListBackupsResponse{
		Backups: protoBackups,
		Pagination: &skylexv1.Pagination{
			Page:     int32(page),
			PageSize: int32(pageSize),
			Total:    int32(total),
		},
	}, nil
}

func (s *BackupService) GetBackup(ctx context.Context, req *skylexv1.GetBackupRequest) (*skylexv1.GetBackupResponse, error) {
	b, err := s.backups.GetBackup(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get backup: %v", err)
	}
	if b == nil {
		return nil, status.Errorf(codes.NotFound, "backup %q not found", req.GetId())
	}

	return &skylexv1.GetBackupResponse{
		Backup: backupToProto(b),
	}, nil
}

func (s *BackupService) DeleteBackup(ctx context.Context, req *skylexv1.DeleteBackupRequest) (*skylexv1.DeleteBackupResponse, error) {
	b, err := s.backups.GetBackup(ctx, req.GetId())
	if err != nil || b == nil {
		return nil, status.Errorf(codes.NotFound, "backup %q not found", req.GetId())
	}

	if err := s.backups.DeleteBackup(ctx, req.GetId()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete backup: %v", err)
	}

	return &skylexv1.DeleteBackupResponse{}, nil
}

func (s *BackupService) CreateRestoreJob(ctx context.Context, req *skylexv1.CreateRestoreJobRequest) (*skylexv1.CreateRestoreJobResponse, error) {
	cluster, err := s.clusters.GetByID(ctx, req.GetClusterId())
	if err != nil || cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.GetClusterId())
	}

	targetType, targetValue := convertRestoreTarget(req)

	job, err := s.backups.CreateRestoreJob(ctx, req.GetClusterId(), req.GetBackupId(), targetType, targetValue, req.GetTargetNode())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create restore job: %v", err)
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := s.engine.ExecuteRestore(bgCtx, job.ID); err != nil {
			s.log.Error("restore failed", "restore_job_id", job.ID, "error", err)
		}
	}()

	return &skylexv1.CreateRestoreJobResponse{
		RestoreJob: restoreJobToProto(job),
	}, nil
}

func (s *BackupService) ListRestoreJobs(ctx context.Context, req *skylexv1.ListRestoreJobsRequest) (*skylexv1.ListRestoreJobsResponse, error) {
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	jobs, total, err := s.backups.ListRestoreJobs(ctx, req.GetClusterId(), offset, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list restore jobs: %v", err)
	}

	var protoJobs []*skylexv1.RestoreJob
	for _, j := range jobs {
		protoJobs = append(protoJobs, restoreJobToProto(j))
	}

	return &skylexv1.ListRestoreJobsResponse{
		RestoreJobs: protoJobs,
		Pagination: &skylexv1.Pagination{
			Page:     int32(page),
			PageSize: int32(pageSize),
			Total:    int32(total),
		},
	}, nil
}

func (s *BackupService) CreateSchedule(ctx context.Context, req *skylexv1.CreateScheduleRequest) (*skylexv1.CreateScheduleResponse, error) {
	cluster, err := s.clusters.GetByID(ctx, req.GetClusterId())
	if err != nil || cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.GetClusterId())
	}

	backupType := convertBackupType(req.GetType())

	schedule, err := s.backups.CreateSchedule(ctx, req.GetClusterId(), req.GetCron(), backupType, int(req.GetRetentionCount()), int(req.GetRetentionDays()), req.GetStorageConfigId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create schedule: %v", err)
	}

	if err := s.worker.ScheduleBackup(schedule); err != nil {
		s.log.Warn("failed to schedule backup", "schedule_id", schedule.ID, "error", err)
	}

	return &skylexv1.CreateScheduleResponse{
		Schedule: scheduleToProto(schedule),
	}, nil
}

func (s *BackupService) ListSchedules(ctx context.Context, req *skylexv1.ListSchedulesRequest) (*skylexv1.ListSchedulesResponse, error) {
	schedules, err := s.backups.ListSchedules(ctx, req.GetClusterId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list schedules: %v", err)
	}

	var protoSchedules []*skylexv1.BackupSchedule
	for _, sch := range schedules {
		protoSchedules = append(protoSchedules, scheduleToProto(sch))
	}

	return &skylexv1.ListSchedulesResponse{
		Schedules: protoSchedules,
	}, nil
}

func (s *BackupService) UpdateSchedule(ctx context.Context, req *skylexv1.UpdateScheduleRequest) (*skylexv1.UpdateScheduleResponse, error) {
	backupType := convertBackupType(req.GetType())

	schedule, err := s.backups.UpdateSchedule(ctx, req.GetId(), req.GetCron(), backupType, int(req.GetRetentionCount()), int(req.GetRetentionDays()), req.GetStorageConfigId(), req.GetEnabled())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update schedule: %v", err)
	}

	if err := s.worker.ScheduleBackup(schedule); err != nil {
		s.log.Warn("failed to update schedule", "schedule_id", schedule.ID, "error", err)
	}

	return &skylexv1.UpdateScheduleResponse{
		Schedule: scheduleToProto(schedule),
	}, nil
}

func (s *BackupService) DeleteSchedule(ctx context.Context, req *skylexv1.DeleteScheduleRequest) (*skylexv1.DeleteScheduleResponse, error) {
	s.worker.UnscheduleBackup(req.GetId())

	if err := s.backups.DeleteSchedule(ctx, req.GetId()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete schedule: %v", err)
	}

	return &skylexv1.DeleteScheduleResponse{}, nil
}

func convertBackupType(t skylexv1.BackupType) models.BackupType {
	switch t {
	case skylexv1.BackupType_BACKUP_TYPE_INCREMENTAL:
		return models.BackupTypeIncremental
	case skylexv1.BackupType_BACKUP_TYPE_DIFFERENTIAL:
		return models.BackupTypeDifferential
	default:
		return models.BackupTypeFull
	}
}

func convertRestoreTarget(req *skylexv1.CreateRestoreJobRequest) (models.RestoreTargetType, string) {
	if req.GetTargetTime() != nil {
		return models.RestoreTargetTime, req.GetTargetTime().AsTime().Format(time.RFC3339)
	}
	if req.GetTargetLsn() != "" {
		return models.RestoreTargetLSN, req.GetTargetLsn()
	}
	return models.RestoreTargetLatest, ""
}

func backupToProto(b *models.Backup) *skylexv1.Backup {
	pb := &skylexv1.Backup{
		Id:        b.ID,
		ClusterId: b.ClusterID,
		NodeId:    b.NodeID,
		Type:      convertBackupTypeToProto(b.Type),
		StoragePath: b.StoragePath,
		WalStart:  b.WALStart,
		WalStop:   b.WALStop,
		Lsn:       b.LSN,
		SizeBytes: b.SizeBytes,
		Status:    convertBackupStatusToProto(b.Status),
		CreatedAt: timestamppb.New(b.CreatedAt),
	}

	if b.CompletedAt != nil {
		pb.CompletedAt = timestamppb.New(*b.CompletedAt)
	}
	return pb
}

func restoreJobToProto(j *models.RestoreJob) *skylexv1.RestoreJob {
	pb := &skylexv1.RestoreJob{
		Id:        j.ID,
		ClusterId: j.SourceClusterID,
		TargetNode: j.TargetNodeID,
		Status:    convertRestoreStatusToProto(j.Status),
		CreatedAt: timestamppb.New(j.CreatedAt),
	}

	if j.CompletedAt != nil {
		pb.CompletedAt = timestamppb.New(*j.CompletedAt)
	}
	return pb
}

func scheduleToProto(s *models.BackupSchedule) *skylexv1.BackupSchedule {
	return &skylexv1.BackupSchedule{
		Id:              s.ID,
		ClusterId:       s.ClusterID,
		Cron:            s.Cron,
		Type:            convertBackupTypeToProto(s.Type),
		RetentionCount:  int32(s.RetentionCount),
		RetentionDays:   int32(s.RetentionDays),
		StorageConfigId: s.StorageConfigID,
		Enabled:         s.Enabled,
		CreatedAt:       timestamppb.New(s.CreatedAt),
		UpdatedAt:       timestamppb.New(s.UpdatedAt),
	}
}

func convertBackupTypeToProto(t models.BackupType) skylexv1.BackupType {
	switch t {
	case models.BackupTypeIncremental:
		return skylexv1.BackupType_BACKUP_TYPE_INCREMENTAL
	case models.BackupTypeDifferential:
		return skylexv1.BackupType_BACKUP_TYPE_DIFFERENTIAL
	default:
		return skylexv1.BackupType_BACKUP_TYPE_FULL
	}
}

func convertBackupStatusToProto(s models.BackupStatus) skylexv1.BackupStatus {
	switch s {
	case models.BackupStatusCompleted:
		return skylexv1.BackupStatus_BACKUP_STATUS_COMPLETED
	case models.BackupStatusFailed:
		return skylexv1.BackupStatus_BACKUP_STATUS_FAILED
	default:
		return skylexv1.BackupStatus_BACKUP_STATUS_RUNNING
	}
}

func convertRestoreStatusToProto(s models.RestoreStatus) skylexv1.RestoreStatus {
	switch s {
	case models.RestoreStatusCompleted:
		return skylexv1.RestoreStatus_RESTORE_STATUS_COMPLETED
	case models.RestoreStatusFailed:
		return skylexv1.RestoreStatus_RESTORE_STATUS_FAILED
	default:
		return skylexv1.RestoreStatus_RESTORE_STATUS_RUNNING
	}
}