package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/mitchellh/mapstructure"
	"github.com/zhinea/skylex/internal/agent"
	"gopkg.in/yaml.v3"
)

func main() {
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
	return cfg
}

func readTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
