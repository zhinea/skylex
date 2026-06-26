package postgres

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// skylex_admin_e2e_test.go proves the full skylex_admin credential path against
// a REAL PostgreSQL server (not mocks): a durable skylex_admin SUPERUSER is
// provisioned, the agent's management layer then connects AS skylex_admin to run
// enable-extension / create-role / delete-role / ensure-database / grant, and —
// critically — those operations keep succeeding even when the agent's in-memory
// bootstrap password is wrong/stale, because management authenticates as
// skylex_admin rather than the bootstrap postgres identity.
//
// HOW TO RUN LOCALLY (the test is skipped cleanly when PG is absent, so it never
// breaks `go test ./...` / CI):
//
//	# Start a throwaway Postgres on 127.0.0.1:5432 with a known superuser pw:
//	docker run --rm -d --name skylex-e2e-pg -p 5432:5432 \
//	    -e POSTGRES_PASSWORD=postgres postgres:16
//
//	# Point the test at it and run (note -short MUST be omitted):
//	SKYLEX_TEST_PG_DSN='postgres://postgres:postgres@127.0.0.1:5432/postgres' \
//	    go test ./internal/postgres/ -run TestSkylexAdminCredentialPathE2E -v
//
// NOTE: postgres.Instance always dials host=127.0.0.1; only the port, superuser
// and password from the DSN are used (host/dbname in the DSN are ignored for the
// management connection). Use a DSN whose Postgres is reachable on 127.0.0.1.
const skylexTestPGDSNEnv = "SKYLEX_TEST_PG_DSN"

// e2eNames are the throwaway objects this test creates. They are dropped on
// entry and via t.Cleanup so the test is idempotent and re-runnable.
const (
	e2eAppRole  = "skylex_e2e_app"
	e2eDropRole = "skylex_e2e_drop"
	e2eDatabase = "skylex_e2e_db"
	e2eAdminPw  = "e2e-admin-pw-Aa1!"
	e2eAppPw    = "e2e-app-pw-Bb2!"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newE2EInstance builds a postgres.Instance with the given superuser identity,
// mirroring how the agent constructs and re-points its Instance. dataDir/binDir
// are irrelevant here because the management methods only open network
// connections; they never shell out to pg_ctl.
func newE2EInstance(port int, superuser, password string) *Instance {
	inst := New("", "", "16", port, superuser, "", "", testLogger())
	inst.SetSuperuserCredentials(superuser, password)
	return inst
}

// parseE2EDSN extracts the port, superuser and password from the DSN using pgx
// (already a project dependency) rather than hand-rolling a parser.
func parseE2EDSN(t *testing.T, dsn string) (port int, user, password string) {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", skylexTestPGDSNEnv, err)
	}
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	return int(cfg.Port), cfg.User, cfg.Password
}

// e2eCleanup drops every object the test creates, using the bootstrap superuser
// connection so cleanup works regardless of which identity created them.
func e2eCleanup(ctx context.Context, t *testing.T, bootstrap *Instance) {
	t.Helper()
	conn, err := bootstrap.localConnect(ctx)
	if err != nil {
		t.Logf("e2e cleanup connect failed (continuing): %v", err)
		return
	}
	defer conn.Close(ctx)
	stmts := []string{
		"DROP DATABASE IF EXISTS " + quoteIdent(e2eDatabase),
		"DROP ROLE IF EXISTS " + quoteIdent(e2eAppRole),
		"DROP ROLE IF EXISTS " + quoteIdent(e2eDropRole),
	}
	for _, s := range stmts {
		if _, err := conn.Exec(ctx, s); err != nil {
			t.Logf("e2e cleanup %q: %v", s, err)
		}
	}
}

