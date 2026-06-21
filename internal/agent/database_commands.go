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
