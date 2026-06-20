package server

import (
	"encoding/json"
	"testing"

	"github.com/zhinea/skylex/internal/models"
)

// ---------------------------------------------------------------------------
// validateClusterSetting
// ---------------------------------------------------------------------------

func TestValidateClusterSetting_RejectsUnknownKey(t *testing.T) {
	if err := validateClusterSetting("fsync", "on"); err == nil {
		t.Fatal("expected error for unknown key fsync")
	}
}

func TestValidateClusterSetting_RejectsEmptyValue(t *testing.T) {
	if err := validateClusterSetting("max_connections", ""); err == nil {
		t.Fatal("expected error for empty value")
	}
	if err := validateClusterSetting("max_connections", "   "); err == nil {
		t.Fatal("expected error for whitespace-only value")
	}
}

func TestValidateClusterSetting_MaxConnections(t *testing.T) {
	valid := []string{"1", "100", "500"}
	for _, v := range valid {
		if err := validateClusterSetting("max_connections", v); err != nil {
			t.Errorf("max_connections=%q should be valid: %v", v, err)
		}
	}
	invalid := []string{"0", "-1", "abc", "100.5"}
	for _, v := range invalid {
		if err := validateClusterSetting("max_connections", v); err == nil {
			t.Errorf("max_connections=%q should be invalid", v)
		}
	}
}

func TestValidateClusterSetting_MaxWalSenders(t *testing.T) {
	if err := validateClusterSetting("max_wal_senders", "10"); err != nil {
		t.Fatalf("max_wal_senders=10 should be valid: %v", err)
	}
	if err := validateClusterSetting("max_wal_senders", "-5"); err == nil {
		t.Fatal("max_wal_senders=-5 should be invalid")
	}
}

func TestValidateClusterSetting_WalLevel(t *testing.T) {
	valid := []string{"replica", "logical", "REPLICA", "LOGICAL"}
	for _, v := range valid {
		if err := validateClusterSetting("wal_level", v); err != nil {
			t.Errorf("wal_level=%q should be valid: %v", v, err)
		}
	}
	if err := validateClusterSetting("wal_level", "minimal"); err == nil {
		t.Fatal("wal_level=minimal should be invalid")
	}
}

func TestValidateClusterSetting_SharedBuffers(t *testing.T) {
	valid := []string{"128MB", "1GB", "256kB", "512m"}
	for _, v := range valid {
		if err := validateClusterSetting("shared_buffers", v); err != nil {
			t.Errorf("shared_buffers=%q should be valid: %v", v, err)
		}
	}
	invalid := []string{"fast", "1.5GB", "-128MB"}
	for _, v := range invalid {
		if err := validateClusterSetting("shared_buffers", v); err == nil {
			t.Errorf("shared_buffers=%q should be invalid", v)
		}
	}
}

func TestValidateClusterSetting_WorkMem(t *testing.T) {
	if err := validateClusterSetting("work_mem", "8MB"); err != nil {
		t.Fatalf("work_mem=8MB should be valid: %v", err)
	}
	if err := validateClusterSetting("work_mem", "verymuch"); err == nil {
		t.Fatal("work_mem=verymuch should be invalid")
	}
}

// ---------------------------------------------------------------------------
// validateClusterSettingsParameters
// ---------------------------------------------------------------------------

func TestValidateClusterSettingsParameters_EmptyReturnsNil(t *testing.T) {
	keys, err := validateClusterSettingsParameters(nil)
	if err != nil {
		t.Fatalf("nil parameters should be valid: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected empty keys, got %v", keys)
	}
}

func TestValidateClusterSettingsParameters_ReturnsSortedKeys(t *testing.T) {
	params := map[string]string{
		"work_mem":        "8MB",
		"max_connections": "100",
		"shared_buffers":  "256MB",
	}
	keys, err := validateClusterSettingsParameters(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"max_connections", "shared_buffers", "work_mem"}
	if len(keys) != len(want) {
		t.Fatalf("expected %v, got %v", want, keys)
	}
	for i, k := range want {
		if keys[i] != k {
			t.Fatalf("expected sorted key[%d]=%q, got %q", i, k, keys[i])
		}
	}
}

// ---------------------------------------------------------------------------
// installCommands
// ---------------------------------------------------------------------------

func makeNode(pgInstalled bool) *models.Node {
	return &models.Node{
		ID:                "node-1",
		AgentID:           "agent-1",
		PostgresInstalled: pgInstalled,
	}
}

func TestInstallCommands_NativeUnresolved_QueuesPreflight(t *testing.T) {
	cmds := installCommands(makeNode(false), "16", models.ServiceLocationNative, false, "cluster-1")
	if len(cmds) != 1 || cmds[0].action != "pg_preflight" {
		t.Fatalf("expected [pg_preflight], got %+v", cmds)
	}
}

func TestInstallCommands_NativeUnresolved_AlreadyInstalled_StillPreflight(t *testing.T) {
	// Even if postgres is installed, native without resolved conflict → preflight.
	cmds := installCommands(makeNode(true), "16", models.ServiceLocationNative, false, "cluster-1")
	if len(cmds) != 1 || cmds[0].action != "pg_preflight" {
		t.Fatalf("expected [pg_preflight], got %+v", cmds)
	}
}

func TestInstallCommands_Docker_QueuesDockerInstall(t *testing.T) {
	cmds := installCommands(makeNode(false), "16", models.ServiceLocationDocker, false, "cluster-1")
	if len(cmds) != 1 || cmds[0].action != "pg_install_docker" {
		t.Fatalf("expected [pg_install_docker], got %+v", cmds)
	}
	// Payload should be JSON with cluster_id and version.
	var payload map[string]string
	if err := json.Unmarshal([]byte(cmds[0].payload), &payload); err != nil {
		t.Fatalf("expected JSON payload, got %q: %v", cmds[0].payload, err)
	}
	if payload["cluster_id"] != "cluster-1" {
		t.Fatalf("expected cluster_id 'cluster-1', got %q", payload["cluster_id"])
	}
	if payload["version"] != "16" {
		t.Fatalf("expected version '16', got %q", payload["version"])
	}
}

func TestInstallCommands_NativeResolved_NotInstalled_QueuesInstall(t *testing.T) {
	cmds := installCommands(makeNode(false), "16", models.ServiceLocationNative, true, "cluster-1")
	if len(cmds) != 1 || cmds[0].action != "pg_install_native" {
		t.Fatalf("expected [pg_install_native], got %+v", cmds)
	}
	if cmds[0].payload != "16" {
		t.Fatalf("expected version payload '16', got %q", cmds[0].payload)
	}
}

func TestInstallCommands_NativeResolved_AlreadyInstalled_ReturnsEmpty(t *testing.T) {
	cmds := installCommands(makeNode(true), "16", models.ServiceLocationNative, true, "cluster-1")
	if len(cmds) != 0 {
		t.Fatalf("expected no install commands when pg already installed, got %+v", cmds)
	}
}
