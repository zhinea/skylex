package models

import "time"

type StorageType string

const (
	StorageTypeS3 StorageType = "s3"
)

type StorageConfig struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Type        StorageType `json:"type"`
	Endpoint    string      `json:"endpoint"`
	Bucket      string      `json:"bucket"`
	Region      string      `json:"region"`
	AccessKeyID string      `json:"access_key_id"`
	SecretKey   string      `json:"secret_key"`
	UseSSL      bool        `json:"use_ssl"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}