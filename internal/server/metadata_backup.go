package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
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
	conn := m.db.Conn()

	var srcPath string
	row := conn.QueryRowContext(ctx, "PRAGMA database_list")
	var seq int
	var name, path string
	if err := row.Scan(&seq, &name, &path); err != nil {
		return "", fmt.Errorf("get database path: %w", err)
	}
	srcPath = path

	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	timestamp := time.Now().UTC().Format("20060102T150405Z")
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

func (m *MetadataBackup) Restore(ctx context.Context, backupPath string) error {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}

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
	for i := 0; i < maxRetries; i++ {
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
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".db" {
			backups = append(backups, filepath.Join(m.dir, entry.Name()))
		}
	}
	return backups, nil
}