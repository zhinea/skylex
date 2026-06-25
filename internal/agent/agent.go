package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	logFile    *os.File
	agentID    string
	nodeID     string
	client     skylexv1.AgentServiceClient
	conn       *grpc.ClientConn
	pg         *postgres.Instance
	pgBackRest *backup.PgBackRest
	native     installer.NativeInstaller
	docker     installer.DockerInstaller
	shutdown   context.CancelFunc

	installMu          sync.RWMutex
	installationState  skylexv1.InstallationState
	conflictDetails    string
	heartbeatLatencyMS int64
}

func New(cfg Config) (*Agent, error) {
	if cfg.Hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("get hostname: %w", err)
		}
		cfg.Hostname = hostname
	}

	var logFile *os.File
	logOutputs := []io.Writer{os.Stderr}
	if logPath := strings.TrimSpace(cfg.LogFile); logPath != "" {
		cleanPath := filepath.Clean(logPath)
		if err := os.MkdirAll(filepath.Dir(cleanPath), 0750); err != nil {
			return nil, fmt.Errorf("create log directory: %w", err)
		}
		file, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		logFile = file
		logOutputs = append(logOutputs, file)
	}

	log := NewLogger(cfg.LogLevel, cfg.LogFormat, logOutputs...)

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
		cfg:               cfg,
		log:               log,
		logFile:           logFile,
		pg:                pg,
		pgBackRest:        pgBackRest,
		native:            installer.NativeInstaller{},
		docker:            installer.DockerInstaller{},
		installationState: skylexv1.InstallationState_INSTALLATION_STATE_PENDING_PREFLIGHT,
	}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	defer a.Close()

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
	a.shutdown = cancel

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

func (a *Agent) Close() error {
	if a.logFile == nil {
		return nil
	}
	err := a.logFile.Close()
	a.logFile = nil
	return err
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
	installationState, conflictDetails := a.installationReport()
	if a.pg.UsesDocker() {
		pgInstalled = detectDockerAvailable(ctx)
		if pgBinVersion == "" {
			pgBinVersion = a.cfg.PGVersion
		}
		if postgresRunning {
			pgDataInitialized = true
		}
	}

	start := time.Now()
	_, err := a.client.Heartbeat(ctx, &skylexv1.HeartbeatRequest{
		AgentId:           a.agentID,
		NodeId:            a.nodeID,
		ObservedLatencyMs: a.heartbeatLatencyMS,
	})
	if err != nil {
		return fmt.Errorf("heartbeat rpc: %w", err)
	}
	a.heartbeatLatencyMS = time.Since(start).Milliseconds()

	report := &skylexv1.NodeStatusReport{
		NodeId:                  a.nodeID,
		PostgresRunning:         postgresRunning,
		PostgresInstalled:       pgInstalled,
		PostgresBinVersion:      pgBinVersion,
		PostgresDataInitialized: pgDataInitialized,
		NodeStatusDetail:        computeAgentStatusDetail(pgInstalled, pgDataInitialized, postgresRunning),
		InstallationState:       installationState,
		ConflictDetails:         conflictDetails,
		SystemMetrics:           collectSystemMetrics(a.cfg.PGDataDir),
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

func collectSystemMetrics(diskPath string) *skylexv1.NodeSystemMetrics {
	if diskPath == "" {
		diskPath = "/"
	}
	if _, err := os.Stat(diskPath); err != nil {
		diskPath = "/"
	}

	metrics := &skylexv1.NodeSystemMetrics{
		Os:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		CpuCores:     int32(runtime.NumCPU()),
	}

	applyOSRelease(metrics)
	applyKernelVersion(metrics)
	applyLoadAverage(metrics)
	applyMemoryStats(metrics)
	applyDiskStats(metrics, diskPath)
	applyUptime(metrics)
	metrics.CpuUsagePercent = cpuUsageFromLoad(metrics.LoadAverage_1M, metrics.CpuCores)
	return metrics
}

func applyOSRelease(metrics *skylexv1.NodeSystemMetrics) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		values[key] = strings.Trim(value, "\"")
	}
	metrics.Platform = values["ID"]
	metrics.PlatformVersion = values["VERSION_ID"]
}

func applyKernelVersion(metrics *skylexv1.NodeSystemMetrics) {
	contents, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return
	}
	metrics.KernelVersion = strings.TrimSpace(string(contents))
}

