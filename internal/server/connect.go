package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	connect "connectrpc.com/connect"
	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/gen/skylex/v1/skylexv1connect"
	"github.com/zhinea/skylex/internal/models"
)

type connectPostgresService struct {
	svc *PostgresManagementService
}

func (c *connectPostgresService) GetConnectionProfile(ctx context.Context, req *connect.Request[skylexv1.GetConnectionProfileRequest]) (*connect.Response[skylexv1.GetConnectionProfileResponse], error) {
	return c.svc.GetConnectionProfile(ctx, req)
}

func (c *connectPostgresService) UpdateConnectionProfile(ctx context.Context, req *connect.Request[skylexv1.UpdateConnectionProfileRequest]) (*connect.Response[skylexv1.UpdateConnectionProfileResponse], error) {
	return c.svc.UpdateConnectionProfile(ctx, req)
}

func (c *connectPostgresService) GetNetworkAccess(ctx context.Context, req *connect.Request[skylexv1.GetNetworkAccessRequest]) (*connect.Response[skylexv1.GetNetworkAccessResponse], error) {
	return c.svc.GetNetworkAccess(ctx, req)
}

func (c *connectPostgresService) UpdateNetworkAccess(ctx context.Context, req *connect.Request[skylexv1.UpdateNetworkAccessRequest]) (*connect.Response[skylexv1.UpdateNetworkAccessResponse], error) {
	return c.svc.UpdateNetworkAccess(ctx, req)
}

func (c *connectPostgresService) ApplyHBA(ctx context.Context, req *connect.Request[skylexv1.ApplyHBARequest]) (*connect.Response[skylexv1.ApplyHBAResponse], error) {
	return c.svc.ApplyHBA(ctx, req)
}

func (c *connectPostgresService) ListRoles(ctx context.Context, req *connect.Request[skylexv1.ListRolesRequest]) (*connect.Response[skylexv1.ListRolesResponse], error) {
	return c.svc.ListRoles(ctx, req)
}

func (c *connectPostgresService) CreateRole(ctx context.Context, req *connect.Request[skylexv1.CreateRoleRequest]) (*connect.Response[skylexv1.CreateRoleResponse], error) {
	return c.svc.CreateRole(ctx, req)
}

func (c *connectPostgresService) RotateRolePassword(ctx context.Context, req *connect.Request[skylexv1.RotateRolePasswordRequest]) (*connect.Response[skylexv1.RotateRolePasswordResponse], error) {
	return c.svc.RotateRolePassword(ctx, req)
}

func (c *connectPostgresService) DeleteRole(ctx context.Context, req *connect.Request[skylexv1.DeleteRoleRequest]) (*connect.Response[skylexv1.DeleteRoleResponse], error) {
	return c.svc.DeleteRole(ctx, req)
}

func (c *connectPostgresService) ListDatabases(ctx context.Context, req *connect.Request[skylexv1.ListDatabasesRequest]) (*connect.Response[skylexv1.ListDatabasesResponse], error) {
	return c.svc.ListDatabases(ctx, req)
}

func (c *connectPostgresService) CreateDatabase(ctx context.Context, req *connect.Request[skylexv1.CreateDatabaseRequest]) (*connect.Response[skylexv1.CreateDatabaseResponse], error) {
	return c.svc.CreateDatabase(ctx, req)
}

func (c *connectPostgresService) DeleteDatabase(ctx context.Context, req *connect.Request[skylexv1.DeleteDatabaseRequest]) (*connect.Response[skylexv1.DeleteDatabaseResponse], error) {
	return c.svc.DeleteDatabase(ctx, req)
}

