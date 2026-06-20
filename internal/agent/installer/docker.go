package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DockerInstaller struct{}

const dockerContainerNamePrefix = "skylex-postgres"

// ComposeDir returns the directory where per-cluster compose files are stored.
func ComposeDir(clusterID string) string {
	return filepath.Join("/etc", "skylex", "clusters", clusterID)
}

// ComposeFilePath returns the path to the docker-compose.yaml for a cluster.
func ComposeFilePath(clusterID string) string {
	return filepath.Join(ComposeDir(clusterID), "docker-compose.yaml")
}

// DockerContainerName returns the deterministic container name for a cluster.
func DockerContainerName(clusterID string) string {
	// Use first 12 chars of cluster ID to keep container name manageable.
	short := clusterID
	if len(short) > 12 {
		short = short[:12]
	}
	return dockerContainerNamePrefix + "-" + short
}

func (DockerInstaller) Install(ctx context.Context, cfg InstallConfig, log LogSink) error {
	if !commandExists("docker") {
		return fmt.Errorf("docker binary not found")
	}
	if cfg.ClusterID == "" {
		return fmt.Errorf("cluster_id is required for docker install")
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	composeDir := ComposeDir(cfg.ClusterID)
	if err := os.MkdirAll(composeDir, 0755); err != nil {
		return fmt.Errorf("create compose dir: %w", err)
	}

	image := "postgres:" + cfg.Version
	containerName := DockerContainerName(cfg.ClusterID)
	composeFile := ComposeFilePath(cfg.ClusterID)

	composeContent := fmt.Sprintf(`# Managed by Skylex — do not edit manually.
services:
  postgres:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:5432"
    environment:
      POSTGRES_USER: %s
      POSTGRES_PASSWORD: %s
    volumes:
      - %s:/var/lib/postgresql/data
`, image, containerName, cfg.Port, cfg.Superuser, cfg.Password, filepath.Clean(cfg.DataDir))

	if err := os.WriteFile(composeFile, []byte(composeContent), 0644); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}

	if log != nil {
		log.Info(fmt.Sprintf("wrote compose file: %s", composeFile))
	}

	// Pull the image first.
	if err := run(ctx, log, "docker", "pull", image); err != nil {
		return err
	}

	// Bring up the container.
	if err := run(ctx, log, "docker", "compose", "-f", composeFile, "up", "-d"); err != nil {
		return err
	}

	return nil
}

func (DockerInstaller) Purge(ctx context.Context, cfg InstallConfig, log LogSink) error {
	if !commandExists("docker") {
		return fmt.Errorf("docker binary not found")
	}
	if cfg.ClusterID == "" {
		return fmt.Errorf("cluster_id is required for docker purge")
	}
	composeFile := ComposeFilePath(cfg.ClusterID)
	// Try compose down first, then fall back to removing the container directly.
	_ = run(ctx, log, "docker", "compose", "-f", composeFile, "down", "-v")
	_ = run(ctx, log, "docker", "rm", "-f", DockerContainerName(cfg.ClusterID))
	_ = os.Remove(composeFile)
	return nil
}

func DockerCommandArgs(dataDir string, port int, extra ...string) []string {
	args := []string{
		"exec",
		"-e", "PGDATA=/var/lib/postgresql/data",
		dockerContainerName,
	}
	args = append(args, extra...)
	return args
}

// dockerContainerName is the legacy default container name (used only for tests).
const dockerContainerName = "skylex-postgres"

// DockerContainerNameLegacy returns the default container name for backwards compatibility.
func DockerContainerNameLegacy() string { return dockerContainerName }

// runningContainer checks if a container with the given name is running.
func runningContainer(ctx context.Context, containerName string) bool {
	out, err := output(ctx, "docker", "inspect", "-f", "{{.State.Running}}", containerName)
	return err == nil && strings.TrimSpace(out) == "true"
}
