package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

type DB struct {
	conn   *sql.DB
	log    *slog.Logger
	driver string
}

type Config struct {
	Driver          string
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func New(cfg Config, log *slog.Logger) (*DB, error) {
	conn, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if cfg.Driver == "postgres" || cfg.Driver == "pgx" {
		if cfg.MaxOpenConns == 0 {
			cfg.MaxOpenConns = 25
		}
		if cfg.MaxIdleConns == 0 {
			cfg.MaxIdleConns = 10
		}
		if cfg.ConnMaxLifetime == 0 {
			cfg.ConnMaxLifetime = 30 * time.Minute
		}
	} else {
		cfg.MaxOpenConns = 1
		cfg.MaxIdleConns = 1
		cfg.ConnMaxLifetime = time.Hour
	}

	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	conn.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	setRebind(cfg.Driver)

	db := &DB{
		conn:   conn,
		log:    log,
		driver: cfg.Driver,
	}

	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func (db *DB) Conn() *sql.DB {
	return db.conn
}

func (db *DB) Driver() string {
	return db.driver
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	if err := db.createMigrationsTable(); err != nil {
		return err
	}

	migrationFS := sqliteMigrations
	if db.driver == "postgres" || db.driver == "pgx" {
		migrationFS = postgresMigrations
	}

	migrationDir := "migrations/" + map[bool]string{true: "postgres", false: "sqlite"}[db.driver == "postgres" || db.driver == "pgx"]

	entries, err := migrationFS.ReadDir(migrationDir)
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		version := entry.Name()[:14]
		content, err := migrationFS.ReadFile(migrationDir + "/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		var exists bool
		err = db.conn.QueryRow(Rebind("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)"), version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}

		if exists {
			continue
		}

		tx, err := db.conn.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", version, err)
		}

		if _, err := tx.Exec(Rebind(string(content))); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", version, err)
		}

		if _, err := tx.Exec(Rebind("INSERT INTO schema_migrations (version) VALUES (?)"), version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}

		db.log.Info("applied migration", "version", version)
	}

	return nil
}

func (db *DB) createMigrationsTable() error {
	query := `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`
	_, err := db.conn.Exec(Rebind(query))
	return err
}