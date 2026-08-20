package servers

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
)

// fakeRuntime is a stand-in dockerRuntime used to observe when the store
// provisions a container and to return canned log output.
type fakeRuntime struct {
	mu       sync.Mutex
	created  bool
	createID string
	lastOpts docker.CreateOpts
	// existing container ids reported present by Exists.
	existing map[string]bool
}

func (f *fakeRuntime) Ping(context.Context) error { return nil }
func (f *fakeRuntime) RuntimeStatus(context.Context) docker.RuntimeStatus {
	return docker.RuntimeStatus{Reachable: true}
}
func (f *fakeRuntime) Remove(context.Context, string) error { return nil }
func (f *fakeRuntime) Start(context.Context, string) error  { return nil }
func (f *fakeRuntime) Stop(context.Context, string, time.Duration) error {
	return nil
}
func (f *fakeRuntime) Status(context.Context, string) (string, error) {
	return "stopped", nil
}
func (f *fakeRuntime) Exists(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.existing == nil {
		return false, nil
	}
	return f.existing[id], nil
}
func (f *fakeRuntime) HostAddress() string { return "127.0.0.1" }
func (f *fakeRuntime) Create(_ context.Context, opts docker.CreateOpts) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = true
	f.lastOpts = opts
	f.createID = "created-" + opts.ID
	if f.existing == nil {
		f.existing = map[string]bool{}
	}
	f.existing["created-"+opts.ID] = true
	return f.createID, nil
}
func (f *fakeRuntime) Logs(context.Context, string, bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("console output")), nil
}
func (f *fakeRuntime) SendConsole(context.Context, string, string) error {
	return nil
}

// TestConsoleProvisionsContainer verifies that opening the console for a server
// with no container auto-provisions one (via Create) and returns logs instead
// of erroring with "server has no container".
func TestConsoleProvisionsContainer(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	s := &Store{db: dbHandle.DB, docker: fake, dataDir: dir}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
		id, "console-test", "paper", "1.21.1", "120", 2048, 0, 0, 25601, "[]", StateStopped, now, now); err != nil {
		t.Fatalf("insert server: %v", err)
	}

	rc, err := s.Console(context.Background(), id, false)
	if err != nil {
		t.Fatalf("Console: %v", err)
	}
	defer rc.Close()
	buf, _ := io.ReadAll(rc)
	if string(buf) != "console output" {
		t.Errorf("console output = %q, want %q", string(buf), "console output")
	}
	if !fake.created {
		t.Fatal("expected the store to auto-create a container when none exists")
	}
	if fake.lastOpts.ServerType != "paper" || fake.lastOpts.HostPort != 25601 {
		t.Errorf("unexpected create opts: %+v", fake.lastOpts)
	}

	// The provisioned container id must be persisted so subsequent calls reuse
	// the same container rather than creating a new one.
	got, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get after console: %v", err)
	}
	if got.ContainerID != fake.createID {
		t.Errorf("persisted ContainerID = %q, want %q", got.ContainerID, fake.createID)
	}
}

// TestConsoleWithoutContainerUsedToError is a regression guard: the previous
// behavior returned a bare error before provisioning. This test asserts the
// error path is gone in favor of auto-provisioning.
func TestConsoleReusesExistingContainer(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	s := &Store{db: dbHandle.DB, docker: fake, dataDir: dir}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'existing-cid', ?, ?, ?)`,
		id, "console-test", "paper", "1.21.1", "120", 2048, 0, 0, 25601, "[]", StateRunning, now, now); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	fake.existing = map[string]bool{"existing-cid": true}

	if _, err := s.Console(context.Background(), id, false); err != nil {
		t.Fatalf("Console: %v", err)
	}
	if fake.created {
		t.Error("expected no create when a container already exists")
	}
}

func insertServerForConsoleCmd(t *testing.T, dbHandle *db.Store, id, state string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'existing-cid', ?, ?, ?)`,
		id, "console-cmd", "paper", "1.21.1", "120", 2048, 0, 0, 25603, "[]", state, now, now); err != nil {
		t.Fatalf("insert server: %v", err)
	}
}

func TestSendConsoleCommandRejectsStoppedServer(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	s := &Store{db: dbHandle.DB, docker: &fakeRuntime{}, dataDir: dir}
	id := uuid.NewString()
	insertServerForConsoleCmd(t, dbHandle, id, StateStopped)

	if err := s.SendConsoleCommand(context.Background(), id, "say hello"); !errors.Is(err, ErrServerNotRunning) {
		t.Fatalf("SendConsoleCommand error = %v, want ErrServerNotRunning", err)
	}
}

func TestSendConsoleCommandRejectsEmptyOrUnsafe(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	s := &Store{db: dbHandle.DB, docker: &fakeRuntime{}, dataDir: dir}
	id := uuid.NewString()
	insertServerForConsoleCmd(t, dbHandle, id, StateRunning)

	if err := s.SendConsoleCommand(context.Background(), id, "   "); err == nil {
		t.Fatal("expected empty command to be rejected")
	}
	if err := s.SendConsoleCommand(context.Background(), id, "say hello\x00vil"); err == nil {
		t.Fatal("expected control character to be rejected")
	}
}

func TestSendConsoleCommandSendsToRunningServer(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	s := &Store{db: dbHandle.DB, docker: fake, dataDir: dir}
	id := uuid.NewString()
	insertServerForConsoleCmd(t, dbHandle, id, StateRunning)

	if err := s.SendConsoleCommand(context.Background(), id, "say hello"); err != nil {
		t.Fatalf("SendConsoleCommand: %v", err)
	}
}

// TestStartRecreatesDeletedContainer verifies that when the recorded container
// id no longer exists (e.g. it was removed with `docker rm` outside MCM), the
// store provisions a fresh container and persists the new id instead of failing
// against a ghost container.
func TestStartRecreatesDeletedContainer(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	s := &Store{db: dbHandle.DB, docker: fake, dataDir: dir}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ghost-container', ?, ?, ?)`,
		id, "start-test", "paper", "1.21.1", "120", 2048, 0, 0, 25602, "[]", StateStopped, now, now); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	// The recorded id is not present on the daemon.
	fake.existing = map[string]bool{}

	srv, err := s.Start(context.Background(), id)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !fake.created {
		t.Fatal("expected Start to recreate the missing container")
	}
	want := "created-" + id
	if srv.ContainerID != want {
		t.Errorf("ContainerID after Start = %q, want %q", srv.ContainerID, want)
	}
	if srv.State != StateRunning {
		t.Errorf("State after Start = %q, want %q", srv.State, StateRunning)
	}
}