func applyLoadAverage(metrics *skylexv1.NodeSystemMetrics) {
	contents, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return
	}
	fields := strings.Fields(string(contents))
	if len(fields) < 3 {
		return
	}
	metrics.LoadAverage_1M = parseScaledFloat(fields[0])
	metrics.LoadAverage_5M = parseScaledFloat(fields[1])
	metrics.LoadAverage_15M = parseScaledFloat(fields[2])
}

func applyMemoryStats(metrics *skylexv1.NodeSystemMetrics) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer file.Close()

	values := make(map[string]int64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		values[strings.TrimSuffix(fields[0], ":")] = value * 1024
	}

	metrics.MemoryTotalBytes = values["MemTotal"]
	metrics.MemoryAvailableBytes = values["MemAvailable"]
	if metrics.MemoryTotalBytes > 0 {
		metrics.MemoryUsedBytes = metrics.MemoryTotalBytes - metrics.MemoryAvailableBytes
		metrics.MemoryUsagePercent = percent(metrics.MemoryUsedBytes, metrics.MemoryTotalBytes)
	}
}

func applyDiskStats(metrics *skylexv1.NodeSystemMetrics, path string) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return
	}

	total := int64(stat.Blocks) * int64(stat.Bsize)
	available := int64(stat.Bavail) * int64(stat.Bsize)
	used := total - available
	metrics.DiskTotalBytes = total
	metrics.DiskAvailableBytes = available
	metrics.DiskUsedBytes = used
	metrics.DiskUsagePercent = percent(used, total)
}

func applyUptime(metrics *skylexv1.NodeSystemMetrics) {
	contents, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return
	}
	fields := strings.Fields(string(contents))
	if len(fields) == 0 {
		return
	}
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return
	}
	metrics.UptimeSeconds = int64(uptime)
}

func cpuUsageFromLoad(loadAverage1M int64, cores int32) float64 {
	if loadAverage1M <= 0 || cores <= 0 {
		return 0
	}
	return math.Min(100, (float64(loadAverage1M)/100/float64(cores))*100)
}

func parseScaledFloat(value string) int64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return int64(parsed * 100)
}

func percent(used, total int64) float64 {
	if total <= 0 || used <= 0 {
		return 0
	}
	return (float64(used) / float64(total)) * 100
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
		logger := newCommandLogger(a.agentID, cmd.GetId(), a.client, a.log.With("command_id", cmd.GetId(), "action", cmd.GetAction()))
		logger.Info(fmt.Sprintf("executing command: %s", cmd.GetAction()))
		cmdCtx := postgres.WithLogSink(ctx, logger)
		success, output, errMsg := a.executeCommand(cmdCtx, cmd, logger)
		if !success && errMsg != "" {
			a.log.Error("command failed", "command_id", cmd.GetId(), "action", cmd.GetAction(), "error", errMsg)
			logger.Error(errMsg)
		}
		logger.Info(fmt.Sprintf("command finished: success=%v", success))
		logger.Close()
		if reportErr := a.reportCommandResult(ctx, cmd.GetId(), success, output, errMsg); reportErr != nil {
			a.log.Error("report command result failed", "error", reportErr)
		}
		if cmd.GetAction() == "agent_deactivate" && success && a.shutdown != nil {
			a.shutdown()
		}
	}

	return nil
}

