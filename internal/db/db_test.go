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

func TestDNSMigrationPreservesExistingNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-dns.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the pre-migration DNS schema with already published records.
	_, err = store.DB.Exec(`DROP TABLE server_dns_preferences;
		ALTER TABLE dns_records DROP COLUMN provider;
		DELETE FROM schema_migrations WHERE name='0020_dns_preferences_and_providers.sql';
		INSERT INTO servers (id, name, server_type, version, ram_mb, host_port, created_at, updated_at)
		VALUES ('custom','Server Name','paper','1.21.4',4096,25565,'',''), ('apex','Other Server','paper','1.21.4',4096,25566,'','');
		INSERT INTO dns_records (server_id, record_id, name, subdomain, target, port, updated_at)
		VALUES ('custom','one','_minecraft._tcp.play.example.com','play','node.example.com',25565,''),
		('apex','two','_minecraft._tcp.example.com','@','node.example.com',25566,'');`)
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for id, want := range map[string]string{"custom": "play", "apex": "@"} {
		var sub, provider string
		err := reopened.DB.QueryRow(`SELECT p.subdomain, d.provider FROM server_dns_preferences p JOIN dns_records d ON p.server_id=d.server_id WHERE p.server_id=?`, id).Scan(&sub, &provider)
		if err != nil || sub != want || provider != "cloudflare" {
			t.Fatalf("migration lost %s: sub=%q provider=%q err=%v", id, sub, provider, err)
		}
	}
}
