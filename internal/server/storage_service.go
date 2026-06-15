package server

import (
	"context"
	"log/slog"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/backup"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type StorageService struct {
	skylexv1.UnimplementedStorageServiceServer
	configs *db.StorageConfigRepository
	log     *slog.Logger
}

func NewStorageService(configs *db.StorageConfigRepository, log *slog.Logger) *StorageService {
	return &StorageService{configs: configs, log: log}
}

func (s *StorageService) CreateStorageConfig(ctx context.Context, req *skylexv1.CreateStorageConfigRequest) (*skylexv1.CreateStorageConfigResponse, error) {
	existing, err := s.configs.GetByName(ctx, req.GetName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check name: %v", err)
	}
	if existing != nil {
		return nil, status.Errorf(codes.AlreadyExists, "storage config %q already exists", req.GetName())
	}

	cfg, err := s.configs.Create(ctx, req.GetName(), req.GetType(), req.GetEndpoint(), req.GetBucket(), req.GetRegion(), req.GetAccessKey(), req.GetSecretKey(), req.GetUseTls())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create storage config: %v", err)
	}

	return &skylexv1.CreateStorageConfigResponse{
		StorageConfig: storageConfigToProto(cfg),
	}, nil
}

func (s *StorageService) ListStorageConfigs(ctx context.Context, req *skylexv1.ListStorageConfigsRequest) (*skylexv1.ListStorageConfigsResponse, error) {
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	configs, total, err := s.configs.List(ctx, offset, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list storage configs: %v", err)
	}

	var protoConfigs []*skylexv1.StorageConfig
	for _, c := range configs {
		protoConfigs = append(protoConfigs, storageConfigToProto(c))
	}

	return &skylexv1.ListStorageConfigsResponse{
		StorageConfigs: protoConfigs,
		Pagination: &skylexv1.Pagination{
			Page:     int32(page),
			PageSize: int32(pageSize),
			Total:    int32(total),
		},
	}, nil
}

func (s *StorageService) GetStorageConfig(ctx context.Context, req *skylexv1.GetStorageConfigRequest) (*skylexv1.GetStorageConfigResponse, error) {
	cfg, err := s.configs.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get storage config: %v", err)
	}
	if cfg == nil {
		return nil, status.Errorf(codes.NotFound, "storage config %q not found", req.GetId())
	}

	return &skylexv1.GetStorageConfigResponse{
		StorageConfig: storageConfigToProto(cfg),
	}, nil
}

func (s *StorageService) DeleteStorageConfig(ctx context.Context, req *skylexv1.DeleteStorageConfigRequest) (*skylexv1.DeleteStorageConfigResponse, error) {
	cfg, err := s.configs.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get storage config: %v", err)
	}
	if cfg == nil {
		return nil, status.Errorf(codes.NotFound, "storage config %q not found", req.GetId())
	}

	if err := s.configs.Delete(ctx, req.GetId()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete storage config: %v", err)
	}

	return &skylexv1.DeleteStorageConfigResponse{}, nil
}

func (s *StorageService) ValidateStorageConfig(ctx context.Context, req *skylexv1.ValidateStorageConfigRequest) (*skylexv1.ValidateStorageConfigResponse, error) {
	cfg, err := s.configs.GetByID(ctx, req.GetId())
	if err != nil || cfg == nil {
		return &skylexv1.ValidateStorageConfigResponse{Valid: false, Message: "storage config not found"}, nil
	}

	accessKey, secretKey, err := s.configs.GetDecryptedCredentials(ctx, req.GetId())
	if err != nil {
		return &skylexv1.ValidateStorageConfigResponse{Valid: false, Message: "failed to decrypt credentials"}, nil
	}

	s3Client, err := backup.NewS3Client(ctx, backup.S3Config{
		Endpoint:  cfg.Endpoint,
		Bucket:    cfg.Bucket,
		Region:    cfg.Region,
		AccessKey: accessKey,
		SecretKey: secretKey,
		UseSSL:    cfg.UseSSL,
	}, s.log)
	if err != nil {
		return &skylexv1.ValidateStorageConfigResponse{Valid: false, Message: err.Error()}, nil
	}

	if err := s3Client.Validate(ctx); err != nil {
		return &skylexv1.ValidateStorageConfigResponse{Valid: false, Message: err.Error()}, nil
	}

	return &skylexv1.ValidateStorageConfigResponse{Valid: true, Message: "storage config is valid"}, nil
}

func storageConfigToProto(c *models.StorageConfig) *skylexv1.StorageConfig {
	return &skylexv1.StorageConfig{
		Id:        c.ID,
		Name:      c.Name,
		Type:      string(c.Type),
		Endpoint:  c.Endpoint,
		Bucket:    c.Bucket,
		Region:    c.Region,
		UseTls:    c.UseSSL,
		CreatedAt: timestamppb.New(c.CreatedAt),
		UpdatedAt: timestamppb.New(c.UpdatedAt),
	}
}