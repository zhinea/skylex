package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	connect "connectrpc.com/connect"
	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/engine"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// extensionCatalog resolves the engine's extension catalog for a cluster, or a
// FailedPrecondition error if the engine does not support the Extensions module.
func (s *PostgresManagementService) extensionCatalog(ctx context.Context, clusterID string) (engine.ExtensionCatalog, error) {
	provider, err := s.requireModule(ctx, clusterID, engine.ModuleExtensions)
	if err != nil {
		return nil, err
	}
	catalog, ok := provider.(engine.ExtensionCatalog)
	if !ok {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("engine %q advertises extensions but provides no catalog", provider.Engine()))
	}
	return catalog, nil
}

// GetExtensions returns the engine extension catalog merged with the cluster's
// stored toggle state. Extensions absent from the stored state default to off.
func (s *PostgresManagementService) GetExtensions(
	ctx context.Context,
	req *connect.Request[skylexv1.GetExtensionsRequest],
) (*connect.Response[skylexv1.GetExtensionsResponse], error) {
	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	catalog, err := s.extensionCatalog(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if s.extensions == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("extension repository is not configured"))
	}

	stored, err := s.extensions.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list cluster extensions: %w", err))
	}
	byName := make(map[string]*db.ClusterExtension, len(stored))
	for _, e := range stored {
		byName[e.ExtensionName] = e
	}

	protos := make([]*skylexv1.Extension, 0, len(catalog.AvailableExtensions()))
	for _, c := range catalog.AvailableExtensions() {
		protos = append(protos, extensionToProto(c, byName[c.Name]))
	}
	return connect.NewResponse(&skylexv1.GetExtensionsResponse{Extensions: protos}), nil
}

// SetExtension persists the desired enabled state for one extension. The change
// only takes effect on the next ApplyExtensions.
func (s *PostgresManagementService) SetExtension(
	ctx context.Context,
	req *connect.Request[skylexv1.SetExtensionRequest],
) (*connect.Response[skylexv1.SetExtensionResponse], error) {
	if role, _ := ctx.Value(ctxKeyUserRole).(models.Role); role == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot change extensions"))
	}
	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	name := strings.TrimSpace(req.Msg.GetName())
	if clusterID == "" || name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id and name are required"))
	}
	catalog, err := s.extensionCatalog(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if err := catalog.ValidateExtensionName(name); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if s.extensions == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("extension repository is not configured"))
	}

	mu := s.clusterLock(clusterID)
	mu.Lock()
	defer mu.Unlock()

	updated, err := s.extensions.SetEnabled(ctx, clusterID, name, req.Msg.GetEnabled())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("set extension: %w", err))
	}
	s.auditExtensionChange(ctx, clusterID, "set", name)

	// Re-attach catalog metadata (label/description) to the response.
	var meta engine.Extension
	for _, c := range catalog.AvailableExtensions() {
		if c.Name == name {
			meta = c
			break
		}
	}
	return connect.NewResponse(&skylexv1.SetExtensionResponse{Extension: extensionToProto(meta, updated)}), nil
}

