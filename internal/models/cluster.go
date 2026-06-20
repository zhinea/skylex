package models

import "time"

type EngineType string

const (
	EnginePostgreSQL EngineType = "postgresql"
)

type NodeRole string

const (
	NodeRolePrimary NodeRole = "primary"
	NodeRoleReplica NodeRole = "replica"
)

type ReplicationMode string

const (
	ReplicationSync  ReplicationMode = "synchronous"
	ReplicationAsync ReplicationMode = "asynchronous"
)

type ClusterStatus string

const (
	ClusterStatusCreating ClusterStatus = "creating"
	ClusterStatusRunning  ClusterStatus = "running"
	ClusterStatusDegraded ClusterStatus = "degraded"
	ClusterStatusStopped  ClusterStatus = "stopped"
	ClusterStatusDeleting ClusterStatus = "deleting"
)

// ServiceLocation describes where PostgreSQL runs on a cluster node.
type ServiceLocation string

const (
	ServiceLocationUnspecified ServiceLocation = ""
	ServiceLocationNative      ServiceLocation = "native"
	ServiceLocationDocker      ServiceLocation = "docker"
)

type InstallationState string

const (
	InstallationStateUnspecified      InstallationState = ""
	InstallationStatePendingPreflight InstallationState = "pending_preflight"
	InstallationStateNothingFound     InstallationState = "nothing_found"
	InstallationStateConflict         InstallationState = "conflict"
	InstallationStateInstalling       InstallationState = "installing"
	InstallationStateInstalled        InstallationState = "installed"
	InstallationStateFailed           InstallationState = "failed"
	InstallationStateAdopted          InstallationState = "adopted"
)

type Cluster struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Engine          EngineType        `json:"engine"`
	Version         string            `json:"version"`
	ReplicationMode ReplicationMode   `json:"replication_mode"`
	Replicas        int               `json:"replicas"`
	StorageConfigID string            `json:"storage_config_id"`
	DataDir         string            `json:"data_dir"`
	PITREnabled     bool              `json:"pitr_enabled"`
	Status          ClusterStatus     `json:"status"`
	Tags            map[string]string `json:"tags,omitempty"`
	// Phase 2: service location model
	ServiceLocation ServiceLocation `json:"service_location"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type NodeStatus string

const (
	NodeStatusOnline    NodeStatus = "online"
	NodeStatusOffline   NodeStatus = "offline"
	NodeStatusDegraded  NodeStatus = "degraded"
	NodeStatusSyncing   NodeStatus = "syncing"
	NodeStatusPromoting NodeStatus = "promoting"
	NodeStatusDrained   NodeStatus = "drained"
	NodeStatusDeleting  NodeStatus = "deleting"
)

type Node struct {
	ID           string            `json:"id"`
	ClusterID    string            `json:"cluster_id"`
	Hostname     string            `json:"hostname"`
	Address      string            `json:"address"`
	Port         int               `json:"port"`
	Role         NodeRole          `json:"role"`
	Status       NodeStatus        `json:"status"`
	AgentVersion string            `json:"agent_version"`
	AgentID      string            `json:"agent_id"`
	Labels       map[string]string `json:"labels,omitempty"`
	LastSeen     time.Time         `json:"last_seen"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	// Phase 2: PostgreSQL installation & health visibility
	PostgresInstalled       bool   `json:"postgres_installed"`
	PostgresVersion         string `json:"postgres_version"`
	PostgresDataInitialized bool   `json:"postgres_data_initialized"`
	// Phase 4: human-readable status detail
	StatusDetail string `json:"status_detail"`
	// Phase 2: service location model
	ServiceLocation ServiceLocation `json:"service_location"`
	DockerAvailable bool            `json:"docker_available"`
	// Phase 4: native PostgreSQL conflict detection.
	InstallationState InstallationState `json:"installation_state"`
	ConflictDetails   string            `json:"conflict_details"`
	AgentConnected    bool              `json:"agent_connected"`
	AgentLatencyMS    int64             `json:"agent_latency_ms"`
	LatestMetrics     *NodeMetric       `json:"latest_metrics,omitempty"`
}

type NodeMetric struct {
	ID                   string    `json:"id"`
	NodeID               string    `json:"node_id"`
	RecordedAt           time.Time `json:"recorded_at"`
	OS                   string    `json:"os"`
	Platform             string    `json:"platform"`
	PlatformVersion      string    `json:"platform_version"`
	KernelVersion        string    `json:"kernel_version"`
	Architecture         string    `json:"architecture"`
	CPUCores             int       `json:"cpu_cores"`
	CPUUsagePercent      float64   `json:"cpu_usage_percent"`
	LoadAverage1M        int64     `json:"load_average_1m"`
	LoadAverage5M        int64     `json:"load_average_5m"`
	LoadAverage15M       int64     `json:"load_average_15m"`
	MemoryTotalBytes     int64     `json:"memory_total_bytes"`
	MemoryUsedBytes      int64     `json:"memory_used_bytes"`
	MemoryAvailableBytes int64     `json:"memory_available_bytes"`
	MemoryUsagePercent   float64   `json:"memory_usage_percent"`
	DiskTotalBytes       int64     `json:"disk_total_bytes"`
	DiskUsedBytes        int64     `json:"disk_used_bytes"`
	DiskAvailableBytes   int64     `json:"disk_available_bytes"`
	DiskUsagePercent     float64   `json:"disk_usage_percent"`
	UptimeSeconds        int64     `json:"uptime_seconds"`
}
