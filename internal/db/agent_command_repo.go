package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/zhinea/skylex/internal/id"
	"github.com/zhinea/skylex/internal/models"
)

type AgentCommandRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

func NewAgentCommandRepository(conn *sql.DB, log *slog.Logger) *AgentCommandRepository {
	return &AgentCommandRepository{conn: conn, log: log}
}

type AgentCommand struct {
	ID          string
	AgentID     string
	NodeID      string
	Action      string
	Payload     string
	Status      string
	Result      string
	Error       string
	CreatedAt   time.Time
	CompletedAt *time.Time
}

func (r *AgentCommandRepository) Create(ctx context.Context, agentID, nodeID, action, payload string) (*AgentCommand, error) {
	cmdID := id.New()
	now := time.Now().UTC()

	_, err := r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO agent_commands (id, agent_id, node_id, action, payload, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		cmdID, agentID, nodeID, action, payload, models.CommandStatusPending, now)
	if err != nil {
		return nil, fmt.Errorf("insert agent command: %w", err)
	}

	return &AgentCommand{
		ID:        cmdID,
		AgentID:   agentID,
		NodeID:    nodeID,
		Action:    action,
		Payload:   payload,
		Status:    models.CommandStatusPending,
		CreatedAt: now,
	}, nil
}

func (r *AgentCommandRepository) ListPending(ctx context.Context, agentID, nodeID string) ([]*AgentCommand, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT id, agent_id, node_id, action, payload, status, result, error, created_at, completed_at
		 FROM agent_commands WHERE (agent_id = ? OR node_id = ?) AND status = ? ORDER BY created_at ASC LIMIT 100`),
		agentID, nodeID, models.CommandStatusPending)
	if err != nil {
		return nil, fmt.Errorf("query pending commands: %w", err)
	}
	defer rows.Close()

	var cmds []*AgentCommand
	for rows.Next() {
		var c AgentCommand
		var result, errMsg sql.NullString
		if err := rows.Scan(&c.ID, &c.AgentID, &c.NodeID, &c.Action, &c.Payload,
			&c.Status, &result, &errMsg, &c.CreatedAt, &c.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		c.Result = result.String
		c.Error = errMsg.String
		cmds = append(cmds, &c)
	}
	return cmds, rows.Err()
}

func (r *AgentCommandRepository) UpdateResult(ctx context.Context, commandID string, success bool, output, errMsg string) error {
	status := models.CommandStatusCompleted
	if !success {
		status = models.CommandStatusFailed
	}

	now := time.Now().UTC()
	_, err := r.conn.ExecContext(ctx,
		Rebind(`UPDATE agent_commands SET status = ?, result = ?, error = ?, completed_at = ? WHERE id = ?`),
		status, output, errMsg, now, commandID)
	if err != nil {
		return fmt.Errorf("update command result: %w", err)
	}
	return nil
}