package server

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
)

func mustToken(t *testing.T) string {
	t.Helper()
	tok, err := crypto.GenerateToken(16)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func now() time.Time { return time.Now().UTC() }

func newTestDeps(t *testing.T) (*db.DB, *slog.Logger) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	database, err := db.New(db.Config{Driver: "sqlite", DSN: ":memory:"}, log)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database, log
}

func TestAgentService_RegisterAgent_RequiresToken(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, db.NewClusterRepository(conn, log), db.NewNodeRepository(conn, log), db.NewAgentCommandRepository(conn, log), db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)

	_, err := svc.RegisterAgent(context.Background(), &skylexv1.RegisterAgentRequest{
		AgentToken: "",
		Hostname:   "test-node",
		Address:    "10.0.0.1",
		Port:       5432,
	})
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got: %v", err)
	}
}

func TestAgentService_RegisterAgent_InvalidToken(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, db.NewClusterRepository(conn, log), db.NewNodeRepository(conn, log), db.NewAgentCommandRepository(conn, log), db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)

	_, err := svc.RegisterAgent(context.Background(), &skylexv1.RegisterAgentRequest{
		AgentToken: "sklx_at_invalidtoken",
		Hostname:   "test-node",
		Address:    "10.0.0.1",
		Port:       5432,
	})
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got: %v", err)
	}
}

func TestAgentService_RegisterAgent_ValidToken(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()

	agentTokens := db.NewAgentTokenRepository(conn, log)
	raw := "sklx_at_" + mustToken(t)
	token := &models.AgentToken{
		ID:        id.New(),
		Name:      "test",
		TokenHash: crypto.HashToken(raw),
		Role:      models.RoleOperator,
		CreatedAt: now(),
	}
	if err := agentTokens.Create(token); err != nil {
		t.Fatalf("create token: %v", err)
	}

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, db.NewClusterRepository(conn, log), db.NewNodeRepository(conn, log), db.NewAgentCommandRepository(conn, log), db.NewCommandLogRepository(conn, log), agentTokens, log)

	resp, err := svc.RegisterAgent(context.Background(), &skylexv1.RegisterAgentRequest{
		AgentToken: raw,
		Hostname:   "test-node",
		Address:    "10.0.0.1",
		Port:       5432,
	})
	if err != nil {
		t.Fatalf("register agent: %v", err)
	}
	if resp.GetAgentId() == "" {
		t.Fatal("expected agent id")
	}
}

func TestAgentService_RegisterAgent_DevTokenFallback(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()

	svc := NewAgentService(&Config{Agent: AgentConfig{AgentToken: "dev-token"}}, db.NewClusterRepository(conn, log), db.NewNodeRepository(conn, log), db.NewAgentCommandRepository(conn, log), db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)

	resp, err := svc.RegisterAgent(context.Background(), &skylexv1.RegisterAgentRequest{
		AgentToken: "dev-token",
		Hostname:   "test-node",
		Address:    "10.0.0.1",
		Port:       5432,
	})
	if err != nil {
		t.Fatalf("register agent: %v", err)
	}
	if resp.GetAgentId() == "" {
		t.Fatal("expected agent id")
	}
}

