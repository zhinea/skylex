package postgres

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogSink receives per-line stdout/stderr emitted while a PostgreSQL command
// is running. It is typically implemented by the agent command logger.
type LogSink interface {
	Info(message string)
	Error(message string)
}

type logSinkKey struct{}

// WithLogSink adds a LogSink to the context so PostgreSQL helpers can stream
// command output without changing every method signature.
func WithLogSink(ctx context.Context, sink LogSink) context.Context {
	return context.WithValue(ctx, logSinkKey{}, sink)
}

func logSinkFromContext(ctx context.Context) LogSink {
	if sink, ok := ctx.Value(logSinkKey{}).(LogSink); ok {
		return sink
	}
	return nil
}

type Instance struct {
	DataDir   string
	BinDir    string
	Version   string
	Port      int
	Superuser string
	ReplUser  string
	ReplPass  string
	Docker    *DockerRuntime
	log       *slog.Logger
}

type DockerRuntime struct {
	Image         string
	ContainerName string
}

func New(dataDir, binDir, version string, port int, superuser, replUser, replPass string, log *slog.Logger) *Instance {
	return &Instance{
		DataDir:   dataDir,
		BinDir:    binDir,
		Version:   version,
		Port:      port,
		Superuser: superuser,
		ReplUser:  replUser,
		ReplPass:  replPass,
		log:       log,
	}
}

func (p *Instance) UseDocker(image, containerName string) {
	p.Docker = &DockerRuntime{Image: image, ContainerName: containerName}
}

func (p *Instance) UseNative(binDir, version string) {
	p.BinDir = binDir
	p.Version = version
	p.Docker = nil
}

func (p *Instance) UsesDocker() bool {
	return p.dockerEnabled()
}

func (p *Instance) dockerEnabled() bool {
	return p.Docker != nil && p.Docker.ContainerName != "" && p.Docker.Image != ""
}

func (p *Instance) pgCmd(ctx context.Context, binary string, args ...string) *exec.Cmd {
	if !p.dockerEnabled() {
		return exec.CommandContext(ctx, filepath.Join(p.BinDir, binary), args...)
	}
	dockerArgs := []string{"exec", "-e", "PGDATA=/var/lib/postgresql/data", p.Docker.ContainerName, binary}
	dockerArgs = append(dockerArgs, args...)
	return exec.CommandContext(ctx, "docker", dockerArgs...)
}

func (p *Instance) pgDataDir() string {
	if p.dockerEnabled() {
		return "/var/lib/postgresql/data"
	}
	return p.DataDir
}

func (p *Instance) dockerRun(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "docker", args...)
}

// runStreamingCmd executes a command while streaming stdout/stderr to the
// LogSink in ctx (if any). It returns the combined output so callers can keep
// returning descriptive error messages.
func runStreamingCmd(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	sink := logSinkFromContext(ctx)

	if sink == nil {
		return cmd.CombinedOutput()
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var outMu sync.Mutex
	var output []byte

	scan := func(r io.Reader, level string) {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if level == "error" {
				sink.Error(line)
			} else {
				sink.Info(line)
			}
			outMu.Lock()
			output = append(output, line...)
			output = append(output, '\n')
			outMu.Unlock()
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); scan(stdoutPipe, "info") }()
	go func() { defer wg.Done(); scan(stderrPipe, "error") }()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		wg.Wait()
		return output, ctx.Err()
	case err := <-done:
		wg.Wait()
		return output, err
	}
}

// DetectInstallation probes the host for a PostgreSQL installation without
// requiring a running instance.  It first tries "pg_config --version"; if that
// is not on PATH it falls back to "postgres --version".  Returns
// (installed=true, version string) on success or (false, "") when neither
// binary is found.
func DetectInstallation(ctx context.Context) (installed bool, version string) {
	// Prefer pg_config because it reliably prints "PostgreSQL <version>".
	if out, err := exec.CommandContext(ctx, "pg_config", "--version").Output(); err == nil {
		ver := strings.TrimSpace(string(out))
		if ver != "" {
			return true, ver
		}
	}

	// Fallback: "postgres --version" prints "postgres (PostgreSQL) <version>".
	if out, err := exec.CommandContext(ctx, "postgres", "--version").Output(); err == nil {
		ver := strings.TrimSpace(string(out))
		if ver != "" {
			return true, ver
		}
	}

	return false, ""
}

