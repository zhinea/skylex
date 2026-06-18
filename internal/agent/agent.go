package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/agent/installer"
	"github.com/zhinea/skylex/internal/backup"
	"github.com/zhinea/skylex/internal/postgres"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Agent struct {
	cfg        Config
	log        *slog.Logger
	agentID    string
	nodeID     string
	client     skylexv1.AgentServiceClient
	conn       *grpc.ClientConn
	pg         *postgres.Instance
	pgBackRest *backup.PgBackRest
	native     installer.NativeInstaller
	docker     installer.DockerInstaller
}

func New(cfg Config) (*Agent, error) {
	log := NewLogger(cfg.LogLevel, cfg.LogFormat)

	if cfg.Hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("get hostname: %w", err)
		}
		cfg.Hostname = hostname
	}

	pg := postgres.New(
		cfg.PGDataDir,
		cfg.PGBinDir,
		cfg.PGVersion,
		cfg.Port,
		cfg.PGSuperuser,
		cfg.PGReplUser,
		cfg.PGReplPass,
		log,
	)

	pgBackRest := backup.NewPgBackRest(cfg.PGBackRestPath, log)

	return &Agent{
		cfg:        cfg,
		log:        log,
		pg:         pg,
		pgBackRest: pgBackRest,
		native:     installer.NativeInstaller{},
		docker:     installer.DockerInstaller{},
	}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	a.log.Info("starting skylex agent",
		"version", Version,
		"hostname", a.cfg.Hostname,
		"server_addr", a.cfg.ServerAddr,
	)

	conn, err := grpc.NewClient(a.cfg.ServerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}
	defer conn.Close()

	a.conn = conn
	a.client = skylexv1.NewAgentServiceClient(conn)

	if err := a.register(ctx); err != nil {
		return fmt.Errorf("register agent: %w", err)
	}

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.heartbeatLoop(ctx)
	})

	g.Go(func() error {
		return a.commandLoop(ctx)
	})

	<-ctx.Done()
	a.log.Info("shutting down skylex agent")

	if a.pg.IsRunning() {
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelShutdown()
		if err := a.pg.Stop(shutdownCtx); err != nil {
			a.log.Error("failed to stop postgresql", "error", err)
		}
	}

	return g.Wait()
}

func (a *Agent) register(ctx context.Context) error {
	pgInstalled, pgBinVersion := postgres.DetectInstallation(ctx)
	dockerAvailable := detectDockerAvailable(ctx)

	resp, err := a.client.RegisterAgent(ctx, &skylexv1.RegisterAgentRequest{
		AgentToken:   a.cfg.AgentToken,
		Hostname:     a.cfg.Hostname,
		Address:      a.cfg.Address,
		Port:         int32(a.cfg.Port),
		AgentVersion: "0.1.0",
		Labels:       a.cfg.Labels,
		Capabilities: &skylexv1.NodeCapabilities{
			PostgresAvailable: pgInstalled,
			PostgresVersion:   pgBinVersion,
			PostgresBinDir:    a.cfg.PGBinDir,
			DataDir:           a.cfg.PGDataDir,
			DockerAvailable:   dockerAvailable,
		},
	})
	if err != nil {
		return fmt.Errorf("register agent rpc: %w", err)
	}

	a.agentID = resp.GetAgentId()
	a.log.Info("agent registered", "agent_id", a.agentID,
		"pg_installed", pgInstalled, "pg_bin_version", pgBinVersion,
		"docker_available", dockerAvailable)
	return nil
}

func (a *Agent) heartbeatLoop(ctx context.Context) error {
	ticker := time.NewTicker(a.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := a.sendHeartbeat(ctx); err != nil {
				a.log.Error("heartbeat failed", "error", err)
			}
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) error {
	postgresRunning := a.pg.IsRunning()
	pgInstalled, pgBinVersion := postgres.DetectInstallation(ctx)
	pgDataInitialized := a.pg.IsDataDirInitialized()

	_, err := a.client.Heartbeat(ctx, &skylexv1.HeartbeatRequest{
		AgentId: a.agentID,
		NodeId:  a.nodeID,
	})
	if err != nil {
		return fmt.Errorf("heartbeat rpc: %w", err)
	}

	report := &skylexv1.NodeStatusReport{
		NodeId:                  a.nodeID,
		PostgresRunning:         postgresRunning,
		PostgresInstalled:       pgInstalled,
		PostgresBinVersion:      pgBinVersion,
		PostgresDataInitialized: pgDataInitialized,
		NodeStatusDetail:        computeAgentStatusDetail(pgInstalled, pgDataInitialized, postgresRunning),
	}

	if postgresRunning {
		pgVersion, _ := a.pg.GetVersion(ctx)
		lag, _ := a.pg.GetReplicationLag(ctx)
		report.PostgresVersion = pgVersion
		report.ReplicationLagBytes = lag
		report.ReplicationLagSeconds = lag
	} else if a.pg.UsesDocker() {
		report.PostgresInstalled = detectDockerAvailable(ctx)
		report.PostgresBinVersion = a.cfg.PGVersion
	}

	_, err = a.client.ReportStatus(ctx, &skylexv1.ReportStatusRequest{
		AgentId:      a.agentID,
		NodeStatuses: []*skylexv1.NodeStatusReport{report},
	})
	if err != nil {
		a.log.Warn("report status failed", "error", err)
	}

	return nil
}

func (a *Agent) commandLoop(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := a.fetchCommands(ctx); err != nil {
				a.log.Error("fetch commands failed", "error", err)
			}
		}
	}
}

