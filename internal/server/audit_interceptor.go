package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/grpc"
)

type AuditInterceptor struct {
	auditRepo *db.AuditRepository
	log       *slog.Logger
}

func NewAuditInterceptor(auditRepo *db.AuditRepository, log *slog.Logger) *AuditInterceptor {
	return &AuditInterceptor{auditRepo: auditRepo, log: log}
}

func (i *AuditInterceptor) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)

		action := methodToAuditAction(info.FullMethod)
		if action == "" {
			return resp, err
		}

		entry := &models.AuditLog{
			UserID:    UserIDFromContext(ctx),
			Action:    action,
			Resource:  info.FullMethod,
			Detail:    fmt.Sprintf("method=%s", info.FullMethod),
			IPAddress: clientIPFromContext(ctx),
			CreatedAt: time.Now(),
		}

		if err != nil {
			entry.Detail = fmt.Sprintf("method=%s error=%v", info.FullMethod, err)
		}

		if auditErr := i.auditRepo.Log(entry); auditErr != nil {
			i.log.Error("audit log failed", "error", auditErr)
		}

		return resp, err
	}
}

func methodToAuditAction(method string) models.AuditAction {
	actionMap := map[string]models.AuditAction{
		"/skylex.v1.ClusterService/CreateCluster":                 models.AuditActionCreateCluster,
		"/skylex.v1.ClusterService/UpdateCluster":                 models.AuditActionUpdateCluster,
		"/skylex.v1.ClusterService/DeleteCluster":                 models.AuditActionDeleteCluster,
		"/skylex.v1.ClusterService/FailoverCluster":               models.AuditActionFailover,
		"/skylex.v1.ClusterService/ScaleCluster":                  models.AuditActionUpdateCluster,
		"/skylex.v1.NodeService/DrainNode":                        models.AuditActionDrainNode,
		"/skylex.v1.NodeService/RejoinNode":                       models.AuditActionRejoinNode,
		"/skylex.v1.NodeService/DeleteNode":                       models.AuditActionDeleteNode,
		"/skylex.v1.NodeService/ResolveInstallationConflict":      models.AuditActionResolveConflict,
		"/skylex.v1.PostgresManagementService/CreateRole":         models.AuditActionCreateRole,
		"/skylex.v1.PostgresManagementService/RotateRolePassword": models.AuditActionRotateRole,
		"/skylex.v1.PostgresManagementService/DeleteRole":         models.AuditActionDeleteRole,
		"/skylex.v1.PostgresManagementService/CreateDatabase":     models.AuditActionCreateDatabase,
		"/skylex.v1.PostgresManagementService/DeleteDatabase":     models.AuditActionDeleteDatabase,
		"/skylex.v1.BackupService/CreateBackup":                   models.AuditActionCreateBackup,
		"/skylex.v1.BackupService/DeleteBackup":                   models.AuditActionDeleteBackup,
		"/skylex.v1.BackupService/CreateRestoreJob":               models.AuditActionRestore,
		"/skylex.v1.AuthService/Login":                            models.AuditActionLogin,
		"/skylex.v1.AuthService/CreateUser":                       models.AuditActionCreateUser,
		"/skylex.v1.AuthService/DeleteUser":                       models.AuditActionDeleteUser,
		"/skylex.v1.AuthService/CreateAPIKey":                     models.AuditActionCreateToken,
		"/skylex.v1.AuthService/DeleteAPIKey":                     models.AuditActionDeleteToken,
	}
	return actionMap[method]
}

func clientIPFromContext(ctx context.Context) string {
	return ""
}
