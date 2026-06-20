package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type LogSink interface {
	Info(message string)
	Error(message string)
}

type Installer interface {
	Install(ctx context.Context, cfg InstallConfig, log LogSink) error
	Purge(ctx context.Context, cfg InstallConfig, log LogSink) error
}

type InstallConfig struct {
	Version   string
	DataDir   string
	BinDir    string
	Port      int
	Superuser string
	Password  string
	ClusterID string
}

func run(ctx context.Context, log LogSink, name string, args ...string) error {
	if log != nil {
		log.Info(formatCommand(name, args...))
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if len(out) > 0 && log != nil {
		for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
			if line != "" {
				log.Info(line)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func formatCommand(name string, args ...string) string {
	if len(args) == 0 {
		return "$ " + name
	}
	return "$ " + name + " " + strings.Join(args, " ")
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