func (a *Agent) fetchCommands(ctx context.Context) error {
	resp, err := a.client.FetchCommand(ctx, &skylexv1.FetchCommandRequest{
		AgentId: a.agentID,
	})
	if err != nil {
		return fmt.Errorf("fetch command rpc: %w", err)
	}

	for _, cmd := range resp.GetCommands() {
		a.log.Info("executing command", "command_id", cmd.GetId(), "action", cmd.GetAction())
		logger := newCommandLogger(a.agentID, cmd.GetId(), a.client)
		logger.Info(fmt.Sprintf("executing command: %s", cmd.GetAction()))
		cmdCtx := postgres.WithLogSink(ctx, logger)
		success, output, errMsg := a.executeCommand(cmdCtx, cmd, logger)
		logger.Info(fmt.Sprintf("command finished: success=%v", success))
		logger.Close()
		if reportErr := a.reportCommandResult(ctx, cmd.GetId(), success, output, errMsg); reportErr != nil {
			a.log.Error("report command result failed", "error", reportErr)
		}
	}

	return nil
}

func (a *Agent) executeCommand(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	switch cmd.GetAction() {
	case "pg_preflight":
		pgInstalled, pgBinVersion := postgres.DetectInstallation(ctx)
		dockerAvailable := detectDockerAvailable(ctx)
		return true, fmt.Sprintf("preflight complete: postgres_installed=%v postgres_version=%q docker_available=%v", pgInstalled, pgBinVersion, dockerAvailable), ""

	case "pg_install_native":
		if installed, version := postgres.DetectInstallation(ctx); installed {
			binDir := installer.DetectNativeBinDir(ctx, a.cfg.PGBinDir)
			a.pg.UseNative(binDir, installer.DetectNativeVersion(ctx, version))
			return true, fmt.Sprintf("PostgreSQL already installed: %s", version), ""
		}
		if err := a.native.Install(ctx, a.installConfig(), logger); err != nil {
			return false, "", a.enrichError("pg_install_native", err)
		}
		binDir := installer.DetectNativeBinDir(ctx, a.cfg.PGBinDir)
		version := installer.DetectNativeVersion(ctx, a.cfg.PGVersion)
		a.pg.UseNative(binDir, version)
		return true, fmt.Sprintf("PostgreSQL %s installed natively", version), ""

	case "pg_install_docker":
		if err := a.docker.Install(ctx, a.installConfig(), logger); err != nil {
			return false, "", a.enrichError("pg_install_docker", err)
		}
		a.pg.UseDocker("postgres:"+a.cfg.PGVersion, installer.DockerContainerName())
		return true, fmt.Sprintf("PostgreSQL Docker container %q installed", installer.DockerContainerName()), ""

	case "pg_purge_native":
		if err := a.native.Purge(ctx, a.installConfig(), logger); err != nil {
			return false, "", a.enrichError("pg_purge_native", err)
		}
		return true, "Native PostgreSQL packages removed", ""

	case "pg_init":
		if err := a.pg.InitDB(ctx); err != nil {
			return false, "", a.enrichError("pg_init", err)
		}
		return true, "PostgreSQL data directory initialized successfully", ""

	case "pg_start":
		if err := a.pg.Start(ctx); err != nil {
			return false, "", a.enrichError("pg_start", err)
		}
		return true, fmt.Sprintf("PostgreSQL started on port %d", a.cfg.Port), ""

	case "pg_stop":
		if err := a.pg.Stop(ctx); err != nil {
			return false, "", a.enrichError("pg_stop", err)
		}
		return true, "PostgreSQL stopped gracefully", ""

	case "pg_basebackup":
		primaryHost := cmd.GetPayload()
		if err := a.pg.BaseBackup(ctx, primaryHost, a.cfg.Port, a.cfg.PGReplUser, a.cfg.PGReplPass); err != nil {
			return false, "", a.enrichError("pg_basebackup", err)
		}
		return true, fmt.Sprintf("Base backup completed from primary at %s:%d", primaryHost, a.cfg.Port), ""

	case "pg_promote":
		if err := a.pg.Promote(ctx); err != nil {
			return false, "", a.enrichError("pg_promote", err)
		}
		return true, "Promoted to primary — node is now writable", ""

	case "pg_rewind":
		parts := strings.SplitN(cmd.GetPayload(), ":", 2)
		targetHost := parts[0]
		targetPort := a.cfg.Port
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &targetPort)
		}
		if err := a.pg.Rewind(ctx, targetHost, targetPort, a.cfg.PGReplUser, a.cfg.PGReplPass); err != nil {
			return false, "", a.enrichError("pg_rewind", err)
		}
		return true, fmt.Sprintf("pg_rewind completed — repointed to new primary at %s:%d", targetHost, targetPort), ""

	case "repoint_replica":
		parts := strings.SplitN(cmd.GetPayload(), ":", 2)
		primaryHost := parts[0]
		primaryPort := a.cfg.Port
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &primaryPort)
		}
		logger.Info(fmt.Sprintf("updating standby signal to %s:%d", primaryHost, primaryPort))
		if err := a.pg.UpdateStandbySignal(primaryHost, primaryPort); err != nil {
			return false, "", a.enrichError("repoint_replica", err)
		}
		return true, fmt.Sprintf("Replica repointed to follow primary at %s:%d", primaryHost, primaryPort), ""

	case "pg_create_repl_user":
		if err := a.pg.CreateReplicationUser(ctx); err != nil {
			return false, "", a.enrichError("pg_create_repl_user", err)
		}
		return true, "Replication user created successfully", ""

	case "pg_create_repl_slot":
		if err := a.pg.CreateReplicationSlot(ctx, "skylex_replica"); err != nil {
			return false, "", a.enrichError("pg_create_repl_slot", err)
		}
		return true, "Replication slot 'skylex_replica' created", ""

	case "pg_apply_settings":
		settings := make(map[string]string)
		if err := json.Unmarshal([]byte(cmd.GetPayload()), &settings); err != nil {
			return false, "", fmt.Sprintf("invalid settings payload: %v", err)
		}
		method, err := a.pg.ApplySettings(ctx, settings)
		if err != nil {
			return false, "", a.enrichError("pg_apply_settings", err)
		}
		return true, fmt.Sprintf("Settings applied via %s — a reload was issued", method), ""

	case "pgbackrest_backup":
		stanza := cmd.GetPayload()
		if _, err := a.pgBackRest.Backup(ctx, stanza, "full", "", a.cfg.PGDataDir); err != nil {
			return false, "", a.enrichError("pgbackrest_backup", err)
		}
		return true, "pgBackRest backup completed successfully", ""

	case "pgbackrest_restore":
		stanza := cmd.GetPayload()
		if _, err := a.pgBackRest.Restore(ctx, stanza, "", "", "", a.cfg.PGDataDir); err != nil {
			return false, "", a.enrichError("pgbackrest_restore", err)
		}
		return true, "pgBackRest restore completed successfully", ""

	case "pgbackrest_stanza_create":
		stanza := cmd.GetPayload()
		if err := a.pgBackRest.StanzaCreate(ctx, stanza, "", a.cfg.PGDataDir); err != nil {
			return false, "", a.enrichError("pgbackrest_stanza_create", err)
		}
		return true, "pgBackRest stanza created successfully", ""

	case "pgbackrest_stanza_check":
		stanza := cmd.GetPayload()
		if err := a.pgBackRest.StanzaCheck(ctx, stanza, "", a.cfg.PGDataDir); err != nil {
			return false, "", a.enrichError("pgbackrest_stanza_check", err)
		}
		return true, "pgBackRest stanza check passed", ""

	default:
		return false, "", fmt.Sprintf("unknown command action: %s", cmd.GetAction())
	}
}

