package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type captureLogSink struct {
	info  []string
	error []string
}

func (s *captureLogSink) Info(message string)  { s.info = append(s.info, message) }
func (s *captureLogSink) Error(message string) { s.error = append(s.error, message) }

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

func TestRunCommandStreamsStdoutAndStderr(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=TestHelperProcessRunCommand", "--")
	cmd.Env = append(os.Environ(), "SKYLEX_HELPER_PROCESS=1")
	log := &captureLogSink{}

	output, err := runCommand(context.Background(), cmd, log)
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if !strings.Contains(string(output), "stdout line") || !strings.Contains(string(output), "stderr line") {
		t.Fatalf("expected combined output to include stdout and stderr, got %q", string(output))
	}
	if len(log.info) != 1 || log.info[0] != "stdout line" {
		t.Fatalf("expected stdout log line, got %#v", log.info)
	}
	if len(log.error) != 1 || log.error[0] != "stderr line" {
		t.Fatalf("expected stderr log line, got %#v", log.error)
	}
}

func TestHelperProcessRunCommand(t *testing.T) {
	if os.Getenv("SKYLEX_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "stdout line")
	fmt.Fprintln(os.Stderr, "stderr line")
	os.Exit(0)
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
