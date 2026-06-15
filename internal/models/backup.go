package models

import "time"

type BackupType string

const (
	BackupTypeFull         BackupType = "full"
	BackupTypeIncremental  BackupType = "incremental"
	BackupTypeDifferential BackupType = "differential"
)

type BackupStatus string

const (
	BackupStatusRunning   BackupStatus = "running"
	BackupStatusCompleted BackupStatus = "completed"
	BackupStatusFailed    BackupStatus = "failed"
	BackupStatusDeleting  BackupStatus = "deleting"
)

type Backup struct {
	ID              string       `json:"id"`
	ClusterID       string       `json:"cluster_id"`
	NodeID          string       `json:"node_id"`
	Type            BackupType   `json:"type"`
	StoragePath     string       `json:"storage_path"`
	StorageConfigID string       `json:"storage_config_id"`
	WALStart        string       `json:"wal_start,omitempty"`
	WALStop         string       `json:"wal_stop,omitempty"`
	LSN             string       `json:"lsn,omitempty"`
	SizeBytes       int64        `json:"size_bytes"`
	Status          BackupStatus `json:"status"`
	CreatedAt       time.Time    `json:"created_at"`
	CompletedAt     *time.Time   `json:"completed_at,omitempty"`
}

type RestoreTargetType string

const (
	RestoreTargetTime      RestoreTargetType = "time"
	RestoreTargetLSN       RestoreTargetType = "lsn"
	RestoreTargetXID       RestoreTargetType = "xid"
	RestoreTargetLatest    RestoreTargetType = "latest"
)

type RestoreStatus string

const (
	RestoreStatusRunning   RestoreStatus = "running"
	RestoreStatusCompleted RestoreStatus = "completed"
	RestoreStatusFailed    RestoreStatus = "failed"
)

type RestoreJob struct {
	ID             string            `json:"id"`
	SourceClusterID string           `json:"source_cluster_id"`
	TargetClusterID string           `json:"target_cluster_id,omitempty"`
	TargetType     RestoreTargetType `json:"target_type"`
	TargetValue    string            `json:"target_value,omitempty"`
	TargetNodeID   string            `json:"target_node_id,omitempty"`
	Status         RestoreStatus     `json:"status"`
	CreatedAt      time.Time         `json:"created_at"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`
}

type BackupSchedule struct {
	ID              string    `json:"id"`
	ClusterID       string    `json:"cluster_id"`
	Cron            string    `json:"cron"`
	Type            BackupType `json:"type"`
	RetentionCount  int       `json:"retention_count"`
	RetentionDays   int       `json:"retention_days"`
	StorageConfigID string    `json:"storage_config_id"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}