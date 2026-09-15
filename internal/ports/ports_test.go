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

func TestParsePortPool(t *testing.T) {
	cases := []struct {
		input    string
		expected []int
		wantErr  bool
	}{
		{"25565", []int{25565}, false},
		{"25565-25568", []int{25565, 25566, 25567, 25568}, false},
		{"25565, 25567, 25570-25572", []int{25565, 25567, 25570, 25571, 25572}, false},
		{"25565, 25565, 25566", []int{25565, 25566}, false}, // Deduplication
		{"", nil, true},
		{"   ", nil, true},
		{"abc", nil, true},
		{"25568-25565", nil, true}, // Invalid reversed range
		{"70000", nil, true},       // Out of range
	}

	for _, c := range cases {
		got, err := ParsePortPool(c.input)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParsePortPool(%q) expected error, got nil", c.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParsePortPool(%q) unexpected error: %v", c.input, err)
			continue
		}
		if len(got) != len(c.expected) {
			t.Errorf("ParsePortPool(%q) expected %v, got %v", c.input, c.expected, got)
			continue
		}
		for i := range got {
			if got[i] != c.expected[i] {
				t.Errorf("ParsePortPool(%q) index %d: expected %d, got %d", c.input, i, c.expected[i], got[i])
			}
		}
	}
}

func TestConfiguredPortPoolFromSettings(t *testing.T) {
	store := openTestStore(t)
	p := NewPool(store.DB, 25565, 25570)

	// Before setting is added, default range is used.
	avail, err := p.Available(context.Background())
	if err != nil {
		t.Fatalf("Available: %v", err)
	}
	if len(avail) != 6 {
		t.Fatalf("expected 6 available ports, got %d", len(avail))
	}

	// Insert custom port pool into settings: only 25580 and 25585.
	_, err = store.DB.ExecContext(context.Background(),
		`INSERT INTO settings (key, value) VALUES ('port_pool', '25580, 25585')`)
	if err != nil {
		t.Fatalf("insert setting: %v", err)
	}

	configured, err := p.ConfiguredPorts(context.Background())
	if err != nil {
		t.Fatalf("ConfiguredPorts: %v", err)
	}
	if len(configured) != 2 || configured[0] != 25580 || configured[1] != 25585 {
		t.Fatalf("unexpected configured ports: %v", configured)
	}

	// Allocate from custom pool.
	allocated, err := p.Allocate(context.Background())
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if allocated != 25580 {
		t.Fatalf("expected 25580, got %d", allocated)
	}

	// Reserve 25580
	insertServer(t, store, "s1", 25580)
	allocated, err = p.Allocate(context.Background())
	if err != nil {
		t.Fatalf("Allocate 2nd: %v", err)
	}
	if allocated != 25585 {
		t.Fatalf("expected 25585, got %d", allocated)
	}
}
