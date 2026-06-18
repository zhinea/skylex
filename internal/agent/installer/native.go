package installer

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type NativeInstaller struct{}

func (NativeInstaller) Install(ctx context.Context, cfg InstallConfig, log LogSink) error {
	pm, err := detectPackageManager()
	if err != nil {
		return err
	}
	if log != nil {
		log.Info(fmt.Sprintf("detected package manager: %s", pm))
	}

	switch pm {
	case "apt-get":
		if err := run(ctx, log, "apt-get", "update"); err != nil {
			return err
		}
		return run(ctx, log, "apt-get", "install", "-y", "--no-install-recommends", "postgresql-"+cfg.Version, "postgresql-client-"+cfg.Version)
	case "dnf":
		return run(ctx, log, "dnf", "install", "-y", "postgresql"+cfg.Version, "postgresql"+cfg.Version+"-server")
	case "apk":
		pkg := "postgresql" + cfg.Version
		return run(ctx, log, "apk", "add", "--no-cache", pkg, pkg+"-client")
	case "zypper":
		return run(ctx, log, "zypper", "--non-interactive", "install", "postgresql"+cfg.Version, "postgresql"+cfg.Version+"-server")
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
}

func (NativeInstaller) Purge(ctx context.Context, cfg InstallConfig, log LogSink) error {
	pm, err := detectPackageManager()
	if err != nil {
		return err
	}

	switch pm {
	case "apt-get":
		return run(ctx, log, "apt-get", "purge", "-y", "postgresql-"+cfg.Version, "postgresql-client-"+cfg.Version)
	case "dnf":
		return run(ctx, log, "dnf", "remove", "-y", "postgresql"+cfg.Version, "postgresql"+cfg.Version+"-server")
	case "apk":
		pkg := "postgresql" + cfg.Version
		return run(ctx, log, "apk", "del", pkg, pkg+"-client")
	case "zypper":
		return run(ctx, log, "zypper", "--non-interactive", "remove", "postgresql"+cfg.Version, "postgresql"+cfg.Version+"-server")
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
}

func DetectNativeBinDir(ctx context.Context, configuredBinDir string) string {
	if configuredBinDir != "" && commandAt(filepath.Join(configuredBinDir, "postgres")) {
		return configuredBinDir
	}
	if path, err := exec.LookPath("postgres"); err == nil {
		return filepath.Dir(path)
	}
	if path, err := exec.LookPath("pg_ctl"); err == nil {
		return filepath.Dir(path)
	}
	return configuredBinDir
}

func DetectNativeVersion(ctx context.Context, fallback string) string {
	if out, err := output(ctx, "pg_config", "--version"); err == nil && out != "" {
		return strings.TrimPrefix(out, "PostgreSQL ")
	}
	if out, err := output(ctx, "postgres", "--version"); err == nil && out != "" {
		return strings.TrimPrefix(strings.TrimPrefix(out, "postgres (PostgreSQL) "), "postgres ")
	}
	return fallback
}

func detectPackageManager() (string, error) {
	for _, name := range []string{"apt-get", "dnf", "apk", "zypper"} {
		if commandExists(name) {
			return name, nil
		}
	}
	return "", fmt.Errorf("no supported package manager found (supported: apt-get, dnf, apk, zypper)")
}

func commandAt(path string) bool {
	if path == "" {
		return false
	}
	_, err := exec.LookPath(path)
	return err == nil
}
