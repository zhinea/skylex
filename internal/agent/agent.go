package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

type Agent struct {
	cfg     Config
	log     *slog.Logger
	agentID string
}

func New(cfg Config) (*Agent, error) {
	log := NewLogger(cfg.LogLevel, cfg.LogFormat)

	if cfg.Hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("get hostname: %w", err)
		}
		cfg.Hostname = hostname
	}

	return &Agent{
		cfg: cfg,
		log: log,
	}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	a.log.Info("starting skylex agent",
		"version", "0.1.0",
		"hostname", a.cfg.Hostname,
		"server_addr", a.cfg.ServerAddr,
	)

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.heartbeatLoop(ctx)
	})

	g.Go(func() error {
		return a.commandLoop(ctx)
	})

	<-ctx.Done()
	a.log.Info("shutting down skylex agent")

	return g.Wait()
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
	a.log.Debug("sending heartbeat", "agent_id", a.agentID)
	return nil
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
	a.log.Debug("fetching commands", "agent_id", a.agentID)
	return nil
}