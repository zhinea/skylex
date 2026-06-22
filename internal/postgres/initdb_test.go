package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestWriteInitPasswordFileCreatesSecureTemporaryFile(t *testing.T) {
	p := &Instance{ReplPass: "secret-password"}

	path, cleanup, err := p.writeInitPasswordFile()
	if err != nil {
		t.Fatalf("write init password file: %v", err)
	}
	defer cleanup()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat password file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("expected mode 0600, got %o", got)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read password file: %v", err)
	}
	if string(contents) != "secret-password\n" {
		t.Fatalf("unexpected password file contents %q", string(contents))
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove password file, got %v", err)
	}
}

func TestStartupLogSnippetIncludesRecentPgLog(t *testing.T) {
	dir := t.TempDir()
	logPath := dir + "/pg.log"
	if err := os.WriteFile(logPath, []byte("old line\nFATAL: lock file \"postmaster.pid\" already exists\n"), 0600); err != nil {
		t.Fatalf("write pg.log: %v", err)
	}

	snippet := startupLogSnippet(logPath)
	if !strings.Contains(snippet, "startup log (") {
		t.Fatalf("expected startup log header, got %q", snippet)
	}
	if !strings.Contains(snippet, "postmaster.pid") {
		t.Fatalf("expected pg.log contents, got %q", snippet)
	}
}
