package postgres

import (
	"os"
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
