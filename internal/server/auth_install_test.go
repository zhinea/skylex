package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
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

func TestGetAgentInstallCommandDevModeUsesLocalScript(t *testing.T) {
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

	cfg := &Config{Server: ServerConfig{DevMode: true, HTTPPort: 18080, AdvertiseAddr: "localhost:9090"}}
	svc := NewAuthService(cfg, userRepo, apiKeyRepo, agentTokenRepo, jwtManager, logger)

	resp, err := svc.GetAgentInstallCommand(context.Background(), nil)
	if err != nil {
		t.Fatalf("get install command: %v", err)
	}

	want := "http://localhost:18080/install-agent.sh"
	if resp.ScriptUrl != want {
		t.Fatalf("expected script_url %q, got %q", want, resp.ScriptUrl)
	}
	if resp.AgentDownloadUrl != "http://localhost:18080/skylex-agent" {
		t.Fatalf("expected local agent download URL, got %q", resp.AgentDownloadUrl)
	}
}

func TestLoadConfigReadsNestedSkyLexEnvVars(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(`database:
  driver: sqlite
  dsn: ":memory:"
etcd:
  endpoints:
    - "localhost:2379"
  dial_timeout: 5s
auth:
  jwt_secret: "file-secret"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("SKYLEX_SERVER_DEV_MODE", "true")
	t.Setenv("SKYLEX_SERVER_HTTP_PORT", "18080")
	t.Setenv("SKYLEX_SERVER_ADVERTISE_ADDR", "localhost:19090")
	t.Setenv("SKYLEX_AUTH_JWT_SECRET", "env-secret")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !cfg.Server.DevMode {
		t.Fatal("expected dev mode from SKYLEX_SERVER_DEV_MODE")
	}
	if cfg.Server.HTTPPort != 18080 {
		t.Fatalf("expected http port 18080, got %d", cfg.Server.HTTPPort)
	}
	if cfg.Server.AdvertiseAddr != "localhost:19090" {
		t.Fatalf("expected advertise addr from env, got %q", cfg.Server.AdvertiseAddr)
	}
}
