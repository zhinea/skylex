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

	logs, err := commandLogs.ListByCommandID(context.Background(), cmd.ID, 10, 0)
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
