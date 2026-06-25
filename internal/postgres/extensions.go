package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// EnsureExtension idempotently enables an extension in the target database via
// CREATE EXTENSION IF NOT EXISTS. This is a zero-downtime operation: it requires
// no server restart and replicates to standbys through WAL. It must run on the
// writable primary.
//
// The extension name is validated against the engine allowlist by the control
// plane before this is ever queued; quoteIdent is still applied as defense in
// depth so the identifier can never break out of its quoted context.
func (p *Instance) EnsureExtension(ctx context.Context, databaseName, extensionName string, allowPromote bool) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := p.connectWritableDatabase(ctx, databaseName, allowPromote)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	stmt := fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s", quoteIdent(extensionName))
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("create extension %q in database %q: %w", extensionName, databaseName, redactPGError(err))
	}
	return nil
}

// DropExtension idempotently disables an extension in the target database via
// DROP EXTENSION IF EXISTS. Like EnsureExtension this needs no restart. It must
// run on the writable primary.
func (p *Instance) DropExtension(ctx context.Context, databaseName, extensionName string, allowPromote bool) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := p.connectWritableDatabase(ctx, databaseName, allowPromote)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	stmt := fmt.Sprintf("DROP EXTENSION IF EXISTS %s", quoteIdent(extensionName))
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("drop extension %q in database %q: %w", extensionName, databaseName, redactPGError(err))
	}
	return nil
}

// connectWritableDatabase connects to a specific database and verifies the node
// is writable (the primary). Extensions are per-database objects, so unlike
// connectWritable (which targets the default database) we must connect to the
// database we intend to change.
func (p *Instance) connectWritableDatabase(ctx context.Context, databaseName string, allowPromote bool) (*pgx.Conn, error) {
	conn, err := p.localConnectDatabase(ctx, databaseName)
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
		return nil, fmt.Errorf("postgresql is in recovery/read-only; extension management must run on the writable primary")
	}
	return nil, fmt.Errorf("postgresql is in recovery/read-only after promotion command; retry after promotion completes")
}
