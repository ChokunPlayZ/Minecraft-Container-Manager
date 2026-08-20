package servers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
)

// TestRecreateClearsContainer verifies that Recreate detaches an existing
// container id and resets the server state so the next Start rebuilds the
// container (e.g. onto a new runtime image). Removal is best-effort: the fake
// container id will not exist on any daemon, and any error is tolerated.
func TestRecreateClearsContainer(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	mgr, err := docker.New("unix:///var/run/docker.sock", "itzg/minecraft-server")
	if err != nil {
		t.Fatalf("docker manager: %v", err)
	}
	s := &Store{db: dbHandle.DB, docker: mgr, dataDir: dir}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "test", "paper", "1.21.1", "120", 2048, 0, 0, 25565, "[]", "migrating-fake-container", StateRunning, now, now); err != nil {
		t.Fatalf("insert server: %v", err)
	}

	srv, err := s.Recreate(context.Background(), id)
	if err != nil {
		t.Fatalf("Recreate: %v", err)
	}
	if srv.ContainerID != "" {
		t.Errorf("ContainerID after Recreate = %q, want empty", srv.ContainerID)
	}
	if srv.State != StateStopped {
		t.Errorf("State after Recreate = %q, want %q", srv.State, StateStopped)
	}

	// Persisted state must match.
	got, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get after recreate: %v", err)
	}
	if got.ContainerID != "" || got.State != StateStopped {
		t.Errorf("persisted server after recreate = container_id %q state %q", got.ContainerID, got.State)
	}
}
