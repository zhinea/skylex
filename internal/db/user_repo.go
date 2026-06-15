package db

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/models"
)

type UserRepository struct {
	db  *sql.DB
	log *slog.Logger
}

func NewUserRepository(db *sql.DB, log *slog.Logger) *UserRepository {
	return &UserRepository{db: db, log: log}
}

func (r *UserRepository) Create(user *models.User) error {
	query := Rebind(`INSERT INTO users (id, email, password_hash, display_name, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	_, err := r.db.Exec(query, user.ID, user.Email, user.PasswordHash, user.DisplayName, user.Role, user.CreatedAt, user.UpdatedAt)
	return err
}

func (r *UserRepository) GetByEmail(email string) (*models.User, error) {
	query := Rebind(`SELECT id, email, password_hash, display_name, role, created_at, updated_at FROM users WHERE email = ?`)
	user := &models.User{}
	err := r.db.QueryRow(query, email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) GetByID(id string) (*models.User, error) {
	query := Rebind(`SELECT id, email, password_hash, display_name, role, created_at, updated_at FROM users WHERE id = ?`)
	user := &models.User{}
	err := r.db.QueryRow(query, id).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) List(page, pageSize int) ([]models.User, int, error) {
	var total int
	if err := r.db.QueryRow(Rebind("SELECT COUNT(*) FROM users")).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := Rebind(`SELECT id, email, password_hash, display_name, role, created_at, updated_at FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`)
	rows, err := r.db.Query(query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *UserRepository) Delete(id string) error {
	_, err := r.db.Exec(Rebind("DELETE FROM users WHERE id = ?"), id)
	return err
}

type APIKeyRepository struct {
	db  *sql.DB
	log *slog.Logger
}

func NewAPIKeyRepository(db *sql.DB, log *slog.Logger) *APIKeyRepository {
	return &APIKeyRepository{db: db, log: log}
}

func (r *APIKeyRepository) Create(apiKey *models.APIKey) error {
	query := Rebind(`INSERT INTO api_keys (id, user_id, name, key_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`)
	_, err := r.db.Exec(query, apiKey.ID, apiKey.UserID, apiKey.Name, apiKey.KeyHash, apiKey.ExpiresAt, apiKey.CreatedAt)
	return err
}

func (r *APIKeyRepository) GetByKeyHash(hash string) (*models.APIKey, error) {
	query := Rebind(`SELECT id, user_id, name, key_hash, expires_at, created_at FROM api_keys WHERE key_hash = ?`)
	apiKey := &models.APIKey{}
	var expiresAt sql.NullTime
	err := r.db.QueryRow(query, hash).Scan(
		&apiKey.ID, &apiKey.UserID, &apiKey.Name, &apiKey.KeyHash, &expiresAt, &apiKey.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		apiKey.ExpiresAt = &expiresAt.Time
	}
	return apiKey, nil
}

func (r *APIKeyRepository) ListByUserID(userID string) ([]models.APIKey, error) {
	query := Rebind(`SELECT id, user_id, name, key_hash, expires_at, created_at FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`)
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		var expiresAt sql.NullTime
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyHash, &expiresAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			k.ExpiresAt = &expiresAt.Time
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (r *APIKeyRepository) Delete(id string) error {
	_, err := r.db.Exec(Rebind("DELETE FROM api_keys WHERE id = ?"), id)
	return err
}

type AgentTokenRepository struct {
	db  *sql.DB
	log *slog.Logger
}

func NewAgentTokenRepository(db *sql.DB, log *slog.Logger) *AgentTokenRepository {
	return &AgentTokenRepository{db: db, log: log}
}

func (r *AgentTokenRepository) Create(token *models.AgentToken) error {
	query := Rebind(`INSERT INTO agent_tokens (id, name, token_hash, role, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`)
	_, err := r.db.Exec(query, token.ID, token.Name, token.TokenHash, token.Role, token.ExpiresAt, token.CreatedAt)
	return err
}

func (r *AgentTokenRepository) GetByTokenHash(hash string) (*models.AgentToken, error) {
	query := Rebind(`SELECT id, name, token_hash, role, expires_at, created_at FROM agent_tokens WHERE token_hash = ?`)
	token := &models.AgentToken{}
	var expiresAt sql.NullTime
	err := r.db.QueryRow(query, hash).Scan(
		&token.ID, &token.Name, &token.TokenHash, &token.Role, &expiresAt, &token.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Time
	}
	return token, nil
}

func (r *AgentTokenRepository) List() ([]models.AgentToken, error) {
	query := Rebind(`SELECT id, name, token_hash, role, expires_at, created_at FROM agent_tokens ORDER BY created_at DESC`)
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []models.AgentToken
	for rows.Next() {
		var t models.AgentToken
		var expiresAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.TokenHash, &t.Role, &expiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			t.ExpiresAt = &expiresAt.Time
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (r *AgentTokenRepository) Delete(id string) error {
	_, err := r.db.Exec(Rebind("DELETE FROM agent_tokens WHERE id = ?"), id)
	return err
}

func NullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}