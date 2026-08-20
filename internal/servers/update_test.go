package servers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/db"
)

func insertServer(t *testing.T, dbh *db.Store, id string, hostPort int, containerID, state string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := dbh.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "test", "paper", "1.21.1", "120", 2048, 0, 0, hostPort, "[]", containerID, state, now, now); err != nil {
		t.Fatalf("insert server: %v", err)
	}
}

func TestUpdateHostPortDetachesContainer(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	s := &Store{db: dbHandle.DB, docker: fake, dataDir: dir}

	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "container-abc", StateRunning)

	newPort := 25600
	srv, err := s.Update(context.Background(), id, UpdateInput{HostPort: &newPort})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if srv.HostPort != newPort {
		t.Errorf("HostPort after update = %d, want %d", srv.HostPort, newPort)
	}
	if srv.ContainerID != "" {
		t.Errorf("expected container to be detached after port change, got %q", srv.ContainerID)
	}
	if srv.State != StateStopped {
		t.Errorf("State after port change = %q, want %q", srv.State, StateStopped)
	}

	got, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.HostPort != newPort || got.ContainerID != "" || got.State != StateStopped {
		t.Errorf("persisted state after port change = %+v", got)
	}
}

func TestUpdateHostPortRejectsConflict(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	s := &Store{db: dbHandle.DB, docker: fake, dataDir: dir}

	other := uuid.NewString()
	insertServer(t, dbHandle, other, 30000, "", StateStopped)

	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "", StateStopped)

	conflict := 30000
	if _, err := s.Update(context.Background(), id, UpdateInput{HostPort: &conflict}); err == nil {
		t.Fatal("expected Update to reject a host_port already in use")
	}

	// Original port must be unchanged.
	srv, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if srv.HostPort != 25565 {
		t.Errorf("HostPort after rejected update = %d, want 25565", srv.HostPort)
	}
}