// IsDataDirInitialized reports whether the data directory has been initialised
// by initdb (i.e. a PG_VERSION file is present).
func (p *Instance) IsDataDirInitialized() bool {
	return p.IsInitialized()
}

func (p *Instance) IsInitialized() bool {
	_, err := os.Stat(filepath.Join(p.DataDir, "PG_VERSION"))
	return err == nil
}

func (p *Instance) IsRunning() bool {
	if p.dockerEnabled() {
		out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", p.Docker.ContainerName).Output()
		return err == nil && strings.TrimSpace(string(out)) == "true"
	}

	pidFile := filepath.Join(p.DataDir, "postmaster.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid := strings.TrimSpace(strings.Split(string(data), "\n")[0])
	_, err = os.Stat(fmt.Sprintf("/proc/%s", pid))
	return err == nil
}

func (p *Instance) InitDB(ctx context.Context) error {
	if p.IsInitialized() {
		p.log.Info("postgresql already initialized", "data_dir", p.DataDir)
		return nil
	}
	if p.dockerEnabled() {
		return fmt.Errorf("postgresql container is not initialized; check Docker provisioning logs")
	}

	if err := os.MkdirAll(p.DataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	cmd := p.pgCmd(ctx, "initdb",
		"-D", p.pgDataDir(),
		"--username", p.Superuser,
		"--auth", "scram-sha-256",
		"--pwprompt",
	)

	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", p.ReplPass),
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("initdb failed: %w\n%s", err, string(output))
	}

	p.log.Info("initdb completed", "data_dir", p.DataDir)

	if err := p.writePostgresqlConf(); err != nil {
		return fmt.Errorf("write postgresql.conf: %w", err)
	}

	if err := p.writePgHBAConf(); err != nil {
		return fmt.Errorf("write pg_hba.conf: %w", err)
	}

	return nil
}

func (p *Instance) Start(ctx context.Context) error {
	if p.IsRunning() {
		p.log.Info("postgresql already running", "data_dir", p.DataDir)
		return nil
	}

	if p.dockerEnabled() {
		cmd := p.dockerRun(ctx, "start", p.Docker.ContainerName)
		output, err := runStreamingCmd(ctx, cmd)
		if err == nil {
			p.log.Info("postgresql container started", "container", p.Docker.ContainerName)
			return nil
		}

		cmd = p.dockerRun(ctx, "run", "-d",
			"--name", p.Docker.ContainerName,
			"--restart", "unless-stopped",
			"-p", fmt.Sprintf("%d:5432", p.Port),
			"-e", fmt.Sprintf("POSTGRES_USER=%s", p.Superuser),
			"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", p.ReplPass),
			"-v", filepath.Clean(p.DataDir)+":/var/lib/postgresql/data",
			p.Docker.Image,
		)
		output, err = runStreamingCmd(ctx, cmd)
		if err != nil {
			return fmt.Errorf("docker run failed: %w\n%s", err, string(output))
		}
		p.log.Info("postgresql container created and started", "container", p.Docker.ContainerName)
		return nil
	}

	cmd := p.pgCmd(ctx, "pg_ctl",
		"start",
		"-D", p.pgDataDir(),
		"-l", filepath.Join(p.DataDir, "pg.log"),
		"-o", fmt.Sprintf("-p %d", p.Port),
		"-w",
		"-t", "60",
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("pg_ctl start failed: %w\n%s", err, string(output))
	}

	p.log.Info("postgresql started", "data_dir", p.DataDir, "port", p.Port)
	return nil
}

