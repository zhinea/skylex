package engine_test

import (
	"testing"

	"github.com/zhinea/skylex/internal/engine"
	_ "github.com/zhinea/skylex/internal/engine/postgres"
	"github.com/zhinea/skylex/internal/models"
)

func TestProviderRegisteredForPostgres(t *testing.T) {
	p, err := engine.For(models.EnginePostgreSQL)
	if err != nil {
		t.Fatalf("expected postgres provider, got error: %v", err)
	}
	if p.Engine() != models.EnginePostgreSQL {
		t.Fatalf("unexpected engine: %q", p.Engine())
	}
	if p.DefaultPort() != 5432 {
		t.Fatalf("unexpected default port: %d", p.DefaultPort())
	}
}

func TestForUnknownEngineErrors(t *testing.T) {
	if _, err := engine.For(models.EngineType("mysql")); err == nil {
		t.Fatal("expected error for unregistered engine")
	}
}

func TestPostgresAdvertisesExtensions(t *testing.T) {
	p, err := engine.For(models.EnginePostgreSQL)
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	if !p.Supports(engine.ModuleExtensions) {
		t.Fatal("expected postgres to support the extensions module")
	}
	// Universal modules every engine should advertise.
	for _, m := range []engine.ModuleID{engine.ModuleDatabases, engine.ModuleRoles, engine.ModuleTLS} {
		if !p.Supports(m) {
			t.Fatalf("expected postgres to support module %q", m)
		}
	}
}

func TestActionMappingRoundTrip(t *testing.T) {
	p, err := engine.For(models.EnginePostgreSQL)
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	cases := map[engine.LogicalOp]string{
		engine.OpEnsureDatabase:          "pg_ensure_database",
		engine.OpDropDatabase:            "pg_drop_database",
		engine.OpGrantDatabasePrivileges: "pg_grant_database_privileges",
		engine.OpEnsureRole:              "pg_ensure_role",
		engine.OpRotateRolePassword:      "pg_rotate_role_password",
		engine.OpDropRole:                "pg_drop_role",
		engine.OpApplyHBA:                "pg_apply_hba",
		engine.OpApplyTLS:                "pg_apply_tls",
	}
	for op, want := range cases {
		got, ok := p.Action(op)
		if !ok || got != want {
			t.Fatalf("Action(%q) = %q, %v; want %q", op, got, ok, want)
		}
		// Reverse lookup must resolve back to the same logical op.
		gotOp, ok := engine.LogicalOpForAction(want)
		if !ok || gotOp != op {
			t.Fatalf("LogicalOpForAction(%q) = %q, %v; want %q", want, gotOp, ok, op)
		}
	}
}

func TestValidation(t *testing.T) {
	p, err := engine.For(models.EnginePostgreSQL)
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	if err := p.ValidateRoleName("app_user"); err != nil {
		t.Fatalf("expected valid role name: %v", err)
	}
	if err := p.ValidateRoleName("1bad"); err == nil {
		t.Fatal("expected invalid role name to fail")
	}
	if err := p.ValidateDatabaseName("app_db"); err != nil {
		t.Fatalf("expected valid database name: %v", err)
	}
	if err := p.ValidateDatabaseName("bad-name"); err == nil {
		t.Fatal("expected invalid database name to fail")
	}
}
