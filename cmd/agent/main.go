package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mitchellh/mapstructure"
	"github.com/zhinea/skylex/internal/agent"
	"gopkg.in/yaml.v3"
)

func main() {
	cfg := agent.DefaultConfig()

	configPath := os.Getenv("SKYLEX_AGENT_CONFIG")
	if configPath == "" {
		configPath = "/etc/skylex/agent.yaml"
	}
	if err := loadConfigFile(configPath, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load config file %s: %v\n", configPath, err)
	}

	if cfg.AgentToken == "" {
		cfg.AgentToken = os.Getenv("SKYLEX_AGENT_TOKEN")
	}
	if cfg.ServerAddr == "" {
		cfg.ServerAddr = os.Getenv("SKYLEX_SERVER_ADDR")
	}
	if cfg.Hostname == "" {
		hostname, _ := os.Hostname()
		cfg.Hostname = hostname
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
