package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
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

func (r *AgentCommandRepository) GetByID(ctx context.Context, id string) (*AgentCommand, error) {
	row := r.conn.QueryRowContext(ctx,
		Rebind(`SELECT id, agent_id, node_id, action, payload, status, result, error, created_at, completed_at
		 FROM agent_commands WHERE id = ?`), id)
	var c AgentCommand
	var result, errMsg sql.NullString
	if err := row.Scan(&c.ID, &c.AgentID, &c.NodeID, &c.Action, &c.Payload,
		&c.Status, &result, &errMsg, &c.CreatedAt, &c.CompletedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get command by id: %w", err)
	}
	c.Result = result.String
	c.Error = errMsg.String
	return &c, nil
}

func (r *AgentCommandRepository) ListPending(ctx context.Context, agentID, nodeID string) ([]*AgentCommand, error) {
	return r.listPending(ctx, agentID, nodeID, 100)
}

func (r *AgentCommandRepository) ListPendingLimit(ctx context.Context, agentID, nodeID string, limit int) ([]*AgentCommand, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	return r.listPending(ctx, agentID, nodeID, limit)
}

func (r *AgentCommandRepository) listPending(ctx context.Context, agentID, nodeID string, limit int) ([]*AgentCommand, error) {
	rows, err := r.conn.QueryContext(ctx,
		Rebind(`SELECT id, agent_id, node_id, action, payload, status, result, error, created_at, completed_at
		 FROM agent_commands WHERE (agent_id = ? OR node_id = ?) AND status = ? ORDER BY created_at ASC LIMIT ?`),
		agentID, nodeID, models.CommandStatusPending, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending commands: %w", err)
	}
	defer rows.Close()

	var cmds []*AgentCommand
	for rows.Next() {
		c, err := scanAgentCommand(rows)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}
	return cmds, rows.Err()
}

func (r *AgentCommandRepository) MarkPendingFailedByNodeIDs(ctx context.Context, nodeIDs []string, actions []string, errMsg string) error {
	if len(nodeIDs) == 0 || len(actions) == 0 {
		return nil
	}

	nodePlaceholders := make([]string, len(nodeIDs))
	actionPlaceholders := make([]string, len(actions))
	args := make([]interface{}, 0, len(nodeIDs)+len(actions)+4)
	for i, id := range nodeIDs {
		nodePlaceholders[i] = "?"
		args = append(args, id)
	}
	for i, action := range actions {
		actionPlaceholders[i] = "?"
		args = append(args, action)
	}
	args = append(args, models.CommandStatusFailed, errMsg, time.Now().UTC(), models.CommandStatusPending)

	query := fmt.Sprintf(`UPDATE agent_commands SET status = ?, error = ?, completed_at = ?
		 WHERE status = ? AND node_id IN (%s) AND action IN (%s)`, strings.Join(nodePlaceholders, ", "), strings.Join(actionPlaceholders, ", "))
	if _, err := r.conn.ExecContext(ctx, Rebind(query), args...); err != nil {
		return fmt.Errorf("mark pending commands failed: %w", err)
	}
	return nil
}

func (r *AgentCommandRepository) ListByNodeIDs(ctx context.Context, nodeIDs []string, limit int) ([]*AgentCommand, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}

	placeholders := make([]string, len(nodeIDs))
	args := make([]interface{}, 0, len(nodeIDs)+1)
	for i, id := range nodeIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, limit)

	query := fmt.Sprintf(`SELECT id, agent_id, node_id, action, payload, status, result, error, created_at, completed_at
		 FROM agent_commands WHERE node_id IN (%s) ORDER BY created_at ASC LIMIT ?`, strings.Join(placeholders, ", "))
	rows, err := r.conn.QueryContext(ctx, Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("query commands by node ids: %w", err)
	}
	defer rows.Close()

	var cmds []*AgentCommand
	for rows.Next() {
		c, err := scanAgentCommand(rows)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
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

func scanAgentCommand(rows *sql.Rows) (*AgentCommand, error) {
	var c AgentCommand
	var result, errMsg sql.NullString
	if err := rows.Scan(&c.ID, &c.AgentID, &c.NodeID, &c.Action, &c.Payload,
		&c.Status, &result, &errMsg, &c.CreatedAt, &c.CompletedAt); err != nil {
		return nil, fmt.Errorf("scan command: %w", err)
	}
	c.Result = result.String
	c.Error = errMsg.String
	return &c, nil
}
