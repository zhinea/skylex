package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/zhinea/skylex/internal/id"
)

type CommandLog struct {
	ID        string
	CommandID string
	AgentID   string
	Level     string
	Message   string
	CreatedAt time.Time

	// Enriched (populated by List* queries via JOIN, empty on Create paths).
	NodeID   string
	Hostname string
	Action   string
}

type CommandLogRepository struct {
	conn *sql.DB
	log  *slog.Logger
}

func NewCommandLogRepository(conn *sql.DB, log *slog.Logger) *CommandLogRepository {
	return &CommandLogRepository{conn: conn, log: log}
}

func (r *CommandLogRepository) Create(ctx context.Context, commandID, agentID, level, message string) (*CommandLog, error) {
	logID := id.New()
	now := time.Now().UTC()

	_, err := r.conn.ExecContext(ctx,
		Rebind(`INSERT INTO agent_command_logs (id, command_id, agent_id, level, message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`),
		logID, commandID, agentID, level, message, now)
	if err != nil {
		return nil, fmt.Errorf("insert command log: %w", err)
	}

	return &CommandLog{
		ID:        logID,
		CommandID: commandID,
		AgentID:   agentID,
		Level:     level,
		Message:   message,
		CreatedAt: now,
	}, nil
}

func (r *CommandLogRepository) CreateBatch(ctx context.Context, logs []*CommandLog) error {
	if len(logs) == 0 {
		return nil
	}

	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin command log batch: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		Rebind(`INSERT INTO agent_command_logs (id, command_id, agent_id, level, message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`))
	if err != nil {
		return fmt.Errorf("prepare command log insert: %w", err)
	}
	defer stmt.Close()

	for _, log := range logs {
		if log.ID == "" {
			log.ID = id.New()
		}
		if log.CreatedAt.IsZero() {
			log.CreatedAt = time.Now().UTC()
		}
		if _, err := stmt.ExecContext(ctx, log.ID, log.CommandID, log.AgentID, log.Level, log.Message, log.CreatedAt); err != nil {
			return fmt.Errorf("insert command log row: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit command log batch: %w", err)
	}
	return nil
}

// selectEnriched is the shared column list + joins. Every List* query enriches
// each log row with its node_id, hostname, and command action in a single query
// (avoids an N+1 lookup per row in the service layer). agent_commands/nodes are
// LEFT JOINed so a log is never dropped if its parent command/node was pruned.
const selectEnriched = `SELECT l.id, l.command_id, l.agent_id, l.level, l.message, l.created_at,
		c.node_id, n.hostname, c.action
	 FROM agent_command_logs l
	 LEFT JOIN agent_commands c ON l.command_id = c.id
	 LEFT JOIN nodes n ON c.node_id = n.id`

func clampLogLimit(limit int) int {
	if limit <= 0 {
		return 500
	}
	if limit > 10000 {
		return 10000
	}
	return limit
}

// LogFilter holds optional, parameterized constraints shared by every List*
// query. Zero values mean "no constraint" so callers only set what they need.
type LogFilter struct {
	Level string    // exact level match (info|warn|error|debug); empty = all
	Since time.Time // created_at >= Since when non-zero
	Until time.Time // created_at <= Until when non-zero
}

// apply appends the filter's SQL conditions (each prefixed with " AND ") and
// their bind args. Values are always passed as bind parameters, never
// interpolated, so user input cannot reach the SQL string.
func (f LogFilter) apply(sb *strings.Builder, args *[]any) {
	if f.Level != "" {
		sb.WriteString(" AND l.level = ?")
		*args = append(*args, f.Level)
	}
	if !f.Since.IsZero() {
		sb.WriteString(" AND l.created_at >= ?")
		*args = append(*args, f.Since.UTC())
	}
	if !f.Until.IsZero() {
		sb.WriteString(" AND l.created_at <= ?")
		*args = append(*args, f.Until.UTC())
	}
}

