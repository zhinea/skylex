package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
)

type contextKey string

const (
	ctxKeyUserID    contextKey = "user_id"
	ctxKeyUserRole  contextKey = "user_role"
	ctxKeyUserEmail contextKey = "user_email"
)

var (
	unauthenticatedMethods = map[string]bool{
		"/skylex.v1.AuthService/Login":                true,
		"/skylex.v1.AuthService/RefreshToken":         true,
		"/skylex.v1.AgentService/RegisterAgent":       true,
		"/skylex.v1.AgentService/Heartbeat":           true,
		"/skylex.v1.AgentService/ReportStatus":        true,
		"/skylex.v1.AgentService/FetchCommand":        true,
		"/skylex.v1.AgentService/ReportCommandResult": true,
		"/skylex.v1.AgentService/ReportCommandLog":    true,
	}

	viewerAllowedMethods = map[string]bool{
		"/skylex.v1.ClusterService/GetCluster":                      true,
		"/skylex.v1.ClusterService/ListClusters":                    true,
		"/skylex.v1.ClusterService/GetClusterSettings":              true,
		"/skylex.v1.NodeService/ListNodes":                          true,
		"/skylex.v1.NodeService/GetNode":                            true,
		"/skylex.v1.NodeService/ListNodeMetrics":                    true,
		"/skylex.v1.NodeService/ListNodeCommandLogs":                true,
		"/skylex.v1.PostgresManagementService/GetConnectionProfile": true,
		"/skylex.v1.PostgresManagementService/GetNetworkAccess":     true,
		"/skylex.v1.PostgresManagementService/ListRoles":            true,
		"/skylex.v1.PostgresManagementService/ListDatabases":        true,
		"/skylex.v1.BackupService/GetBackup":                        true,
		"/skylex.v1.BackupService/ListBackups":                      true,
		"/skylex.v1.AuthService/ListUsers":                          true,
		"/skylex.v1.AuthService/ListAPIKeys":                        true,
	}
)

type AuthInterceptor struct {
	jwtManager *JWTManager
	apiKeyRepo *db.APIKeyRepository
	userRepo   *db.UserRepository
	log        *slog.Logger
}

func NewAuthInterceptor(jwtManager *JWTManager, apiKeyRepo *db.APIKeyRepository, userRepo *db.UserRepository, log *slog.Logger) *AuthInterceptor {
	return &AuthInterceptor{
		jwtManager: jwtManager,
		apiKeyRepo: apiKeyRepo,
		userRepo:   userRepo,
		log:        log,
	}
}

func (i *AuthInterceptor) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if unauthenticatedMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		claims, err := i.authenticate(ctx)
		if err != nil {
			i.log.Warn("auth failed", "method", info.FullMethod, "error", err)
			return nil, status.Error(codes.Unauthenticated, "authentication required")
		}

		ctx = context.WithValue(ctx, ctxKeyUserID, claims.UserID)
		ctx = context.WithValue(ctx, ctxKeyUserRole, claims.Role)
		ctx = context.WithValue(ctx, ctxKeyUserEmail, claims.Email)

		if claims.Role == models.RoleViewer && !viewerAllowedMethods[info.FullMethod] {
			i.log.Warn("insufficient permissions", "method", info.FullMethod, "role", claims.Role)
			return nil, status.Error(codes.PermissionDenied, "insufficient permissions")
		}

		return handler(ctx, req)
	}
}

func (i *AuthInterceptor) authenticate(ctx context.Context) (*JWTClaims, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("missing metadata")
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, fmt.Errorf("missing authorization header")
	}

	authHeader := authHeaders[0]

	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		return i.jwtManager.ValidateToken(token)
	}

	if strings.HasPrefix(authHeader, "ApiKey ") {
		key := strings.TrimPrefix(authHeader, "ApiKey ")
		return i.authenticateAPIKey(key)
	}

	return nil, fmt.Errorf("invalid authorization scheme")
}

func (i *AuthInterceptor) authenticateAPIKey(key string) (*JWTClaims, error) {
	keyHash := crypto.HashToken(key)

	apiKey, err := i.apiKeyRepo.GetByKeyHash(keyHash)
	if err != nil {
		return nil, fmt.Errorf("invalid api key")
	}

	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(apiKey.CreatedAt) {
		if apiKey.ExpiresAt.Before(apiKey.CreatedAt) {
			return nil, fmt.Errorf("expired api key")
		}
	}

	user, err := i.userRepo.GetByID(apiKey.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found for api key")
	}

	return &JWTClaims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
	}, nil
}

func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyUserID).(string); ok {
		return v
	}
	return ""
}

func UserRoleFromContext(ctx context.Context) models.Role {
	if v, ok := ctx.Value(ctxKeyUserRole).(models.Role); ok {
		return v
	}
	return ""
}

func UserEmailFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyUserEmail).(string); ok {
		return v
	}
	return ""
}
