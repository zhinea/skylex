package installer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
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
	out, err := runCommand(ctx, cmd, log)
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runCommand(ctx context.Context, cmd *exec.Cmd, log LogSink) ([]byte, error) {
	if log == nil {
		return cmd.CombinedOutput()
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var outMu sync.Mutex
	var output []byte
	scan := func(r io.Reader, level string) {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if level == "error" {
				log.Error(line)
			} else {
				log.Info(line)
			}
			outMu.Lock()
			output = append(output, line...)
			output = append(output, '\n')
			outMu.Unlock()
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); scan(stdoutPipe, "info") }()
	go func() { defer wg.Done(); scan(stderrPipe, "error") }()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		wg.Wait()
		return output, ctx.Err()
	case err := <-done:
		wg.Wait()
		return output, err
	}
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
