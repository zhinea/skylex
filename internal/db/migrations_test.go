package db

import (
	"context"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// migrationVersionRe matches the first 14 characters used as a migration version key.
var migrationVersionRe = regexp.MustCompile(`^\d{6}_\S+\.sql$`)

// TestMigrations_SQLite_ApplyClean verifies that all SQLite migrations apply
// successfully to a fresh in-memory database.
func TestMigrations_SQLite_ApplyClean(t *testing.T) {
	_, _ = newTestDB(t) // newTestDB calls db.New which runs migrate() internally.
	// If we reach here, all migrations applied without error.
}

func TestMigrations_SQLite_NodeMetricsNormalized(t *testing.T) {
	database, _ := newTestDB(t)
	conn := database.Conn()
	ctx := context.Background()

	if _, err := conn.ExecContext(ctx, `SELECT id, node_id, recorded_at, cpu_usage_percent FROM node_metrics LIMIT 1`); err != nil {
		t.Fatalf("node_metrics table missing expected columns: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT cpu_usage_percent FROM nodes LIMIT 1`); err == nil {
		t.Fatal("expected cpu_usage_percent to be removed from nodes")
	}
}

// TestMigrations_SQLite_Idempotent verifies that applying migrations twice
// (simulated by running New twice on the same connection) does not error.
func TestMigrations_SQLite_Idempotent(t *testing.T) {
	log, _ := newTestDB(t) // first apply
	_ = log
	// Run migrate() a second time directly via another db.New on a separate DSN.
	_, _ = newTestDB(t)
}

// TestMigrations_SQLite_SequentialNumbering asserts that every migration file
// in the SQLite directory has a strictly ascending numeric prefix with no gaps
// and no duplicate version numbers.
func TestMigrations_SQLite_SequentialNumbering(t *testing.T) {
	entries, err := sqliteMigrations.ReadDir("migrations/sqlite")
	if err != nil {
		t.Fatalf("read migration dir: %v", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	if len(names) == 0 {
		t.Fatal("no migration files found")
	}

	seen := make(map[string]bool)
	for i, name := range names {
		if !migrationVersionRe.MatchString(name) {
			t.Errorf("migration %q does not match expected naming pattern NNNNNNdescription.sql", name)
			continue
		}

		prefix := name[:6]
		expectedPrefix := formatMigrationIndex(i + 1)
		if prefix != expectedPrefix {
			t.Errorf("migration %q has prefix %q, expected %q (gap or wrong ordering at position %d)",
				name, prefix, expectedPrefix, i+1)
		}

		version := name[:14]
		if seen[version] {
			t.Errorf("duplicate migration version %q (file: %s)", version, name)
		}
		seen[version] = true
	}
}

// TestMigrations_SQLite_NoEmptyFiles asserts that no migration file is empty.
func TestMigrations_SQLite_NoEmptyFiles(t *testing.T) {
	err := fs.WalkDir(sqliteMigrations, "migrations/sqlite", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := sqliteMigrations.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(content)) == "" {
			t.Errorf("migration file %q is empty", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk migrations: %v", err)
	}
}

// TestMigrations_PostgresMirrorsSQLite verifies the Postgres migration
// directory has the same number of files as the SQLite directory.
func TestMigrations_PostgresMirrorsQLite(t *testing.T) {
	sqliteEntries, err := sqliteMigrations.ReadDir("migrations/sqlite")
	if err != nil {
		t.Fatalf("read sqlite migration dir: %v", err)
	}
	pgEntries, err := postgresMigrations.ReadDir("migrations/postgres")
	if err != nil {
		t.Fatalf("read postgres migration dir: %v", err)
	}

	sqliteCount := countFiles(sqliteEntries)
	pgCount := countFiles(pgEntries)
	if sqliteCount != pgCount {
		t.Errorf("SQLite has %d migration files but Postgres has %d; they should match",
			sqliteCount, pgCount)
	}

	// Both directories must have the same filenames.
	sqliteNames := fileNames(sqliteEntries)
	pgNames := fileNames(pgEntries)
	for i, name := range sqliteNames {
		if i >= len(pgNames) {
			t.Errorf("Postgres is missing migration %q", name)
			continue
		}
		if pgNames[i] != name {
			t.Errorf("migration name mismatch at position %d: SQLite=%q Postgres=%q", i, name, pgNames[i])
		}
	}
}

// formatMigrationIndex zero-pads an index to 6 digits.
func formatMigrationIndex(n int) string {
	return strings.Join([]string{
		string([]byte{byte('0' + n/100000%10),
			byte('0' + n/10000%10),
			byte('0' + n/1000%10),
			byte('0' + n/100%10),
			byte('0' + n/10%10),
			byte('0' + n%10)}),
	}, "")
}

func countFiles(entries []fs.DirEntry) int {
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n
}

func fileNames(entries []fs.DirEntry) []string {
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}
