package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
)

type ClusterRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

func NewClusterRepository(conn *sql.DB, log *slog.Logger) *ClusterRepository {
	return &ClusterRepository{conn: conn, log: log}
}

func (r *ClusterRepository) Create(ctx context.Context, name, storageConfigID, dataDir string, engine models.EngineType, version string, mode models.ReplicationMode, replicas int, pitrEnabled bool, labels map[string]string) (*models.Cluster, error) {
	clusterID := id.New()
	now := time.Now().UTC()

	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, fmt.Errorf("marshal labels: %w", err)
	}

	pitrInt := boolToInt(pitrEnabled)

	_, err = r.conn.ExecContext(ctx,
		`INSERT INTO clusters (id, name, engine, version, replication_mode, replica_count, storage_config_id, data_dir, pitr_enabled, status, labels, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		clusterID, name, engine, version, mode, replicas, storageConfigID, dataDir, pitrInt, models.ClusterStatusCreating, string(labelsJSON), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert cluster: %w", err)
	}

	return &models.Cluster{
		ID:              clusterID,
		Name:            name,
		Engine:          engine,
		Version:         version,
		ReplicationMode: mode,
		Replicas:        replicas,
		StorageConfigID: storageConfigID,
		DataDir:         dataDir,
		PITREnabled:     pitrEnabled,
		Status:          models.ClusterStatusCreating,
		Tags:            labels,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func (r *ClusterRepository) GetByID(ctx context.Context, id string) (*models.Cluster, error) {
	return r.scanCluster(r.conn.QueryRowContext(ctx,
		`SELECT id, name, engine, version, replication_mode, replica_count, storage_config_id, data_dir, pitr_enabled, status, labels, created_at, updated_at
		 FROM clusters WHERE id = ?`, id))
}

func (r *ClusterRepository) GetByName(ctx context.Context, name string) (*models.Cluster, error) {
	return r.scanCluster(r.conn.QueryRowContext(ctx,
		`SELECT id, name, engine, version, replication_mode, replica_count, storage_config_id, data_dir, pitr_enabled, status, labels, created_at, updated_at
		 FROM clusters WHERE name = ?`, name))
}

func (r *ClusterRepository) List(ctx context.Context, offset, limit int) ([]*models.Cluster, int, error) {
	var total int
	if err := r.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM clusters`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count clusters: %w", err)
	}

	rows, err := r.conn.QueryContext(ctx,
		`SELECT id, name, engine, version, replication_mode, replica_count, storage_config_id, data_dir, pitr_enabled, status, labels, created_at, updated_at
		 FROM clusters ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()

	var clusters []*models.Cluster
	for rows.Next() {
		c, err := scanClusterRow(rows)
		if err != nil {
			return nil, 0, err
		}
		clusters = append(clusters, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate clusters: %w", err)
	}

	return clusters, total, nil
}

func (r *ClusterRepository) UpdateStatus(ctx context.Context, id string, status models.ClusterStatus) error {
	_, err := r.conn.ExecContext(ctx,
		`UPDATE clusters SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update cluster status: %w", err)
	}
	return nil
}

func (r *ClusterRepository) Delete(ctx context.Context, id string) error {
	_, err := r.conn.ExecContext(ctx, `DELETE FROM clusters WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete cluster: %w", err)
	}
	return nil
}

func (r *ClusterRepository) scanCluster(row *sql.Row) (*models.Cluster, error) {
	var c models.Cluster
	var labelsJSON string
	var pitrInt int

	err := row.Scan(&c.ID, &c.Name, &c.Engine, &c.Version, &c.ReplicationMode, &c.Replicas,
		&c.StorageConfigID, &c.DataDir, &pitrInt, &c.Status, &labelsJSON, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan cluster: %w", err)
	}

	c.PITREnabled = intToBool(pitrInt)
	c.Tags = unmarshalLabels(labelsJSON)
	return &c, nil
}

func scanClusterRow(rows *sql.Rows) (*models.Cluster, error) {
	var c models.Cluster
	var labelsJSON string
	var pitrInt int

	if err := rows.Scan(&c.ID, &c.Name, &c.Engine, &c.Version, &c.ReplicationMode, &c.Replicas,
		&c.StorageConfigID, &c.DataDir, &pitrInt, &c.Status, &labelsJSON, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan cluster row: %w", err)
	}

	c.PITREnabled = intToBool(pitrInt)
	c.Tags = unmarshalLabels(labelsJSON)
	return &c, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool {
	return i == 1
}

func unmarshalLabels(raw string) map[string]string {
	labels := make(map[string]string)
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &labels); err != nil {
			labels = make(map[string]string)
		}
	}
	return labels
}