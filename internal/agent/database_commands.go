package agent

import (
	"context"
	"encoding/json"
	"fmt"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/postgres"
)

type databaseCommandPayload struct {
	DatabaseName  string `json:"database_name"`
	OwnerRoleName string `json:"owner_role_name"`
	OwnerRoleKind string `json:"owner_role_kind"`
	GrantRoleName string `json:"grant_role_name"`
	GrantRoleKind string `json:"grant_role_kind"`
	AllowPromote  bool   `json:"allow_promote"`
}

type hbaCommandPayload struct {
	AdminCIDRs       []string `json:"admin_cidrs"`
	ReplicationCIDRs []string `json:"replication_cidrs"`
	ApplicationCIDRs []string `json:"application_cidrs"`
	AdminRoles       []string `json:"admin_roles"`
	ApplicationRoles []string `json:"application_roles"`
	ApplicationDBs   []string `json:"application_databases"`
	AllowPromote     bool     `json:"allow_promote"`
}

type tlsCommandPayload struct {
	TLSMode  string `json:"tls_mode"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
	CAFile   string `json:"ca_file"`
}

func (a *Agent) executeEnsureDatabase(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	var p databaseCommandPayload
	if err := json.Unmarshal([]byte(cmd.GetPayload()), &p); err != nil {
		return false, "", fmt.Sprintf("pg_ensure_database: invalid payload: %v", err)
	}
	if p.DatabaseName == "" {
		return false, "", "pg_ensure_database: database_name is required"
	}

	logger.Info(fmt.Sprintf("ensuring database %q", p.DatabaseName))
	if err := a.pg.EnsureDatabase(ctx, p.DatabaseName, p.OwnerRoleName, p.AllowPromote); err != nil {
		return false, "", fmt.Sprintf("pg_ensure_database failed: %v", err)
	}
	return true, fmt.Sprintf("database %q ensured", p.DatabaseName), ""
}

func (a *Agent) executeDropDatabase(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	var p databaseCommandPayload
	if err := json.Unmarshal([]byte(cmd.GetPayload()), &p); err != nil {
		return false, "", fmt.Sprintf("pg_drop_database: invalid payload: %v", err)
	}
	if p.DatabaseName == "" {
		return false, "", "pg_drop_database: database_name is required"
	}

	logger.Info(fmt.Sprintf("dropping database %q", p.DatabaseName))
	if err := a.pg.DropDatabase(ctx, p.DatabaseName, p.AllowPromote); err != nil {
		return false, "", fmt.Sprintf("pg_drop_database failed: %v", err)
	}
	return true, fmt.Sprintf("database %q dropped", p.DatabaseName), ""
}

func (a *Agent) executeGrantDatabasePrivileges(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	var p databaseCommandPayload
	if err := json.Unmarshal([]byte(cmd.GetPayload()), &p); err != nil {
		return false, "", fmt.Sprintf("pg_grant_database_privileges: invalid payload: %v", err)
	}
	if p.DatabaseName == "" {
		return false, "", "pg_grant_database_privileges: database_name is required"
	}
	if p.GrantRoleName == "" {
		return false, "", "pg_grant_database_privileges: grant_role_name is required"
	}
	roleKind := p.GrantRoleKind
	if roleKind == "" {
		roleKind = "custom"
	}

	logger.Info(fmt.Sprintf("granting %s privileges on database %q to role %q", roleKind, p.DatabaseName, p.GrantRoleName))
	if err := a.pg.GrantDatabasePrivileges(ctx, p.DatabaseName, p.GrantRoleName, postgres.RoleKind(roleKind), p.AllowPromote); err != nil {
		return false, "", fmt.Sprintf("pg_grant_database_privileges failed: %v", err)
	}
	return true, fmt.Sprintf("database privileges granted on %q to %q", p.DatabaseName, p.GrantRoleName), ""
}

func (a *Agent) executeApplyHBA(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	var p hbaCommandPayload
	if err := json.Unmarshal([]byte(cmd.GetPayload()), &p); err != nil {
		return false, "", fmt.Sprintf("pg_apply_hba: invalid payload: %v", err)
	}

	logger.Info(fmt.Sprintf("applying HBA allowlists: admin=%d replication=%d application=%d", len(p.AdminCIDRs), len(p.ReplicationCIDRs), len(p.ApplicationCIDRs)))
	if p.AllowPromote {
		if err := a.pg.Promote(ctx); err != nil {
			return false, "", fmt.Sprintf("pg_apply_hba promote failed: %v", err)
		}
	}
	if err := a.pg.ApplyHBA(ctx, postgres.HBAPolicy{
		AdminCIDRs:       p.AdminCIDRs,
		ReplicationCIDRs: p.ReplicationCIDRs,
		ApplicationCIDRs: p.ApplicationCIDRs,
		AdminRoles:       p.AdminRoles,
		ApplicationRoles: p.ApplicationRoles,
		ApplicationDBs:   p.ApplicationDBs,
	}); err != nil {
		return false, "", fmt.Sprintf("pg_apply_hba failed: %v", err)
	}
	return true, "HBA allowlists applied and PostgreSQL reloaded", ""
}

func (a *Agent) executeApplyTLS(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	var p tlsCommandPayload
	if err := json.Unmarshal([]byte(cmd.GetPayload()), &p); err != nil {
		return false, "", fmt.Sprintf("pg_apply_tls: invalid payload: %v", err)
	}
	if p.TLSMode == "" {
		return false, "", "pg_apply_tls: tls_mode is required"
	}

	logger.Info(fmt.Sprintf("applying PostgreSQL TLS mode %q", p.TLSMode))
	if err := a.pg.ApplyTLS(ctx, postgres.TLSConfig{
		Mode:     p.TLSMode,
		CertFile: p.CertFile,
		KeyFile:  p.KeyFile,
		CAFile:   p.CAFile,
	}); err != nil {
		return false, "", fmt.Sprintf("pg_apply_tls failed: %v", err)
	}
	if p.TLSMode == postgres.TLSModeDisabled {
		return true, "PostgreSQL TLS disabled and configuration verified", ""
	}
	return true, "PostgreSQL TLS configuration applied and verified active", ""
}
