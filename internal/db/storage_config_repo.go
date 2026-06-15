package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
)

type StorageConfigRepository struct {
	conn       *sql.DB
	log        *slog.Logger
	encryptKey []byte
}

func NewStorageConfigRepository(conn *sql.DB, log *slog.Logger, encryptKey []byte) *StorageConfigRepository {
	return &StorageConfigRepository{conn: conn, log: log, encryptKey: encryptKey}
}

func (r *StorageConfigRepository) Create(ctx context.Context, name, storageType, endpoint, bucket, region, accessKeyID, secretKey string, useSSL bool) (*models.StorageConfig, error) {
	cfgID := id.New()
	now := time.Now().UTC()

	encAccessKey, err := crypto.EncryptAES256GCM([]byte(accessKeyID), r.encryptKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt access key: %w", err)
	}
	encSecretKey, err := crypto.EncryptAES256GCM([]byte(secretKey), r.encryptKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret key: %w", err)
	}

	sslInt := boolToInt(useSSL)

	_, err = r.conn.ExecContext(ctx,
		`INSERT INTO storage_configs (id, name, type, endpoint, bucket, region, access_key_id, secret_key, use_ssl, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cfgID, name, storageType, endpoint, bucket, region, string(encAccessKey), string(encSecretKey), sslInt, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert storage config: %w", err)
	}

	return &models.StorageConfig{
		ID:        cfgID,
		Name:      name,
		Type:      models.StorageType(storageType),
		Endpoint:  endpoint,
		Bucket:    bucket,
		Region:    region,
		UseSSL:    useSSL,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (r *StorageConfigRepository) GetByID(ctx context.Context, id string) (*models.StorageConfig, error) {
	return r.scanConfig(r.conn.QueryRowContext(ctx,
		`SELECT id, name, type, endpoint, bucket, region, access_key_id, secret_key, use_ssl, created_at, updated_at
		 FROM storage_configs WHERE id = ?`, id))
}

func (r *StorageConfigRepository) GetByName(ctx context.Context, name string) (*models.StorageConfig, error) {
	return r.scanConfig(r.conn.QueryRowContext(ctx,
		`SELECT id, name, type, endpoint, bucket, region, access_key_id, secret_key, use_ssl, created_at, updated_at
		 FROM storage_configs WHERE name = ?`, name))
}

func (r *StorageConfigRepository) List(ctx context.Context, offset, limit int) ([]*models.StorageConfig, int, error) {
	var total int
	if err := r.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM storage_configs`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count storage configs: %w", err)
	}

	rows, err := r.conn.QueryContext(ctx,
		`SELECT id, name, type, endpoint, bucket, region, access_key_id, secret_key, use_ssl, created_at, updated_at
		 FROM storage_configs ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query storage configs: %w", err)
	}
	defer rows.Close()

	var configs []*models.StorageConfig
	for rows.Next() {
		c, err := scanStorageConfigRow(rows)
		if err != nil {
			return nil, 0, err
		}
		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate storage configs: %w", err)
	}

	return configs, total, nil
}

func (r *StorageConfigRepository) Delete(ctx context.Context, id string) error {
	_, err := r.conn.ExecContext(ctx, `DELETE FROM storage_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete storage config: %w", err)
	}
	return nil
}

func (r *StorageConfigRepository) GetDecryptedCredentials(ctx context.Context, id string) (accessKey, secretKey string, err error) {
	row := r.conn.QueryRowContext(ctx,
		`SELECT access_key_id, secret_key FROM storage_configs WHERE id = ?`, id)

	var encAccessKey, encSecretKey string
	if err := row.Scan(&encAccessKey, &encSecretKey); err != nil {
		return "", "", fmt.Errorf("scan credentials: %w", err)
	}

	accessKeyBytes, err := crypto.DecryptAES256GCM([]byte(encAccessKey), r.encryptKey)
	if err != nil {
		return "", "", fmt.Errorf("decrypt access key: %w", err)
	}
	secretKeyBytes, err := crypto.DecryptAES256GCM([]byte(encSecretKey), r.encryptKey)
	if err != nil {
		return "", "", fmt.Errorf("decrypt secret key: %w", err)
	}

	return string(accessKeyBytes), string(secretKeyBytes), nil
}

func (r *StorageConfigRepository) scanConfig(row *sql.Row) (*models.StorageConfig, error) {
	var c models.StorageConfig
	var useSSLInt int

	err := row.Scan(&c.ID, &c.Name, &c.Type, &c.Endpoint, &c.Bucket, &c.Region,
		&c.AccessKeyID, &c.SecretKey, &useSSLInt, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan storage config: %w", err)
	}

	c.UseSSL = intToBool(useSSLInt)
	return &c, nil
}

func scanStorageConfigRow(rows *sql.Rows) (*models.StorageConfig, error) {
	var c models.StorageConfig
	var useSSLInt int

	if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.Endpoint, &c.Bucket, &c.Region,
		&c.AccessKeyID, &c.SecretKey, &useSSLInt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan storage config row: %w", err)
	}

	c.UseSSL = intToBool(useSSLInt)
	return &c, nil
}

func marshalLabels(labels map[string]string) string {
	if labels == nil {
		return "{}"
	}
	data, err := json.Marshal(labels)
	if err != nil {
		return "{}"
	}
	return string(data)
}