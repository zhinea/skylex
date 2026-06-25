// Package pgbackrest wraps the external `pgbackrest` CLI. It deliberately
// depends only on the standard library (os/exec) so that callers which only
// need backup orchestration — notably the agent — do not transitively link the
// server-only backup engine dependencies (minio S3 client, SQLite, cron).
package pgbackrest

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
)

type PgBackRest struct {
	binPath string
	log     *slog.Logger
}

func NewPgBackRest(binPath string, log *slog.Logger) *PgBackRest {
	return &PgBackRest{binPath: binPath, log: log}
}

func (p *PgBackRest) Backup(ctx context.Context, stanza, backupType string, repoPath string, pgDataDir string) (string, error) {
	args := []string{
		"--stanza", stanza,
		"--type", backupType,
		"backup",
	}

	if repoPath != "" {
		args = append(args, "--repo1-path", repoPath)
	}
	if pgDataDir != "" {
		args = append(args, "--pg1-path", pgDataDir)
	}

	cmd := exec.CommandContext(ctx, p.binPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pgbackrest backup failed: %w\n%s", err, string(output))
	}

	p.log.Info("pgbackrest backup completed", "stanza", stanza, "type", backupType)
	return string(output), nil
}

func (p *PgBackRest) Restore(ctx context.Context, stanza, targetTime, targetLSN, repoPath, pgDataDir string) (string, error) {
	args := []string{
		"--stanza", stanza,
		"restore",
	}

	if targetTime != "" {
		args = append(args, "--type", "time", "--target", targetTime)
	} else if targetLSN != "" {
		args = append(args, "--type", "lsn", "--target", targetLSN)
	}

	if repoPath != "" {
		args = append(args, "--repo1-path", repoPath)
	}
	if pgDataDir != "" {
		args = append(args, "--pg1-path", pgDataDir)
	}

	cmd := exec.CommandContext(ctx, p.binPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pgbackrest restore failed: %w\n%s", err, string(output))
	}

	p.log.Info("pgbackrest restore completed", "stanza", stanza)
	return string(output), nil
}

func (p *PgBackRest) StanzaCreate(ctx context.Context, stanza, repoPath, pgDataDir string) error {
	args := []string{
		"--stanza", stanza,
		"stanza-create",
	}

	if repoPath != "" {
		args = append(args, "--repo1-path", repoPath)
	}
	if pgDataDir != "" {
		args = append(args, "--pg1-path", pgDataDir)
	}

	cmd := exec.CommandContext(ctx, p.binPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pgbackrest stanza-create failed: %w\n%s", err, string(output))
	}

	p.log.Info("pgbackrest stanza created", "stanza", stanza)
	return nil
}

func (p *PgBackRest) StanzaCheck(ctx context.Context, stanza, repoPath, pgDataDir string) error {
	args := []string{
		"--stanza", stanza,
		"check",
	}

	if repoPath != "" {
		args = append(args, "--repo1-path", repoPath)
	}
	if pgDataDir != "" {
		args = append(args, "--pg1-path", pgDataDir)
	}

	cmd := exec.CommandContext(ctx, p.binPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pgbackrest check failed: %w\n%s", err, string(output))
	}

	p.log.Info("pgbackrest stanza check passed", "stanza", stanza)
	return nil
}

func (p *PgBackRest) Info(ctx context.Context, stanza, repoPath string) (string, error) {
	args := []string{
		"--stanza", stanza,
		"info",
	}

	if repoPath != "" {
		args = append(args, "--repo1-path", repoPath)
	}

	cmd := exec.CommandContext(ctx, p.binPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pgbackrest info failed: %w\n%s", err, string(output))
	}

	return string(output), nil
}

func (p *PgBackRest) ArchivePush(ctx context.Context, stanza, walPath string) error {
	archiveCmd := filepath.Join(filepath.Dir(p.binPath), "pgbackrest")
	args := []string{
		"--stanza", stanza,
		"archive-push",
		walPath,
	}

	cmd := exec.CommandContext(ctx, archiveCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		p.log.Error("archive push failed", "error", err, "output", string(output))
		return fmt.Errorf("archive push: %w", err)
	}

	return nil
}

func (p *PgBackRest) Expire(ctx context.Context, stanza string, retentionCount, retentionDays int, repoPath string) error {
	args := []string{
		"--stanza", stanza,
	}

	if retentionCount > 0 {
		args = append(args, "--retention-full", fmt.Sprintf("%d", retentionCount))
	}
	if retentionDays > 0 {
		args = append(args, "--repo1-retention-full", fmt.Sprintf("%d", retentionDays))
	}
	if repoPath != "" {
		args = append(args, "--repo1-path", repoPath)
	}

	args = append(args, "expire")

	cmd := exec.CommandContext(ctx, p.binPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pgbackrest expire failed: %w\n%s", err, string(output))
	}

	p.log.Info("pgbackrest expire completed", "stanza", stanza)
	return nil
}