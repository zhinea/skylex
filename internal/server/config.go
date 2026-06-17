package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	Server   ServerConfig   `validate:"required"`
	Database DatabaseConfig `validate:"required"`
	Etcd     EtcdConfig     `validate:"required"`
	Backup   BackupConfig   `validate:"required"`
	Agent    AgentConfig    `validate:"required"`
	Auth     AuthConfig     `validate:"required"`
	Logging  LoggingConfig  `validate:"required"`
	TLS      TLSConfig      `validate:"required"`
	Postgres PostgresConfig `validate:"required"`
	Webhook  WebhookConfig  `koanf:"webhook"`
}

type ServerConfig struct {
	ListenAddr   string `koanf:"listen_addr" validate:"required"`
	GRPCPort     int    `koanf:"grpc_port" validate:"required,min=1,max=65535"`
	HTTPPort     int    `koanf:"http_port" validate:"required,min=1,max=65535"`
	MetricsPort  int    `koanf:"metrics_port" validate:"required,min=1,max=65535"`
	AdvertiseAddr string `koanf:"advertise_addr"`
}

type DatabaseConfig struct {
	Driver          string        `koanf:"driver" validate:"required,oneof=sqlite sqlite3 postgres pgx"`
	DSN             string        `koanf:"dsn" validate:"required"`
	MaxOpenConns    int           `koanf:"max_open_conns"`
	MaxIdleConns    int           `koanf:"max_idle_conns"`
	ConnMaxLifetime time.Duration `koanf:"conn_max_lifetime"`
}

type EtcdConfig struct {
	Endpoints   []string      `koanf:"endpoints" validate:"required,min=1"`
	DialTimeout time.Duration `koanf:"dial_timeout" validate:"required"`
	Username    string        `koanf:"username"`
	Password    string        `koanf:"password"`
}

type BackupConfig struct {
	PgBackRestPath string `koanf:"pgbackrest_path" validate:"required"`
	DefaultStorage string `koanf:"default_storage" validate:"required"`
}

type AgentConfig struct {
	HeartbeatInterval time.Duration `koanf:"heartbeat_interval" validate:"required"`
	HeartbeatTimeout  time.Duration `koanf:"heartbeat_timeout" validate:"required"`
	AgentToken        string        `koanf:"agent_token"`
}

type AuthConfig struct {
	JWTSecret     string        `koanf:"jwt_secret"`
	TokenExpiry   time.Duration `koanf:"token_expiry" validate:"required"`
	RefreshExpiry time.Duration `koanf:"refresh_expiry" validate:"required"`
}

type LoggingConfig struct {
	Level  string `koanf:"level" validate:"required,oneof=debug info warn error"`
	Format string `koanf:"format" validate:"required,oneof=json text"`
	Output string `koanf:"output" validate:"required"`
}

type TLSConfig struct {
	Enabled  bool   `koanf:"enabled"`
	CertFile string `koanf:"cert_file"`
	KeyFile  string `koanf:"key_file"`
	CAFile   string `koanf:"ca_file"`
}

type PostgresConfig struct {
	DefaultVersion  string `koanf:"default_version" validate:"required"`
	DefaultDataDir  string `koanf:"default_data_dir" validate:"required"`
	DefaultPort     int    `koanf:"default_port" validate:"required,min=1,max=65535"`
	Superuser       string `koanf:"superuser" validate:"required"`
	ReplicationUser string `koanf:"replication_user" validate:"required"`
}

func LoadConfig(path string) (*Config, error) {
	k := koanf.New(".")

	if path != "" {
		if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load config file: %w", err)
		}
	}

	k.Load(env.Provider("SKYLEX_", ".", func(s string) string {
		return s
	}), nil)

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.setDefaults(); err != nil {
		return nil, fmt.Errorf("set defaults: %w", err)
	}

	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) setDefaults() error {
	if c.Auth.JWTSecret == "" {
		secret, err := generateSecret(32)
		if err != nil {
			return fmt.Errorf("generate jwt secret: %w", err)
		}
		c.Auth.JWTSecret = secret
	}

	if c.Server.ListenAddr == "" {
		c.Server.ListenAddr = "0.0.0.0"
	}
	if c.Server.GRPCPort == 0 {
		c.Server.GRPCPort = 9090
	}
	if c.Server.HTTPPort == 0 {
		c.Server.HTTPPort = 8080
	}
	if c.Server.MetricsPort == 0 {
		c.Server.MetricsPort = 9091
	}
	if c.Server.AdvertiseAddr == "" {
		c.Server.AdvertiseAddr = fmt.Sprintf("%s:%d", c.Server.ListenAddr, c.Server.GRPCPort)
	}

	if c.Database.Driver == "" {
		c.Database.Driver = "sqlite"
	}

	if c.Agent.HeartbeatInterval == 0 {
		c.Agent.HeartbeatInterval = 10 * time.Second
	}
	if c.Agent.HeartbeatTimeout == 0 {
		c.Agent.HeartbeatTimeout = 30 * time.Second
	}

	if c.Auth.TokenExpiry == 0 {
		c.Auth.TokenExpiry = 24 * time.Hour
	}
	if c.Auth.RefreshExpiry == 0 {
		c.Auth.RefreshExpiry = 7 * 24 * time.Hour
	}

	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	if c.Logging.Output == "" {
		c.Logging.Output = os.Stdout.Name()
	}

	if c.Postgres.DefaultVersion == "" {
		c.Postgres.DefaultVersion = "16"
	}
	if c.Postgres.DefaultDataDir == "" {
		c.Postgres.DefaultDataDir = "/var/lib/postgresql/data"
	}
	if c.Postgres.DefaultPort == 0 {
		c.Postgres.DefaultPort = 5432
	}
	if c.Postgres.Superuser == "" {
		c.Postgres.Superuser = "postgres"
	}
	if c.Postgres.ReplicationUser == "" {
		c.Postgres.ReplicationUser = "replicator"
	}

	if c.Backup.PgBackRestPath == "" {
		c.Backup.PgBackRestPath = "/usr/bin/pgbackrest"
	}
	if c.Backup.DefaultStorage == "" {
		c.Backup.DefaultStorage = "s3"
	}

	c.Webhook.setDefaults()

	return nil
}

func generateSecret(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}