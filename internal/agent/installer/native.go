package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type NativeInstaller struct{}

const (
	PreflightNothingFound = "NOTHING_FOUND"
	PreflightPGExists     = "PG_EXISTS"
)

type PreflightResult struct {
	State           string `json:"state"`
	Version         string `json:"version"`
	DataDir         string `json:"data_dir"`
	DataPresent     bool   `json:"data_present"`
	DataInitialized bool   `json:"data_initialized"`
}

func (r PreflightResult) Details() string {
	if r.State == PreflightNothingFound {
		return "no native PostgreSQL installation or data directory content found"
	}
	version := r.Version
	if version == "" {
		version = "unknown"
	}
	return fmt.Sprintf("existing PostgreSQL/data detected: version=%s data_dir=%s data_present=%v data_initialized=%v", version, r.DataDir, r.DataPresent, r.DataInitialized)
}

func (NativeInstaller) Preflight(ctx context.Context, cfg InstallConfig, log LogSink) (PreflightResult, error) {
	installed := false
	version := ""
	if out, err := output(ctx, "pg_config", "--version"); err == nil && out != "" {
		installed = true
		version = out
	} else if out, err := output(ctx, "postgres", "--version"); err == nil && out != "" {
		installed = true
		version = out
	}

	dataPresent, dataInitialized, err := inspectDataDir(cfg.DataDir)
	if err != nil {
		return PreflightResult{}, err
	}

	state := PreflightNothingFound
	if installed || dataPresent {
		state = PreflightPGExists
	}

	result := PreflightResult{
		State:           state,
		Version:         version,
		DataDir:         filepath.Clean(cfg.DataDir),
		DataPresent:     dataPresent,
		DataInitialized: dataInitialized,
	}
	if log != nil {
		log.Info(result.Details())
	}
	return result, nil
}

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
		if err := runPrivileged(ctx, log, "apt-get", "update"); err != nil {
			return err
		}
		if err := runPrivileged(ctx, log, "apt-get", "install", "-y", "--no-install-recommends", "postgresql-"+cfg.Version, "postgresql-client-"+cfg.Version); err != nil {
			return err
		}
	case "dnf":
		if err := runPrivileged(ctx, log, "dnf", "install", "-y", "postgresql"+cfg.Version, "postgresql"+cfg.Version+"-server"); err != nil {
			return err
		}
	case "apk":
		pkg := "postgresql" + cfg.Version
		if err := runPrivileged(ctx, log, "apk", "add", "--no-cache", pkg, pkg+"-client"); err != nil {
			return err
		}
	case "zypper":
		if err := runPrivileged(ctx, log, "zypper", "--non-interactive", "install", "postgresql"+cfg.Version, "postgresql"+cfg.Version+"-server"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
	return ensureDataDirWritable(ctx, cfg.DataDir, log)
}

func (NativeInstaller) Purge(ctx context.Context, cfg InstallConfig, log LogSink) error {
	pm, err := detectPackageManager()
	if err != nil {
		return err
	}

	stopNativePostgres(ctx, cfg, log)

	var purgeErr error
	switch pm {
	case "apt-get":
		purgeErr = runPrivileged(ctx, log, "apt-get", "purge", "-y", "postgresql-"+cfg.Version, "postgresql-client-"+cfg.Version)
	case "dnf":
		purgeErr = runPrivileged(ctx, log, "dnf", "remove", "-y", "postgresql"+cfg.Version, "postgresql"+cfg.Version+"-server")
	case "apk":
		pkg := "postgresql" + cfg.Version
		purgeErr = runPrivileged(ctx, log, "apk", "del", pkg, pkg+"-client")
	case "zypper":
		purgeErr = runPrivileged(ctx, log, "zypper", "--non-interactive", "remove", "postgresql"+cfg.Version, "postgresql"+cfg.Version+"-server")
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
	if purgeErr != nil {
		return purgeErr
	}

	if err := removeDataDir(ctx, cfg.DataDir, log); err != nil {
		return err
	}
	if log != nil {
		log.Info(fmt.Sprintf("removed PostgreSQL data directory: %s", filepath.Clean(cfg.DataDir)))
	}
	return nil
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

func inspectDataDir(dataDir string) (dataPresent bool, dataInitialized bool, err error) {
	clean := filepath.Clean(dataDir)
	if clean == "." || clean == string(filepath.Separator) {
		return false, false, fmt.Errorf("unsafe data directory: %q", dataDir)
	}
	if _, err := os.Stat(filepath.Join(clean, "PG_VERSION")); err == nil {
		return true, true, nil
	} else if !os.IsNotExist(err) {
		return false, false, fmt.Errorf("inspect PG_VERSION: %w", err)
	}
	entries, err := os.ReadDir(clean)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("inspect data dir: %w", err)
	}
	return len(entries) > 0, false, nil
}

func stopNativePostgres(ctx context.Context, cfg InstallConfig, log LogSink) {
	binDir := DetectNativeBinDir(ctx, cfg.BinDir)
	if binDir != "" && commandAt(filepath.Join(binDir, "pg_ctl")) {
		if err := run(ctx, log, filepath.Join(binDir, "pg_ctl"), "stop", "-D", filepath.Clean(cfg.DataDir), "-m", "fast", "-w", "-t", "30"); err == nil {
			return
		}
	}
	if commandExists("systemctl") {
		_ = runPrivileged(ctx, log, "systemctl", "stop", "postgresql")
		_ = runPrivileged(ctx, log, "systemctl", "stop", "postgresql@"+cfg.Version+"-main")
	}
}

func removeDataDir(ctx context.Context, dataDir string, log LogSink) error {
	clean := filepath.Clean(dataDir)
	protected := map[string]bool{
		"/": true, ".": true, "": true,
		"/etc": true, "/home": true, "/tmp": true,
		"/usr": true, "/var": true, "/var/lib": true, "/var/lib/postgresql": true,
	}
	if !filepath.IsAbs(clean) || protected[clean] {
		return fmt.Errorf("refusing to remove unsafe data directory %q", dataDir)
	}
	return runPrivileged(ctx, log, "rm", "-rf", "--", clean)
}

func ensureDataDirWritable(ctx context.Context, dataDir string, log LogSink) error {
	clean := filepath.Clean(dataDir)
	if clean == "." || clean == string(filepath.Separator) || !filepath.IsAbs(clean) {
		return fmt.Errorf("unsafe data directory: %q", dataDir)
	}
	if err := runPrivileged(ctx, log, "mkdir", "-p", clean); err != nil {
		return fmt.Errorf("create PostgreSQL data directory: %w", err)
	}
	owner := fmt.Sprintf("%d:%d", os.Geteuid(), os.Getegid())
	if err := runPrivileged(ctx, log, "chown", "-R", owner, clean); err != nil {
		return fmt.Errorf("set PostgreSQL data directory owner: %w", err)
	}
	return nil
}
