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

type Cluster struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Engine          EngineType       `json:"engine"`
	Version         string           `json:"version"`
	ReplicationMode ReplicationMode  `json:"replication_mode"`
	Replicas        int              `json:"replicas"`
	StorageConfigID string           `json:"storage_config_id"`
	DataDir         string           `json:"data_dir"`
	PITREnabled     bool             `json:"pitr_enabled"`
	Status          ClusterStatus    `json:"status"`
	Tags            map[string]string `json:"tags,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type NodeStatus string

const (
	NodeStatusOnline     NodeStatus = "online"
	NodeStatusOffline    NodeStatus = "offline"
	NodeStatusDegraded   NodeStatus = "degraded"
	NodeStatusSyncing    NodeStatus = "syncing"
	NodeStatusPromoting  NodeStatus = "promoting"
)

type Node struct {
	ID                     string            `json:"id"`
	ClusterID              string            `json:"cluster_id"`
	Hostname               string            `json:"hostname"`
	Address                string            `json:"address"`
	Port                   int               `json:"port"`
	Role                   NodeRole          `json:"role"`
	Status                 NodeStatus        `json:"status"`
	AgentVersion           string            `json:"agent_version"`
	AgentID                string            `json:"agent_id"`
	Labels                 map[string]string `json:"labels,omitempty"`
	LastSeen               time.Time         `json:"last_seen"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
	// Phase 2: PostgreSQL installation & health visibility
	PostgresInstalled      bool   `json:"postgres_installed"`
	PostgresVersion        string `json:"postgres_version"`
	PostgresDataInitialized bool  `json:"postgres_data_initialized"`
	// Phase 4: human-readable status detail
	StatusDetail            string `json:"status_detail"`
}