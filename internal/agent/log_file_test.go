package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"google.golang.org/grpc"
)

func TestAgentNewWritesLogsToConfiguredFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nested", "agent.log")

	ag, err := New(testConfig(logPath))
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	t.Cleanup(func() { _ = ag.Close() })

	ag.log.Info("file logging enabled", "component", "test")
	if err := ag.Close(); err != nil {
		t.Fatalf("close agent: %v", err)
	}

	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !bytes.Contains(contents, []byte("file logging enabled")) {
		t.Fatalf("expected startup log in file, got %q", string(contents))
	}
}

func TestDefaultConfigEnablesFileLogging(t *testing.T) {
	if DefaultConfig().LogFile != DefaultAgentLogFile {
		t.Fatalf("expected default log file %q, got %q", DefaultAgentLogFile, DefaultConfig().LogFile)
	}
}

func TestAgentNewFallsBackToStderrWhenLogFileUnopenable(t *testing.T) {
	// Point the log file at a path whose parent is an existing regular file,
	// so MkdirAll/OpenFile must fail. The agent should still start.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatalf("seed blocker file: %v", err)
	}

	ag, err := New(testConfig(filepath.Join(blocker, "agent.log")))
	if err != nil {
		t.Fatalf("expected agent to start despite unopenable log file, got: %v", err)
	}
	t.Cleanup(func() { _ = ag.Close() })

	if ag.logFile != nil {
		t.Fatal("expected no log file handle when the path cannot be opened")
	}
}

func TestAgentNewWithoutLogFileDoesNotCreateDefaultFile(t *testing.T) {
	ag, err := New(testConfig(""))
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	defer ag.Close()

	if ag.logFile != nil {
		t.Fatal("expected no log file when log_file is empty")
	}
}

func TestCommandLoggerWritesRedactedLocalLogs(t *testing.T) {
	var out bytes.Buffer
	localLog := NewLogger("info", "text", &out)
	client := &captureAgentClient{}
	logger := newCommandLogger("agent-1", "cmd-1", client, localLog)

	logger.Info("running PGPASSWORD=super-secret psql")
	logger.Error(`payload {"token":"secret-token"}`)
	logger.Close()

	logged := out.String()
	if strings.Contains(logged, "super-secret") || strings.Contains(logged, "secret-token") {
		t.Fatalf("expected secrets to be redacted, got %q", logged)
	}
	if !strings.Contains(logged, "PGPASSWORD=***") || !strings.Contains(logged, "***") {
		t.Fatalf("expected redacted command output in local log, got %q", logged)
	}
}

func TestAgentLoggerRedactsStructuredSecrets(t *testing.T) {
	var out bytes.Buffer
	log := NewLogger("info", "text", &out)

	log.Error("command failed", "error", os.ErrPermission, "output", "connect password=secret-token")

	logged := out.String()
	if strings.Contains(logged, "secret-token") {
		t.Fatalf("expected structured secret to be redacted, got %q", logged)
	}
	if !strings.Contains(logged, "password=***") {
		t.Fatalf("expected redacted structured output, got %q", logged)
	}
}

func testConfig(logFile string) Config {
	cfg := DefaultConfig()
	cfg.AgentToken = "test-token"
	cfg.Hostname = "test-host"
	cfg.LogFormat = "text"
	cfg.LogFile = logFile
	cfg.HeartbeatInterval = time.Hour
	return cfg
}

type captureAgentClient struct {
	skylexv1.AgentServiceClient
}

func (c *captureAgentClient) RegisterAgent(context.Context, *skylexv1.RegisterAgentRequest, ...grpc.CallOption) (*skylexv1.RegisterAgentResponse, error) {
	return &skylexv1.RegisterAgentResponse{AgentId: "agent-1"}, nil
}

func (c *captureAgentClient) Heartbeat(context.Context, *skylexv1.HeartbeatRequest, ...grpc.CallOption) (*skylexv1.HeartbeatResponse, error) {
	return &skylexv1.HeartbeatResponse{}, nil
}

func (c *captureAgentClient) ReportStatus(context.Context, *skylexv1.ReportStatusRequest, ...grpc.CallOption) (*skylexv1.ReportStatusResponse, error) {
	return &skylexv1.ReportStatusResponse{}, nil
}

func (c *captureAgentClient) FetchCommand(context.Context, *skylexv1.FetchCommandRequest, ...grpc.CallOption) (*skylexv1.FetchCommandResponse, error) {
	return &skylexv1.FetchCommandResponse{}, nil
}

func (c *captureAgentClient) ReportCommandResult(context.Context, *skylexv1.ReportCommandResultRequest, ...grpc.CallOption) (*skylexv1.ReportCommandResultResponse, error) {
	return &skylexv1.ReportCommandResultResponse{}, nil
}

func (c *captureAgentClient) ReportCommandLog(context.Context, *skylexv1.ReportCommandLogRequest, ...grpc.CallOption) (*skylexv1.ReportCommandLogResponse, error) {
	return &skylexv1.ReportCommandLogResponse{}, nil
}
