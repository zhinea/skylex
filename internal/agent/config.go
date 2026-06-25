package agent

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DeactivationMarkerName = ".skylex-agent-deactivated"

// DefaultAgentLogFile is where the agent appends its own operational logs by
// default, so a node's activity and errors can always be traced from disk
// without extra configuration. It lives under /var/log per the FHS convention
// for variable log data (rather than /etc, which is for static config, or the
// PostgreSQL data directory, which initdb requires to be empty).
// Set log_file to "" in the agent config file to disable file logging.
const DefaultAgentLogFile = "/var/log/skylex/agent.log"

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
	LogFile           string            `mapstructure:"log_file" yaml:"log_file"`
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
		LogFile:           DefaultAgentLogFile,
		PGDataDir:         "/var/lib/postgresql/data",
		PGBinDir:          "/usr/lib/postgresql/16/bin",
		PGVersion:         "16",
		PGSuperuser:       "postgres",
		PGReplUser:        "replicator",
		PGReplPass:        "replicator",
		PGBackRestPath:    "/usr/bin/pgbackrest",
	}
}

func IsDeactivated(cfg Config) bool {
	for _, path := range deactivationMarkerPaths(cfg) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func WriteDeactivationMarker(cfg Config) error {
	var lastErr error
	for _, path := range deactivationMarkerPaths(cfg) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			lastErr = err
			continue
		}
		if err := os.WriteFile(path, []byte("deactivated\n"), 0600); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func deactivationMarkerPaths(cfg Config) []string {
	paths := []string{
		"/etc/skylex/" + DeactivationMarkerName,
		"/var/lib/skylex/" + DeactivationMarkerName,
		"/tmp/skylex-agent-deactivated",
	}
	if cfg.PGDataDir != "" {
		paths = append(paths, filepath.Join(cfg.PGDataDir, DeactivationMarkerName))
	}
	return paths
}

func NewLogger(level, format string, outputs ...io.Writer) *slog.Logger {
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
		Level:       logLevel,
		ReplaceAttr: redactLogAttr,
	}

	writers := make([]io.Writer, 0, len(outputs))
	for _, output := range outputs {
		if output != nil {
			writers = append(writers, output)
		}
	}
	if len(writers) == 0 {
		writers = append(writers, os.Stderr)
	}

	output := writers[0]
	if len(writers) > 1 {
		output = io.MultiWriter(writers...)
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(output, opts)
	} else {
		handler = slog.NewTextHandler(output, opts)
	}

	return slog.New(handler)
}

func redactLogAttr(_ []string, attr slog.Attr) slog.Attr {
	attr.Value = redactLogValue(attr.Value)
	return attr
}

func redactLogValue(value slog.Value) slog.Value {
	switch value.Kind() {
	case slog.KindString:
		return slog.StringValue(RedactSecrets(value.String()))
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return slog.StringValue(RedactSecrets(err.Error()))
		}
	case slog.KindGroup:
		attrs := value.Group()
		for i := range attrs {
			attrs[i] = redactLogAttr(nil, attrs[i])
		}
		return slog.GroupValue(attrs...)
	}
	return value
}