func (a *Agent) executeCommand(ctx context.Context, cmd *skylexv1.AgentCommand, logger *commandLogger) (bool, string, string) {
	switch cmd.GetAction() {
	case "pg_preflight":
		result, err := a.native.Preflight(ctx, a.installConfig(), logger)
		if err != nil {
			a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_FAILED, err.Error())
			return false, "", a.enrichError("pg_preflight", err)
		}
		if result.State == installer.PreflightPGExists {
			a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_CONFLICT, result.Details())
		} else {
			a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_NOTHING_FOUND, "")
		}
		return true, mustMarshalJSON(result), ""

	case "pg_install_native":
		a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_INSTALLING, "")
		if installed, version := postgres.DetectInstallation(ctx); installed {
			binDir := installer.DetectNativeBinDir(ctx, a.cfg.PGBinDir)
			a.pg.UseNative(binDir, installer.DetectNativeVersion(ctx, version))
			a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_INSTALLED, "")
			return true, fmt.Sprintf("PostgreSQL already installed: %s", version), ""
		}
		if err := a.native.Install(ctx, a.installConfig(), logger); err != nil {
			a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_FAILED, err.Error())
			return false, "", a.enrichError("pg_install_native", err)
		}
		binDir := installer.DetectNativeBinDir(ctx, a.cfg.PGBinDir)
		version := installer.DetectNativeVersion(ctx, a.cfg.PGVersion)
		a.pg.UseNative(binDir, version)
		a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_INSTALLED, "")
		return true, fmt.Sprintf("PostgreSQL %s installed natively", version), ""

	case "pg_install_docker":
		a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_INSTALLING, "")
		// Parse payload: JSON with cluster_id and version, or legacy plain version string.
		clusterID := ""
		version := a.cfg.PGVersion
		if payload := cmd.GetPayload(); payload != "" {
			if strings.HasPrefix(payload, "{") {
				var installPayload struct {
					ClusterID string `json:"cluster_id"`
					Version   string `json:"version"`
				}
				if err := json.Unmarshal([]byte(payload), &installPayload); err == nil {
					clusterID = installPayload.ClusterID
					if installPayload.Version != "" {
						version = installPayload.Version
					}
				} else {
					a.log.Warn("failed to parse pg_install_docker JSON payload, using legacy format", "error", err)
					version = payload
				}
			} else {
				version = payload
			}
		}

		installCfg := a.installConfig()
		installCfg.ClusterID = clusterID
		installCfg.Version = version
		if err := a.docker.Install(ctx, installCfg, logger); err != nil {
			if errors.Is(err, installer.ErrDockerNeedsRestart) {
				// The engine was installed and the user was added to the docker group,
				// but the running process still has the old group set. Surface a clear,
				// actionable message instead of a raw error.
				a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_FAILED, err.Error())
				return false, "", a.enrichError("pg_install_docker", err)
			}
			a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_FAILED, err.Error())
			return false, "", a.enrichError("pg_install_docker", err)
		}
		containerName := installer.DockerContainerName(clusterID)
		composeFile := installer.ComposeFilePath(clusterID)
		a.pg.UseDocker("postgres:"+version, containerName, composeFile, "postgres")
		a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_INSTALLED, "")
		return true, fmt.Sprintf("PostgreSQL Docker container %q installed", containerName), ""

	case "pg_purge_native":
		a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_INSTALLING, "")
		if err := a.native.Purge(ctx, a.installConfig(), logger); err != nil {
			a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_FAILED, err.Error())
			return false, "", a.enrichError("pg_purge_native", err)
		}
		return true, "Native PostgreSQL packages and configured data directory removed", ""

	case "pg_adopt_native":
		if err := a.applyPostgresAdminCredentials(cmd); err != nil {
			a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_FAILED, err.Error())
			return false, "", fmt.Sprintf("pg_adopt_native: %v", err)
		}
		if installed, version := postgres.DetectInstallation(ctx); installed {
			binDir := installer.DetectNativeBinDir(ctx, a.cfg.PGBinDir)
			a.pg.UseNative(binDir, installer.DetectNativeVersion(ctx, version))
			a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_ADOPTED, "")
			return true, fmt.Sprintf("Adopted existing PostgreSQL installation: %s", version), ""
		}
		a.setInstallationReport(skylexv1.InstallationState_INSTALLATION_STATE_FAILED, "adopt native failed: no PostgreSQL installation found")
		return false, "", "adopt native failed: no PostgreSQL installation found"

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

	case "agent_deactivate":
		if err := a.deactivate(ctx, logger); err != nil {
			return false, "", a.enrichError("agent_deactivate", err)
		}
		return true, "Skylex agent deactivated", ""

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
		if err := a.applyPostgresAdminCredentials(cmd); err != nil {
			return false, "", fmt.Sprintf("pg_create_repl_user: %v", err)
		}
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

	case "pg_ensure_role":
		return a.executeEnsureRole(ctx, cmd, logger)

	case "pg_rotate_role_password":
		return a.executeRotateRolePassword(ctx, cmd, logger)

	case "pg_drop_role":
		return a.executeDropRole(ctx, cmd, logger)

	case "pg_ensure_database":
		return a.executeEnsureDatabase(ctx, cmd, logger)

	case "pg_drop_database":
		return a.executeDropDatabase(ctx, cmd, logger)

	case "pg_grant_database_privileges":
		return a.executeGrantDatabasePrivileges(ctx, cmd, logger)

	case "pg_apply_hba":
		return a.executeApplyHBA(ctx, cmd, logger)

	case "pg_apply_tls":
		return a.executeApplyTLS(ctx, cmd, logger)

	default:
		return false, "", fmt.Sprintf("unknown command action: %s", cmd.GetAction())
	}
}

