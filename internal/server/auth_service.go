package server

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/db"
	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
)

type AuthService struct {
	skylexv1.UnimplementedAuthServiceServer
	userRepo       *db.UserRepository
	apiKeyRepo     *db.APIKeyRepository
	agentTokenRepo *db.AgentTokenRepository
	jwtManager     *JWTManager
	log            *slog.Logger
}

func NewAuthService(
	userRepo *db.UserRepository,
	apiKeyRepo *db.APIKeyRepository,
	agentTokenRepo *db.AgentTokenRepository,
	jwtManager *JWTManager,
	log *slog.Logger,
) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		apiKeyRepo:     apiKeyRepo,
		agentTokenRepo: agentTokenRepo,
		jwtManager:     jwtManager,
		log:            log,
	}
}

func (s *AuthService) Login(ctx context.Context, req *skylexv1.LoginRequest) (*skylexv1.LoginResponse, error) {
	user, err := s.userRepo.GetByEmail(req.Email)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid email or password")
	}

	if !crypto.VerifyPassword(req.Password, user.PasswordHash) {
		return nil, status.Error(codes.Unauthenticated, "invalid email or password")
	}

	accessToken, err := s.jwtManager.GenerateAccessToken(user)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate access token: %v", err)
	}

	refreshToken, err := s.jwtManager.GenerateRefreshToken(user)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate refresh token: %v", err)
	}

	return &skylexv1.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         userToProto(user),
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, req *skylexv1.RefreshTokenRequest) (*skylexv1.RefreshTokenResponse, error) {
	userID, err := s.jwtManager.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid refresh token")
	}

	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "user not found")
	}

	accessToken, err := s.jwtManager.GenerateAccessToken(user)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate access token: %v", err)
	}

	refreshToken, err := s.jwtManager.GenerateRefreshToken(user)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate refresh token: %v", err)
	}

	return &skylexv1.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) ListUsers(ctx context.Context, req *skylexv1.ListUsersRequest) (*skylexv1.ListUsersResponse, error) {
	page := int(req.Page)
	pageSize := int(req.PageSize)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	users, total, err := s.userRepo.List(page, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list users: %v", err)
	}

	var protoUsers []*skylexv1.User
	for i := range users {
		protoUsers = append(protoUsers, userToProto(&users[i]))
	}

	return &skylexv1.ListUsersResponse{
		Users: protoUsers,
		Pagination: &skylexv1.Pagination{
			Page:     int32(page),
			PageSize: int32(pageSize),
			Total:    int32(total),
		},
	}, nil
}

func (s *AuthService) CreateUser(ctx context.Context, req *skylexv1.CreateUserRequest) (*skylexv1.CreateUserResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	passwordHash, err := crypto.HashPassword(req.Password)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "hash password: %v", err)
	}

	role := models.RoleViewer
	switch req.Role {
	case skylexv1.Role_ROLE_ADMIN:
		role = models.RoleAdmin
	case skylexv1.Role_ROLE_OPERATOR:
		role = models.RoleOperator
	}

	now := time.Now()
	user := &models.User{
		ID:           id.New(),
		Email:        req.Email,
		PasswordHash: passwordHash,
		DisplayName:  req.DisplayName,
		Role:         role,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, status.Errorf(codes.Internal, "create user: %v", err)
	}

	return &skylexv1.CreateUserResponse{User: userToProto(user)}, nil
}

func (s *AuthService) DeleteUser(ctx context.Context, req *skylexv1.DeleteUserRequest) (*skylexv1.DeleteUserResponse, error) {
	if err := s.userRepo.Delete(req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete user: %v", err)
	}
	return &skylexv1.DeleteUserResponse{}, nil
}

func (s *AuthService) CreateAPIKey(ctx context.Context, req *skylexv1.CreateAPIKeyRequest) (*skylexv1.CreateAPIKeyResponse, error) {
	userID := UserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	rawKey, err := crypto.GenerateToken(32)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate key: %v", err)
	}

	keyHash := crypto.HashToken(rawKey)

	apiKey := &models.APIKey{
		ID:        id.New(),
		UserID:    userID,
		Name:      req.Name,
		KeyHash:   keyHash,
		CreatedAt: time.Now(),
	}

	if req.ExpiresAt != nil {
		t := req.ExpiresAt.AsTime()
		apiKey.ExpiresAt = &t
	}

	if err := s.apiKeyRepo.Create(apiKey); err != nil {
		return nil, status.Errorf(codes.Internal, "create api key: %v", err)
	}

	return &skylexv1.CreateAPIKeyResponse{
		ApiKey: apiKeyToProto(apiKey),
		Key:    rawKey,
	}, nil
}

func (s *AuthService) ListAPIKeys(ctx context.Context, req *skylexv1.ListAPIKeysRequest) (*skylexv1.ListAPIKeysResponse, error) {
	userID := UserIDFromContext(ctx)
	if userID == "" {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	keys, err := s.apiKeyRepo.ListByUserID(userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list api keys: %v", err)
	}

	var protoKeys []*skylexv1.APIKey
	for i := range keys {
		protoKeys = append(protoKeys, apiKeyToProto(&keys[i]))
	}

	return &skylexv1.ListAPIKeysResponse{ApiKeys: protoKeys}, nil
}

func (s *AuthService) DeleteAPIKey(ctx context.Context, req *skylexv1.DeleteAPIKeyRequest) (*skylexv1.DeleteAPIKeyResponse, error) {
	if err := s.apiKeyRepo.Delete(req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete api key: %v", err)
	}
	return &skylexv1.DeleteAPIKeyResponse{}, nil
}

func userToProto(u *models.User) *skylexv1.User {
	role := skylexv1.Role_ROLE_VIEWER
	switch u.Role {
	case models.RoleAdmin:
		role = skylexv1.Role_ROLE_ADMIN
	case models.RoleOperator:
		role = skylexv1.Role_ROLE_OPERATOR
	}

	return &skylexv1.User{
		Id:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        role,
		CreatedAt:   timestamppb.New(u.CreatedAt),
	}
}

func apiKeyToProto(k *models.APIKey) *skylexv1.APIKey {
	pk := &skylexv1.APIKey{
		Id:        k.ID,
		Name:      k.Name,
		CreatedAt: timestamppb.New(k.CreatedAt),
	}
	if k.ExpiresAt != nil {
		pk.ExpiresAt = timestamppb.New(*k.ExpiresAt)
	}
	return pk
}