package agent

import (
	"log/slog"
	"testing"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/models"
	"github.com/zhinea/skylex/internal/postgres"
)

// newTestAgentWithPG builds an Agent carrying a postgres.Instance whose
// superuser identity starts as the bootstrap "postgres" account, mirroring the
// state after a fresh start before any credential injection.
func newTestAgentWithPG() *Agent {
	pg := postgres.New("/tmp/data", "/tmp/bin", "16", 5432, "postgres", "repl", "bootstrap-pw", slog.Default())
	return &Agent{pg: pg}
}

func TestApplySkylexAdminCredentialsIfPresent_SetsCredsWhenSecretPresent(t *testing.T) {
	a := newTestAgentWithPG()
	cmd := &skylexv1.AgentCommand{
		Action:  "pg_apply_extensions",
		Secrets: map[string]string{"skylex_admin_password": "durable-admin-pw"},
	}

	a.applySkylexAdminCredentialsIfPresent(cmd)

	if a.pg.Superuser != models.SkylexAdminRole {
		t.Fatalf("expected superuser %q, got %q", models.SkylexAdminRole, a.pg.Superuser)
	}
	if a.pg.SuperuserPassword != "durable-admin-pw" {
		t.Fatalf("expected superuser password to be the injected admin secret, got %q", a.pg.SuperuserPassword)
	}
}

func TestApplySkylexAdminCredentialsIfPresent_NoopWhenSecretAbsent(t *testing.T) {
	a := newTestAgentWithPG()
	beforeUser := a.pg.Superuser
	beforePass := a.pg.SuperuserPassword

	// No secrets at all.
	a.applySkylexAdminCredentialsIfPresent(&skylexv1.AgentCommand{Action: "pg_ensure_database"})
	if a.pg.Superuser != beforeUser || a.pg.SuperuserPassword != beforePass {
		t.Fatalf("expected credentials untouched when no secret present, got (%q,%q)", a.pg.Superuser, a.pg.SuperuserPassword)
	}

	// Secret map present but empty value for the key.
	a.applySkylexAdminCredentialsIfPresent(&skylexv1.AgentCommand{
		Action:  "pg_ensure_database",
		Secrets: map[string]string{"skylex_admin_password": ""},
	})
	if a.pg.Superuser != beforeUser || a.pg.SuperuserPassword != beforePass {
		t.Fatalf("expected credentials untouched when secret is empty, got (%q,%q)", a.pg.Superuser, a.pg.SuperuserPassword)
	}
}
