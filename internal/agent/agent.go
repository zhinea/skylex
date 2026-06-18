package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
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
		},
	})
	if err != nil {
		return fmt.Errorf("register agent rpc: %w", err)
	}

	a.agentID = resp.GetAgentId()
	a.log.Info("agent registered", "agent_id", a.agentID,
		"pg_installed", pgInstalled, "pg_bin_version", pgBinVersion)
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
