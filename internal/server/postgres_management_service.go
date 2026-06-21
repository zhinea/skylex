package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	connect "connectrpc.com/connect"
	"github.com/go-playground/validator/v10"
	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// validEndpointModes is the exhaustive set of accepted endpoint_mode values.
var validEndpointModes = map[string]bool{
	"direct_primary":         true,
	"manual_stable_endpoint": true,
}

// validSSLModes is the exhaustive set of accepted ssl_mode values.
var validSSLModes = map[string]bool{
	"prefer":  true,
	"require": true,
	"disable": true,
}

// validRoleKinds is the exhaustive set of accepted role_kind values.
var validRoleKinds = map[string]bool{
	"admin":      true,
	"read_write": true,
	"read_only":  true,
	"custom":     true,
}

// roleNamePattern allows only safe PostgreSQL identifier characters.
var roleNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_$]{0,62}$`)

// PostgresManagementService implements the PostgresManagementService Connect-RPC handler.
type PostgresManagementService struct {
	profiles       *db.ConnectionProfileRepository
	nodes          *db.NodeRepository
	clusters       *db.ClusterRepository
	roles          *db.PostgresRoleRepository
	roleEncryptKey []byte
	validate       *validator.Validate
	clusterLocksMu sync.Mutex
	clusterLocks   map[string]*sync.Mutex
	log            *slog.Logger
}

func NewPostgresManagementService(
	profiles *db.ConnectionProfileRepository,
	nodes *db.NodeRepository,
	clusters *db.ClusterRepository,
	roles *db.PostgresRoleRepository,
	roleEncryptKey []byte,
	log *slog.Logger,
) *PostgresManagementService {
	validate := validator.New()
	_ = validate.RegisterValidation("pgrole", func(fl validator.FieldLevel) bool {
		return roleNamePattern.MatchString(fl.Field().String())
	})

	return &PostgresManagementService{
		profiles:       profiles,
		nodes:          nodes,
		clusters:       clusters,
		roles:          roles,
		roleEncryptKey: roleEncryptKey,
		validate:       validate,
		clusterLocks:   make(map[string]*sync.Mutex),
		log:            log,
	}
}

type createRoleInput struct {
	ClusterID string `validate:"required"`
	RoleName  string `validate:"required,pgrole"`
	RoleKind  string `validate:"required,oneof=admin read_write read_only custom"`
}

func (s *PostgresManagementService) clusterLock(clusterID string) *sync.Mutex {
	s.clusterLocksMu.Lock()
	defer s.clusterLocksMu.Unlock()
	mu := s.clusterLocks[clusterID]
	if mu == nil {
		mu = &sync.Mutex{}
		s.clusterLocks[clusterID] = mu
	}
	return mu
}

// GetConnectionProfile returns the stored connection profile plus computed primary/replica endpoints.
func (s *PostgresManagementService) GetConnectionProfile(
	ctx context.Context,
	req *connect.Request[skylexv1.GetConnectionProfileRequest],
) (*connect.Response[skylexv1.GetConnectionProfileResponse], error) {
	clusterID := req.Msg.GetClusterId()
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}

	cluster, err := s.clusters.GetByID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}
	if cluster == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster %q not found", clusterID))
	}

	profile, err := s.profiles.GetByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get connection profile: %w", err))
	}

	nodes, _, err := s.nodes.ListByCluster(ctx, clusterID, 0, 100)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list nodes: %w", err))
	}

	protoProfile := profileToProto(profile)
	primaryEndpoint, replicaEndpoints := computeEndpoints(nodes)
	effectiveHost, effectivePort := computeEffectiveEndpoint(profile, primaryEndpoint)

	return connect.NewResponse(&skylexv1.GetConnectionProfileResponse{
		Profile:          protoProfile,
		PrimaryEndpoint:  primaryEndpoint,
		ReplicaEndpoints: replicaEndpoints,
		EffectiveHost:    effectiveHost,
		EffectivePort:    int32(effectivePort),
	}), nil
}

// UpdateConnectionProfile validates and persists connection profile changes.
// Requires operator or admin role.
func (s *PostgresManagementService) UpdateConnectionProfile(
	ctx context.Context,
	req *connect.Request[skylexv1.UpdateConnectionProfileRequest],
) (*connect.Response[skylexv1.UpdateConnectionProfileResponse], error) {
	role, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if role == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot update connection profiles"))
	}

	clusterID := req.Msg.GetClusterId()
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}

	cluster, err := s.clusters.GetByID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}
	if cluster == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster %q not found", clusterID))
	}

	// Validate and apply fields; fall back to defaults when the request omits them.
	endpointMode := req.Msg.GetEndpointMode()
	if endpointMode == "" {
		endpointMode = db.DefaultEndpointMode
	}
	if !validEndpointModes[endpointMode] {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid endpoint_mode %q: must be one of %v", endpointMode, sortedKeys(validEndpointModes)))
	}

	publicHost := req.Msg.GetPublicHost()
	if publicHost != "" {
		if err := validateHostname(publicHost); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid public_host: %w", err))
		}
	}

	publicPort := int(req.Msg.GetPublicPort())
	if publicPort == 0 {
		publicPort = db.DefaultPublicPort
	}
	if publicPort < 1 || publicPort > 65535 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid public_port %d: must be in range 1–65535", publicPort))
	}

	sslMode := req.Msg.GetSslMode()
	if sslMode == "" {
		sslMode = db.DefaultSSLMode
	}
	if !validSSLModes[sslMode] {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid ssl_mode %q: must be one of prefer, require, disable", sslMode))
	}

	allowedCIDRs := req.Msg.GetAllowedCidrs()
	for _, cidr := range allowedCIDRs {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid CIDR %q: %w", cidr, err))
		}
	}
	if allowedCIDRs == nil {
		allowedCIDRs = []string{}
	}

	updated, err := s.profiles.Upsert(ctx, &db.ConnectionProfile{
		ClusterID:    clusterID,
		EndpointMode: endpointMode,
		PublicHost:   publicHost,
		PublicPort:   publicPort,
		SSLMode:      sslMode,
		AllowedCIDRs: allowedCIDRs,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("upsert connection profile: %w", err))
	}

	return connect.NewResponse(&skylexv1.UpdateConnectionProfileResponse{
		Profile: profileToProto(updated),
	}), nil
}

// profileToProto converts a db.ConnectionProfile to the proto message.
func profileToProto(p *db.ConnectionProfile) *skylexv1.ConnectionProfile {
	proto := &skylexv1.ConnectionProfile{
		ClusterId:    p.ClusterID,
		EndpointMode: p.EndpointMode,
		PublicHost:   p.PublicHost,
		PublicPort:   int32(p.PublicPort),
		SslMode:      p.SSLMode,
		AllowedCidrs: p.AllowedCIDRs,
	}
	if !p.CreatedAt.IsZero() {
		proto.CreatedAt = timestamppb.New(p.CreatedAt)
	}
	if !p.UpdatedAt.IsZero() {
		proto.UpdatedAt = timestamppb.New(p.UpdatedAt)
	}
	return proto
}

// computeEndpoints derives primary and replica NodeEndpoint messages from node data.
func computeEndpoints(nodes []*models.Node) (*skylexv1.NodeEndpoint, []*skylexv1.NodeEndpoint) {
	var primary *skylexv1.NodeEndpoint
	var replicas []*skylexv1.NodeEndpoint

	for _, n := range nodes {
		if !n.PostgresInstalled || !n.PostgresDataInitialized {
			continue
		}
		host := n.Address
		if host == "" {
			host = n.Hostname
		}
		port := n.Port
		if port == 0 {
			port = 5432
		}
		ep := &skylexv1.NodeEndpoint{
			NodeId:   n.ID,
			Hostname: n.Hostname,
			Host:     host,
			Port:     int32(port),
			Role:     string(n.Role),
		}
		if n.Role == models.NodeRolePrimary && primary == nil {
			primary = ep
		} else if n.Role == models.NodeRoleReplica {
			replicas = append(replicas, ep)
		}
	}

	return primary, replicas
}

// computeEffectiveEndpoint returns the host/port that operators should use to connect,
// respecting the configured endpoint_mode.
func computeEffectiveEndpoint(profile *db.ConnectionProfile, primary *skylexv1.NodeEndpoint) (string, int) {
	if profile.EndpointMode == "manual_stable_endpoint" && profile.PublicHost != "" {
		return profile.PublicHost, profile.PublicPort
	}
	if primary != nil {
		return primary.Host, int(primary.Port)
	}
	return "", 0
}

// validateHostname rejects hostnames containing control characters or whitespace.
func validateHostname(h string) error {
	for _, r := range h {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return fmt.Errorf("hostname must not contain control characters or whitespace")
		}
	}
	if strings.ContainsAny(h, "\x00") {
		return fmt.Errorf("hostname must not contain null bytes")
	}
	return nil
}

// sortedKeys returns the keys of a bool map in a stable order for error messages.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ─── Role Management (Phase 3) ───────────────────────────────────────────────

// ListRoles returns all managed PostgreSQL roles for a cluster.
func (s *PostgresManagementService) ListRoles(
	ctx context.Context,
	req *connect.Request[skylexv1.ListRolesRequest],
) (*connect.Response[skylexv1.ListRolesResponse], error) {
	clusterID := req.Msg.GetClusterId()
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}

	roles, err := s.roles.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list roles: %w", err))
	}

	protoRoles := make([]*skylexv1.PostgresRole, 0, len(roles))
	for _, r := range roles {
		protoRoles = append(protoRoles, roleToProto(r))
	}
	return connect.NewResponse(&skylexv1.ListRolesResponse{Roles: protoRoles}), nil
}

// CreateRole validates and creates a managed role, queues pg_ensure_role on the primary.
// Requires operator or admin role.
func (s *PostgresManagementService) CreateRole(
	ctx context.Context,
	req *connect.Request[skylexv1.CreateRoleRequest],
) (*connect.Response[skylexv1.CreateRoleResponse], error) {
	userRole, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if userRole == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot create roles"))
	}

	clusterID := req.Msg.GetClusterId()
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}

	roleName := strings.TrimSpace(req.Msg.GetRoleName())

	roleKind := req.Msg.GetRoleKind()
	if roleKind == "" {
		roleKind = "custom"
	}
	if err := s.validate.Struct(createRoleInput{ClusterID: clusterID, RoleName: roleName, RoleKind: roleKind}); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid create role request: %w", err))
	}
	if !validRoleKinds[roleKind] {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid role_kind %q: must be one of admin, read_write, read_only, custom", roleKind))
	}

	mu := s.clusterLock(clusterID)
	mu.Lock()
	defer mu.Unlock()

	primary, err := s.resolveManagementPrimary(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	allowPromote, err := s.allowPromotionForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	existing, err := s.roles.GetByClusterAndName(ctx, clusterID, roleName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check existing role: %w", err))
	}
	if existing != nil && existing.Status != "failed" {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("role %q already exists in cluster %q", roleName, clusterID))
	}

	var expiresAt *time.Time
	if req.Msg.GetExpiresAt() != nil && req.Msg.GetExpiresAt().IsValid() {
		t := req.Msg.GetExpiresAt().AsTime()
		expiresAt = &t
	}

	// Generate a cryptographically-secure password.
	plainPassword, err := crypto.GenerateToken(24)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("generate password: %w", err))
	}

	// Encrypt the password for at-rest storage.
	encryptedBytes, err := crypto.EncryptAES256GCM([]byte(plainPassword), s.roleEncryptKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("encrypt password: %w", err))
	}
	commandSecret, err := crypto.EncryptAES256GCM([]byte(plainPassword), s.roleEncryptKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("encrypt command secret: %w", err))
	}
	storedPassword := base64.StdEncoding.EncodeToString(encryptedBytes)

	roleID := id.New()
	opID := id.New()
	cmdID := id.New()
	retryRoleID := roleID
	if existing != nil {
		retryRoleID = existing.ID
	}
	payload, err := json.Marshal(map[string]interface{}{
		"role_id":             retryRoleID,
		"operation_id":        opID,
		"role_name":           roleName,
		"role_kind":           roleKind,
		"password_secret_key": "password",
		"allow_promote":       allowPromote,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("marshal role payload: %w", err))
	}
	secretExpiresAt := time.Now().UTC().Add(24 * time.Hour)

	createInput := db.CreateRoleTxInput{
		RoleID:                 retryRoleID,
		OperationID:            opID,
		CommandID:              cmdID,
		ClusterID:              clusterID,
		NodeID:                 primary.ID,
		AgentID:                primary.AgentID,
		RoleName:               roleName,
		RoleKind:               roleKind,
		EncryptedPassword:      storedPassword,
		Payload:                string(payload),
		BeforeAction:           managementBeforeAction(allowPromote),
		EncryptedCommandSecret: commandSecret,
		SecretExpiresAt:        &secretExpiresAt,
		ExpiresAt:              expiresAt,
	}
	var txResult *db.PostgresRoleTx
	if existing != nil {
		txResult, err = s.roles.RetryCreateWithCommand(ctx, createInput)
	} else {
		txResult, err = s.roles.CreateWithCommand(ctx, createInput)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create role and queue command: %w", err))
	}
	s.log.Info("queued pg_ensure_role", "role_id", txResult.Role.ID, "command_id", txResult.Command.ID)

	return connect.NewResponse(&skylexv1.CreateRoleResponse{
		Role:            roleToProto(txResult.Role),
		OneTimePassword: plainPassword,
	}), nil
}

// RotateRolePassword generates a new password, re-encrypts at rest, and queues pg_rotate_role_password.
// Requires operator or admin role.
func (s *PostgresManagementService) RotateRolePassword(
	ctx context.Context,
	req *connect.Request[skylexv1.RotateRolePasswordRequest],
) (*connect.Response[skylexv1.RotateRolePasswordResponse], error) {
	userRole, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if userRole == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot rotate role passwords"))
	}

	roleID := req.Msg.GetRoleId()
	if roleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("role_id is required"))
	}

	role, err := s.roles.GetByID(ctx, roleID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get role: %w", err))
	}
	if role == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role %q not found", roleID))
	}
	if role.Status == "deleting" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("role is being deleted"))
	}

	mu := s.clusterLock(role.ClusterID)
	mu.Lock()
	defer mu.Unlock()

	primary, err := s.resolveManagementPrimary(ctx, role.ClusterID)
	if err != nil {
		return nil, err
	}
	allowPromote, err := s.allowPromotionForCluster(ctx, role.ClusterID)
	if err != nil {
		return nil, err
	}

	plainPassword, err := crypto.GenerateToken(24)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("generate password: %w", err))
	}

	encryptedBytes, err := crypto.EncryptAES256GCM([]byte(plainPassword), s.roleEncryptKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("encrypt password: %w", err))
	}
	commandSecret, err := crypto.EncryptAES256GCM([]byte(plainPassword), s.roleEncryptKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("encrypt command secret: %w", err))
	}
	storedPassword := base64.StdEncoding.EncodeToString(encryptedBytes)

	opID := id.New()
	cmdID := id.New()
	payload, err := json.Marshal(map[string]interface{}{
		"role_id":             role.ID,
		"operation_id":        opID,
		"role_name":           role.RoleName,
		"password_secret_key": "password",
		"allow_promote":       allowPromote,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("marshal role payload: %w", err))
	}
	secretExpiresAt := time.Now().UTC().Add(24 * time.Hour)
	txResult, err := s.roles.RotateWithCommand(ctx, db.RotateRoleTxInput{
		RoleID:                 role.ID,
		OperationID:            opID,
		CommandID:              cmdID,
		NodeID:                 primary.ID,
		AgentID:                primary.AgentID,
		EncryptedPassword:      storedPassword,
		Payload:                string(payload),
		BeforeAction:           managementBeforeAction(allowPromote),
		EncryptedCommandSecret: commandSecret,
		SecretExpiresAt:        &secretExpiresAt,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("rotate password and queue command: %w", err))
	}
	s.log.Info("queued pg_rotate_role_password", "role_id", role.ID, "command_id", txResult.Command.ID)

	return connect.NewResponse(&skylexv1.RotateRolePasswordResponse{
		Role:            roleToProto(txResult.Role),
		OneTimePassword: plainPassword,
	}), nil
}

// DeleteRole marks the role as deleting and queues pg_drop_role on the primary.
// Requires operator or admin role.
func (s *PostgresManagementService) DeleteRole(
	ctx context.Context,
	req *connect.Request[skylexv1.DeleteRoleRequest],
) (*connect.Response[skylexv1.DeleteRoleResponse], error) {
	userRole, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if userRole == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot delete roles"))
	}

	roleID := req.Msg.GetRoleId()
	if roleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("role_id is required"))
	}

	role, err := s.roles.GetByID(ctx, roleID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get role: %w", err))
	}
	if role == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role %q not found", roleID))
	}
	if role.Status == "deleting" {
		return connect.NewResponse(&skylexv1.DeleteRoleResponse{}), nil
	}

	mu := s.clusterLock(role.ClusterID)
	mu.Lock()
	defer mu.Unlock()

	primary, err := s.resolveManagementPrimary(ctx, role.ClusterID)
	if err != nil {
		return nil, err
	}
	allowPromote, err := s.allowPromotionForCluster(ctx, role.ClusterID)
	if err != nil {
		return nil, err
	}

	opID := id.New()
	cmdID := id.New()
	payload, err := json.Marshal(map[string]interface{}{
		"role_id":       role.ID,
		"operation_id":  opID,
		"role_name":     role.RoleName,
		"allow_promote": allowPromote,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("marshal role payload: %w", err))
	}

	txResult, err := s.roles.DeleteWithCommand(ctx, db.DeleteRoleTxInput{
		RoleID:       role.ID,
		OperationID:  opID,
		CommandID:    cmdID,
		NodeID:       primary.ID,
		AgentID:      primary.AgentID,
		Payload:      string(payload),
		BeforeAction: managementBeforeAction(allowPromote),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("mark role deleting and queue command: %w", err))
	}
	s.log.Info("queued pg_drop_role", "role_id", role.ID, "command_id", txResult.Command.ID)

	return connect.NewResponse(&skylexv1.DeleteRoleResponse{}), nil
}

func (s *PostgresManagementService) resolveManagementPrimary(ctx context.Context, clusterID string) (*models.Node, error) {
	primary, err := s.nodes.GetPrimary(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resolve primary: %w", err))
	}
	if primary == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no primary node found for cluster %q", clusterID))
	}
	if primary.AgentID == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("primary node has no linked agent"))
	}
	if !primary.PostgresInstalled || !primary.PostgresDataInitialized {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("primary node is not ready for PostgreSQL management"))
	}
	return primary, nil
}

func (s *PostgresManagementService) allowPromotionForCluster(ctx context.Context, clusterID string) (bool, error) {
	nodes, _, err := s.nodes.ListByCluster(ctx, clusterID, 0, 1000)
	if err != nil {
		return false, connect.NewError(connect.CodeInternal, fmt.Errorf("list cluster nodes: %w", err))
	}
	// Auto-promote only for a single-node cluster. Multi-node clusters must not
	// risk split brain if metadata and PostgreSQL recovery state disagree.
	return len(nodes) == 1, nil
}

func managementBeforeAction(allowPromote bool) string {
	if allowPromote {
		return "pg_promote"
	}
	return ""
}

// requireCluster checks that a cluster exists, returning a connect error if not.
func (s *PostgresManagementService) requireCluster(ctx context.Context, clusterID string) error {
	cluster, err := s.clusters.GetByID(ctx, clusterID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}
	if cluster == nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster %q not found", clusterID))
	}
	return nil
}

// roleToProto converts a db.PostgresRole to the proto message.
// The encrypted_password field is never included in the response.
func roleToProto(r *db.PostgresRole) *skylexv1.PostgresRole {
	proto := &skylexv1.PostgresRole{
		Id:              r.ID,
		ClusterId:       r.ClusterID,
		RoleName:        r.RoleName,
		RoleKind:        r.RoleKind,
		PasswordVersion: int32(r.PasswordVersion),
		Status:          r.Status,
	}
	if !r.CreatedAt.IsZero() {
		proto.CreatedAt = timestamppb.New(r.CreatedAt)
	}
	if !r.UpdatedAt.IsZero() {
		proto.UpdatedAt = timestamppb.New(r.UpdatedAt)
	}
	if r.ExpiresAt != nil {
		proto.ExpiresAt = timestamppb.New(*r.ExpiresAt)
	}
	return proto
}
