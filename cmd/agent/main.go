package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof" // registered only when SKYLEX_AGENT_PPROF is set; see maybeStartPprof
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/zhinea/skylex/internal/agent"
	"gopkg.in/yaml.v3"
)

func main() {
	// Bound the Go runtime's memory footprint before anything allocates. The
	// agent is a long-lived idle poller, so without a soft limit the heap
	// high-water mark (command execution, /proc scraping, exec subprocesses)
	// stays resident and RSS creeps into the hundreds of MB. GOMEMLIMIT makes
	// the GC keep the heap near the target; a lower GOGC trades a little CPU
	// for tighter, more frequent collection. Both are overridable via the
	// standard env vars for operators who need different behavior.
	applyRuntimeMemoryDefaults()
	maybeStartPprof()

	var (
		configPath = flag.String("config", "/etc/skylex/agent.yaml", "path to agent config file")
		serverAddr = flag.String("server", "", "control plane gRPC address (host:port)")
		agentToken = flag.String("token", "", "agent registration token")
		tokenFile  = flag.String("token-file", "", "path to a file containing the agent registration token")
		hostname   = flag.String("hostname", "", "hostname reported to the control plane")
		port       = flag.Int("port", 0, "PostgreSQL port on this machine")
		dataDir    = flag.String("data-dir", "", "PostgreSQL data directory")
		logLevel   = flag.String("log-level", "", "log level: debug, info, warn, error")
		logFormat  = flag.String("log-format", "", "log format: json, text")
		logFile    = flag.String("log-file", "", "path to append agent logs; empty disables file logging")
	)
	flag.Parse()

	cfg := agent.DefaultConfig()

	// 1. Load config file (optional). Permission-denied on the default
	// system path is expected when running as a non-privileged user and
	// all settings are supplied via flags/env, so don't warn about it.
	if err := loadConfigFile(*configPath, &cfg); err != nil && !errors.Is(err, os.ErrPermission) {
		fmt.Fprintf(os.Stderr, "warning: failed to load config file %s: %v\n", *configPath, err)
	}

	// 2. Apply env vars (backwards compatibility).
	cfg = applyEnv(cfg)

	// 3. Apply CLI flags (highest precedence).
	if *agentToken != "" && *tokenFile != "" {
		fmt.Fprintln(os.Stderr, "error: only one of --token or --token-file may be provided")
		os.Exit(1)
	}
	if *serverAddr != "" {
		cfg.ServerAddr = *serverAddr
	}
	if *agentToken != "" {
		cfg.AgentToken = *agentToken
	}
	if *tokenFile != "" {
		tok, err := readTokenFile(*tokenFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: read token file: %v\n", err)
			os.Exit(1)
		}
		cfg.AgentToken = tok
	}
	if *hostname != "" {
		cfg.Hostname = *hostname
	}
	if *port != 0 {
		cfg.Port = *port
	}
	if *dataDir != "" {
		cfg.PGDataDir = *dataDir
	}
	if *logLevel != "" {
		cfg.LogLevel = *logLevel
	}
	if *logFormat != "" {
		cfg.LogFormat = *logFormat
	}
	if *logFile != "" {
		cfg.LogFile = *logFile
	}

	if cfg.Hostname == "" {
		h, _ := os.Hostname()
		cfg.Hostname = h
	}

	if agent.IsDeactivated(cfg) {
		fmt.Fprintln(os.Stderr, "skylex-agent is deactivated; reinstall the agent to reactivate this node")
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		<-ctx.Done()
		return
	}

	if cfg.AgentToken == "" {
		fmt.Fprintln(os.Stderr, "error: agent token is required; set --token, --token-file, SKYLEX_AGENT_TOKEN, or agent_token in the config file")
		os.Exit(1)
	}

	ag, err := agent.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create agent: %v\n", err)
		os.Exit(1)
	}

	if err := ag.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "agent error: %v\n", err)
		os.Exit(1)
	}
}

func loadConfigFile(path string, cfg *agent.Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		Result:           cfg,
	})
	if err != nil {
		return fmt.Errorf("create decoder: %w", err)
	}
	if err := decoder.Decode(raw); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	return nil
}

func applyEnv(cfg agent.Config) agent.Config {
	if v := os.Getenv("SKYLEX_AGENT_TOKEN"); v != "" {
		cfg.AgentToken = v
	}
	if v := os.Getenv("SKYLEX_SERVER_ADDR"); v != "" {
		cfg.ServerAddr = v
	}
	if v := os.Getenv("SKYLEX_HOSTNAME"); v != "" {
		cfg.Hostname = v
	}
	if v := os.Getenv("SKYLEX_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Port = port
		}
	}
	if v := os.Getenv("SKYLEX_PG_DATA_DIR"); v != "" {
		cfg.PGDataDir = v
	}
	if v := os.Getenv("SKYLEX_AGENT_LOG_FILE"); v != "" {
		cfg.LogFile = v
	}
	return cfg
}

// applyRuntimeMemoryDefaults sets a soft heap ceiling and a tighter GC target
// for the long-lived agent process. Both honor the operator's environment: if
// GOMEMLIMIT or GOGC are already set, the Go runtime has already read them and
// we leave them untouched.
func applyRuntimeMemoryDefaults() {
	// 64 MiB soft limit. The agent's steady-state live set is a few MB; this
	// leaves generous headroom for command execution bursts while keeping RSS
	// far below the previous hundreds of MB. Operators can raise it via the
	// standard GOMEMLIMIT env var (e.g. "256MiB").
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(64 << 20)
	}
	// Collect more eagerly than the default GOGC=100. Trades a little CPU
	// (negligible for an idle poller) for promptly returning freed pages.
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(50)
	}
}

// maybeStartPprof exposes net/http/pprof on a loopback address only when
// SKYLEX_AGENT_PPROF is set (e.g. "127.0.0.1:6060"), so memory can be profiled
// in production without exposing a debug endpoint by default.
func maybeStartPprof() {
	addr := strings.TrimSpace(os.Getenv("SKYLEX_AGENT_PPROF"))
	if addr == "" {
		return
	}
	srv := &http.Server{Addr: addr, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "pprof server error: %v\n", err)
		}
	}()
}

func readTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
