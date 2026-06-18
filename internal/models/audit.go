package models

import "time"

type AuditAction string

const (
	AuditActionCreateCluster   AuditAction = "cluster.create"
	AuditActionUpdateCluster   AuditAction = "cluster.update"
	AuditActionDeleteCluster   AuditAction = "cluster.delete"
	AuditActionFailover        AuditAction = "cluster.failover"
	AuditActionCreateBackup    AuditAction = "backup.create"
	AuditActionDeleteBackup    AuditAction = "backup.delete"
	AuditActionRestore         AuditAction = "restore.create"
	AuditActionLogin           AuditAction = "auth.login"
	AuditActionCreateUser      AuditAction = "user.create"
	AuditActionDeleteUser      AuditAction = "user.delete"
	AuditActionCreateToken     AuditAction = "token.create"
	AuditActionDeleteToken     AuditAction = "token.delete"
	AuditActionDrainNode       AuditAction = "node.drain"
	AuditActionRejoinNode      AuditAction = "node.rejoin"
	AuditActionDeleteNode      AuditAction = "node.delete"
	AuditActionResolveConflict AuditAction = "node.resolve_conflict"
)

type AuditLog struct {
	ID        int64       `json:"id"`
	UserID    string      `json:"user_id,omitempty"`
	Action    AuditAction `json:"action"`
	Resource  string      `json:"resource"`
	Detail    string      `json:"detail,omitempty"`
	IPAddress string      `json:"ip_address,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}