func (p *Instance) Stop(ctx context.Context) error {
	if !p.IsRunning() {
		return nil
	}

	if p.dockerEnabled() {
		cmd := p.dockerRun(ctx, "stop", p.Docker.ContainerName)
		output, err := runStreamingCmd(ctx, cmd)
		if err != nil {
			return fmt.Errorf("docker stop failed: %w\n%s", err, string(output))
		}
		p.log.Info("postgresql container stopped", "container", p.Docker.ContainerName)
		return nil
	}

	cmd := p.pgCmd(ctx, "pg_ctl",
		"stop",
		"-D", p.pgDataDir(),
		"-m", "fast",
		"-w",
		"-t", "60",
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("pg_ctl stop failed: %w\n%s", err, string(output))
	}

	p.log.Info("postgresql stopped", "data_dir", p.DataDir)
	return nil
}

func (p *Instance) BaseBackup(ctx context.Context, primaryHost string, primaryPort int, replUser, replPass string) error {
	if p.IsInitialized() {
		p.log.Info("data dir already initialized, skipping base backup")
		return nil
	}

	if err := os.RemoveAll(p.DataDir); err != nil {
		return fmt.Errorf("remove existing data dir: %w", err)
	}

	cmd := p.pgCmd(ctx, "pg_basebackup",
		"-h", primaryHost,
		"-p", fmt.Sprintf("%d", primaryPort),
		"-U", replUser,
		"-D", p.pgDataDir(),
		"-P",
		"-v",
		"--wal-method", "stream",
	)

	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", replPass),
	)
	if p.dockerEnabled() {
		cmd = p.dockerRun(ctx, "run", "--rm",
			"-e", fmt.Sprintf("PGPASSWORD=%s", replPass),
			"-v", filepath.Clean(p.DataDir)+":/var/lib/postgresql/data",
			p.Docker.Image,
			"pg_basebackup",
			"-h", primaryHost,
			"-p", fmt.Sprintf("%d", primaryPort),
			"-U", replUser,
			"-D", "/var/lib/postgresql/data",
			"-P",
			"-v",
			"--wal-method", "stream",
		)
	}

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("pg_basebackup failed: %w\n%s", err, string(output))
	}

	p.log.Info("pg_basebackup completed", "data_dir", p.DataDir)

	if err := p.writePostgresqlConf(); err != nil {
		return fmt.Errorf("write postgresql.conf: %w", err)
	}

	if err := p.writeStandbySignal(); err != nil {
		return fmt.Errorf("write standby signal: %w", err)
	}

	return nil
}

func (p *Instance) CreateReplicationSlot(ctx context.Context, slotName string) error {
	cmd := p.pgCmd(ctx, "psql",
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-v", fmt.Sprintf("slot=%s", slotName),
		"-c", "SELECT pg_create_physical_replication_slot(:'slot', true) WHERE NOT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = :'slot')",
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("create replication slot: %w\n%s", err, string(output))
	}

	p.log.Info("replication slot created", "slot", slotName)
	return nil
}

func (p *Instance) CreateReplicationUser(ctx context.Context) error {
	cmd := p.pgCmd(ctx, "psql",
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-c", fmt.Sprintf("DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = %s) THEN EXECUTE format('CREATE ROLE %%I WITH LOGIN REPLICATION ENCRYPTED PASSWORD %%L', %s, %s); END IF; END $$", pqQuoteLiteral(p.ReplUser), pqQuoteLiteral(p.ReplUser), pqQuoteLiteral(p.ReplPass)),
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("create replication user: %w\n%s", err, string(output))
	}

	p.log.Info("replication user ensured", "user", p.ReplUser)
	return nil
}

func pqQuoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func (p *Instance) HealthCheck(ctx context.Context) error {
	cmd := p.pgCmd(ctx, "psql",
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-c", "SELECT 1",
		"-t",
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("health check failed: %w\n%s", err, string(output))
	}

	p.log.Debug("health check passed", "port", p.Port)
	return nil
}

