package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"unicode"

	connect "connectrpc.com/connect"
	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// validEndpointModes is the exhaustive set of accepted endpoint_mode values.
var validEndpointModes = map[string]bool{
	"direct_primary":        true,
	"manual_stable_endpoint": true,
}

// validSSLModes is the exhaustive set of accepted ssl_mode values.
var validSSLModes = map[string]bool{
	"prefer":  true,
	"require": true,
	"disable": true,
}

// PostgresManagementService implements the PostgresManagementService Connect-RPC handler.
type PostgresManagementService struct {
	profiles *db.ConnectionProfileRepository
	nodes    *db.NodeRepository
	clusters *db.ClusterRepository
	log      *slog.Logger
}

func NewPostgresManagementService(
	profiles *db.ConnectionProfileRepository,
	nodes *db.NodeRepository,
	clusters *db.ClusterRepository,
	log *slog.Logger,
) *PostgresManagementService {
	return &PostgresManagementService{
		profiles: profiles,
		nodes:    nodes,
		clusters: clusters,
		log:      log,
	}
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
