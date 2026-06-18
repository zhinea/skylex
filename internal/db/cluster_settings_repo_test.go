package db

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func newTestDB(t *testing.T) (*DB, *slog.Logger) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := New(Config{Driver: "sqlite", DSN: ":memory:"}, log)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, log
}

func TestClusterSettingsRepository_GetByClusterID(t *testing.T) {
	db, log := newTestDB(t)
	repo := NewClusterSettingsRepository(db.Conn(), log)
	ctx := context.Background()

	clusterID := "cluster-1"

	// Empty map is returned when no settings exist.
	params, err := repo.GetByClusterID(ctx, clusterID)
	if err != nil {
		t.Fatalf("get empty settings: %v", err)
	}
	if len(params) != 0 {
		t.Fatalf("expected empty params, got %d", len(params))
	}

	if err := repo.Set(ctx, clusterID, "max_connections", "300"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if err := repo.Set(ctx, clusterID, "work_mem", "8MB"); err != nil {
		t.Fatalf("set setting: %v", err)
	}

	params, err = repo.GetByClusterID(ctx, clusterID)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}
	if params["max_connections"] != "300" {
		t.Fatalf("unexpected max_connections: %q", params["max_connections"])
	}

	keys, err := repo.ListKeys(ctx, clusterID)
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 2 || keys[0] != "max_connections" || keys[1] != "work_mem" {
		t.Fatalf("unexpected keys: %v", keys)
	}
}

func TestClusterSettingsRepository_SetReplacesExistingValue(t *testing.T) {
	db, log := newTestDB(t)
	repo := NewClusterSettingsRepository(db.Conn(), log)
	ctx := context.Background()

	clusterID := "cluster-1"
	if err := repo.Set(ctx, clusterID, "shared_buffers", "128MB"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if err := repo.Set(ctx, clusterID, "shared_buffers", "256MB"); err != nil {
		t.Fatalf("set setting again: %v", err)
	}

	params, err := repo.GetByClusterID(ctx, clusterID)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if params["shared_buffers"] != "256MB" {
		t.Fatalf("expected updated value, got %q", params["shared_buffers"])
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
}

func TestClusterSettingsRepository_ReplaceAll(t *testing.T) {
	db, log := newTestDB(t)
	repo := NewClusterSettingsRepository(db.Conn(), log)
	ctx := context.Background()

	clusterID := "cluster-1"
	if err := repo.Set(ctx, clusterID, "wal_level", "replica"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if err := repo.Set(ctx, clusterID, "max_wal_senders", "5"); err != nil {
		t.Fatalf("set setting: %v", err)
	}

	if err := repo.ReplaceAll(ctx, clusterID, map[string]string{"max_connections": "200"}); err != nil {
		t.Fatalf("replace all: %v", err)
	}

	params, err := repo.GetByClusterID(ctx, clusterID)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if len(params) != 1 || params["max_connections"] != "200" {
		t.Fatalf("unexpected params after replace: %v", params)
	}

	// Replacing with an empty map clears all settings.
	if err := repo.ReplaceAll(ctx, clusterID, nil); err != nil {
		t.Fatalf("replace all empty: %v", err)
	}
	params, err = repo.GetByClusterID(ctx, clusterID)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if len(params) != 0 {
		t.Fatalf("expected empty params after clear, got %d", len(params))
	}
}
