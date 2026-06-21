package installer

import (
	"context"
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

func TestPrivilegedCommand_RootUsesOriginalCommand(t *testing.T) {
	name, args, err := privilegedCommand(0, false, "apt-get", "update")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "apt-get" {
		t.Fatalf("expected apt-get, got %q", name)
	}
	if strings.Join(args, " ") != "update" {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestPrivilegedCommand_NonRootUsesNonInteractiveSudo(t *testing.T) {
	name, args, err := privilegedCommand(1000, true, "apt-get", "purge", "-y", "postgresql-16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "sudo" {
		t.Fatalf("expected sudo, got %q", name)
	}
	got := strings.Join(args, " ")
	want := "-n apt-get purge -y postgresql-16"
	if got != want {
		t.Fatalf("expected args %q, got %q", want, got)
	}
}

func TestPrivilegedCommand_NonRootWithoutSudoReturnsActionableError(t *testing.T) {
	_, _, err := privilegedCommand(1000, false, "systemctl", "stop", "postgresql")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "requires root privileges") {
		t.Fatalf("expected privilege error, got %q", err.Error())
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

func TestDockerInstaller_Install_RequiresClusterID(t *testing.T) {
	var inst DockerInstaller
	err := inst.Install(context.Background(), InstallConfig{ClusterID: ""}, nil)
	if err == nil {
		t.Fatal("expected error for empty cluster id")
	}
	if !strings.Contains(err.Error(), "cluster_id is required") {
		t.Fatalf("expected cluster_id error, got %q", err.Error())
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
