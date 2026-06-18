package server

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
)

func TestGetAgentInstallCommandScriptUrl(t *testing.T) {
	logger := slog.Default()
	database, err := db.New(db.Config{Driver: "sqlite", DSN: ":memory:"}, logger)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	userRepo := db.NewUserRepository(database.Conn(), logger)
	agentTokenRepo := db.NewAgentTokenRepository(database.Conn(), logger)
	apiKeyRepo := db.NewAPIKeyRepository(database.Conn(), logger)
	jwtManager := NewJWTManager("test-secret", 24*time.Hour, 7*24*time.Hour)

	pwdHash, err := crypto.HashPassword("password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &models.User{
		ID:           id.New(),
		Email:        "admin@example.com",
		PasswordHash: pwdHash,
		DisplayName:  "Admin",
		Role:         models.RoleAdmin,
	}
	if err := userRepo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	cfg := &Config{Server: ServerConfig{AdvertiseAddr: "skylex.test:9090"}}
	svc := NewAuthService(cfg, userRepo, apiKeyRepo, agentTokenRepo, jwtManager, logger)

	ctx := context.WithValue(context.Background(), ctxKeyUserID, user.ID)
	ctx = context.WithValue(ctx, ctxKeyUserRole, models.RoleAdmin)

	resp, err := svc.GetAgentInstallCommand(ctx, nil)
	if err != nil {
		t.Fatalf("get install command: %v", err)
	}

	if resp.ServerAddr != cfg.Server.AdvertiseAddr {
		t.Fatalf("expected server_addr %q, got %q", cfg.Server.AdvertiseAddr, resp.ServerAddr)
	}
	if resp.Token == "" || !strings.HasPrefix(resp.Token, "sklx_at_") {
		t.Fatalf("expected agent token, got %q", resp.Token)
	}
	want := "https://github.com/zhinea/skylex/releases/download/"
	if !strings.HasPrefix(resp.ScriptUrl, want) {
		t.Fatalf("expected script_url to start with %q, got %q", want, resp.ScriptUrl)
	}
}
