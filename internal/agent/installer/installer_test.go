package installer

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// formatCommand
// ---------------------------------------------------------------------------

func TestFormatCommand_NoArgs(t *testing.T) {
	got := formatCommand("pg_ctl")
	if got != "$ pg_ctl" {
		t.Fatalf("expected '$ pg_ctl', got %q", got)
	}
}

func TestFormatCommand_WithArgs(t *testing.T) {
	got := formatCommand("apt-get", "install", "-y", "postgresql-16")
	want := "$ apt-get install -y postgresql-16"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// PreflightResult.Details
// ---------------------------------------------------------------------------

func TestPreflightResult_Details_NothingFound(t *testing.T) {
	r := PreflightResult{State: PreflightNothingFound}
	d := r.Details()
	if !strings.Contains(d, "no native PostgreSQL") {
		t.Fatalf("unexpected details for NOTHING_FOUND: %q", d)
	}
}

func TestPreflightResult_Details_PGExists(t *testing.T) {
	r := PreflightResult{
		State:           PreflightPGExists,
		Version:         "16.2",
		DataDir:         "/var/lib/postgresql/16/main",
		DataPresent:     true,
		DataInitialized: true,
	}
	d := r.Details()
	if !strings.Contains(d, "16.2") {
		t.Fatalf("expected version in details, got %q", d)
	}
	if !strings.Contains(d, "/var/lib/postgresql/16/main") {
		t.Fatalf("expected data_dir in details, got %q", d)
	}
	if !strings.Contains(d, "data_present=true") {
		t.Fatalf("expected data_present in details, got %q", d)
	}
}

func TestPreflightResult_Details_PGExists_UnknownVersion(t *testing.T) {
	r := PreflightResult{State: PreflightPGExists, Version: "", DataDir: "/data"}
	d := r.Details()
	if !strings.Contains(d, "unknown") {
		t.Fatalf("expected 'unknown' version in details, got %q", d)
	}
}

// ---------------------------------------------------------------------------
// DockerContainerName / DockerCommandArgs
// ---------------------------------------------------------------------------

func TestDockerContainerName(t *testing.T) {
	name := DockerContainerName("cluster-abc123")
	if name == "" {
		t.Fatal("expected non-empty container name")
	}
	if name[:len(dockerContainerNamePrefix)] != dockerContainerNamePrefix {
		t.Fatalf("expected container name to start with %q, got %q", dockerContainerNamePrefix, name)
	}
}

func TestDockerCommandArgs_ContainsContainerName(t *testing.T) {
	args := DockerCommandArgs("/data", 5432, "psql", "-U", "postgres")
	// Must include "exec" and the container name.
	foundExec := false
	foundContainer := false
	for _, a := range args {
		if a == "exec" {
			foundExec = true
		}
		if a == "skylex-postgres" {
			foundContainer = true
		}
	}
	if !foundExec {
		t.Fatalf("expected 'exec' in args: %v", args)
	}
	if !foundContainer {
		t.Fatalf("expected container name in args: %v", args)
	}
	// Extra command args must appear at the end.
	last := args[len(args)-1]
	if last != "postgres" {
		t.Fatalf("expected last arg 'postgres', got %q", last)
	}
}