type postgresAdminCredentialPayload struct {
	PostgresAdminUser      string `json:"postgres_admin_user"`
	PasswordSecretKey      string `json:"password_secret_key"`
	PostgresAdminSecretKey string `json:"postgres_admin_secret_key"`
}

func (a *Agent) applyPostgresAdminCredentials(cmd *skylexv1.AgentCommand) error {
	if cmd.GetPayload() == "" {
		return nil
	}
	var payload postgresAdminCredentialPayload
	if err := json.Unmarshal([]byte(cmd.GetPayload()), &payload); err != nil {
		return fmt.Errorf("invalid credential payload: %w", err)
	}
	secretKey := payload.PasswordSecretKey
	if secretKey == "" {
		secretKey = payload.PostgresAdminSecretKey
	}
	if secretKey == "" {
		secretKey = "postgres_admin_password"
	}
	password := cmd.GetSecrets()[secretKey]
	if payload.PostgresAdminUser == "" || password == "" {
		return fmt.Errorf("PostgreSQL admin credentials are required")
	}
	a.pg.SetSuperuserCredentials(payload.PostgresAdminUser, password)
	return nil
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

func mustMarshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func (a *Agent) setInstallationReport(state skylexv1.InstallationState, details string) {
	a.installMu.Lock()
	defer a.installMu.Unlock()
	a.installationState = state
	a.conflictDetails = details
}

func (a *Agent) installationReport() (skylexv1.InstallationState, string) {
	a.installMu.RLock()
	defer a.installMu.RUnlock()
	return a.installationState, a.conflictDetails
}

// enrichError wraps an error with actionable hints based on the command type
// and the error message content.
func (a *Agent) enrichError(action string, err error) string {
	msg := err.Error()
	lower := strings.ToLower(msg)

	switch action {
	case "pg_preflight":
		return fmt.Sprintf("preflight failed: %s", msg)

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

	case "agent_deactivate":
		return fmt.Sprintf("agent_deactivate failed: %s", msg)

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
		if strings.Contains(lower, "restart") && strings.Contains(lower, "docker group") {
			return fmt.Sprintf("docker engine was installed and the agent user was added to the docker group; restart the skylex-agent service to apply the new group membership and retry the cluster creation")
		}
		if strings.Contains(lower, "docker binary not found") || strings.Contains(lower, "permission denied") {
			return fmt.Sprintf("docker install failed: Docker is unavailable or the agent cannot access the Docker daemon — ensure the agent user is in the docker group or has sufficient privileges: %s", msg)
		}
		return fmt.Sprintf("docker install failed: %s", msg)

	case "pg_purge_native":
		return fmt.Sprintf("native purge failed: %s", msg)

	default:
		return fmt.Sprintf("%s failed: %s", action, msg)
	}
}

func (a *Agent) deactivate(ctx context.Context, logger *commandLogger) error {
	if err := a.pg.Stop(ctx); err != nil {
		return err
	}
	if err := WriteDeactivationMarker(a.cfg); err != nil {
		return err
	}
	if err := disableSystemdAgent(ctx, logger); err != nil {
		return err
	}
	if err := removeAgentTokenFiles(logger); err != nil {
		return err
	}
	return nil
}

func disableSystemdAgent(ctx context.Context, logger *commandLogger) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return nil
	}

	if err := runAgentLifecycleCommand(ctx, logger, "systemctl", "disable", "skylex-agent"); err != nil {
		if logger != nil {
			logger.Error(err.Error())
		}
	}
	if err := runAgentLifecycleCommand(ctx, logger, "systemctl", "reset-failed", "skylex-agent"); err != nil {
		if logger != nil {
			logger.Error(err.Error())
		}
	}
	return nil
}

func removeAgentTokenFiles(logger *commandLogger) error {
	paths := []string{
		"/etc/skylex/agent.yaml",
		"/etc/skylex/token",
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			if logger != nil {
				logger.Error(fmt.Sprintf("remove %s: %v", path, err))
			}
			continue
		}
		if logger != nil {
			logger.Info(fmt.Sprintf("removed %s", path))
		}
	}

	return nil
}

func runAgentLifecycleCommand(ctx context.Context, logger *commandLogger, name string, args ...string) error {
	if logger != nil {
		logger.Info("$ " + name + " " + strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if len(out) > 0 && logger != nil {
		for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
			if line != "" {
				logger.Info(line)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
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
