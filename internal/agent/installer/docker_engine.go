package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"time"
)

// ErrDockerNeedsRestart is returned when Docker Engine was installed or the
// agent user was added to the docker group, but the running process still
// cannot access the Docker API. A service restart is required for the new
// group membership to take effect.
var ErrDockerNeedsRestart = errors.New("docker setup completed; restart the skylex-agent service to apply docker group membership")

// EnsureDockerEngine checks whether Docker Engine is installed and reachable.
// If not, it attempts to install it using the distribution's package manager,
// starts the docker service, and adds the current user to the docker group.
// It returns true when the caller must restart the agent process before docker
// commands can be used (group membership changed).
func EnsureDockerEngine(ctx context.Context, log LogSink) (bool, error) {
	if dockerReady(ctx) {
		if log != nil {
			log.Info("docker engine is already installed and reachable")
		}
		return false, nil
	}

	if runtime.GOOS != "linux" {
		return false, fmt.Errorf("automatic docker engine installation is only supported on Linux")
	}

	binaryExists := commandExists("docker")

	if !binaryExists {
		pm, err := detectPackageManager()
		if err != nil {
			return false, fmt.Errorf("cannot install docker engine: %w", err)
		}
		if log != nil {
			log.Info(fmt.Sprintf("installing docker engine using %s", pm))
		}
		if err := installDockerEngine(ctx, pm, log); err != nil {
			return false, fmt.Errorf("install docker engine: %w", err)
		}
	}

	if commandExists("systemctl") {
		_ = run(ctx, log, "systemctl", "enable", "docker")
		_ = run(ctx, log, "systemctl", "start", "docker")
	}

	if dockerReady(ctx) {
		return false, nil
	}

	// The docker binary may exist but the current user cannot talk to the daemon.
	// Add the user to the docker group; a restart is then needed for the change
	// to take effect in the running service.
	currentUser, err := user.Current()
	if err != nil {
		return false, fmt.Errorf("detect current user: %w", err)
	}
	if currentUser.Username == "root" {
		return false, fmt.Errorf("docker engine is present but the daemon is not reachable; check docker service status")
	}

	if log != nil {
		log.Info(fmt.Sprintf("adding user %s to docker group", currentUser.Username))
	}
	_ = run(ctx, log, "usermod", "-aG", "docker", currentUser.Username)

	if dockerReady(ctx) {
		return false, nil
	}
	return true, ErrDockerNeedsRestart
}

func dockerReady(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	cmd.Env = os.Environ()
	err := cmd.Run()
	return err == nil
}

func installDockerEngine(ctx context.Context, pm string, log LogSink) error {
	switch pm {
	case "apt-get":
		if err := run(ctx, log, "apt-get", "update"); err != nil {
			return err
		}
		return run(ctx, log, "apt-get", "install", "-y", "--no-install-recommends", "docker.io")
	case "dnf":
		// Prefer upstream "docker", fall back to the Moby build in RHEL/Fedora derivatives.
		if err := run(ctx, log, "dnf", "install", "-y", "docker"); err != nil {
			if log != nil {
				log.Info("docker package not available; trying moby-engine fallback")
			}
			return run(ctx, log, "dnf", "install", "-y", "moby-engine")
		}
		return nil
	case "apk":
		return run(ctx, log, "apk", "add", "--no-cache", "docker")
	case "zypper":
		return run(ctx, log, "zypper", "--non-interactive", "install", "docker")
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
}
