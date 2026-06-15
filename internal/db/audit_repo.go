package db

import (
	"database/sql"
	"log/slog"

	"github.com/zhinea/skylex/internal/models"
)

type AuditRepository struct {
	db  *sql.DB
	log *slog.Logger
}

func NewAuditRepository(db *sql.DB, log *slog.Logger) *AuditRepository {
	return &AuditRepository{db: db, log: log}
}

func (r *AuditRepository) Log(entry *models.AuditLog) error {
	query := `INSERT INTO audit_logs (user_id, action, resource, detail, ip_address) VALUES (?, ?, ?, ?, ?)`
	result, err := r.db.Exec(query, entry.UserID, entry.Action, entry.Resource, entry.Detail, entry.IPAddress)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	entry.ID = id
	return nil
}

func (r *AuditRepository) List(page, pageSize int) ([]models.AuditLog, int, error) {
	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := `SELECT id, user_id, action, resource, detail, ip_address, created_at FROM audit_logs ORDER BY created_at DESC LIMIT ? OFFSET ?`
	rows, err := r.db.Query(query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		var userID sql.NullString
		if err := rows.Scan(&l.ID, &userID, &l.Action, &l.Resource, &l.Detail, &l.IPAddress, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		if userID.Valid {
			l.UserID = userID.String
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}