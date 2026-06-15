package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zhinea/skylex/internal/agent"
)

func main() {
	cfg := agent.DefaultConfig()

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