func connectInterceptors(h http.Handler, srv *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Connect-Protocol-Version")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		authCtx := r.Context()
		if !isUnauthenticated(srv, r.URL.Path) {
			var userID, roleStr, userEmail string
			var err error
			userID, roleStr, userEmail, err = extractHTTPAuth(r, srv)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":"unauthenticated","message":"invalid or missing credentials"}`))
				return
			}
			authCtx = context.WithValue(authCtx, ctxKeyUserID, userID)
			authCtx = context.WithValue(authCtx, ctxKeyUserRole, models.Role(roleStr))
			authCtx = context.WithValue(authCtx, ctxKeyUserEmail, userEmail)

			if models.Role(roleStr) == models.RoleViewer && isWriteMethod(r.URL.Path) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"code":"permission_denied","message":"viewer role cannot perform write operations"}`))
				return
			}
		}

		srv.log.Debug("connect request", "method", r.Method, "path", r.URL.Path)
		h.ServeHTTP(w, r.WithContext(authCtx))
	})
}

var unauthenticatedPaths = map[string]bool{
	skylexv1connect.AuthServiceLoginProcedure:        true,
	skylexv1connect.AuthServiceRefreshTokenProcedure: true,
	"/version": true,
}

var writeMethods = map[string]bool{
	skylexv1connect.PostgresManagementServiceUpdateConnectionProfileProcedure: true,
	skylexv1connect.PostgresManagementServiceUpdateNetworkAccessProcedure:     true,
	skylexv1connect.PostgresManagementServiceApplyHBAProcedure:                true,
	skylexv1connect.PostgresManagementServiceCreateRoleProcedure:              true,
	skylexv1connect.PostgresManagementServiceRotateRolePasswordProcedure:      true,
	skylexv1connect.PostgresManagementServiceDeleteRoleProcedure:              true,
	skylexv1connect.PostgresManagementServiceCreateDatabaseProcedure:          true,
	skylexv1connect.PostgresManagementServiceDeleteDatabaseProcedure:          true,
	skylexv1connect.ClusterServiceCreateClusterProcedure:                      true,
	skylexv1connect.ClusterServiceUpdateClusterProcedure:                      true,
	skylexv1connect.ClusterServiceDeleteClusterProcedure:                      true,
	skylexv1connect.ClusterServiceFailoverClusterProcedure:                    true,
	skylexv1connect.ClusterServiceRestartNodeProcedure:                        true,
	skylexv1connect.ClusterServiceScaleClusterProcedure:                       true,
	skylexv1connect.ClusterServiceUpdateClusterSettingsProcedure:              true,
	skylexv1connect.NodeServiceDrainNodeProcedure:                             true,
	skylexv1connect.NodeServiceRejoinNodeProcedure:                            true,
	skylexv1connect.NodeServiceDeleteNodeProcedure:                            true,
	skylexv1connect.NodeServiceResolveInstallationConflictProcedure:           true,
	skylexv1connect.BackupServiceCreateBackupProcedure:                        true,
	skylexv1connect.BackupServiceDeleteBackupProcedure:                        true,
	skylexv1connect.BackupServiceCreateRestoreJobProcedure:                    true,
	skylexv1connect.ScheduleServiceCreateScheduleProcedure:                    true,
	skylexv1connect.ScheduleServiceUpdateScheduleProcedure:                    true,
	skylexv1connect.ScheduleServiceDeleteScheduleProcedure:                    true,
	skylexv1connect.StorageServiceCreateStorageConfigProcedure:                true,
	skylexv1connect.StorageServiceDeleteStorageConfigProcedure:                true,
	skylexv1connect.StorageServiceValidateStorageConfigProcedure:              true,
	skylexv1connect.AuthServiceCreateUserProcedure:                            true,
	skylexv1connect.AuthServiceDeleteUserProcedure:                            true,
	skylexv1connect.AuthServiceCreateAPIKeyProcedure:                          true,
	skylexv1connect.AuthServiceDeleteAPIKeyProcedure:                          true,
	skylexv1connect.AuthServiceCreateAgentTokenProcedure:                      true,
	skylexv1connect.AuthServiceDeleteAgentTokenProcedure:                      true,
}

func isUnauthenticated(srv *Server, path string) bool {
	if path == "/install-agent.sh" || path == "/skylex-agent" {
		return srv.cfg.Server.DevMode
	}
	return unauthenticatedPaths[path]
}

func isWriteMethod(path string) bool {
	return writeMethods[path]
}

type connectAuthService struct {
	svc *AuthService
}

func (c *connectAuthService) Login(ctx context.Context, req *connect.Request[skylexv1.LoginRequest]) (*connect.Response[skylexv1.LoginResponse], error) {
	resp, err := c.svc.Login(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) RefreshToken(ctx context.Context, req *connect.Request[skylexv1.RefreshTokenRequest]) (*connect.Response[skylexv1.RefreshTokenResponse], error) {
	resp, err := c.svc.RefreshToken(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) ListUsers(ctx context.Context, req *connect.Request[skylexv1.ListUsersRequest]) (*connect.Response[skylexv1.ListUsersResponse], error) {
	resp, err := c.svc.ListUsers(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) CreateUser(ctx context.Context, req *connect.Request[skylexv1.CreateUserRequest]) (*connect.Response[skylexv1.CreateUserResponse], error) {
	resp, err := c.svc.CreateUser(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) DeleteUser(ctx context.Context, req *connect.Request[skylexv1.DeleteUserRequest]) (*connect.Response[skylexv1.DeleteUserResponse], error) {
	resp, err := c.svc.DeleteUser(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) CreateAPIKey(ctx context.Context, req *connect.Request[skylexv1.CreateAPIKeyRequest]) (*connect.Response[skylexv1.CreateAPIKeyResponse], error) {
	resp, err := c.svc.CreateAPIKey(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) ListAPIKeys(ctx context.Context, req *connect.Request[skylexv1.ListAPIKeysRequest]) (*connect.Response[skylexv1.ListAPIKeysResponse], error) {
	resp, err := c.svc.ListAPIKeys(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) DeleteAPIKey(ctx context.Context, req *connect.Request[skylexv1.DeleteAPIKeyRequest]) (*connect.Response[skylexv1.DeleteAPIKeyResponse], error) {
	resp, err := c.svc.DeleteAPIKey(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) CreateAgentToken(ctx context.Context, req *connect.Request[skylexv1.CreateAgentTokenRequest]) (*connect.Response[skylexv1.CreateAgentTokenResponse], error) {
	resp, err := c.svc.CreateAgentToken(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) ListAgentTokens(ctx context.Context, req *connect.Request[skylexv1.ListAgentTokensRequest]) (*connect.Response[skylexv1.ListAgentTokensResponse], error) {
	resp, err := c.svc.ListAgentTokens(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) DeleteAgentToken(ctx context.Context, req *connect.Request[skylexv1.DeleteAgentTokenRequest]) (*connect.Response[skylexv1.DeleteAgentTokenResponse], error) {
	resp, err := c.svc.DeleteAgentToken(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectAuthService) GetAgentInstallCommand(ctx context.Context, req *connect.Request[skylexv1.GetAgentInstallCommandRequest]) (*connect.Response[skylexv1.GetAgentInstallCommandResponse], error) {
	resp, err := c.svc.GetAgentInstallCommand(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

type connectClusterService struct {
	svc *ClusterService
}

func (c *connectClusterService) CreateCluster(ctx context.Context, req *connect.Request[skylexv1.CreateClusterRequest]) (*connect.Response[skylexv1.CreateClusterResponse], error) {
	resp, err := c.svc.CreateCluster(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectClusterService) GetCluster(ctx context.Context, req *connect.Request[skylexv1.GetClusterRequest]) (*connect.Response[skylexv1.GetClusterResponse], error) {
	resp, err := c.svc.GetCluster(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectClusterService) ListClusters(ctx context.Context, req *connect.Request[skylexv1.ListClustersRequest]) (*connect.Response[skylexv1.ListClustersResponse], error) {
	resp, err := c.svc.ListClusters(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectClusterService) UpdateCluster(ctx context.Context, req *connect.Request[skylexv1.UpdateClusterRequest]) (*connect.Response[skylexv1.UpdateClusterResponse], error) {
	resp, err := c.svc.UpdateCluster(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectClusterService) DeleteCluster(ctx context.Context, req *connect.Request[skylexv1.DeleteClusterRequest]) (*connect.Response[skylexv1.DeleteClusterResponse], error) {
	resp, err := c.svc.DeleteCluster(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectClusterService) FailoverCluster(ctx context.Context, req *connect.Request[skylexv1.FailoverClusterRequest]) (*connect.Response[skylexv1.FailoverClusterResponse], error) {
	resp, err := c.svc.FailoverCluster(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectClusterService) RestartNode(ctx context.Context, req *connect.Request[skylexv1.RestartNodeRequest]) (*connect.Response[skylexv1.RestartNodeResponse], error) {
	resp, err := c.svc.RestartNode(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectClusterService) ScaleCluster(ctx context.Context, req *connect.Request[skylexv1.ScaleClusterRequest]) (*connect.Response[skylexv1.ScaleClusterResponse], error) {
	resp, err := c.svc.ScaleCluster(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectClusterService) GetClusterSettings(ctx context.Context, req *connect.Request[skylexv1.GetClusterSettingsRequest]) (*connect.Response[skylexv1.GetClusterSettingsResponse], error) {
	resp, err := c.svc.GetClusterSettings(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectClusterService) UpdateClusterSettings(ctx context.Context, req *connect.Request[skylexv1.UpdateClusterSettingsRequest]) (*connect.Response[skylexv1.UpdateClusterSettingsResponse], error) {
	resp, err := c.svc.UpdateClusterSettings(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

type connectNodeService struct {
	svc *NodeService
}

func (c *connectNodeService) ListNodes(ctx context.Context, req *connect.Request[skylexv1.ListNodesRequest]) (*connect.Response[skylexv1.ListNodesResponse], error) {
	resp, err := c.svc.ListNodes(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectNodeService) GetNode(ctx context.Context, req *connect.Request[skylexv1.GetNodeRequest]) (*connect.Response[skylexv1.GetNodeResponse], error) {
	resp, err := c.svc.GetNode(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectNodeService) ListNodeMetrics(ctx context.Context, req *connect.Request[skylexv1.ListNodeMetricsRequest]) (*connect.Response[skylexv1.ListNodeMetricsResponse], error) {
	resp, err := c.svc.ListNodeMetrics(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectNodeService) DrainNode(ctx context.Context, req *connect.Request[skylexv1.DrainNodeRequest]) (*connect.Response[skylexv1.DrainNodeResponse], error) {
	resp, err := c.svc.DrainNode(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectNodeService) RejoinNode(ctx context.Context, req *connect.Request[skylexv1.RejoinNodeRequest]) (*connect.Response[skylexv1.RejoinNodeResponse], error) {
	resp, err := c.svc.RejoinNode(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectNodeService) DeleteNode(ctx context.Context, req *connect.Request[skylexv1.DeleteNodeRequest]) (*connect.Response[skylexv1.DeleteNodeResponse], error) {
	resp, err := c.svc.DeleteNode(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectNodeService) ResolveInstallationConflict(ctx context.Context, req *connect.Request[skylexv1.ResolveInstallationConflictRequest]) (*connect.Response[skylexv1.ResolveInstallationConflictResponse], error) {
	resp, err := c.svc.ResolveInstallationConflict(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectNodeService) ListNodeCommandLogs(ctx context.Context, req *connect.Request[skylexv1.ListNodeCommandLogsRequest]) (*connect.Response[skylexv1.ListNodeCommandLogsResponse], error) {
	resp, err := c.svc.ListNodeCommandLogs(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

type connectStorageService struct {
	svc *StorageService
}

func (c *connectStorageService) CreateStorageConfig(ctx context.Context, req *connect.Request[skylexv1.CreateStorageConfigRequest]) (*connect.Response[skylexv1.CreateStorageConfigResponse], error) {
	resp, err := c.svc.CreateStorageConfig(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectStorageService) ListStorageConfigs(ctx context.Context, req *connect.Request[skylexv1.ListStorageConfigsRequest]) (*connect.Response[skylexv1.ListStorageConfigsResponse], error) {
	resp, err := c.svc.ListStorageConfigs(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectStorageService) GetStorageConfig(ctx context.Context, req *connect.Request[skylexv1.GetStorageConfigRequest]) (*connect.Response[skylexv1.GetStorageConfigResponse], error) {
	resp, err := c.svc.GetStorageConfig(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectStorageService) DeleteStorageConfig(ctx context.Context, req *connect.Request[skylexv1.DeleteStorageConfigRequest]) (*connect.Response[skylexv1.DeleteStorageConfigResponse], error) {
	resp, err := c.svc.DeleteStorageConfig(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectStorageService) ValidateStorageConfig(ctx context.Context, req *connect.Request[skylexv1.ValidateStorageConfigRequest]) (*connect.Response[skylexv1.ValidateStorageConfigResponse], error) {
	resp, err := c.svc.ValidateStorageConfig(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

type connectBackupService struct {
	svc *BackupService
}

func (c *connectBackupService) CreateBackup(ctx context.Context, req *connect.Request[skylexv1.CreateBackupRequest]) (*connect.Response[skylexv1.CreateBackupResponse], error) {
	resp, err := c.svc.CreateBackup(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectBackupService) ListBackups(ctx context.Context, req *connect.Request[skylexv1.ListBackupsRequest]) (*connect.Response[skylexv1.ListBackupsResponse], error) {
	resp, err := c.svc.ListBackups(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectBackupService) GetBackup(ctx context.Context, req *connect.Request[skylexv1.GetBackupRequest]) (*connect.Response[skylexv1.GetBackupResponse], error) {
	resp, err := c.svc.GetBackup(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectBackupService) DeleteBackup(ctx context.Context, req *connect.Request[skylexv1.DeleteBackupRequest]) (*connect.Response[skylexv1.DeleteBackupResponse], error) {
	resp, err := c.svc.DeleteBackup(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectBackupService) CreateRestoreJob(ctx context.Context, req *connect.Request[skylexv1.CreateRestoreJobRequest]) (*connect.Response[skylexv1.CreateRestoreJobResponse], error) {
	resp, err := c.svc.CreateRestoreJob(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectBackupService) ListRestoreJobs(ctx context.Context, req *connect.Request[skylexv1.ListRestoreJobsRequest]) (*connect.Response[skylexv1.ListRestoreJobsResponse], error) {
	resp, err := c.svc.ListRestoreJobs(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

type connectScheduleService struct {
	svc *BackupService
}

func (c *connectScheduleService) CreateSchedule(ctx context.Context, req *connect.Request[skylexv1.CreateScheduleRequest]) (*connect.Response[skylexv1.CreateScheduleResponse], error) {
	resp, err := c.svc.CreateSchedule(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectScheduleService) ListSchedules(ctx context.Context, req *connect.Request[skylexv1.ListSchedulesRequest]) (*connect.Response[skylexv1.ListSchedulesResponse], error) {
	resp, err := c.svc.ListSchedules(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectScheduleService) UpdateSchedule(ctx context.Context, req *connect.Request[skylexv1.UpdateScheduleRequest]) (*connect.Response[skylexv1.UpdateScheduleResponse], error) {
	resp, err := c.svc.UpdateSchedule(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectScheduleService) DeleteSchedule(ctx context.Context, req *connect.Request[skylexv1.DeleteScheduleRequest]) (*connect.Response[skylexv1.DeleteScheduleResponse], error) {
	resp, err := c.svc.DeleteSchedule(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (s *Server) serveConnectHTTP(ctx context.Context) error {
	mux := http.NewServeMux()

	authPath, authHandler := skylexv1connect.NewAuthServiceHandler(&connectAuthService{svc: s.authService})
	mux.Handle(authPath, authHandler)

	clusterPath, clusterHandler := skylexv1connect.NewClusterServiceHandler(&connectClusterService{svc: s.clusterService})
	mux.Handle(clusterPath, clusterHandler)

	nodePath, nodeHandler := skylexv1connect.NewNodeServiceHandler(&connectNodeService{svc: s.nodeService})
	mux.Handle(nodePath, nodeHandler)

	storagePath, storageHandler := skylexv1connect.NewStorageServiceHandler(&connectStorageService{svc: s.storageService})
	mux.Handle(storagePath, storageHandler)

	backupPath, backupHandler := skylexv1connect.NewBackupServiceHandler(&connectBackupService{svc: s.backupService})
	mux.Handle(backupPath, backupHandler)

	schedulePath, scheduleHandler := skylexv1connect.NewScheduleServiceHandler(&connectScheduleService{svc: s.backupService})
	mux.Handle(schedulePath, scheduleHandler)

	postgresPath, postgresHandler := skylexv1connect.NewPostgresManagementServiceHandler(&connectPostgresService{svc: s.postgresService})
	mux.Handle(postgresPath, postgresHandler)

	mux.HandleFunc("/version", s.serveVersion)
	if s.cfg.Server.DevMode {
		mux.HandleFunc("/install-agent.sh", s.serveAgentInstallScript)
		mux.HandleFunc("/skylex-agent", s.serveAgentBinary)
	}

	mux.HandleFunc("/skylex.v1.AuthService/ListAuditLogs", s.handleListAuditLogs)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := connectInterceptors(mux, s)

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.ListenAddr, s.cfg.Server.HTTPPort)

	s.http = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	s.log.Info("connect http server listening", "addr", addr, "mode", "connect-rpc+json")

	go func() {
		<-ctx.Done()
		s.http.Shutdown(context.Background())
	}()

	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("connect http serve: %w", err)
	}

	return nil
}

type auditListRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"code": "method_not_allowed", "message": "POST required"})
		return
	}

	page := 1
	pageSize := 50

	if r.Body != nil {
		var req auditListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if req.Page > 0 {
				page = req.Page
			}
			if req.PageSize > 0 && req.PageSize <= 100 {
				pageSize = req.PageSize
			}
		}
	}

	logs, total, err := s.auditRepo.List(page, pageSize)
	if err != nil {
		s.log.Error("failed to list audit logs", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"code": "internal", "message": "failed to retrieve audit logs"})
		return
	}

	type auditEntry struct {
		ID        int64  `json:"id"`
		UserID    string `json:"userId"`
		Action    string `json:"action"`
		Resource  string `json:"resource"`
		Detail    string `json:"detail"`
		IPAddress string `json:"ipAddress"`
		Timestamp string `json:"timestamp"`
	}

	entries := make([]auditEntry, len(logs))
	for i, l := range logs {
		entries[i] = auditEntry{
			ID:        l.ID,
			UserID:    l.UserID,
			Action:    string(l.Action),
			Resource:  l.Resource,
			Detail:    l.Detail,
			IPAddress: l.IPAddress,
			Timestamp: l.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": entries,
		"pagination": map[string]interface{}{
			"page":     page,
			"pageSize": pageSize,
			"total":    total,
		},
	})
}

func (s *Server) serveAgentInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(installScriptWithAgentBinaryURL(devAgentBinaryURL(s.cfg.Server.HTTPPort))))
}

func (s *Server) serveAgentBinary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "bin/skylex-agent")
}

func (s *Server) serveVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(versionString()))
}

func extractHTTPAuth(r *http.Request, srv *Server) (userID, userRole, userEmail string, err error) {
	authHeader := r.Header.Get("Authorization")

	if authHeader == "" {
		srv.log.Warn("connect auth: missing authorization header", "path", r.URL.Path)
		return "", "", "", fmt.Errorf("missing authorization header")
	}

	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := srv.jwtManager.ValidateToken(token)
		if err != nil {
			srv.log.Warn("connect auth: invalid bearer token", "path", r.URL.Path, "error", err)
			return "", "", "", fmt.Errorf("invalid bearer token: %w", err)
		}
		return claims.UserID, string(claims.Role), claims.Email, nil
	}

	srv.log.Warn("connect auth: unsupported auth scheme", "path", r.URL.Path, "header", authHeader[:20])
	return "", "", "", fmt.Errorf("unsupported auth scheme")
}
