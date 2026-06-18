package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/zhinea/skylex/internal/id"
)

// ClusterSettingsRepository stores PostgreSQL parameters configured for a
// cluster.  Settings are applied by agent commands and are deliberately
// scoped per-cluster so all nodes converge to the same configuration.
type ClusterSettingsRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

func NewClusterSettingsRepository(conn *sql.DB, log *slog.Logger) *ClusterSettingsRepository {
	return &ClusterSettingsRepository{conn: conn, log: log}
}

// ClusterSetting is a single key/value pair persisted for a cluster.
type ClusterSetting struct {
	ID        string
	ClusterID string
	Key       string
	Value     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GetByClusterID returns all settings for the cluster as a parameters map.
// An empty but non-nil map is returned when no settings exist.
func (r *ClusterSettingsRepository) GetByClusterID(ctx context.Context, clusterID string) (map[string]string, error) {
	parameters := make(map[string]string)

	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT key, value FROM cluster_settings WHERE cluster_id = ? ORDER BY key ASC`),
		clusterID)
	if err != nil {
		return nil, fmt.Errorf("query cluster settings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan cluster setting: %w", err)
		}
		parameters[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cluster settings: %w", err)
	}

	return parameters, nil
}

// ListKeys returns the sorted list of configured keys for a cluster.
func (r *ClusterSettingsRepository) ListKeys(ctx context.Context, clusterID string) ([]string, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT key FROM cluster_settings WHERE cluster_id = ? ORDER BY key ASC`),
		clusterID)
	if err != nil {
		return nil, fmt.Errorf("query cluster setting keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan cluster setting key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cluster setting keys: %w", err)
	}

	return keys, nil
}

// Set stores a single parameter, creating or replacing the existing row.
func (r *ClusterSettingsRepository) Set(ctx context.Context, clusterID, key, value string) error {
	now := time.Now().UTC()

	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set cluster setting: %w", err)
	}
	defer tx.Rollback()

	// Upsert path for SQLite (no native ON CONFLICT on older versions) and
	// PostgreSQL.  We could branch on driver, but delete-then-insert keeps
	// both dialects identical and the operation is low frequency.
	if _, err := tx.ExecContext(ctx,
		Rebind(`DELETE FROM cluster_settings WHERE cluster_id = ? AND key = ?`),
		clusterID, key); err != nil {
		return fmt.Errorf("delete old cluster setting: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		Rebind(`INSERT INTO cluster_settings (id, cluster_id, key, value, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`),
		id.New(), clusterID, key, value, now, now); err != nil {
		return fmt.Errorf("insert cluster setting: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set cluster setting: %w", err)
	}
	return nil
}

// Delete removes a single parameter from the cluster settings.
func (r *ClusterSettingsRepository) Delete(ctx context.Context, clusterID, key string) error {
	_, err := r.conn.ExecContext(ctx,
		Rebind(`DELETE FROM cluster_settings WHERE cluster_id = ? AND key = ?`),
		clusterID, key)
	if err != nil {
		return fmt.Errorf("delete cluster setting: %w", err)
	}
	return nil
}

// ReplaceAll atomically replaces all settings for a cluster with the provided
// parameters map.  Keys not present in the map are deleted.  The caller is
// responsible for validating keys and values.
func (r *ClusterSettingsRepository) ReplaceAll(ctx context.Context, clusterID string, parameters map[string]string) error {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace cluster settings: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, Rebind(`DELETE FROM cluster_settings WHERE cluster_id = ?`), clusterID); err != nil {
		return fmt.Errorf("delete cluster settings: %w", err)
	}

	if len(parameters) > 0 {
		stmt, err := tx.PrepareContext(ctx,
			Rebind(`INSERT INTO cluster_settings (id, cluster_id, key, value, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)`))
		if err != nil {
			return fmt.Errorf("prepare cluster settings insert: %w", err)
		}
		defer stmt.Close()

		now := time.Now().UTC()
		keys := make([]string, 0, len(parameters))
		for k := range parameters {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			if _, err := stmt.ExecContext(ctx, id.New(), clusterID, key, parameters[key], now, now); err != nil {
				return fmt.Errorf("insert cluster setting %s: %w", key, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace cluster settings: %w", err)
	}
	return nil
}