func (a *Agent) installConfig() installer.InstallConfig {
	return installer.InstallConfig{
		Version:   a.cfg.PGVersion,
		DataDir:   a.cfg.PGDataDir,
		BinDir:    a.cfg.PGBinDir,
		Port:      a.cfg.Port,
		Superuser: a.cfg.PGSuperuser,
		Password:  a.cfg.PGReplPass,
	}
}

// enrichError wraps an error with actionable hints based on the command type
// and the error message content.
func (a *Agent) enrichError(action string, err error) string {
	msg := err.Error()
	lower := strings.ToLower(msg)

	switch action {
	case "pg_init":
		if strings.Contains(lower, "permission denied") {
			return fmt.Sprintf("initdb failed: permission denied — check that the data directory %s is owned by the current user", msg)
		}
		if strings.Contains(lower, "exists") && strings.Contains(lower, "not empty") {
			return fmt.Sprintf("initdb failed: data directory is not empty — remove or clear %s before initializing", msg)
		}
		return fmt.Sprintf("initdb failed: %s — check that PostgreSQL binaries are in PATH and disk space is available", msg)

	case "pg_start":
		if strings.Contains(lower, "permission denied") {
			return fmt.Sprintf("pg_start failed: permission denied — check that the data directory %s is owned by the postgres user", msg)
		}
		if strings.Contains(lower, "port") || strings.Contains(lower, "already in use") || strings.Contains(lower, "address already in use") {
			return fmt.Sprintf("pg_start failed: port is already in use — stop the existing PostgreSQL process on port %d or change the port", a.cfg.Port)
		}
		if strings.Contains(lower, "no such file") || strings.Contains(lower, "not found") {
			return fmt.Sprintf("pg_start failed: PostgreSQL binary not found — ensure PostgreSQL is installed and PGBinDir (%s) is correct", a.cfg.PGBinDir)
		}
		return fmt.Sprintf("pg_start failed: %s — check pg_log for details", msg)

	case "pg_stop":
		return fmt.Sprintf("pg_stop failed: %s — the process may need to be terminated manually", msg)

	case "pg_basebackup":
		if strings.Contains(lower, "connection refused") || strings.Contains(lower, "no route to host") {
			return fmt.Sprintf("pg_basebackup failed: cannot connect to primary — ensure the primary is running and reachable on the network")
		}
		if strings.Contains(lower, "password authentication failed") {
			return fmt.Sprintf("pg_basebackup failed: authentication failed — verify the replication user credentials in pg_hba.conf on the primary")
		}
		return fmt.Sprintf("pg_basebackup failed: %s — verify the primary is running and the replication user is configured", msg)

	case "pg_apply_settings":
		return fmt.Sprintf("pg_apply_settings failed: %s — check the parameter values and ensure PostgreSQL can reload", msg)

	case "pg_install_native":
		if strings.Contains(lower, "permission denied") || strings.Contains(lower, "operation not permitted") {
			return fmt.Sprintf("native install failed: insufficient permissions — run the agent with package installation privileges: %s", msg)
		}
		if strings.Contains(lower, "no supported package manager") {
			return fmt.Sprintf("native install failed: unsupported operating system package manager: %s", msg)
		}
		return fmt.Sprintf("native install failed: %s", msg)

	case "pg_install_docker":
		if strings.Contains(lower, "docker binary not found") || strings.Contains(lower, "permission denied") {
			return fmt.Sprintf("docker install failed: Docker is unavailable or the agent cannot access the Docker daemon: %s", msg)
		}
		return fmt.Sprintf("docker install failed: %s", msg)

	case "pg_purge_native":
		return fmt.Sprintf("native purge failed: %s", msg)

	default:
		return fmt.Sprintf("%s failed: %s", action, msg)
	}
}

func (a *Agent) reportCommandResult(ctx context.Context, commandID string, success bool, output, errMsg string) error {
	_, err := a.client.ReportCommandResult(ctx, &skylexv1.ReportCommandResultRequest{
		AgentId:   a.agentID,
		CommandId: commandID,
		Success:   success,
		Output:    RedactSecrets(output),
		Error:     RedactSecrets(errMsg),
	})
	return err
}

// computeAgentStatusDetail derives a human-readable status detail from the
// agent's local PostgreSQL state.
func computeAgentStatusDetail(pgInstalled, pgDataInitialized, pgRunning bool) string {
	if !pgInstalled {
		return "waiting_for_postgres"
	}
	if !pgDataInitialized {
		return "initializing_data_directory"
	}
	if !pgRunning {
		return "stopped"
	}
	return "running"
}

// detectDockerAvailable returns true when the `docker version` command succeeds,
// indicating Docker Engine is installed and the daemon is reachable.
func detectDockerAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	err := cmd.Run()
	return err == nil
}