func (p *Instance) GetReplicationLag(ctx context.Context) (string, error) {
	cmd := p.pgCmd(ctx, "psql",
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-t", "-A",
		"-c", "SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0) FROM pg_stat_replication WHERE application_name = 'skylex_replica'",
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("replication lag check: %w\n%s", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

func (p *Instance) GetVersion(ctx context.Context) (string, error) {
	cmd := p.pgCmd(ctx, "psql",
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-t", "-A",
		"-c", "SELECT version()",
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("version check: %w\n%s", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

func (p *Instance) IsReplicating() bool {
	_, err := os.Stat(filepath.Join(p.DataDir, "standby.signal"))
	return err == nil
}

func (p *Instance) writePostgresqlConf() error {
	conf := fmt.Sprintf(`%s
listen_addresses = '*'
port = %d
max_connections = 200
shared_buffers = 128MB
wal_level = replica
max_wal_senders = 10
max_replication_slots = 10
wal_keep_size = 1024
hot_standby = on
log_directory = 'pg_log'
log_filename = 'postgresql-%%a.log'
log_truncate_on_rotation = on
log_rotation_age = 1d
log_rotation_size = 0
logging_collector = on
`, includeDirective+"\n", p.Port)

	confPath := filepath.Join(p.DataDir, "postgresql.conf")
	return os.WriteFile(confPath, []byte(conf), 0600)
}

func (p *Instance) WriteSyncReplicationConf() error {
	conf := fmt.Sprintf(`%s
listen_addresses = '*'
port = %d
max_connections = 200
shared_buffers = 128MB
wal_level = replica
max_wal_senders = 10
max_replication_slots = 10
wal_keep_size = 1024
hot_standby = on
synchronous_commit = on
synchronous_standby_names = 'skylex_replica'
log_directory = 'pg_log'
log_filename = 'postgresql-%%a.log'
log_truncate_on_rotation = on
log_rotation_age = 1d
log_rotation_size = 0
logging_collector = on
`, includeDirective+"\n", p.Port)

	confPath := filepath.Join(p.DataDir, "postgresql.conf")
	return os.WriteFile(confPath, []byte(conf), 0600)
}

func (p *Instance) writePgHBAConf() error {
	conf := `
local   all             all                                     scram-sha-256
host    all             all             0.0.0.0/0               scram-sha-256
host    replication     all             0.0.0.0/0               scram-sha-256
`

	confPath := filepath.Join(p.DataDir, "pg_hba.conf")
	return os.WriteFile(confPath, []byte(conf), 0600)
}

func (p *Instance) writeStandbySignal() error {
	return p.UpdateStandbySignal("primary", p.Port)
}

func (p *Instance) UpdateStandbySignal(primaryHost string, primaryPort int) error {
	conf := fmt.Sprintf(`primary_conninfo = 'host=%s port=%d user=%s password=%s application_name=skylex_replica'
primary_slot_name = 'skylex_replica'
`, primaryHost, primaryPort, p.ReplUser, p.ReplPass)

	confPath := filepath.Join(p.DataDir, "standby.signal")
	return os.WriteFile(confPath, []byte(conf), 0600)
}

func (p *Instance) Promote(ctx context.Context) error {
	if !p.IsRunning() {
		return fmt.Errorf("postgresql is not running, cannot promote")
	}

	cmd := p.pgCmd(ctx, "pg_ctl",
		"promote",
		"-D", p.pgDataDir(),
		"-w",
		"-t", "60",
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("pg_ctl promote failed: %w\n%s", err, string(output))
	}

	p.log.Info("postgresql promoted to primary", "data_dir", p.DataDir)

	standbySignalPath := filepath.Join(p.DataDir, "standby.signal")
	os.Remove(standbySignalPath)
	os.Remove(standbySignalPath + ".backup")

	recoveryConf := filepath.Join(p.DataDir, "recovery.conf")
	os.Remove(recoveryConf)
	os.Remove(recoveryConf + ".backup")

	return nil
}

func (p *Instance) Rewind(ctx context.Context, targetHost string, targetPort int, replUser, replPass string) error {
	cmd := p.pgCmd(ctx, "pg_rewind",
		"--target-pgdata", p.pgDataDir(),
		"--source-server", fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=postgres",
			targetHost, targetPort, replUser, replPass),
		"-P",
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("pg_rewind failed: %w\n%s", err, string(output))
	}

	p.log.Info("pg_rewind completed", "data_dir", p.DataDir, "target", targetHost)

	if err := p.UpdateStandbySignal(targetHost, targetPort); err != nil {
		return fmt.Errorf("update standby signal after rewind: %w", err)
	}

	return nil
}

func (p *Instance) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := p.HealthCheck(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return fmt.Errorf("postgresql did not become ready within %s", timeout)
}

const (
	includeFileName  = "skylex.conf.include"
	includeDirective = "include_if_exists = 'skylex.conf.include'"
)

// reloadableSettings lists parameters in the curated set that can be changed
// with a simple SIGHUP reload.  All other curated parameters require a full
// restart.
var reloadableSettings = map[string]bool{
	"work_mem": true,
}

// ApplySettings writes the provided parameters to an include file, ensures
// the main postgresql.conf loads it, and reloads or restarts the instance as
// required.  It returns "reload" or "restart" when successful.
func (p *Instance) ApplySettings(ctx context.Context, settings map[string]string) (string, error) {
	if !p.IsInitialized() {
		return "", fmt.Errorf("data directory is not initialized")
	}

	if err := p.writeSkylexInclude(settings); err != nil {
		return "", fmt.Errorf("write include file: %w", err)
	}
	if err := p.ensureIncludeDirective(); err != nil {
		return "", fmt.Errorf("ensure include directive: %w", err)
	}

	requiresRestart := false
	for key := range settings {
		if !reloadableSettings[key] {
			requiresRestart = true
			break
		}
	}

	if requiresRestart {
		p.log.Info("restarting postgresql to apply settings", "data_dir", p.DataDir)
		if err := p.Stop(ctx); err != nil {
			return "", fmt.Errorf("stop before restart: %w", err)
		}
		if err := p.Start(ctx); err != nil {
			return "", fmt.Errorf("start after restart: %w", err)
		}
		return "restart", nil
	}

	p.log.Info("reloading postgresql to apply settings", "data_dir", p.DataDir)
	if err := p.Reload(ctx); err != nil {
		return "", fmt.Errorf("reload settings: %w", err)
	}
	return "reload", nil
}

// Reload signals PostgreSQL to reload its configuration without restarting.
func (p *Instance) Reload(ctx context.Context) error {
	cmd := p.pgCmd(ctx, "pg_ctl",
		"reload",
		"-D", p.pgDataDir(),
	)

	output, err := runStreamingCmd(ctx, cmd)
	if err != nil {
		return fmt.Errorf("pg_ctl reload failed: %w\n%s", err, string(output))
	}

	p.log.Info("postgresql configuration reloaded", "data_dir", p.DataDir)
	return nil
}

func (p *Instance) writeSkylexInclude(settings map[string]string) error {
	if len(settings) == 0 {
		// Write an empty include file so PostgreSQL never complains.
		return os.WriteFile(filepath.Join(p.DataDir, includeFileName), []byte{}, 0600)
	}

	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Managed by Skylex. Last updated: %s\n", time.Now().UTC().Format(time.RFC3339)))
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("%s = %s\n", k, settings[k]))
	}

	return os.WriteFile(filepath.Join(p.DataDir, includeFileName), []byte(b.String()), 0600)
}

// ensureIncludeDirective ensures the main postgresql.conf loads the Skylex
// include file.  The directive is added at the top of the file if missing.
func (p *Instance) ensureIncludeDirective() error {
	confPath := filepath.Join(p.DataDir, "postgresql.conf")
	data, err := os.ReadFile(confPath)
	if err != nil {
		return fmt.Errorf("read postgresql.conf: %w", err)
	}

	content := string(data)
	if strings.Contains(content, includeDirective) {
		return nil
	}

	updated := includeDirective + "\n" + content
	return os.WriteFile(confPath, []byte(updated), 0600)
}
