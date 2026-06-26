package agent

import (
	"context"
	"testing"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
)

// These tests focus on payload/secret handling for executeEnsureAdminRole.
// The error paths return before a.pg is touched, so no live PostgreSQL or
// commandLogger client is required. The happy path (EnsureRole invocation) is
// exercised by integration testing against a live instance.

func TestExecuteEnsureAdminRole_MissingSecretFails(t *testing.T) {
	a := &Agent{}
	cmd := &skylexv1.AgentCommand{
		Action:  "pg_ensure_admin_role",
		Payload: `{"role_name":"skylex_admin","role_kind":"admin","password_secret_key":"skylex_admin_password","allow_promote":false}`,
		Secrets: map[string]string{},
	}

	ok, _, errMsg := a.executeEnsureAdminRole(context.Background(), cmd, nil)
	if ok {
		t.Fatal("expected failure when password secret is absent")
	}
	if errMsg == "" {
		t.Fatal("expected a non-empty error message when password secret is absent")
	}
}

func TestExecuteEnsureAdminRole_InvalidPayloadFails(t *testing.T) {
	a := &Agent{}
	cmd := &skylexv1.AgentCommand{
		Action:  "pg_ensure_admin_role",
		Payload: `not-json`,
		Secrets: map[string]string{"skylex_admin_password": "secret"},
	}

	ok, _, errMsg := a.executeEnsureAdminRole(context.Background(), cmd, nil)
	if ok {
		t.Fatal("expected failure on invalid payload")
	}
	if errMsg == "" {
		t.Fatal("expected a non-empty error message on invalid payload")
	}
}

func TestExecuteEnsureAdminRole_DefaultsSecretKeyWhenUnset(t *testing.T) {
	a := &Agent{}
	// Payload omits password_secret_key; the executor must default to
	// "skylex_admin_password". With no matching secret it must still fail
	// cleanly rather than proceeding without a password.
	cmd := &skylexv1.AgentCommand{
		Action:  "pg_ensure_admin_role",
		Payload: `{"role_name":"ignored","role_kind":"custom","allow_promote":true}`,
		Secrets: map[string]string{"unrelated": "value"},
	}

	ok, _, errMsg := a.executeEnsureAdminRole(context.Background(), cmd, nil)
	if ok {
		t.Fatal("expected failure when default secret key is not resolved")
	}
	if errMsg == "" {
		t.Fatal("expected a non-empty error message when default secret key is missing")
	}
}
