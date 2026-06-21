package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// RoleKind maps a managed role kind to PostgreSQL role attributes.
type RoleKind string

const (
	RoleKindAdmin     RoleKind = "admin"
	RoleKindReadWrite RoleKind = "read_write"
	RoleKindReadOnly  RoleKind = "read_only"
	RoleKindCustom    RoleKind = "custom"
)

const alterRolePasswordFunction = `CREATE OR REPLACE FUNCTION pg_temp.skylex_alter_role_password(role_name text, role_password text)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
  EXECUTE format('ALTER ROLE %I WITH ENCRYPTED PASSWORD %L', role_name, role_password);
END;
$$`

// localConnect opens a single pgx connection to the local PostgreSQL instance.
func (p *Instance) localConnect(ctx context.Context) (*pgx.Conn, error) {
	connStr := fmt.Sprintf("host=127.0.0.1 port=%d user=%s dbname=postgres sslmode=disable", p.Port, p.Superuser)
	cfg, err := pgx.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse local postgres connection config: %w", err)
	}
	cfg.Password = p.ReplPass

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to local postgres: %w", redactPGError(err))
	}
	return conn, nil
}

func (p *Instance) connectWritable(ctx context.Context, allowPromote bool) (*pgx.Conn, error) {
	conn, err := p.localConnect(ctx)
	if err != nil {
		return nil, err
	}

	writable, err := postgresWritable(ctx, conn)
	if err != nil {
		conn.Close(ctx)
		return nil, err
	}
	if writable {
		return conn, nil
	}

	conn.Close(ctx)
	if !allowPromote {
		return nil, fmt.Errorf("postgresql is in recovery/read-only; PostgreSQL management must run on the writable primary")
	}
	return nil, fmt.Errorf("postgresql is in recovery/read-only after promotion command; retry after promotion completes")
}

func postgresWritable(ctx context.Context, conn *pgx.Conn) (bool, error) {
	var inRecovery bool
	if err := conn.QueryRow(ctx, "SELECT pg_is_in_recovery()").Scan(&inRecovery); err != nil {
		return false, fmt.Errorf("check recovery state: %w", redactPGError(err))
	}
	return !inRecovery, nil
}

func quoteIdent(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

func roleAttributes(kind RoleKind) string {
	switch kind {
	case RoleKindAdmin:
		return "SUPERUSER LOGIN"
	case RoleKindReadWrite, RoleKindReadOnly, RoleKindCustom:
		return "NOSUPERUSER LOGIN"
	default:
		return "NOSUPERUSER LOGIN"
	}
}

// EnsureRole creates or updates a PostgreSQL role idempotently.
func (p *Instance) EnsureRole(ctx context.Context, roleName string, kind RoleKind, password string, allowPromote bool) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := p.connectWritable(ctx, allowPromote)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	var exists bool
	if err := conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", roleName,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check role existence: %w", redactPGError(err))
	}

	quotedRole := quoteIdent(roleName)
	attrs := roleAttributes(kind)
	if exists {
		if _, err := conn.Exec(ctx, fmt.Sprintf("ALTER ROLE %s WITH %s", quotedRole, attrs)); err != nil {
			return fmt.Errorf("alter role %q: %w", roleName, redactPGError(err))
		}
	} else {
		if _, err := conn.Exec(ctx, fmt.Sprintf("CREATE ROLE %s WITH %s", quotedRole, attrs)); err != nil {
			return fmt.Errorf("create role %q: %w", roleName, redactPGError(err))
		}
	}

	if err := setRolePassword(ctx, conn, roleName, password); err != nil {
		return fmt.Errorf("set password for role %q: %w", roleName, err)
	}
	return nil
}

// RotateRolePassword updates the password for an existing role.
func (p *Instance) RotateRolePassword(ctx context.Context, roleName, newPassword string, allowPromote bool) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	conn, err := p.connectWritable(ctx, allowPromote)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	if err := setRolePassword(ctx, conn, roleName, newPassword); err != nil {
		return fmt.Errorf("rotate password for role %q: %w", roleName, err)
	}
	return nil
}

func setRolePassword(ctx context.Context, conn *pgx.Conn, roleName, password string) error {
	if _, err := conn.Exec(ctx, alterRolePasswordFunction); err != nil {
		return fmt.Errorf("prepare password helper: %w", redactPGError(err))
	}
	if _, err := conn.Exec(ctx, "SELECT pg_temp.skylex_alter_role_password($1, $2)", roleName, password); err != nil {
		return redactPGError(err)
	}
	return nil
}

// DropRole drops a PostgreSQL role if it exists.
func (p *Instance) DropRole(ctx context.Context, roleName string, allowPromote bool) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	conn, err := p.connectWritable(ctx, allowPromote)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, fmt.Sprintf("DROP ROLE IF EXISTS %s", quoteIdent(roleName))); err != nil {
		return fmt.Errorf("drop role %q: %w", roleName, redactPGError(err))
	}
	return nil
}

// redactPGError strips any embedded password-like values from PostgreSQL error messages.
func redactPGError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	if idx := strings.Index(lower, "password"); idx >= 0 {
		return fmt.Errorf("%s[REDACTED]", msg[:idx])
	}
	return err
}
