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
	"github.com/zhinea/skylex/internal/engine"
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
	"disabled": true,
	"prefer":   true,
	"required": true,
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

// databaseNamePattern allows safe PostgreSQL identifier characters for database names.
var databaseNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`)

// PostgresManagementService implements the PostgresManagementService Connect-RPC handler.
type PostgresManagementService struct {
	profiles       *db.ConnectionProfileRepository
	nodes          *db.NodeRepository
	clusters       *db.ClusterRepository
	roles          *db.PostgresRoleRepository
	databases      *db.PostgresDatabaseRepository
	access         *db.PostgresAccessRepository
	tls            *db.PostgresTLSRepository
	tlsCA          *db.PostgresTLSCARepository
	audit          *db.AuditRepository
	roleEncryptKey []byte
	validate       *validator.Validate
	clusterLocksMu sync.Mutex
	clusterLocks   map[string]*sync.Mutex
	log            *slog.Logger
}

func (s *PostgresManagementService) SetAuditRepository(repo *db.AuditRepository) {
	s.audit = repo
}

func NewPostgresManagementService(
	profiles *db.ConnectionProfileRepository,
	nodes *db.NodeRepository,
	clusters *db.ClusterRepository,
	roles *db.PostgresRoleRepository,
	databases *db.PostgresDatabaseRepository,
	access *db.PostgresAccessRepository,
	tls *db.PostgresTLSRepository,
	tlsCA *db.PostgresTLSCARepository,
	roleEncryptKey []byte,
	log *slog.Logger,
) *PostgresManagementService {
	validate := validator.New()
	_ = validate.RegisterValidation("pgrole", func(fl validator.FieldLevel) bool {
		return roleNamePattern.MatchString(fl.Field().String())
	})
	_ = validate.RegisterValidation("pgdatabase", func(fl validator.FieldLevel) bool {
		return databaseNamePattern.MatchString(fl.Field().String())
	})

	return &PostgresManagementService{
		profiles:       profiles,
		nodes:          nodes,
		clusters:       clusters,
		roles:          roles,
		databases:      databases,
		access:         access,
		tls:            tls,
		tlsCA:          tlsCA,
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

type createDatabaseInput struct {
	ClusterID    string `validate:"required"`
	DatabaseName string `validate:"required,pgdatabase"`
	OwnerRoleID  string `validate:"omitempty"`
}

type updateNetworkAccessInput struct {
	ClusterID string `validate:"required"`
}

type updateTLSConfigInput struct {
	ClusterID string `validate:"required"`
	TLSMode   string `validate:"required,oneof=disabled prefer required"`
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
	tlsStatuses, err := s.tlsStatuses(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	ca, err := s.tlsCAForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	warnings := tlsWarnings(profile, tlsStatuses, ca)

	return connect.NewResponse(&skylexv1.GetConnectionProfileResponse{
		Profile:          protoProfile,
		PrimaryEndpoint:  primaryEndpoint,
		ReplicaEndpoints: replicaEndpoints,
		EffectiveHost:    effectiveHost,
		EffectivePort:    int32(effectivePort),
		Warnings:         warnings,
		TlsConfig:        tlsConfigToProto(profile, tlsStatuses, warnings, ca),
		TlsStatuses:      tlsStatuses,
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
	sslMode = normalizeTLSMode(sslMode)
	if !validSSLModes[sslMode] {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid ssl_mode %q: must be one of disabled, prefer, required", sslMode))
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

	current, err := s.profiles.GetByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get existing connection profile: %w", err))
	}

	updated, err := s.profiles.Upsert(ctx, &db.ConnectionProfile{
		ClusterID:               clusterID,
		EndpointMode:            endpointMode,
		PublicHost:              publicHost,
		PublicPort:              publicPort,
		SSLMode:                 sslMode,
		AllowedCIDRs:            allowedCIDRs,
		AllowedAdminCIDRs:       current.AllowedAdminCIDRs,
		AllowedReplicationCIDRs: current.AllowedReplicationCIDRs,
		TLSCertFile:             current.TLSCertFile,
		TLSKeyFile:              current.TLSKeyFile,
		TLSCAFile:               current.TLSCAFile,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("upsert connection profile: %w", err))
	}

	return connect.NewResponse(&skylexv1.UpdateConnectionProfileResponse{
		Profile: profileToProto(updated),
	}), nil
}

func (s *PostgresManagementService) GetNetworkAccess(
	ctx context.Context,
	req *connect.Request[skylexv1.GetNetworkAccessRequest],
) (*connect.Response[skylexv1.GetNetworkAccessResponse], error) {
	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}

	profile, err := s.profiles.GetByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get connection profile: %w", err))
	}
	statuses, err := s.hbaStatuses(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&skylexv1.GetNetworkAccessResponse{
		AllowedApplicationCidrs:  profile.AllowedCIDRs,
		AllowedAdminCidrs:        profile.AllowedAdminCIDRs,
		InternalReplicationCidrs: profile.AllowedReplicationCIDRs,
		HbaStatuses:              statuses,
	}), nil
}

func (s *PostgresManagementService) UpdateNetworkAccess(
	ctx context.Context,
	req *connect.Request[skylexv1.UpdateNetworkAccessRequest],
) (*connect.Response[skylexv1.UpdateNetworkAccessResponse], error) {
	userRole, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if userRole == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot update network access"))
	}

	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	if err := s.validate.Struct(updateNetworkAccessInput{ClusterID: clusterID}); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid network access request: %w", err))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}

	applicationCIDRs, err := normalizeCIDRs(req.Msg.GetAllowedApplicationCidrs(), "allowed_application_cidrs")
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	adminCIDRs, err := normalizeCIDRs(req.Msg.GetAllowedAdminCidrs(), "allowed_admin_cidrs")
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	replicationCIDRs, err := normalizeCIDRs(req.Msg.GetInternalReplicationCidrs(), "internal_replication_cidrs")
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	mu := s.clusterLock(clusterID)
	mu.Lock()
	defer mu.Unlock()

	current, err := s.profiles.GetByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get connection profile: %w", err))
	}
	updated, err := s.profiles.Upsert(ctx, &db.ConnectionProfile{
		ClusterID:               clusterID,
		EndpointMode:            current.EndpointMode,
		PublicHost:              current.PublicHost,
		PublicPort:              current.PublicPort,
		SSLMode:                 current.SSLMode,
		AllowedCIDRs:            applicationCIDRs,
		AllowedAdminCIDRs:       adminCIDRs,
		AllowedReplicationCIDRs: replicationCIDRs,
		TLSCertFile:             current.TLSCertFile,
		TLSKeyFile:              current.TLSKeyFile,
		TLSCAFile:               current.TLSCAFile,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("upsert network access: %w", err))
	}
	s.auditAccessChange(ctx, clusterID, "update", updated)

	return connect.NewResponse(&skylexv1.UpdateNetworkAccessResponse{
		AllowedApplicationCidrs:  updated.AllowedCIDRs,
		AllowedAdminCidrs:        updated.AllowedAdminCIDRs,
		InternalReplicationCidrs: updated.AllowedReplicationCIDRs,
	}), nil
}

func (s *PostgresManagementService) ApplyHBA(
	ctx context.Context,
	req *connect.Request[skylexv1.ApplyHBARequest],
) (*connect.Response[skylexv1.ApplyHBAResponse], error) {
	userRole, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if userRole == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot apply HBA"))
	}
	if s.access == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("postgres access repository is not configured"))
	}

	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}

	mu := s.clusterLock(clusterID)
	mu.Lock()
	defer mu.Unlock()

	profile, err := s.profiles.GetByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get connection profile: %w", err))
	}
	nodes, _, err := s.nodes.ListByCluster(ctx, clusterID, 0, 1000)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list cluster nodes: %w", err))
	}
	readyNodes := make([]*models.Node, 0, len(nodes))
	for _, node := range nodes {
		if node.AgentID == "" || !node.PostgresInstalled || !node.PostgresDataInitialized {
			continue
		}
		readyNodes = append(readyNodes, node)
	}
	if len(readyNodes) == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no ready nodes found for HBA apply"))
	}

	roles, err := s.roles.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list managed roles: %w", err))
	}
	databases, err := s.databases.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list managed databases: %w", err))
	}
	appRules, adminRules, replicationRules := hbaRuleSets(profile, readyNodes)
	adminRoles := readyAdminRoleNames(roles)
	appRoles := readyApplicationRoleNames(roles)
	appDatabases := readyApplicationDatabaseNames(databases)
	commands := make([]db.ApplyHBANodeCommand, 0, len(readyNodes))
	for _, node := range readyNodes {
		payload, err := json.Marshal(map[string]interface{}{
			"cluster_id":            clusterID,
			"node_id":               node.ID,
			"admin_cidrs":           adminRules,
			"replication_cidrs":     replicationRules,
			"application_cidrs":     appRules,
			"admin_roles":           adminRoles,
			"application_roles":     appRoles,
			"application_databases": appDatabases,
			"allow_promote":         node.Role == models.NodeRolePrimary && len(readyNodes) == 1,
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("marshal hba payload: %w", err))
		}
		commands = append(commands, db.ApplyHBANodeCommand{
			NodeID:  node.ID,
			AgentID: node.AgentID,
			Payload: string(payload),
		})
	}

	statuses, err := s.access.QueueApplyHBACommands(ctx, clusterID, commands)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("queue hba apply commands: %w", err))
	}
	s.auditAccessChange(ctx, clusterID, "apply_hba", profile)

	protoStatuses := make([]*skylexv1.HBAApplyStatus, 0, len(statuses))
	for _, status := range statuses {
		protoStatuses = append(protoStatuses, hbaStatusToProto(status))
	}
	return connect.NewResponse(&skylexv1.ApplyHBAResponse{HbaStatuses: protoStatuses}), nil
}

func normalizeCIDRs(raw []string, field string) ([]string, error) {
	if raw == nil {
		return []string{}, nil
	}
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		cidr := strings.TrimSpace(value)
		if cidr == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid %s CIDR %q: %w", field, cidr, err)
		}
		canonical := prefix.Masked().String()
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		out = append(out, canonical)
	}
	return out, nil
}

func hbaRuleSets(profile *db.ConnectionProfile, nodes []*models.Node) ([]string, []string, []string) {
	app := append([]string{}, profile.AllowedCIDRs...)
	admin := append([]string{}, profile.AllowedAdminCIDRs...)
	replication := append([]string{}, profile.AllowedReplicationCIDRs...)
	seenReplication := make(map[string]bool, len(replication)+len(nodes)*2)
	for _, cidr := range replication {
		seenReplication[cidr] = true
	}
	for _, node := range nodes {
		for _, host := range []string{node.Address, node.Hostname} {
			cidr := hostToHostCIDR(host)
			if cidr == "" || seenReplication[cidr] {
				continue
			}
			seenReplication[cidr] = true
			replication = append(replication, cidr)
		}
	}
	return app, admin, replication
}

func readyAdminRoleNames(roles []*db.PostgresRole) []string {
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		if role.Status != "ready" || role.RoleKind != "admin" {
			continue
		}
		names = append(names, role.RoleName)
	}
	return names
}

func readyApplicationRoleNames(roles []*db.PostgresRole) []string {
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		if role.Status != "ready" || role.RoleKind == "admin" {
			continue
		}
		names = append(names, role.RoleName)
	}
	return names
}

func readyApplicationDatabaseNames(databases []*db.PostgresDatabase) []string {
	names := make([]string, 0, len(databases))
	for _, database := range databases {
		if database.Status != "ready" {
			continue
		}
		names = append(names, database.DatabaseName)
	}
	return names
}

func hostToHostCIDR(host string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return ""
	}
	if addr.Is4() {
		return netip.PrefixFrom(addr, 32).String()
	}
	return netip.PrefixFrom(addr, 128).String()
}

func (s *PostgresManagementService) hbaStatuses(ctx context.Context, clusterID string) ([]*skylexv1.HBAApplyStatus, error) {
	if s.access == nil {
		return []*skylexv1.HBAApplyStatus{}, nil
	}
	statuses, err := s.access.ListHBAStatusByCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list hba apply status: %w", err))
	}
	protoStatuses := make([]*skylexv1.HBAApplyStatus, 0, len(statuses))
	for _, status := range statuses {
		protoStatuses = append(protoStatuses, hbaStatusToProto(status))
	}
	return protoStatuses, nil
}

func hbaStatusToProto(s *db.PostgresHBAApplyStatus) *skylexv1.HBAApplyStatus {
	proto := &skylexv1.HBAApplyStatus{
		ClusterId: s.ClusterID,
		NodeId:    s.NodeID,
		CommandId: s.CommandID,
		Status:    s.Status,
		Error:     s.Error,
	}
	if s.AppliedAt != nil {
		proto.AppliedAt = timestamppb.New(*s.AppliedAt)
	}
	if !s.UpdatedAt.IsZero() {
		proto.UpdatedAt = timestamppb.New(s.UpdatedAt)
	}
	return proto
}

func (s *PostgresManagementService) GetTLSConfig(
	ctx context.Context,
	req *connect.Request[skylexv1.GetTLSConfigRequest],
) (*connect.Response[skylexv1.GetTLSConfigResponse], error) {
	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}

	profile, err := s.profiles.GetByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get connection profile: %w", err))
	}
	statuses, err := s.tlsStatuses(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	ca, err := s.tlsCAForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	warnings := tlsWarnings(profile, statuses, ca)
	return connect.NewResponse(&skylexv1.GetTLSConfigResponse{Config: tlsConfigToProto(profile, statuses, warnings, ca)}), nil
}

func (s *PostgresManagementService) GenerateTLSCA(
	ctx context.Context,
	req *connect.Request[skylexv1.GenerateTLSCARequest],
) (*connect.Response[skylexv1.GenerateTLSCAResponse], error) {
	userRole, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if userRole == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot generate TLS CA"))
	}
	if s.tlsCA == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("postgres TLS CA repository is not configured"))
	}
	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}

	mu := s.clusterLock(clusterID)
	mu.Lock()
	defer mu.Unlock()

	caCertPEM, caKeyPEM, err := generateClusterCA(clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	ca, err := s.tlsCA.Upsert(ctx, clusterID, caCertPEM, caKeyPEM)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("store TLS CA: %w", err))
	}
	profile, err := s.profiles.GetByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get connection profile: %w", err))
	}
	statuses, err := s.tlsStatuses(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	warnings := tlsWarnings(profile, statuses, ca)
	s.auditTLSChange(ctx, clusterID, "generate_ca", profile)
	return connect.NewResponse(&skylexv1.GenerateTLSCAResponse{
		Config:    tlsConfigToProto(profile, statuses, warnings, ca),
		CaCertPem: ca.CACertPEM,
	}), nil
}

func (s *PostgresManagementService) GetTLSCACert(
	ctx context.Context,
	req *connect.Request[skylexv1.GetTLSCACertRequest],
) (*connect.Response[skylexv1.GetTLSCACertResponse], error) {
	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}
	ca, err := s.tlsCAForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if ca == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("TLS CA has not been generated for cluster %q", clusterID))
	}
	return connect.NewResponse(&skylexv1.GetTLSCACertResponse{CaCertPem: ca.CACertPEM}), nil
}

func (s *PostgresManagementService) UpdateTLSConfig(
	ctx context.Context,
	req *connect.Request[skylexv1.UpdateTLSConfigRequest],
) (*connect.Response[skylexv1.UpdateTLSConfigResponse], error) {
	userRole, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if userRole == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot update TLS configuration"))
	}

	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	tlsMode := normalizeTLSMode(req.Msg.GetTlsMode())
	if tlsMode == "" {
		tlsMode = db.DefaultSSLMode
	}
	if err := s.validate.Struct(updateTLSConfigInput{ClusterID: clusterID, TLSMode: tlsMode}); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid TLS config request: %w", err))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}

	certFile := strings.TrimSpace(req.Msg.GetCertFile())
	keyFile := strings.TrimSpace(req.Msg.GetKeyFile())
	caFile := strings.TrimSpace(req.Msg.GetCaFile())
	if tlsMode != "disabled" && ((certFile == "") != (keyFile == "")) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cert_file and key_file must both be set for manual TLS certificates, or both left empty for Skylex-managed certificates"))
	}
	for field, path := range map[string]string{"cert_file": certFile, "key_file": keyFile, "ca_file": caFile} {
		if err := validateAgentPath(path); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid %s: %w", field, err))
		}
	}

	mu := s.clusterLock(clusterID)
	mu.Lock()
	defer mu.Unlock()

	current, err := s.profiles.GetByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get connection profile: %w", err))
	}
	updated, err := s.profiles.Upsert(ctx, &db.ConnectionProfile{
		ClusterID:               clusterID,
		EndpointMode:            current.EndpointMode,
		PublicHost:              current.PublicHost,
		PublicPort:              current.PublicPort,
		SSLMode:                 tlsMode,
		AllowedCIDRs:            current.AllowedCIDRs,
		AllowedAdminCIDRs:       current.AllowedAdminCIDRs,
		AllowedReplicationCIDRs: current.AllowedReplicationCIDRs,
		TLSCertFile:             certFile,
		TLSKeyFile:              keyFile,
		TLSCAFile:               caFile,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("upsert TLS config: %w", err))
	}
	statuses, err := s.tlsStatuses(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	ca, err := s.tlsCAForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	warnings := tlsWarnings(updated, statuses, ca)
	s.auditTLSChange(ctx, clusterID, "update", updated)
	return connect.NewResponse(&skylexv1.UpdateTLSConfigResponse{Config: tlsConfigToProto(updated, statuses, warnings, ca)}), nil
}

func (s *PostgresManagementService) ApplyTLS(
	ctx context.Context,
	req *connect.Request[skylexv1.ApplyTLSRequest],
) (*connect.Response[skylexv1.ApplyTLSResponse], error) {
	userRole, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if userRole == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot apply TLS configuration"))
	}
	if s.tls == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("postgres TLS repository is not configured"))
	}

	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}

	mu := s.clusterLock(clusterID)
	mu.Lock()
	defer mu.Unlock()

	profile, err := s.profiles.GetByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get connection profile: %w", err))
	}
	tlsMode := normalizeTLSMode(profile.SSLMode)
	if !validSSLModes[tlsMode] {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid TLS mode %q", profile.SSLMode))
	}
	ca, err := s.tlsCAForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if tlsMode != "disabled" && profile.TLSCertFile == "" && profile.TLSKeyFile == "" && ca == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("generate a TLS CA before applying Skylex-managed TLS"))
	}
	var caKeyPEM string
	if tlsMode != "disabled" && profile.TLSCertFile == "" && profile.TLSKeyFile == "" {
		caKeyPEM, err = s.tlsCA.DecryptCAKey(ca)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("decrypt TLS CA key: %w", err))
		}
	}
	nodes, _, err := s.nodes.ListByCluster(ctx, clusterID, 0, 1000)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list cluster nodes: %w", err))
	}
	readyNodes := make([]*models.Node, 0, len(nodes))
	for _, node := range nodes {
		if node.AgentID == "" || !node.PostgresInstalled || !node.PostgresDataInitialized {
			continue
		}
		readyNodes = append(readyNodes, node)
	}
	if len(readyNodes) == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no ready nodes found for TLS apply"))
	}

	commands := make([]db.ApplyTLSNodeCommand, 0, len(readyNodes))
	for _, node := range readyNodes {
		certHosts := tlsCertificateHosts(profile, node)
		secrets := map[string]string{}
		certSecretKey := ""
		keySecretKey := ""
		caSecretKey := ""
		if tlsMode != "disabled" && profile.TLSCertFile == "" && profile.TLSKeyFile == "" {
			certPEM, keyPEM, err := generateServerCertificate(ca.CACertPEM, caKeyPEM, certHosts)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("generate server certificate for node %q: %w", node.ID, err))
			}
			certSecretKey = "server_cert_pem"
			keySecretKey = "server_key_pem"
			caSecretKey = "ca_cert_pem"
			secrets[certSecretKey] = certPEM
			secrets[keySecretKey] = keyPEM
			secrets[caSecretKey] = ca.CACertPEM
		}
		payload, err := json.Marshal(map[string]interface{}{
			"cluster_id":         clusterID,
			"node_id":            node.ID,
			"tls_mode":           tlsMode,
			"cert_file":          profile.TLSCertFile,
			"key_file":           profile.TLSKeyFile,
			"ca_file":            profile.TLSCAFile,
			"cert_hosts":         certHosts,
			"cert_secret_key":    certSecretKey,
			"key_secret_key":     keySecretKey,
			"ca_cert_secret_key": caSecretKey,
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("marshal tls payload: %w", err))
		}
		commands = append(commands, db.ApplyTLSNodeCommand{NodeID: node.ID, AgentID: node.AgentID, Payload: string(payload), Secrets: secrets})
	}

	statuses, err := s.tls.QueueApplyTLSCommands(ctx, clusterID, tlsMode, commands)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("queue tls apply commands: %w", err))
	}
	s.auditTLSChange(ctx, clusterID, "apply_tls", profile)
	protoStatuses := make([]*skylexv1.TLSApplyStatus, 0, len(statuses))
	for _, status := range statuses {
		protoStatuses = append(protoStatuses, tlsStatusToProto(status))
	}
	return connect.NewResponse(&skylexv1.ApplyTLSResponse{Statuses: protoStatuses}), nil
}

func (s *PostgresManagementService) tlsStatuses(ctx context.Context, clusterID string) ([]*skylexv1.TLSApplyStatus, error) {
	if s.tls == nil {
		return []*skylexv1.TLSApplyStatus{}, nil
	}
	statuses, err := s.tls.ListStatusByCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list tls apply status: %w", err))
	}
	protoStatuses := make([]*skylexv1.TLSApplyStatus, 0, len(statuses))
	for _, status := range statuses {
		protoStatuses = append(protoStatuses, tlsStatusToProto(status))
	}
	return protoStatuses, nil
}

func tlsStatusToProto(s *db.PostgresTLSApplyStatus) *skylexv1.TLSApplyStatus {
	proto := &skylexv1.TLSApplyStatus{
		ClusterId:        s.ClusterID,
		NodeId:           s.NodeID,
		CommandId:        s.CommandID,
		RequestedTlsMode: s.RequestedTLSMode,
		Status:           s.Status,
		Error:            s.Error,
		TlsActive:        s.TLSActive,
	}
	if s.AppliedAt != nil {
		proto.AppliedAt = timestamppb.New(*s.AppliedAt)
	}
	if !s.UpdatedAt.IsZero() {
		proto.UpdatedAt = timestamppb.New(s.UpdatedAt)
	}
	return proto
}

func tlsConfigToProto(p *db.ConnectionProfile, statuses []*skylexv1.TLSApplyStatus, warnings []string, ca *db.PostgresTLSCA) *skylexv1.TLSConfig {
	proto := &skylexv1.TLSConfig{
		ClusterId: p.ClusterID,
		TlsMode:   normalizeTLSMode(p.SSLMode),
		CertFile:  p.TLSCertFile,
		KeyFile:   p.TLSKeyFile,
		CaFile:    p.TLSCAFile,
		Statuses:  statuses,
		Warnings:  warnings,
	}
	if ca != nil {
		proto.CaGenerated = true
		if !ca.CreatedAt.IsZero() {
			proto.CaCreatedAt = timestamppb.New(ca.CreatedAt)
		}
	}
	return proto
}

func tlsWarnings(p *db.ConnectionProfile, statuses []*skylexv1.TLSApplyStatus, ca *db.PostgresTLSCA) []string {
	tlsMode := normalizeTLSMode(p.SSLMode)
	warnings := []string{}
	if tlsMode != "disabled" && p.TLSCertFile == "" && p.TLSKeyFile == "" {
		if ca == nil {
			warnings = append(warnings, "Generate a TLS CA before applying Skylex-managed certificates.")
		} else {
			warnings = append(warnings, "TLS will use Skylex-managed CA-signed certificates unless certificate and key paths are configured.")
		}
	}
	if tlsMode != "disabled" && ((p.TLSCertFile == "") != (p.TLSKeyFile == "")) {
		warnings = append(warnings, "TLS manual certificate paths are incomplete; set both certificate and key paths, or clear both for Skylex-managed certificates.")
	}
	if tlsMode == "required" && len(statuses) == 0 {
		warnings = append(warnings, "TLS required mode is configured but has not been applied to any ready node yet.")
	}
	for _, status := range statuses {
		if status.GetStatus() != "succeeded" {
			warnings = append(warnings, "TLS configuration is not active on all ready nodes.")
			return warnings
		}
		if tlsMode == "required" && !status.GetTlsActive() {
			warnings = append(warnings, "TLS required mode is waiting for PostgreSQL to confirm SSL is active on all nodes.")
			return warnings
		}
	}
	return warnings
}

func (s *PostgresManagementService) tlsCAForCluster(ctx context.Context, clusterID string) (*db.PostgresTLSCA, error) {
	if s.tlsCA == nil {
		return nil, nil
	}
	ca, err := s.tlsCA.GetByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get TLS CA: %w", err))
	}
	return ca, nil
}

func tlsCertificateHosts(profile *db.ConnectionProfile, node *models.Node) []string {
	seen := map[string]bool{}
	hosts := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		hosts = append(hosts, value)
	}
	add(node.Hostname)
	add(node.Address)
	if profile.EndpointMode == "manual_stable_endpoint" {
		add(profile.PublicHost)
	}
	add("localhost")
	add("127.0.0.1")
	add("::1")
	return hosts
}

func normalizeTLSMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "disable":
		return "disabled"
	case "require":
		return "required"
	default:
		return strings.TrimSpace(mode)
	}
}

func validateAgentPath(path string) error {
	if path == "" {
		return nil
	}
	if strings.ContainsAny(path, "\x00\r\n") {
		return fmt.Errorf("path must not contain control characters")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must be absolute")
	}
	return nil
}

func (s *PostgresManagementService) auditTLSChange(ctx context.Context, clusterID, action string, profile *db.ConnectionProfile) {
	if s.audit == nil || profile == nil {
		return
	}
	detail := fmt.Sprintf("action=%s cluster_id=%s tls_mode=%s ca_configured=%v", action, clusterID, normalizeTLSMode(profile.SSLMode), profile.TLSCAFile != "")
	if err := s.audit.Log(&models.AuditLog{
		UserID:    UserIDFromContext(ctx),
		Action:    models.AuditActionUpdatePostgresTLS,
		Resource:  "PostgresManagementService.TLS",
		Detail:    detail,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		s.log.Warn("audit postgres tls change failed", "cluster_id", clusterID, "error", err)
	}
}

func (s *PostgresManagementService) auditAccessChange(ctx context.Context, clusterID, action string, profile *db.ConnectionProfile) {
	if s.audit == nil || profile == nil {
		return
	}
	detail := fmt.Sprintf("action=%s cluster_id=%s application_cidrs=%d admin_cidrs=%d replication_cidrs=%d", action, clusterID, len(profile.AllowedCIDRs), len(profile.AllowedAdminCIDRs), len(profile.AllowedReplicationCIDRs))
	if err := s.audit.Log(&models.AuditLog{
		UserID:    UserIDFromContext(ctx),
		Action:    models.AuditActionUpdatePostgresAccess,
		Resource:  "PostgresManagementService.NetworkAccess",
		Detail:    detail,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		s.log.Warn("audit postgres access change failed", "cluster_id", clusterID, "error", err)
	}
}

// profileToProto converts a db.ConnectionProfile to the proto message.
func profileToProto(p *db.ConnectionProfile) *skylexv1.ConnectionProfile {
	proto := &skylexv1.ConnectionProfile{
		ClusterId:               p.ClusterID,
		EndpointMode:            p.EndpointMode,
		PublicHost:              p.PublicHost,
		PublicPort:              int32(p.PublicPort),
		SslMode:                 p.SSLMode,
		AllowedCidrs:            p.AllowedCIDRs,
		AllowedAdminCidrs:       p.AllowedAdminCIDRs,
		AllowedReplicationCidrs: p.AllowedReplicationCIDRs,
		TlsCertFile:             p.TLSCertFile,
		TlsKeyFile:              p.TLSKeyFile,
		TlsCaFile:               p.TLSCAFile,
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
	provider, err := s.requireModule(ctx, clusterID, engine.ModuleRoles)
	if err != nil {
		return nil, err
	}
	if err := provider.ValidateRoleName(roleName); err != nil {
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

	ensureRoleAction, _ := provider.Action(engine.OpEnsureRole)
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
		EnsureAction:           ensureRoleAction,
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
	provider, err := s.requireModule(ctx, role.ClusterID, engine.ModuleRoles)
	if err != nil {
		return nil, err
	}
	rotateRoleAction, _ := provider.Action(engine.OpRotateRolePassword)
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
		RotateAction:           rotateRoleAction,
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
	if s.databases != nil {
		hasDatabases, err := s.databases.HasByOwnerRole(ctx, role.ID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check databases owned by role: %w", err))
		}
		if hasDatabases {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("role owns one or more managed databases"))
		}
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

	dropRoleAction := ""
	if provider, err := s.requireModule(ctx, role.ClusterID, engine.ModuleRoles); err != nil {
		return nil, err
	} else {
		dropRoleAction, _ = provider.Action(engine.OpDropRole)
	}
	txResult, err := s.roles.DeleteWithCommand(ctx, db.DeleteRoleTxInput{
		RoleID:       role.ID,
		OperationID:  opID,
		CommandID:    cmdID,
		NodeID:       primary.ID,
		AgentID:      primary.AgentID,
		Payload:      string(payload),
		BeforeAction: managementBeforeAction(allowPromote),
		DropAction:   dropRoleAction,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("mark role deleting and queue command: %w", err))
	}
	s.log.Info("queued pg_drop_role", "role_id", role.ID, "command_id", txResult.Command.ID)

	return connect.NewResponse(&skylexv1.DeleteRoleResponse{}), nil
}

// Managed Databases (Phase 4)

func (s *PostgresManagementService) ListDatabases(
	ctx context.Context,
	req *connect.Request[skylexv1.ListDatabasesRequest],
) (*connect.Response[skylexv1.ListDatabasesResponse], error) {
	clusterID := req.Msg.GetClusterId()
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	if err := s.requireCluster(ctx, clusterID); err != nil {
		return nil, err
	}
	if s.databases == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database repository is not configured"))
	}

	databases, err := s.databases.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list databases: %w", err))
	}
	roles, err := s.roles.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list owner roles: %w", err))
	}
	roleNames := make(map[string]string, len(roles))
	for _, role := range roles {
		roleNames[role.ID] = role.RoleName
	}

	protoDatabases := make([]*skylexv1.PostgresDatabase, 0, len(databases))
	for _, database := range databases {
		ownerRoleName := ""
		if database.OwnerRoleID != nil {
			ownerRoleName = roleNames[*database.OwnerRoleID]
		}
		protoDatabases = append(protoDatabases, databaseToProto(database, ownerRoleName))
	}
	return connect.NewResponse(&skylexv1.ListDatabasesResponse{Databases: protoDatabases}), nil
}

func (s *PostgresManagementService) CreateDatabase(
	ctx context.Context,
	req *connect.Request[skylexv1.CreateDatabaseRequest],
) (*connect.Response[skylexv1.CreateDatabaseResponse], error) {
	userRole, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if userRole == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot create databases"))
	}
	if s.databases == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database repository is not configured"))
	}

	clusterID := req.Msg.GetClusterId()
	databaseName := strings.TrimSpace(req.Msg.GetDatabaseName())
	ownerRoleID := strings.TrimSpace(req.Msg.GetOwnerRoleId())
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	provider, err := s.requireModule(ctx, clusterID, engine.ModuleDatabases)
	if err != nil {
		return nil, err
	}
	if err := provider.ValidateDatabaseName(databaseName); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid create database request: %w", err))
	}
	if isReservedDatabaseName(databaseName) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("database %q is reserved", databaseName))
	}

	var ownerRole *db.PostgresRole
	if ownerRoleID != "" {
		role, err := s.roles.GetByID(ctx, ownerRoleID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get owner role: %w", err))
		}
		if role == nil {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("owner role %q not found", ownerRoleID))
		}
		if role.ClusterID != clusterID {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("owner role belongs to a different cluster"))
		}
		if role.Status != "ready" {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("owner role must be ready"))
		}
		if role.RoleKind == "read_only" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("read-only roles cannot own databases"))
		}
		ownerRole = role
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

	existing, err := s.databases.GetByClusterAndName(ctx, clusterID, databaseName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check existing database: %w", err))
	}
	if existing != nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("database %q already exists in cluster %q", databaseName, clusterID))
	}

	databaseID := id.New()
	opID := id.New()
	cmdID := id.New()
	payloadMap := map[string]interface{}{
		"database_id":   databaseID,
		"operation_id":  opID,
		"database_name": databaseName,
		"allow_promote": allowPromote,
	}
	if ownerRole != nil {
		payloadMap["owner_role_name"] = ownerRole.RoleName
		payloadMap["owner_role_kind"] = ownerRole.RoleKind
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("marshal database payload: %w", err))
	}

	ensureDatabaseAction, _ := provider.Action(engine.OpEnsureDatabase)
	txResult, err := s.databases.CreateWithCommand(ctx, db.CreateDatabaseTxInput{
		DatabaseID:   databaseID,
		OperationID:  opID,
		CommandID:    cmdID,
		ClusterID:    clusterID,
		NodeID:       primary.ID,
		AgentID:      primary.AgentID,
		DatabaseName: databaseName,
		OwnerRoleID:  ownerRoleID,
		Payload:      string(payload),
		BeforeAction: managementBeforeAction(allowPromote),
		EnsureAction: ensureDatabaseAction,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create database and queue command: %w", err))
	}
	s.log.Info("queued pg_ensure_database", "database_id", txResult.Database.ID, "command_id", txResult.Command.ID)

	ownerRoleName := ""
	if ownerRole != nil {
		ownerRoleName = ownerRole.RoleName
	}
	return connect.NewResponse(&skylexv1.CreateDatabaseResponse{Database: databaseToProto(txResult.Database, ownerRoleName)}), nil
}

func (s *PostgresManagementService) DeleteDatabase(
	ctx context.Context,
	req *connect.Request[skylexv1.DeleteDatabaseRequest],
) (*connect.Response[skylexv1.DeleteDatabaseResponse], error) {
	userRole, _ := ctx.Value(ctxKeyUserRole).(models.Role)
	if userRole == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot delete databases"))
	}
	if s.databases == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database repository is not configured"))
	}

	databaseID := req.Msg.GetDatabaseId()
	if databaseID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("database_id is required"))
	}
	database, err := s.databases.GetByID(ctx, databaseID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get database: %w", err))
	}
	if database == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("database %q not found", databaseID))
	}
	if database.Status == "deleting" {
		return connect.NewResponse(&skylexv1.DeleteDatabaseResponse{}), nil
	}

	mu := s.clusterLock(database.ClusterID)
	mu.Lock()
	defer mu.Unlock()

	primary, err := s.resolveManagementPrimary(ctx, database.ClusterID)
	if err != nil {
		return nil, err
	}
	allowPromote, err := s.allowPromotionForCluster(ctx, database.ClusterID)
	if err != nil {
		return nil, err
	}

	opID := id.New()
	cmdID := id.New()
	payload, err := json.Marshal(map[string]interface{}{
		"database_id":   database.ID,
		"operation_id":  opID,
		"database_name": database.DatabaseName,
		"allow_promote": allowPromote,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("marshal database delete payload: %w", err))
	}

	dropDatabaseAction := ""
	if provider, err := s.requireModule(ctx, database.ClusterID, engine.ModuleDatabases); err != nil {
		return nil, err
	} else {
		dropDatabaseAction, _ = provider.Action(engine.OpDropDatabase)
	}
	txResult, err := s.databases.DeleteWithCommand(ctx, db.DeleteDatabaseTxInput{
		DatabaseID:   database.ID,
		OperationID:  opID,
		CommandID:    cmdID,
		NodeID:       primary.ID,
		AgentID:      primary.AgentID,
		Payload:      string(payload),
		BeforeAction: managementBeforeAction(allowPromote),
		DropAction:   dropDatabaseAction,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("mark database deleting and queue command: %w", err))
	}
	s.log.Info("queued pg_drop_database", "database_id", database.ID, "command_id", txResult.Command.ID)

	return connect.NewResponse(&skylexv1.DeleteDatabaseResponse{}), nil
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

// providerForCluster resolves the engine.Provider for a cluster, returning a
// connect error if the cluster is missing or its engine has no provider.
func (s *PostgresManagementService) providerForCluster(ctx context.Context, clusterID string) (engine.Provider, error) {
	cluster, err := s.clusters.GetByID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}
	if cluster == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster %q not found", clusterID))
	}
	provider, err := engine.For(cluster.Engine)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return provider, nil
}

// requireModule returns a connect error when the cluster's engine does not
// expose the given management module. This is the API boundary check that keeps
// engine-specific features (e.g. extensions) from being invoked on engines that
// do not support them.
func (s *PostgresManagementService) requireModule(ctx context.Context, clusterID string, module engine.ModuleID) (engine.Provider, error) {
	provider, err := s.providerForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if !provider.Supports(module) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("engine %q does not support the %q module", provider.Engine(), module))
	}
	return provider, nil
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

func databaseToProto(d *db.PostgresDatabase, ownerRoleName string) *skylexv1.PostgresDatabase {
	proto := &skylexv1.PostgresDatabase{
		Id:            d.ID,
		ClusterId:     d.ClusterID,
		DatabaseName:  d.DatabaseName,
		OwnerRoleName: ownerRoleName,
		Status:        d.Status,
	}
	if d.OwnerRoleID != nil {
		proto.OwnerRoleId = *d.OwnerRoleID
	}
	if !d.CreatedAt.IsZero() {
		proto.CreatedAt = timestamppb.New(d.CreatedAt)
	}
	if !d.UpdatedAt.IsZero() {
		proto.UpdatedAt = timestamppb.New(d.UpdatedAt)
	}
	return proto
}

func isReservedDatabaseName(name string) bool {
	switch strings.ToLower(name) {
	case "postgres", "template0", "template1":
		return true
	default:
		return false
	}
}