func TestSkylexAdminCredentialPathE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping skylex_admin e2e test in -short mode")
	}
	dsn := strings.TrimSpace(os.Getenv(skylexTestPGDSNEnv))
	if dsn == "" {
		t.Skipf("skipping skylex_admin e2e test: %s is not set (see file header for run instructions)", skylexTestPGDSNEnv)
	}

	port, bootstrapUser, bootstrapPw := parseE2EDSN(t, dsn)
	if bootstrapUser == "" {
		bootstrapUser = "postgres"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Step 0: the agent's starting point — a server reachable as the bootstrap
	// superuser (postgres) with a known password.
	bootstrap := newE2EInstance(port, bootstrapUser, bootstrapPw)
	e2eCleanup(ctx, t, bootstrap) // clean any leftovers from a prior run
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		e2eCleanup(cleanupCtx, t, bootstrap)
	})

	// Step a: provision the durable skylex_admin SUPERUSER via EnsureRole, exactly
	// as the server-queued pg_ensure_admin_role command does on the agent.
	if err := bootstrap.EnsureRole(ctx, "skylex_admin", RoleKindAdmin, e2eAdminPw, true); err != nil {
		t.Fatalf("provision skylex_admin via bootstrap EnsureRole: %v", err)
	}

	// Step b: build the Instance the agent uses for management AFTER
	// applySkylexAdminCredentialsIfPresent has switched it to skylex_admin, and
	// verify every management method succeeds while authenticated as skylex_admin.
	admin := newE2EInstance(port, "skylex_admin", e2eAdminPw)

	t.Run("EnsureRole_as_skylex_admin", func(t *testing.T) {
		if err := admin.EnsureRole(ctx, e2eAppRole, RoleKindReadWrite, e2eAppPw, true); err != nil {
			t.Fatalf("EnsureRole(%s) as skylex_admin: %v", e2eAppRole, err)
		}
	})

	t.Run("EnsureDatabase_as_skylex_admin", func(t *testing.T) {
		if err := admin.EnsureDatabase(ctx, e2eDatabase, e2eAppRole, true); err != nil {
			t.Fatalf("EnsureDatabase(%s) as skylex_admin: %v", e2eDatabase, err)
		}
	})

	t.Run("EnsureExtension_pgcrypto_as_skylex_admin", func(t *testing.T) {
		if err := admin.EnsureExtension(ctx, e2eDatabase, "pgcrypto", true); err != nil {
			t.Fatalf("EnsureExtension(pgcrypto) as skylex_admin: %v", err)
		}
	})

	t.Run("GrantDatabasePrivileges_as_skylex_admin", func(t *testing.T) {
		if err := admin.GrantDatabasePrivileges(ctx, e2eDatabase, e2eAppRole, RoleKindReadWrite, true); err != nil {
			t.Fatalf("GrantDatabasePrivileges as skylex_admin: %v", err)
		}
	})

	t.Run("DropRole_as_skylex_admin", func(t *testing.T) {
		// Use a dedicated role that owns nothing so the drop is unobstructed.
		if err := admin.EnsureRole(ctx, e2eDropRole, RoleKindReadWrite, e2eAppPw, true); err != nil {
			t.Fatalf("EnsureRole(%s) as skylex_admin: %v", e2eDropRole, err)
		}
		if err := admin.DropRole(ctx, e2eDropRole, true); err != nil {
			t.Fatalf("DropRole(%s) as skylex_admin: %v", e2eDropRole, err)
		}
	})

	// Step c — THE BUG SCENARIO. Simulate an agent whose in-memory bootstrap
	// password is wrong/stale (e.g. after a restart that reverted it to the
	// config default). This is precisely the "failed SASL auth" condition that
	// motivated authenticating as skylex_admin.
	t.Run("BugRepro_stale_bootstrap_password", func(t *testing.T) {
		staleBootstrap := newE2EInstance(port, bootstrapUser, "wrong-stale-bootstrap-password")

		// Lock in the regression: with the wrong bootstrap password and WITHOUT
		// skylex_admin creds applied, management fails with an auth error. If this
		// ever stops failing, the bug-repro below would be meaningless.
		err := staleBootstrap.EnsureRole(ctx, e2eAppRole, RoleKindReadWrite, e2eAppPw, true)
		if err == nil {
			t.Fatal("expected EnsureRole to FAIL with a stale bootstrap password and no skylex_admin creds")
		}
		if !isAuthError(err) {
			t.Fatalf("expected an authentication error with stale bootstrap password, got: %v", err)
		}

		// Now apply skylex_admin creds exactly as applySkylexAdminCredentialsIfPresent
		// does when the command carries the skylex_admin_password secret. The SAME
		// operation must now SUCCEED — proving management no longer depends on the
		// bootstrap password.
		staleBootstrap.SetSuperuserCredentials("skylex_admin", e2eAdminPw)
		if err := staleBootstrap.EnsureRole(ctx, e2eAppRole, RoleKindReadWrite, e2eAppPw, true); err != nil {
			t.Fatalf("EnsureRole after applying skylex_admin creds (stale bootstrap pw): %v", err)
		}
	})
}

// isAuthError reports whether err looks like a PostgreSQL authentication
// failure. The agent's redactPGError keeps the SQLSTATE/auth wording while
// stripping secret values, so a substring match is sufficient and stable.
func isAuthError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "password authentication failed") ||
		strings.Contains(msg, "sasl") ||
		strings.Contains(msg, "authentication") ||
		strings.Contains(msg, "28p01") ||
		strings.Contains(msg, "28000")
}
