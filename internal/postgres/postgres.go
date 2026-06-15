package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Instance struct {
	DataDir   string
	BinDir    string
	Version   string
	Port      int
	Superuser string
	ReplUser  string
	ReplPass  string
	log       *slog.Logger
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

func (p *Instance) IsInitialized() bool {
	_, err := os.Stat(filepath.Join(p.DataDir, "PG_VERSION"))
	return err == nil
}

func (p *Instance) IsRunning() bool {
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

	if err := os.MkdirAll(p.DataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	initdb := filepath.Join(p.BinDir, "initdb")
	cmd := exec.CommandContext(ctx, initdb,
		"-D", p.DataDir,
		"--username", p.Superuser,
		"--auth", "scram-sha-256",
		"--pwprompt",
	)

	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", p.ReplPass),
	)

	output, err := cmd.CombinedOutput()
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

	pgCtl := filepath.Join(p.BinDir, "pg_ctl")
	cmd := exec.CommandContext(ctx, pgCtl,
		"start",
		"-D", p.DataDir,
		"-l", filepath.Join(p.DataDir, "pg.log"),
		"-o", fmt.Sprintf("-p %d", p.Port),
		"-w",
		"-t", "60",
	)

	output, err := cmd.CombinedOutput()
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

	pgCtl := filepath.Join(p.BinDir, "pg_ctl")
	cmd := exec.CommandContext(ctx, pgCtl,
		"stop",
		"-D", p.DataDir,
		"-m", "fast",
		"-w",
		"-t", "60",
	)

	output, err := cmd.CombinedOutput()
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

	pgBasebackup := filepath.Join(p.BinDir, "pg_basebackup")
	cmd := exec.CommandContext(ctx, pgBasebackup,
		"-h", primaryHost,
		"-p", fmt.Sprintf("%d", primaryPort),
		"-U", replUser,
		"-D", p.DataDir,
		"-P",
		"-v",
		"--wal-method", "stream",
	)

	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", replPass),
	)

	output, err := cmd.CombinedOutput()
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
	psql := filepath.Join(p.BinDir, "psql")
	cmd := exec.CommandContext(ctx, psql,
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-c", fmt.Sprintf("SELECT pg_create_physical_replication_slot('%s', true) WHERE NOT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = '%s')", slotName, slotName),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create replication slot: %w\n%s", err, string(output))
	}

	p.log.Info("replication slot created", "slot", slotName)
	return nil
}

func (p *Instance) CreateReplicationUser(ctx context.Context) error {
	psql := filepath.Join(p.BinDir, "psql")
	cmd := exec.CommandContext(ctx, psql,
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-c", fmt.Sprintf("DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '%s') THEN CREATE ROLE %s WITH LOGIN REPLICATION ENCRYPTED PASSWORD '%s'; END IF; END $$", p.ReplUser, p.ReplUser, p.ReplPass),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create replication user: %w\n%s", err, string(output))
	}

	p.log.Info("replication user ensured", "user", p.ReplUser)
	return nil
}

func (p *Instance) HealthCheck(ctx context.Context) error {
	psql := filepath.Join(p.BinDir, "psql")
	cmd := exec.CommandContext(ctx, psql,
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-c", "SELECT 1",
		"-t",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("health check failed: %w\n%s", err, string(output))
	}

	p.log.Debug("health check passed", "port", p.Port)
	return nil
}

func (p *Instance) GetReplicationLag(ctx context.Context) (string, error) {
	psql := filepath.Join(p.BinDir, "psql")
	cmd := exec.CommandContext(ctx, psql,
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-t", "-A",
		"-c", "SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0) FROM pg_stat_replication WHERE application_name = 'skylex_replica'",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("replication lag check: %w\n%s", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

func (p *Instance) GetVersion(ctx context.Context) (string, error) {
	psql := filepath.Join(p.BinDir, "psql")
	cmd := exec.CommandContext(ctx, psql,
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", p.Port),
		"-U", p.Superuser,
		"-t", "-A",
		"-c", "SELECT version()",
	)

	output, err := cmd.CombinedOutput()
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
	conf := fmt.Sprintf(`
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
`, p.Port)

	confPath := filepath.Join(p.DataDir, "postgresql.conf")
	existing, err := os.ReadFile(confPath)
	if err == nil && strings.Contains(string(existing), "listen_addresses") {
		return nil
	}

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
	conf := fmt.Sprintf(`primary_conninfo = 'host=primary port=%d user=%s password=%s application_name=skylex_replica'
primary_slot_name = 'skylex_replica'
`, p.Port, p.ReplUser, p.ReplPass)

	confPath := filepath.Join(p.DataDir, "standby.signal")
	return os.WriteFile(confPath, []byte(conf), 0600)
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