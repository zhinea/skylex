package agent

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	ServerAddr        string            `mapstructure:"server_addr" yaml:"server_addr"`
	AgentToken        string            `mapstructure:"agent_token" yaml:"agent_token"`
	Hostname          string            `mapstructure:"hostname" yaml:"hostname"`
	Address           string            `mapstructure:"address" yaml:"address"`
	Port              int               `mapstructure:"port" yaml:"port"`
	Labels            map[string]string `mapstructure:"labels" yaml:"labels"`
	HeartbeatInterval time.Duration     `mapstructure:"heartbeat_interval" yaml:"heartbeat_interval"`
	LogLevel          string            `mapstructure:"log_level" yaml:"log_level"`
	LogFormat         string            `mapstructure:"log_format" yaml:"log_format"`
	PGDataDir         string            `mapstructure:"pg_data_dir" yaml:"pg_data_dir"`
	PGBinDir          string            `mapstructure:"pg_bin_dir" yaml:"pg_bin_dir"`
	PGVersion         string            `mapstructure:"pg_version" yaml:"pg_version"`
	PGSuperuser       string            `mapstructure:"pg_superuser" yaml:"pg_superuser"`
	PGReplUser        string            `mapstructure:"pg_repl_user" yaml:"pg_repl_user"`
	PGReplPass        string            `mapstructure:"pg_repl_pass" yaml:"pg_repl_pass"`
	PGBackRestPath    string            `mapstructure:"pgbackrest_path" yaml:"pgbackrest_path"`
}

func DefaultConfig() Config {
	return Config{
		ServerAddr:        "localhost:9090",
		Port:              5432,
		HeartbeatInterval: 10 * time.Second,
		LogLevel:          "info",
		LogFormat:         "json",
		PGDataDir:         "/var/lib/postgresql/data",
		PGBinDir:          "/usr/lib/postgresql/16/bin",
		PGVersion:         "16",
		PGSuperuser:       "postgres",
		PGReplUser:        "replicator",
		PGReplPass:        "replicator",
		PGBackRestPath:    "/usr/bin/pgbackrest",
	}
}

func NewLogger(level, format string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}