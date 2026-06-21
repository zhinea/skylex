package agent

import (
	"context"
	"encoding/json"
	"fmt"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/postgres"
)

// roleCommandPayload is the structure sent in agent_commands.payload for
// pg_ensure_role / pg_rotate_role_password / pg_drop_role.
// Passwords are never included here; they come via command secrets resolved
// server-side and injected into the Secrets map on the AgentCommand message.
type roleCommandPayload struct {
	RoleName          string `json:"role_name"`
	RoleKind          string `json:"role_kind"` // only for pg_ensure_role
	PasswordSecretKey string `json:"password_secret_key"`
	AllowPromote      bool   `json:"allow_promote"`
}

func (a *Agent) executeEnsureRole(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	var p roleCommandPayload
	if err := json.Unmarshal([]byte(cmd.GetPayload()), &p); err != nil {
		return false, "", fmt.Sprintf("pg_ensure_role: invalid payload: %v", err)
	}
	if p.RoleName == "" {
		return false, "", "pg_ensure_role: role_name is required"
	}
	if p.RoleKind == "" {
		p.RoleKind = "custom"
	}

	// Password is injected via Secrets by FetchCommand.
	secretKey := p.PasswordSecretKey
	if secretKey == "" {
		secretKey = "password"
	}
	password := cmd.GetSecrets()[secretKey]
	if password == "" {
		return false, "", "pg_ensure_role: password secret not resolved"
	}

	logger.Info(fmt.Sprintf("ensuring role %q (kind=%s)", p.RoleName, p.RoleKind))

	if err := a.pg.EnsureRole(ctx, p.RoleName, postgres.RoleKind(p.RoleKind), password, p.AllowPromote); err != nil {
		return false, "", fmt.Sprintf("pg_ensure_role failed: %v", err)
	}

	return true, fmt.Sprintf("role %q ensured (kind=%s)", p.RoleName, p.RoleKind), ""
}

func (a *Agent) executeRotateRolePassword(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	var p roleCommandPayload
	if err := json.Unmarshal([]byte(cmd.GetPayload()), &p); err != nil {
		return false, "", fmt.Sprintf("pg_rotate_role_password: invalid payload: %v", err)
	}
	if p.RoleName == "" {
		return false, "", "pg_rotate_role_password: role_name is required"
	}

	secretKey := p.PasswordSecretKey
	if secretKey == "" {
		secretKey = "password"
	}
	newPassword := cmd.GetSecrets()[secretKey]
	if newPassword == "" {
		return false, "", "pg_rotate_role_password: password secret not resolved"
	}

	logger.Info(fmt.Sprintf("rotating password for role %q", p.RoleName))

	if err := a.pg.RotateRolePassword(ctx, p.RoleName, newPassword, p.AllowPromote); err != nil {
		return false, "", fmt.Sprintf("pg_rotate_role_password failed: %v", err)
	}

	return true, fmt.Sprintf("password rotated for role %q", p.RoleName), ""
}

func (a *Agent) executeDropRole(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	var p roleCommandPayload
	if err := json.Unmarshal([]byte(cmd.GetPayload()), &p); err != nil {
		return false, "", fmt.Sprintf("pg_drop_role: invalid payload: %v", err)
	}
	if p.RoleName == "" {
		return false, "", "pg_drop_role: role_name is required"
	}

	logger.Info(fmt.Sprintf("dropping role %q", p.RoleName))

	if err := a.pg.DropRole(ctx, p.RoleName, p.AllowPromote); err != nil {
		return false, "", fmt.Sprintf("pg_drop_role failed: %v", err)
	}

	return true, fmt.Sprintf("role %q dropped", p.RoleName), ""
}
