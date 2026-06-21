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

func runPrivileged(ctx context.Context, log LogSink, name string, args ...string) error {
	cmdName, cmdArgs, err := privilegedCommand(os.Geteuid(), commandExists("sudo"), name, args...)
	if err != nil {
		return err
	}
	return run(ctx, log, cmdName, cmdArgs...)
}

func privilegedCommand(euid int, sudoExists bool, name string, args ...string) (string, []string, error) {
	if euid == 0 {
		return name, args, nil
	}
	if !sudoExists {
		return "", nil, fmt.Errorf("%s requires root privileges; install sudo or run skylex-agent with package installation privileges", name)
	}
	cmdArgs := make([]string, 0, len(args)+2)
	cmdArgs = append(cmdArgs, "-n", name)
	cmdArgs = append(cmdArgs, args...)
	return "sudo", cmdArgs, nil
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
