package server

import (
	"context"
	"io"
	"log/slog"
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
