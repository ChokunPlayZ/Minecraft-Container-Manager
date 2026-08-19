package db

import (
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrationsAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcm.db")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	// Opening again on the same file must not fail or duplicate tables.
	second, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	first.Close()
	second.Close()

	// Verify all expected tables exist after reopening.
	reopen, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopen.Close()

	for _, tbl := range []string{"users", "sessions", "servers", "settings", "backups", "schema_migrations"} {
		var n int
		if err := reopen.DB.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&n); err != nil {
			t.Fatalf("query table %s: %v", tbl, err)
		}
		if n == 0 {
			t.Fatalf("table %s missing after migrations", tbl)
		}
	}
}
