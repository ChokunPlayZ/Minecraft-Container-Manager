// Package db provides the SQLite database handle and schema initialization.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/mcm-panel/mcm/migrations"
	_ "modernc.org/sqlite"
)

// Store wraps the SQLite connection and the migrations that were applied.
type Store struct {
	DB *sql.DB
}

// Open opens (creating if needed) the SQLite database at path, enables WAL, and
// applies the embedded migrations in lexicographic filename order.
func Open(path string) (*Store, error) {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}

	dbConn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	dbConn.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := dbConn.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
		dbConn.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := dbConn.ExecContext(ctx, "PRAGMA foreign_keys=ON;"); err != nil {
		dbConn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := applyMigrations(ctx, dbConn); err != nil {
		dbConn.Close()
		return nil, err
	}

	return &Store{DB: dbConn}, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	return s.DB.Close()
}

func applyMigrations(ctx context.Context, dbConn *sql.DB) error {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	// Simple idempotency: create a schema_migrations table and skip applied files.
	if _, err := dbConn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, name := range names {
		var exists int
		if err := dbConn.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE name = ?`, name).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if exists > 0 {
			continue
		}

		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := dbConn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
