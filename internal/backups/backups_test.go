package backups

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
)

func openTestStore(t *testing.T) (*Store, *db.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	handle, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { handle.Close() })
	s := New(handle.DB, S3Config{}, t.TempDir())
	return s, handle
}

func TestListAndFeedbackLifecycle(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	// Insert two records directly and verify List order (newest first).
	records := []struct {
		id, server, name, loc, status, ts string
		size                              int64
	}{
		{"b1", "srv1", "one", "backups/srv1/b1.tar.gz", StatusCompleted, "2026-01-01T00:00:00Z", 100},
		{"b2", "srv1", "two", "backups/srv1/b2.tar.gz", StatusPending, "2026-01-02T00:00:00Z", 200},
	}
	for _, r := range records {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO backups (id, server_id, name, size_bytes, location, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.id, r.server, r.name, r.size, r.loc, r.status, r.ts); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	list, err := s.List(ctx, "srv1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 backups, got %d", len(list))
	}
	if list[0].ID != "b2" || list[1].ID != "b1" {
		t.Fatalf("unexpected order: got %#v", []string{list[0].ID, list[1].ID})
	}

	b, err := s.Get(ctx, "b1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if b.SizeBytes != 100 || b.Status != StatusCompleted {
		t.Fatalf("unexpected backup: %#v", b)
	}

	if err := s.SetStatus(ctx, "b1", StatusFailed); err != nil {
		t.Fatalf("set status: %v", err)
	}
	b, err = s.Get(ctx, "b1")
	if err != nil {
		t.Fatalf("reget: %v", err)
	}
	if b.Status != StatusFailed {
		t.Fatalf("status not updated: %#v", b)
	}
}

func TestDeleteRemovesRecordWithoutS3(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO backups (id, server_id, name, size_bytes, location, status, created_at) VALUES (?, ?, ?, 0, ?, ?, ?)`,
		"b1", "srv1", "n", "loc", StatusCompleted, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := s.Delete(ctx, "b1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(ctx, "b1"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestRetentionKeepsConfiguredCount(t *testing.T) {
	s, handle := openTestStore(t)
	ctx := context.Background()

	// Configure retention of 2 backups per server.
	if _, err := handle.DB.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES ('backup_retention', '2') ON CONFLICT(key) DO UPDATE SET value = excluded.value`); err != nil {
		t.Fatalf("set retention: %v", err)
	}

	// Insert five records for srv2.
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO backups (id, server_id, name, size_bytes, location, status, created_at) VALUES (?, ?, ?, 0, ?, ?, ?)`,
			"b"+id, "srv2", "n", "loc-"+id, StatusCompleted, "2026-01-00T00:00:00Z"); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	if err := s.enforceRetention(ctx, "srv2"); err != nil {
		t.Fatalf("enforce retention: %v", err)
	}

	list, err := s.List(ctx, "srv2")
	if err != nil {
		t.Fatalf("list after retention: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 backups after retention, got %d", len(list))
	}
}

func TestArchiveAndExtractRoundTrip(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	serverID := "srv-rtt"
	srcDir := s.serverDataDir(serverID)
	if err := os.MkdirAll(filepath.Join(srcDir, "world"), 0o755); err != nil {
		t.Fatalf("mkdir world: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "world", "level.dat"), []byte("minecraft-data"), 0o644); err != nil {
		t.Fatalf("write level.dat: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "server.properties"), []byte("online-mode=true\n"), 0o644); err != nil {
		t.Fatalf("write server.properties: %v", err)
	}

	archivePath, err := s.archiveWorld(ctx, serverID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	defer os.Remove(archivePath)

	// Extract into a fresh server id and verify content round-trips.
	newID := "srv-restored"
	if err := s.extractWorld(ctx, newID, archivePath); err != nil {
		t.Fatalf("extract: %v", err)
	}
	restoredData, err := os.ReadFile(filepath.Join(s.serverDataDir(newID), "world", "level.dat"))
	if err != nil {
		t.Fatalf("read restored level.dat: %v", err)
	}
	if string(restoredData) != "minecraft-data" {
		t.Fatalf("restored data mismatch: %q", string(restoredData))
	}
	props, err := os.ReadFile(filepath.Join(s.serverDataDir(newID), "server.properties"))
	if err != nil {
		t.Fatalf("read restored server.properties: %v", err)
	}
	if string(props) != "online-mode=true\n" {
		t.Fatalf("restored props mismatch: %q", string(props))
	}
}