func TestAgentService_HeartbeatUpdatesConnectionState(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	nodes := db.NewNodeRepository(conn, log)

	node, err := nodes.Create(context.Background(), "", "test-node", "10.0.0.1", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	if err := nodes.UpdateStatus(context.Background(), node.ID, models.NodeStatusOffline); err != nil {
		t.Fatalf("mark offline: %v", err)
	}

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, db.NewClusterRepository(conn, log), nodes, db.NewAgentCommandRepository(conn, log), db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)

	_, err = svc.Heartbeat(context.Background(), &skylexv1.HeartbeatRequest{
		AgentId:           "agent-1",
		ObservedLatencyMs: 25,
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	updated, err := nodes.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if updated.Status != models.NodeStatusOnline {
		t.Fatalf("expected online status, got %q", updated.Status)
	}
	if updated.AgentLatencyMS <= 0 {
		t.Fatalf("expected latency to be recorded, got %d", updated.AgentLatencyMS)
	}
	if time.Since(updated.LastSeen) > time.Second {
		t.Fatalf("expected recent last_seen, got %s", updated.LastSeen)
	}
}

func TestAgentService_HeartbeatDoesNotOverwriteDrainedNode(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	nodes := db.NewNodeRepository(conn, log)

	node, err := nodes.Create(context.Background(), "", "test-node", "10.0.0.1", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	if err := nodes.UpdateStatus(context.Background(), node.ID, models.NodeStatusDrained); err != nil {
		t.Fatalf("mark drained: %v", err)
	}

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, db.NewClusterRepository(conn, log), nodes, db.NewAgentCommandRepository(conn, log), db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)

	_, err = svc.Heartbeat(context.Background(), &skylexv1.HeartbeatRequest{
		AgentId:           "agent-1",
		ObservedLatencyMs: 25,
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	updated, err := nodes.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if updated.Status != models.NodeStatusDrained {
		t.Fatalf("expected drained status, got %q", updated.Status)
	}
}

func TestAgentService_ReportStatusStoresDockerPostgresState(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	nodes := db.NewNodeRepository(conn, log)

	node, err := nodes.Create(context.Background(), "", "docker-node", "10.0.0.1", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, db.NewClusterRepository(conn, log), nodes, db.NewAgentCommandRepository(conn, log), db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)

	_, err = svc.ReportStatus(context.Background(), &skylexv1.ReportStatusRequest{
		AgentId: "agent-1",
		NodeStatuses: []*skylexv1.NodeStatusReport{{
			PostgresRunning:         true,
			PostgresInstalled:       true,
			PostgresBinVersion:      "16",
			PostgresVersion:         "PostgreSQL 16.1",
			PostgresDataInitialized: true,
			NodeStatusDetail:        "running",
			InstallationState:       skylexv1.InstallationState_INSTALLATION_STATE_INSTALLED,
		}},
	})
	if err != nil {
		t.Fatalf("report status: %v", err)
	}

	updated, err := nodes.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if !updated.PostgresInstalled || !updated.PostgresDataInitialized {
		t.Fatalf("expected installed initialized postgres state, got installed=%v initialized=%v", updated.PostgresInstalled, updated.PostgresDataInitialized)
	}
	if updated.Status != models.NodeStatusOnline {
		t.Fatalf("expected online status, got %q", updated.Status)
	}
	if updated.StatusDetail != "running" {
		t.Fatalf("expected running status detail, got %q", updated.StatusDetail)
	}
	if updated.InstallationState != models.InstallationStateInstalled {
		t.Fatalf("expected installed state, got %q", updated.InstallationState)
	}
}

func TestAgentService_ReportCommandResultMarksDockerInstallVisible(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	clusters := db.NewClusterRepository(conn, log)
	nodes := db.NewNodeRepository(conn, log)
	commands := db.NewAgentCommandRepository(conn, log)

	cluster, err := clusters.Create(context.Background(), "docker-cluster", "", "/var/lib/postgresql/data", models.EnginePostgreSQL, "16", models.ReplicationAsync, 0, false, nil)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node, err := nodes.Create(context.Background(), cluster.ID, "docker-node", "10.0.0.1", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	if err := nodes.UpdateInstallationState(context.Background(), node.ID, models.InstallationStateInstalling, ""); err != nil {
		t.Fatalf("update installation state: %v", err)
	}
	cmd, err := commands.Create(context.Background(), "agent-1", node.ID, "pg_install_docker", "")
	if err != nil {
		t.Fatalf("create command: %v", err)
	}

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, clusters, nodes, commands, db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)
	_, err = svc.ReportCommandResult(context.Background(), &skylexv1.ReportCommandResultRequest{
		AgentId:   "agent-1",
		CommandId: cmd.ID,
		Success:   true,
		Output:    "PostgreSQL Docker container installed",
	})
	if err != nil {
		t.Fatalf("report command result: %v", err)
	}

	updated, err := nodes.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if !updated.PostgresInstalled {
		t.Fatal("expected postgres_installed to be true after successful docker install command")
	}
	if updated.PostgresVersion != "16" {
		t.Fatalf("expected postgres version 16, got %q", updated.PostgresVersion)
	}
	if updated.InstallationState != models.InstallationStateInstalled {
		t.Fatalf("expected installed state, got %q", updated.InstallationState)
	}
}

func TestAgentService_ReportCommandResultMarksPostgresRunningVisible(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	clusters := db.NewClusterRepository(conn, log)
	nodes := db.NewNodeRepository(conn, log)
	commands := db.NewAgentCommandRepository(conn, log)

	cluster, err := clusters.Create(context.Background(), "docker-cluster", "", "/var/lib/postgresql/data", models.EnginePostgreSQL, "16", models.ReplicationAsync, 0, false, nil)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node, err := nodes.Create(context.Background(), cluster.ID, "docker-node", "10.0.0.1", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	if err := nodes.UpdateInstallationState(context.Background(), node.ID, models.InstallationStateInstalled, ""); err != nil {
		t.Fatalf("update installation state: %v", err)
	}
	cmd, err := commands.Create(context.Background(), "agent-1", node.ID, "pg_start", "")
	if err != nil {
		t.Fatalf("create command: %v", err)
	}

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, clusters, nodes, commands, db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)
	_, err = svc.ReportCommandResult(context.Background(), &skylexv1.ReportCommandResultRequest{
		AgentId:   "agent-1",
		CommandId: cmd.ID,
		Success:   true,
		Output:    "PostgreSQL started on port 5432",
	})
	if err != nil {
		t.Fatalf("report command result: %v", err)
	}

	updated, err := nodes.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if !updated.PostgresInstalled || !updated.PostgresDataInitialized {
		t.Fatalf("expected installed initialized state, got installed=%v initialized=%v", updated.PostgresInstalled, updated.PostgresDataInitialized)
	}
	if updated.Status != models.NodeStatusOnline {
		t.Fatalf("expected online status, got %q", updated.Status)
	}
	if updated.StatusDetail != "healthy" {
		t.Fatalf("expected healthy status detail, got %q", updated.StatusDetail)
	}
}

func TestAgentService_ReportCommandResultAdoptInitializedNativeMarksDataInitialized(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	clusters := db.NewClusterRepository(conn, log)
	nodes := db.NewNodeRepository(conn, log)
	commands := db.NewAgentCommandRepository(conn, log)

	cluster, err := clusters.Create(context.Background(), "native-cluster", "", "/var/lib/postgresql/data", models.EnginePostgreSQL, "16", models.ReplicationAsync, 0, false, nil)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node, err := nodes.Create(context.Background(), cluster.ID, "native-node", "10.0.0.1", 5432, models.NodeRolePrimary, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	conflictDetails := "existing PostgreSQL/data detected: version=PostgreSQL 16.14 data_dir=/var/lib/postgresql/data data_present=true data_initialized=true"
	if err := nodes.UpdateInstallationState(context.Background(), node.ID, models.InstallationStateConflict, conflictDetails); err != nil {
		t.Fatalf("update installation state: %v", err)
	}
	cmd, err := commands.Create(context.Background(), "agent-1", node.ID, "pg_adopt_native", "")
	if err != nil {
		t.Fatalf("create command: %v", err)
	}

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, clusters, nodes, commands, db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)
	_, err = svc.ReportCommandResult(context.Background(), &skylexv1.ReportCommandResultRequest{
		AgentId:   "agent-1",
		CommandId: cmd.ID,
		Success:   true,
		Output:    "Adopted existing PostgreSQL installation: PostgreSQL 16.14",
	})
	if err != nil {
		t.Fatalf("report command result: %v", err)
	}

	updated, err := nodes.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if !updated.PostgresInstalled || !updated.PostgresDataInitialized {
		t.Fatalf("expected adopted initialized postgres state, got installed=%v initialized=%v", updated.PostgresInstalled, updated.PostgresDataInitialized)
	}
	if updated.InstallationState != models.InstallationStateAdopted {
		t.Fatalf("expected adopted state, got %q", updated.InstallationState)
	}
}

func TestAgentService_ReportStatusStoresMetricHistory(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	nodes := db.NewNodeRepository(conn, log)

	node, err := nodes.Create(context.Background(), "", "test-node", "10.0.0.1", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, db.NewClusterRepository(conn, log), nodes, db.NewAgentCommandRepository(conn, log), db.NewCommandLogRepository(conn, log), db.NewAgentTokenRepository(conn, log), log)

	for _, cpu := range []float64{11.5, 42.25} {
		_, err = svc.ReportStatus(context.Background(), &skylexv1.ReportStatusRequest{
			AgentId: "agent-1",
			NodeStatuses: []*skylexv1.NodeStatusReport{{
				SystemMetrics: &skylexv1.NodeSystemMetrics{
					Os:                 "linux",
					CpuCores:           4,
					CpuUsagePercent:    cpu,
					MemoryTotalBytes:   1024,
					MemoryUsedBytes:    512,
					MemoryUsagePercent: 50,
					DiskTotalBytes:     2048,
					DiskUsedBytes:      1024,
					DiskUsagePercent:   50,
				},
			}},
		})
		if err != nil {
			t.Fatalf("report status: %v", err)
		}
	}

	metrics, err := nodes.ListMetrics(context.Background(), node.ID, time.Time{}, 10)
	if err != nil {
		t.Fatalf("list metrics: %v", err)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected two metric samples, got %d", len(metrics))
	}

	updated, err := nodes.GetByID(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if updated.LatestMetrics == nil || updated.LatestMetrics.CPUUsagePercent != 42.25 {
		t.Fatalf("expected latest metric to be attached, got %#v", updated.LatestMetrics)
	}

	var count int
	if err := conn.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM nodes WHERE cpu_usage_percent = 42.25`).Scan(&count); err == nil {
		t.Fatalf("expected nodes metric column to be removed")
	}
}

func TestAgentService_ReportCommandLogStoresEntries(t *testing.T) {
	database, log := newTestDeps(t)
	conn := database.Conn()
	nodes := db.NewNodeRepository(conn, log)
	commands := db.NewAgentCommandRepository(conn, log)
	commandLogs := db.NewCommandLogRepository(conn, log)

	node, err := nodes.Create(context.Background(), "", "test-node", "10.0.0.1", 5432, models.NodeRoleReplica, "0.1.0", nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := nodes.UpdateAgentID(context.Background(), node.ID, "agent-1"); err != nil {
		t.Fatalf("update agent id: %v", err)
	}
	cmd, err := commands.Create(context.Background(), "agent-1", node.ID, "pg_start", "")
	if err != nil {
		t.Fatalf("create command: %v", err)
	}

	svc := NewAgentService(&Config{Agent: AgentConfig{}}, db.NewClusterRepository(conn, log), nodes, commands, commandLogs, db.NewAgentTokenRepository(conn, log), log)

	_, err = svc.ReportCommandLog(context.Background(), &skylexv1.ReportCommandLogRequest{
		AgentId: "agent-1",
		Entries: []*skylexv1.CommandLogEntry{{
			CommandId:   cmd.ID,
			Level:       "info",
			Message:     "started postgres",
			TimestampMs: time.Now().UTC().UnixMilli(),
		}},
	})
	if err != nil {
		t.Fatalf("report command log: %v", err)
	}

	logs, err := commandLogs.ListByCommandID(context.Background(), cmd.ID, 10, 0, db.LogFilter{})
	if err != nil {
		t.Fatalf("list command logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one command log, got %d", len(logs))
	}
	if !strings.Contains(logs[0].Message, "started postgres") {
		t.Fatalf("unexpected log message %q", logs[0].Message)
	}
}