// ApplyExtensions queues a single command on the primary that converges every
// managed database to the desired extension state. CREATE/DROP EXTENSION require
// no restart, so this is a zero-downtime operation. Replicas receive the change
// through WAL replication.
func (s *PostgresManagementService) ApplyExtensions(
	ctx context.Context,
	req *connect.Request[skylexv1.ApplyExtensionsRequest],
) (*connect.Response[skylexv1.ApplyExtensionsResponse], error) {
	if role, _ := ctx.Value(ctxKeyUserRole).(models.Role); role == models.RoleViewer {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("viewer role cannot apply extensions"))
	}
	clusterID := strings.TrimSpace(req.Msg.GetClusterId())
	if clusterID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cluster_id is required"))
	}
	provider, err := s.requireModule(ctx, clusterID, engine.ModuleExtensions)
	if err != nil {
		return nil, err
	}
	if s.extensions == nil || s.databases == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("extension repository is not configured"))
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

	stored, err := s.extensions.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list cluster extensions: %w", err))
	}
	enable, disable := []string{}, []string{}
	for _, e := range stored {
		if e.Enabled {
			enable = append(enable, e.ExtensionName)
		} else {
			disable = append(disable, e.ExtensionName)
		}
	}
	if len(enable) == 0 && len(disable) == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no extensions toggled to apply"))
	}

	databases, err := s.databases.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list managed databases: %w", err))
	}
	dbNames := make([]string, 0, len(databases))
	for _, d := range databases {
		if d.Status == "ready" {
			dbNames = append(dbNames, d.DatabaseName)
		}
	}

	action, _ := provider.Action(engine.OpApplyExtensions)
	if action == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("engine %q does not support applying extensions", provider.Engine()))
	}
	payload, err := json.Marshal(map[string]interface{}{
		"cluster_id":    clusterID,
		"node_id":       primary.ID,
		"databases":     dbNames,
		"enable":        enable,
		"disable":       disable,
		"allow_promote": allowPromote,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("marshal extensions payload: %w", err))
	}

	if err := s.extensions.QueueApplyCommand(ctx, db.ApplyExtensionsCommand{
		ClusterID: clusterID,
		NodeID:    primary.ID,
		AgentID:   primary.AgentID,
		Payload:   string(payload),
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("queue apply extensions command: %w", err))
	}
	s.auditExtensionChange(ctx, clusterID, "apply", "")
	s.log.Info("queued pg_apply_extensions", "cluster_id", clusterID, "enable", len(enable), "disable", len(disable))

	catalog, _ := provider.(engine.ExtensionCatalog)
	return connect.NewResponse(&skylexv1.ApplyExtensionsResponse{Extensions: s.mergedExtensions(ctx, clusterID, catalog)}), nil
}

// mergedExtensions returns the catalog merged with freshly-read stored state.
func (s *PostgresManagementService) mergedExtensions(ctx context.Context, clusterID string, catalog engine.ExtensionCatalog) []*skylexv1.Extension {
	if catalog == nil {
		return []*skylexv1.Extension{}
	}
	stored, err := s.extensions.ListByCluster(ctx, clusterID)
	if err != nil {
		s.log.Warn("list extensions for response failed", "cluster_id", clusterID, "error", err)
		stored = nil
	}
	byName := make(map[string]*db.ClusterExtension, len(stored))
	for _, e := range stored {
		byName[e.ExtensionName] = e
	}
	out := make([]*skylexv1.Extension, 0, len(catalog.AvailableExtensions()))
	for _, c := range catalog.AvailableExtensions() {
		out = append(out, extensionToProto(c, byName[c.Name]))
	}
	return out
}

func extensionToProto(meta engine.Extension, state *db.ClusterExtension) *skylexv1.Extension {
	proto := &skylexv1.Extension{
		Name:        meta.Name,
		Label:       meta.Label,
		Description: meta.Description,
		Status:      "off",
	}
	if state != nil {
		proto.Enabled = state.Enabled
		proto.Status = state.Status
		proto.Error = state.Error
		if state.AppliedAt != nil {
			proto.AppliedAt = timestamppb.New(*state.AppliedAt)
		}
		if !state.UpdatedAt.IsZero() {
			proto.UpdatedAt = timestamppb.New(state.UpdatedAt)
		}
	}
	return proto
}

func (s *PostgresManagementService) auditExtensionChange(ctx context.Context, clusterID, action, name string) {
	if s.audit == nil {
		return
	}
	detail := fmt.Sprintf("action=%s cluster_id=%s extension=%s", action, clusterID, name)
	if err := s.audit.Log(&models.AuditLog{
		UserID:    UserIDFromContext(ctx),
		Action:    models.AuditActionUpdatePostgresExt,
		Resource:  "PostgresManagementService.Extensions",
		Detail:    detail,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		s.log.Warn("audit postgres extension change failed", "cluster_id", clusterID, "error", err)
	}
}
