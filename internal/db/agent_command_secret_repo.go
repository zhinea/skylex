package db

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/crypto"
	"github.com/zhinea/skylex/internal/id"
)

// AgentCommandSecret stores an encrypted secret keyed by (command_id, key).
// The payload sent to agents contains only the key name; FetchCommand resolves
// and decrypts the ciphertext before forwarding it to the owning agent.
type AgentCommandSecret struct {
	ID         string
	CommandID  string
	Key        string
	Ciphertext string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
}

// AgentCommandSecretRepository manages agent_command_secrets rows.
type AgentCommandSecretRepository struct {
	conn       *sql.DB
	log        *slog.Logger
	encryptKey []byte
}

func NewAgentCommandSecretRepository(conn *sql.DB, log *slog.Logger, encryptKey []byte) *AgentCommandSecretRepository {
	return &AgentCommandSecretRepository{conn: conn, log: log, encryptKey: encryptKey}
}

// StoreSecret encrypts plaintext and stores it under (commandID, key).
func (r *AgentCommandSecretRepository) StoreSecret(ctx context.Context, commandID, key, plaintext string, expiresAt *time.Time) error {
	secretID := id.New()
	now := time.Now().UTC()

	ciphertext, err := crypto.EncryptAES256GCM([]byte(plaintext), r.encryptKey)
	if err != nil {
		return fmt.Errorf("encrypt command secret: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	_, err = r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO agent_command_secrets (id, command_id, key, ciphertext, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (command_id, key) DO UPDATE SET ciphertext = excluded.ciphertext, expires_at = excluded.expires_at`),
		secretID, commandID, key, encoded, now, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert command secret: %w", err)
	}
	return nil
}

// ResolveSecret fetches and decrypts the secret for (commandID, key).
// Returns ("", nil) when no matching row exists (already consumed or never set).
func (r *AgentCommandSecretRepository) ResolveSecret(ctx context.Context, commandID, key string) (string, error) {
	var ciphertext string
	var expiresAt sql.NullTime

	err := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT ciphertext, expires_at FROM agent_command_secrets WHERE command_id = ? AND key = ?`),
		commandID, key,
	).Scan(&ciphertext, &expiresAt)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("fetch command secret: %w", err)
	}

	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
		return "", fmt.Errorf("command secret expired")
	}

	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode command secret: %w", err)
	}

	plaintext, err := crypto.DecryptAES256GCM(ciphertextBytes, r.encryptKey)
	if err != nil {
		return "", fmt.Errorf("decrypt command secret: %w", err)
	}
	return string(plaintext), nil
}

// ResolveAllForCommand returns a map of key→plaintext for all secrets attached to commandID.
func (r *AgentCommandSecretRepository) ResolveAllForCommand(ctx context.Context, commandID string) (map[string]string, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT key, ciphertext, expires_at FROM agent_command_secrets WHERE command_id = ?`),
		commandID,
	)
	if err != nil {
		return nil, fmt.Errorf("query command secrets: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var key, ciphertext string
		var expiresAt sql.NullTime
		if err := rows.Scan(&key, &ciphertext, &expiresAt); err != nil {
			return nil, fmt.Errorf("scan command secret: %w", err)
		}
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			continue // skip expired
		}
		ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertext)
		if err != nil {
			return nil, fmt.Errorf("decode command secret %q: %w", key, err)
		}
		plaintext, err := crypto.DecryptAES256GCM(ciphertextBytes, r.encryptKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt command secret %q: %w", key, err)
		}
		result[key] = string(plaintext)
	}
	return result, rows.Err()
}

// DeleteForCommand removes all secrets for a command (call after the command completes).
func (r *AgentCommandSecretRepository) DeleteForCommand(ctx context.Context, commandID string) error {
	_, err := r.conn.ExecContext(ctx,
		Rebind(`DELETE FROM agent_command_secrets WHERE command_id = ?`), commandID)
	if err != nil {
		return fmt.Errorf("delete command secrets: %w", err)
	}
	return nil
}
