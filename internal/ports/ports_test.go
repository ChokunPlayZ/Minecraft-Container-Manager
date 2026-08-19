package ports

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/db"
)

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcm.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestAllocateLowestFreeAndExhaust(t *testing.T) {
	store := openTestStore(t)
	p := NewPool(store.DB, 25565, 25568)

	// Lowest free is the start of the range.
	port, err := p.Allocate(context.Background())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if port != 25565 {
		t.Fatalf("expected 25565, got %d", port)
	}

	// Reserve it in the servers table (mirrors what server creation does).
	insertServer(t, store, "s1", 25565)
	insertServer(t, store, "s2", 25566)

	port, err = p.Allocate(context.Background())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if port != 25567 {
		t.Fatalf("expected 25567, got %d", port)
	}

	// Fill the rest of the range.
	insertServer(t, store, "s3", 25567)
	insertServer(t, store, "s4", 25568)

	if _, err := p.Allocate(context.Background()); !errors.Is(err, ErrPortPoolFull) {
		t.Fatalf("expected ErrPortPoolFull, got %v", err)
	}
}

func TestReleaseViaDelete(t *testing.T) {
	store := openTestStore(t)
	p := NewPool(store.DB, 30000, 30001)

	insertServer(t, store, "a", 30000)
	if err := p.Release(context.Background(), 30000); err != nil {
		t.Fatalf("release: %v", err)
	}
	// The port is only actually freed when the owning server row is gone.
	if _, err := store.DB.ExecContext(context.Background(), `DELETE FROM servers WHERE id = 'a'`); err != nil {
		t.Fatalf("delete server: %v", err)
	}

	port, err := p.Allocate(context.Background())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if port != 30000 {
		t.Fatalf("expected 30000 after release, got %d", port)
	}
}

func TestAvailable(t *testing.T) {
	store := openTestStore(t)
	p := NewPool(store.DB, 40000, 40002)
	insertServer(t, store, "x", 40001)
	free, err := p.Available(context.Background())
	if err != nil {
		t.Fatalf("available: %v", err)
	}
	if len(free) != 2 || free[0] != 40000 || free[1] != 40002 {
		t.Fatalf("unexpected free list: %v", free)
	}
}

func insertServer(t *testing.T, store *db.Store, id string, port int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := store.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, host_port, container_id, state, created_at, updated_at) VALUES (?, 'srv', 'paper', '1.21.1', '120', 2048, ?, '', 'stopped', ?, ?)`,
		id, port, now, now)
	if err != nil {
		t.Fatalf("insert server %s: %v", id, err)
	}
}
