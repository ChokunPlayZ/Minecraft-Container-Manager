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

// Recreate clears tracking only after successful runtime removal.
func TestRecreateClearsContainer(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dbHandle.Close()
	mgr := &fakeRuntime{}
	s := &Store{db: dbHandle.DB, docker: mgr, dataDir: dir}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "test", "paper", "1.21.1", "120", 2048, 0, 0, 25565, "[]", "migrating-fake-container", StateStopped, now, now); err != nil {
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

// TestRecreateOnlineServer verifies that when Recreate is called on an online server,
// it gracefully shuts down the server (sending console stop), deletes the old container,
// provisions a fresh container, and boots it back up.
func TestRecreateOnlineServer(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{
		inspectState: &docker.ContainerState{Status: StateRunning},
		existing:     map[string]bool{"old-container-123": true},
	}
	s := &Store{db: dbHandle.DB, docker: fake, dataDir: dir}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	initialConfig := `{"server_type":"paper","version":"1.21.1","build":"120","ram_mb":2048,"cpu_limit":0,"memory_limit_mb":0,"host_port":25565,"java_version":21,"extra_ports":[],"entrypoint_version":2}`
	if _, err := dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at, container_config, java_version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "test-online", "paper", "1.21.1", "120", 4096, 0, 0, 25565, "[]", "old-container-123", StateRunning, now, now, initialConfig, 21); err != nil {
		t.Fatalf("insert server: %v", err)
	}

	// Verify server needs rebuild because ram_mb is 4096 vs container_config 2048
	srv, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	if !srv.NeedsRebuild {
		t.Fatalf("expected NeedsRebuild == true before Recreate")
	}

	// Recreate the online server
	srv, err = s.Recreate(context.Background(), id)
	if err != nil {
		t.Fatalf("Recreate: %v", err)
	}

	// 1. Verify graceful console stop was sent
	fake.mu.Lock()
	cmds := append([]string(nil), fake.consoleCommands...)
	removed := append([]string(nil), fake.removedIDs...)
	fake.mu.Unlock()

	foundStop := false
	for _, c := range cmds {
		if c == "stop" {
			foundStop = true
			break
		}
	}
	if !foundStop {
		t.Errorf("expected 'stop' command in consoleCommands, got: %v", cmds)
	}

	// 2. Verify old container was removed
	foundRemove := false
	for _, r := range removed {
		if r == "old-container-123" {
			foundRemove = true
			break
		}
	}
	if !foundRemove {
		t.Errorf("expected old-container-123 in removedIDs, got: %v", removed)
	}

	// 3. Verify server was booted up with a new container
	if srv.ContainerID == "" || srv.ContainerID == "old-container-123" {
		t.Errorf("ContainerID after Recreate = %q, want fresh container ID", srv.ContainerID)
	}
	if srv.State != StateRunning {
		t.Errorf("State after Recreate = %q, want %q", srv.State, StateRunning)
	}
	if srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == false after online Recreate, got reasons: %v", srv.RebuildReasons)
	}

	// 4. Verify persisted state in DB
	got, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get persisted server: %v", err)
	}
	if got.State != StateRunning || got.ContainerID != srv.ContainerID {
		t.Errorf("persisted server = container_id %q state %q", got.ContainerID, got.State)
	}
	if got.NeedsRebuild {
		t.Errorf("persisted server NeedsRebuild == true, want false")
	}
}
