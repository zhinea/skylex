package postgres

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (p *Instance) localConnectDatabase(ctx context.Context, databaseName string) (*pgx.Conn, error) {
	u := url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("127.0.0.1:%d", p.Port),
		Path:   "/" + databaseName,
		User:   url.User(p.Superuser),
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()

	cfg, err := pgx.ParseConfig(u.String())
	if err != nil {
		return nil, fmt.Errorf("parse local postgres database connection config: %w", err)
	}
	cfg.Password = p.ReplPass

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to local postgres database: %w", redactPGError(err))
	}
	return conn, nil
}

// EnsureDatabase creates a database if needed and converges optional ownership.
func (p *Instance) EnsureDatabase(ctx context.Context, databaseName, ownerRoleName string, allowPromote bool) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if isReservedDatabase(databaseName) {
		return fmt.Errorf("database %q is reserved", databaseName)
	}

	conn, err := p.connectWritable(ctx, allowPromote)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	var exists bool
	var currentOwner string
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, databaseName,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check database existence: %w", redactPGError(err))
	}

	if !exists {
		statement := fmt.Sprintf("CREATE DATABASE %s", quoteIdent(databaseName))
		if ownerRoleName != "" {
			statement += " OWNER " + quoteIdent(ownerRoleName)
		}
		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("create database %q: %w", databaseName, redactPGError(err))
		}
		return nil
	}

	if ownerRoleName == "" {
		return nil
	}
	if err := conn.QueryRow(ctx,
		`SELECT r.rolname FROM pg_database d JOIN pg_roles r ON r.oid = d.datdba WHERE d.datname = $1`, databaseName,
	).Scan(&currentOwner); err != nil {
		return fmt.Errorf("check database owner: %w", redactPGError(err))
	}
	if currentOwner == ownerRoleName {
		return nil
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf("ALTER DATABASE %s OWNER TO %s", quoteIdent(databaseName), quoteIdent(ownerRoleName))); err != nil {
		return fmt.Errorf("alter database %q owner: %w", databaseName, redactPGError(err))
	}
	return nil
}

// DropDatabase drops a managed database if it exists. It refuses built-in databases.
func (p *Instance) DropDatabase(ctx context.Context, databaseName string, allowPromote bool) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if isReservedDatabase(databaseName) {
		return fmt.Errorf("database %q is reserved", databaseName)
	}

	conn, err := p.connectWritable(ctx, allowPromote)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx,
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, databaseName,
	); err != nil {
		return fmt.Errorf("terminate database sessions: %w", redactPGError(err))
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quoteIdent(databaseName))); err != nil {
		return fmt.Errorf("drop database %q: %w", databaseName, redactPGError(err))
	}
	return nil
}

// GrantDatabasePrivileges idempotently grants role privileges for the target database.
func (p *Instance) GrantDatabasePrivileges(ctx context.Context, databaseName, roleName string, roleKind RoleKind, allowPromote bool) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if isReservedDatabase(databaseName) {
		return fmt.Errorf("database %q is reserved", databaseName)
	}
	if roleName == "" {
		return fmt.Errorf("role name is required")
	}

	adminConn, err := p.connectWritable(ctx, allowPromote)
	if err != nil {
		return err
	}
	defer adminConn.Close(ctx)

	var exists bool
	if err := adminConn.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, databaseName,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check database existence before grant: %w", redactPGError(err))
	}
	if !exists {
		return fmt.Errorf("database %q does not exist", databaseName)
	}
	if err := execGrantStatements(ctx, adminConn, databaseLevelGrantStatements(databaseName, roleName, roleKind)); err != nil {
		return err
	}

	databaseConn, err := p.localConnectDatabase(ctx, databaseName)
	if err != nil {
		return err
	}
	defer databaseConn.Close(ctx)
	writable, err := postgresWritable(ctx, databaseConn)
	if err != nil {
		return err
	}
	if !writable {
		return fmt.Errorf("postgresql is in recovery/read-only; database grants must run on the writable primary")
	}

	return execGrantStatements(ctx, databaseConn, schemaGrantStatements(roleName, roleKind))
}

func execGrantStatements(ctx context.Context, conn *pgx.Conn, statements []string) error {
	for _, statement := range statements {
		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("execute grant statement: %w", redactPGError(err))
		}
	}
	return nil
}

func databaseLevelGrantStatements(databaseName, roleName string, roleKind RoleKind) []string {
	quotedDB := quoteIdent(databaseName)
	quotedRole := quoteIdent(roleName)
	if roleKind == RoleKindReadOnly {
		return []string{fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s", quotedDB, quotedRole)}
	}
	return []string{fmt.Sprintf("GRANT CONNECT, CREATE ON DATABASE %s TO %s", quotedDB, quotedRole)}
}

func schemaGrantStatements(roleName string, roleKind RoleKind) []string {
	quotedRole := quoteIdent(roleName)
	if roleKind == RoleKindReadOnly {
		return []string{
			fmt.Sprintf("GRANT USAGE ON SCHEMA public TO %s", quotedRole),
			fmt.Sprintf("GRANT SELECT ON ALL TABLES IN SCHEMA public TO %s", quotedRole),
			fmt.Sprintf("GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO %s", quotedRole),
			fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO %s", quotedRole),
			fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON SEQUENCES TO %s", quotedRole),
		}
	}
	return []string{
		fmt.Sprintf("GRANT USAGE, CREATE ON SCHEMA public TO %s", quotedRole),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO %s", quotedRole),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO %s", quotedRole),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO %s", quotedRole),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO %s", quotedRole),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO %s", quotedRole),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON FUNCTIONS TO %s", quotedRole),
	}
}

func isReservedDatabase(name string) bool {
	switch strings.ToLower(name) {
	case "postgres", "template0", "template1":
		return true
	default:
		return false
	}
}