// ListByCommandID returns logs for a command in ascending (chronological) order.
// Internally it selects the newest `limit` rows (so the page always tracks the
// latest activity instead of freezing on the oldest rows once a run exceeds the
// page size) and reverses them for display.
func (r *CommandLogRepository) ListByCommandID(ctx context.Context, commandID string, limit, offset int, filter LogFilter) ([]*CommandLog, error) {
	limit = clampLogLimit(limit)
	if offset < 0 {
		offset = 0
	}

	var sb strings.Builder
	sb.WriteString(selectEnriched)
	sb.WriteString(" WHERE l.command_id = ?")
	args := []any{commandID}
	filter.apply(&sb, &args)
	sb.WriteString(" ORDER BY l.created_at DESC, l.id DESC LIMIT ? OFFSET ?")
	args = append(args, limit, offset)

	rows, err := r.conn.QueryContext(ctx, Rebind(sb.String()), args...)
	if err != nil {
		return nil, fmt.Errorf("query command logs by command id: %w", err)
	}
	defer rows.Close()

	return scanCommandLogs(rows, true)
}

func (r *CommandLogRepository) ListByNodeID(ctx context.Context, nodeID string, limit, offset int, filter LogFilter) ([]*CommandLog, error) {
	limit = clampLogLimit(limit)
	if offset < 0 {
		offset = 0
	}

	var sb strings.Builder
	sb.WriteString(selectEnriched)
	sb.WriteString(" WHERE c.node_id = ?")
	args := []any{nodeID}
	filter.apply(&sb, &args)
	sb.WriteString(" ORDER BY l.created_at DESC, l.id DESC LIMIT ? OFFSET ?")
	args = append(args, limit, offset)

	rows, err := r.conn.QueryContext(ctx, Rebind(sb.String()), args...)
	if err != nil {
		return nil, fmt.Errorf("query command logs by node id: %w", err)
	}
	defer rows.Close()

	return scanCommandLogs(rows, true)
}

func (r *CommandLogRepository) ListByClusterID(ctx context.Context, clusterID string, limit, offset int, filter LogFilter) ([]*CommandLog, error) {
	limit = clampLogLimit(limit)
	if offset < 0 {
		offset = 0
	}

	var sb strings.Builder
	sb.WriteString(selectEnriched)
	sb.WriteString(" WHERE n.cluster_id = ?")
	args := []any{clusterID}
	filter.apply(&sb, &args)
	sb.WriteString(" ORDER BY l.created_at DESC, l.id DESC LIMIT ? OFFSET ?")
	args = append(args, limit, offset)

	rows, err := r.conn.QueryContext(ctx, Rebind(sb.String()), args...)
	if err != nil {
		return nil, fmt.Errorf("query command logs by cluster id: %w", err)
	}
	defer rows.Close()

	return scanCommandLogs(rows, true)
}

// GetEnrichedByIDs loads enriched log rows by their IDs, preserving no order.
// Used by the live stream path to attach node/hostname/action to freshly
// inserted rows in one batched query instead of per-row lookups.
func (r *CommandLogRepository) GetEnrichedByIDs(ctx context.Context, ids []string) ([]*CommandLog, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := r.conn.QueryContext(ctx,
		Rebind(selectEnriched+` WHERE l.id IN (`+strings.Join(placeholders, ",")+`)`),
		args...)
	if err != nil {
		return nil, fmt.Errorf("query enriched command logs by ids: %w", err)
	}
	defer rows.Close()

	return scanCommandLogs(rows, false)
}

func scanCommandLogs(rows *sql.Rows, reverse bool) ([]*CommandLog, error) {
	var logs []*CommandLog
	for rows.Next() {
		var l CommandLog
		var nodeID, hostname, action sql.NullString
		if err := rows.Scan(&l.ID, &l.CommandID, &l.AgentID, &l.Level, &l.Message, &l.CreatedAt,
			&nodeID, &hostname, &action); err != nil {
			return nil, fmt.Errorf("scan command log: %w", err)
		}
		l.NodeID = nodeID.String
		l.Hostname = hostname.String
		l.Action = action.String
		logs = append(logs, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate command logs: %w", err)
	}

	if reverse {
		for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
			logs[i], logs[j] = logs[j], logs[i]
		}
	}
	return logs, nil
}
