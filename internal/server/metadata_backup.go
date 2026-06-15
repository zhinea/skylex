package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/zhinea/skylex/internal/db"
)

type MetadataBackup struct {
	db     *db.DB
	dsn    string
	dir    string
	log    *slog.Logger
}

func NewMetadataBackup(database *db.DB, dsn string, backupDir string, log *slog.Logger) *MetadataBackup {
	return &MetadataBackup{
		db:  database,
		dsn: dsn,
		dir: backupDir,
		log: log,
	}
}

func (m *MetadataBackup) Backup(ctx context.Context) (string, error) {
	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	timestamp := time.Now().UTC().Format("20060102T150405Z")

	switch m.db.Driver() {
	case "postgres", "pgx":
		return m.backupPostgres(ctx, timestamp)
	default:
		return m.backupSQLite(ctx, timestamp)
	}
}

func (m *MetadataBackup) restorePostgres(ctx context.Context, backupPath string) error {
	cmd := exec.CommandContext(ctx, "pg_restore", "-c", "-d", m.dsn, backupPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_restore failed: %w", err)
	}
	return nil
}

func (m *MetadataBackup) Restore(ctx context.Context, backupPath string) error {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}

	switch m.db.Driver() {
	case "postgres", "pgx":
		return m.restorePostgres(ctx, backupPath)
	default:
		return m.restoreSQLite(ctx, backupPath)
	}
}

func (m *MetadataBackup) ListBackups(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read backup dir: %w", err)
	}

	var backups []string
	for _, entry := range entries {
		if !entry.IsDir() {
			ext := filepath.Ext(entry.Name())
			if ext == ".db" || ext == ".dump" {
				backups = append(backups, filepath.Join(m.dir, entry.Name()))
			}
		}
	}
	return backups, nil
}

func (m *MetadataBackup) backupPostgres(ctx context.Context, timestamp string) (string, error) {
	backupFile := filepath.Join(m.dir, fmt.Sprintf("skylex-metadata-%s.dump", timestamp))

	cmd := exec.CommandContext(ctx, "pg_dump", "--no-owner", "--no-acl", "-Fc", "-d", m.dsn, "-f", backupFile)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		os.Remove(backupFile)
		return "", fmt.Errorf("pg_dump failed: %w", err)
	}

	m.log.Info("metadata backup created (pg_dump)", "path", backupFile)
	return backupFile, nil
}

func (m *MetadataBackup) backupSQLite(ctx context.Context, timestamp string) (string, error) {
	conn := m.db.Conn()

	var srcPath string
	row := conn.QueryRowContext(ctx, "PRAGMA database_list")
	var seq int
	var name, path string
	if err := row.Scan(&seq, &name, &path); err != nil {
		return "", fmt.Errorf("get database path: %w", err)
	}
	srcPath = path

	backupFile := filepath.Join(m.dir, fmt.Sprintf("skylex-metadata-%s.db", timestamp))

	src, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("open source db: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupFile)
	if err != nil {
		return "", fmt.Errorf("create backup file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(backupFile)
		return "", fmt.Errorf("copy database: %w", err)
	}

	m.log.Info("metadata backup created", "path", backupFile)
	return backupFile, nil
}

func (m *MetadataBackup) restoreSQLite(ctx context.Context, backupPath string) error {
	conn := m.db.Conn()

	var dstPath string
	row := conn.QueryRowContext(ctx, "PRAGMA database_list")
	var seq int
	var name, path string
	if err := row.Scan(&seq, &name, &path); err != nil {
		return fmt.Errorf("get database path: %w", err)
	}
	dstPath = path

	maxRetries := 3
	for i := range maxRetries {
		if err := conn.Close(); err != nil {
			m.log.Warn("close db for restore failed", "attempt", i+1, "error", err)
			time.Sleep(time.Second)
			continue
		}
		break
	}

	src, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create target db: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(dstPath)
		return fmt.Errorf("copy database: %w", err)
	}
	dst.Close()

	m.log.Info("metadata restored from backup", "path", backupPath)
	return nil
}