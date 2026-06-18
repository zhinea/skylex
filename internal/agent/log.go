package agent

import (
	"bufio"
	"context"
	"io"
	"regexp"
	"sync"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
)

var (
	// redactPasswordEnv matches PGPASSWORD=value (with or without quotes).
	redactPasswordEnv = regexp.MustCompile(`(?i)PGPASSWORD\s*=\s*\S+`)
	// redactPasswordClause matches password=... in connection strings.
	redactPasswordClause = regexp.MustCompile(`(?i)password\s*=\s*\S+`)
	// redactEncryptedPassword matches ENCRYPTED PASSWORD '...'.
	redactEncryptedPassword = regexp.MustCompile(`(?i)ENCRYPTED\s+PASSWORD\s+'[^']*'`)
	// redactPostgresPasswordEnv matches POSTGRES_PASSWORD=value used by Docker provisioning.
	redactPostgresPasswordEnv = regexp.MustCompile(`(?i)POSTGRES_PASSWORD\s*=\s*\S+`)
	// redactPsqlVariablePassword matches psql variable arguments containing repl_pass.
	redactPsqlVariablePassword = regexp.MustCompile(`(?i)repl_pass\s*=\s*\S+`)
)

// RedactSecrets removes common password patterns from log output before
// sending it to the server.
func RedactSecrets(input string) string {
	s := redactPasswordEnv.ReplaceAllString(input, "PGPASSWORD=***")
	s = redactPostgresPasswordEnv.ReplaceAllString(s, "POSTGRES_PASSWORD=***")
	s = redactPsqlVariablePassword.ReplaceAllString(s, "repl_pass=***")
	s = redactPasswordClause.ReplaceAllString(s, "password=***")
	s = redactEncryptedPassword.ReplaceAllString(s, "ENCRYPTED PASSWORD '***'")
	return s
}

// commandLogger buffers log lines produced during command execution and flushes
// them to the server in batches.
type commandLogger struct {
	agentID   string
	commandID string
	client    skylexv1.AgentServiceClient

	mu      sync.Mutex
	entries []*skylexv1.CommandLogEntry
	timer   *time.Timer
	closed  bool
}

func newCommandLogger(agentID, commandID string, client skylexv1.AgentServiceClient) *commandLogger {
	return &commandLogger{
		agentID:   agentID,
		commandID: commandID,
		client:    client,
	}
}

func (l *commandLogger) Log(level, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return
	}

	l.entries = append(l.entries, &skylexv1.CommandLogEntry{
		CommandId:   l.commandID,
		Level:       level,
		Message:     RedactSecrets(message),
		TimestampMs: time.Now().UTC().UnixMilli(),
	})

	if len(l.entries) >= 50 {
		l.flushLocked()
		return
	}

	if l.timer == nil {
		l.timer = time.AfterFunc(250*time.Millisecond, func() {
			l.Flush()
		})
	}
}

func (l *commandLogger) Info(message string)  { l.Log("info", message) }
func (l *commandLogger) Error(message string) { l.Log("error", message) }

func (l *commandLogger) StreamReader(reader io.Reader, level string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		l.Log(level, scanner.Text())
	}
}

func (l *commandLogger) Flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.flushLocked()
}

func (l *commandLogger) flushLocked() {
	if len(l.entries) == 0 {
		return
	}
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}

	entries := l.entries
	l.entries = nil

	go func(entries []*skylexv1.CommandLogEntry) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = l.client.ReportCommandLog(ctx, &skylexv1.ReportCommandLogRequest{
			AgentId: l.agentID,
			Entries: entries,
		})
	}(entries)
}

func (l *commandLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	l.flushLocked()
}
