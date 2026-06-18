package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type DockerInstaller struct{}

const dockerContainerName = "skylex-postgres"

func (DockerInstaller) Install(ctx context.Context, cfg InstallConfig, log LogSink) error {
	if !commandExists("docker") {
		return fmt.Errorf("docker binary not found")
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	image := "postgres:" + cfg.Version
	if err := run(ctx, log, "docker", "pull", image); err != nil {
		return err
	}

	if runningContainer(ctx) {
		if log != nil {
			log.Info("PostgreSQL container already running")
		}
		return nil
	}

	_ = run(ctx, log, "docker", "rm", dockerContainerName)
	return run(ctx, log, "docker", "run", "-d",
		"--name", dockerContainerName,
		"--restart", "unless-stopped",
		"-p", fmt.Sprintf("%d:5432", cfg.Port),
		"-e", "POSTGRES_USER="+cfg.Superuser,
		"-e", "POSTGRES_PASSWORD="+cfg.Password,
		"-v", filepath.Clean(cfg.DataDir)+":/var/lib/postgresql/data",
		image,
	)
}

func (DockerInstaller) Purge(ctx context.Context, cfg InstallConfig, log LogSink) error {
	if !commandExists("docker") {
		return fmt.Errorf("docker binary not found")
	}
	return run(ctx, log, "docker", "rm", "-f", dockerContainerName)
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

func DockerContainerName() string { return dockerContainerName }

func runningContainer(ctx context.Context) bool {
	out, err := output(ctx, "docker", "inspect", "-f", "{{.State.Running}}", dockerContainerName)
	return err == nil && out == "true"
}
