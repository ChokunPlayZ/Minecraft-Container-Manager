package servers

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/ports"
)

func TestCreateWithHostPortAndConflict(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { dbHandle.Close() })

	pool := ports.NewPool(dbHandle.DB, 25565, 25570)
	jr := jars.NewResolver()
	s := NewStore(dbHandle, nil, jr, 25565, 25570, dir, "")
	s.ports = pool

	// Create with specific port 25580.
	srv1, err := s.Create(context.Background(), CreateInput{
		Name:       "server-1",
		ServerType: jars.TypePaper,
		Version:    "1.21.1",
		RAMMB:      2048,
		HostPort:   25580,
	})
	if err != nil {
		t.Fatalf("Create srv1: %v", err)
	}
	if srv1.HostPort != 25580 {
		t.Errorf("expected port 25580, got %d", srv1.HostPort)
	}

	// Try creating second server with identical port 25580 -> should fail with ErrPortInUse.
	_, err = s.Create(context.Background(), CreateInput{
		Name:       "server-2",
		ServerType: jars.TypePaper,
		Version:    "1.21.1",
		RAMMB:      2048,
		HostPort:   25580,
	})
	if err == nil {
		t.Fatal("expected Create to fail with port conflict, got nil error")
	}
	if !errors.Is(err, ErrPortInUse) {
		t.Errorf("expected ErrPortInUse, got: %v", err)
	}

	// Create third server with port 0 -> should auto-allocate from pool.
	srv3, err := s.Create(context.Background(), CreateInput{
		Name:       "server-3",
		ServerType: jars.TypePaper,
		Version:    "1.21.1",
		RAMMB:      2048,
	})
	if err != nil {
		t.Fatalf("Create srv3: %v", err)
	}
	if srv3.HostPort != 25565 {
		t.Errorf("expected srv3 to allocate 25565, got %d", srv3.HostPort)
	}

	// Create fourth server with an extra port (e.g. 19132 for GeyserMC)
	_, err = s.Create(context.Background(), CreateInput{
		Name:       "server-with-geyser",
		ServerType: jars.TypePaper,
		Version:    "1.21.1",
		RAMMB:      2048,
		HostPort:   25585,
		ExtraPorts: []ExtraPort{
			{ID: "geyser-1", Description: "Geyser Bedrock", HostPort: 19132, ContainerPort: 19132, Protocol: "udp"},
		},
	})
	if err != nil {
		t.Fatalf("Create server-with-geyser failed: %v", err)
	}

	// Attempting to create standalone server on 19132 should fail with ErrPortInUse
	_, err = s.Create(context.Background(), CreateInput{
		Name:       "geyser-standalone",
		ServerType: jars.TypeGeyser,
		Version:    "2.11.3",
		RAMMB:      1024,
		HostPort:   19132,
	})
	if err == nil {
		t.Fatal("expected Create geyser-standalone on port 19132 to fail, got nil")
	}
	if !errors.Is(err, ErrPortInUse) {
		t.Errorf("expected ErrPortInUse, got: %v", err)
	}
	if !strings.Contains(err.Error(), "19132") || !strings.Contains(err.Error(), "server-with-geyser") {
		t.Errorf("expected error message to mention port 19132 and server-with-geyser, got: %v", err)
	}

	// Attempting to create server with duplicate extra ports should fail
	_, err = s.Create(context.Background(), CreateInput{
		Name:       "dup-ports",
		ServerType: jars.TypePaper,
		Version:    "1.21.1",
		RAMMB:      1024,
		HostPort:   25590,
		ExtraPorts: []ExtraPort{
			{ID: "p1", HostPort: 25590}, // conflicts with HostPort
		},
	})
	if err == nil || !errors.Is(err, ErrPortInUse) {
		t.Errorf("expected duplicate port error, got: %v", err)
	}
}

func TestServerExport(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { dbHandle.Close() })

	s := &Store{db: dbHandle.DB, dataDir: dir}
	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "", StateStopped)

	// Create some files in server dataDir.
	srvDir := s.dataPath(id)
	if err := os.MkdirAll(filepath.Join(srvDir, "plugins"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srvDir, "server.properties"), []byte("motd=Hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srvDir, "plugins", "test.jar"), []byte("jarcontent"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var buf bytes.Buffer
	if err := s.Export(context.Background(), id, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Verify zip contents.
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip reader: %v", err)
	}

	foundProps := false
	foundPlugin := false
	for _, f := range zr.File {
		if f.Name == "server.properties" {
			foundProps = true
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open zipped file: %v", err)
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			if string(data) != "motd=Hello" {
				t.Errorf("unexpected content in zipped file: %s", string(data))
			}
		}
		if f.Name == "plugins/test.jar" {
			foundPlugin = true
		}
	}
	if !foundProps {
		t.Error("server.properties not found in export zip")
	}
	if !foundPlugin {
		t.Error("plugins/test.jar not found in export zip")
	}
}

func TestServerUptimeTracking(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { dbHandle.Close() })

	fake := &fakeRuntime{}
	s := &Store{db: dbHandle.DB, docker: fake, dataDir: dir}

	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "container-uptime-1", StateStopped)

	// 1. Initially stopped: started_at should be empty and uptime 0
	srv, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if srv.StartedAt != "" {
		t.Errorf("expected empty StartedAt on stopped server, got %q", srv.StartedAt)
	}
	if srv.UptimeSeconds != 0 {
		t.Errorf("expected 0 UptimeSeconds on stopped server, got %d", srv.UptimeSeconds)
	}

	// 2. Start server: started_at should be set and uptime >= 0
	srv, err = s.Start(context.Background(), id)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.State != StateRunning {
		t.Fatalf("expected state running, got %s", srv.State)
	}
	if srv.StartedAt == "" {
		t.Error("expected non-empty StartedAt after start")
	}
	if srv.UptimeSeconds < 0 {
		t.Errorf("expected non-negative UptimeSeconds, got %d", srv.UptimeSeconds)
	}

	// 3. Status when container reports running with specific started timestamp
	testStartTime := time.Now().Add(-120 * time.Second).UTC().Format(time.RFC3339)
	fake.mu.Lock()
	// Reconcile status
	fake.mu.Unlock()
	_ = s.setStateWithStartedAt(context.Background(), id, StateRunning, &testStartTime)

	srv, err = s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get after set started_at: %v", err)
	}
	if srv.UptimeSeconds < 119 {
		t.Errorf("expected uptime >= 119s, got %d", srv.UptimeSeconds)
	}

	// 4. List also computes uptime
	list, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 server, got %d", len(list))
	}
	if list[0].UptimeSeconds < 119 {
		t.Errorf("expected list item uptime >= 119s, got %d", list[0].UptimeSeconds)
	}

	// 5. Stop server: started_at cleared and uptime 0
	srv, err = s.Stop(context.Background(), id)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if srv.StartedAt != "" {
		t.Errorf("expected empty StartedAt after stop, got %q", srv.StartedAt)
	}
	if srv.UptimeSeconds != 0 {
		t.Errorf("expected 0 UptimeSeconds after stop, got %d", srv.UptimeSeconds)
	}
}

func TestCopyServer(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { dbHandle.Close() })

	pool := ports.NewPool(dbHandle.DB, 25565, 25575)
	jr := jars.NewResolver()
	s := NewStore(dbHandle, nil, jr, 25565, 25575, dir, "")
	s.ports = pool

	srcID := uuid.NewString()
	insertServer(t, dbHandle, srcID, 25565, "", StateStopped)

	// Populate files in source server directory:
	// - World: world/level.dat, world/region/r.0.0.mca
	// - Player data: world/playerdata/player1.dat, ops.json, whitelist.json
	// - Config: server.properties, config/paper-global.yml
	// - Plugins: plugins/MyPlugin.jar, plugins/MyPlugin/config.yml
	// - Mods: mods/MyMod.jar
	// - Logs: logs/latest.log, crash-reports/crash.txt
	// - Lock: session.lock
	// - Custom: backups/backup1.zip
	srcDir := s.dataPath(srcID)
	filesToCreate := map[string]string{
		"world/level.dat":                  "leveldata",
		"world/region/r.0.0.mca":           "chunkdata",
		"world/playerdata/player1.dat":     "inventorydata",
		"world/session.lock":               "lockfile",
		"ops.json":                         `[{"uuid":"123","name":"Steve"}]`,
		"whitelist.json":                   `[]`,
		"server.properties":                "motd=SourceServer\nlevel-name=world\n",
		"config/paper-global.yml":          "settings: true",
		"plugins/MyPlugin.jar":             "jarbytes",
		"plugins/MyPlugin/config.yml":      "pluginconfig",
		"mods/MyMod.jar":                   "modbytes",
		"logs/latest.log":                  "logcontent",
		"crash-reports/crash.txt":          "crashcontent",
		"backups/backup1.zip":              "backupbytes",
	}

	for p, content := range filesToCreate {
		fullPath := filepath.Join(srcDir, p)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}

	t.Run("Copy all components", func(t *testing.T) {
		trueVal := true
		copied, err := s.Copy(context.Background(), srcID, CopyInput{
			Name:              "Copied All",
			IncludeWorld:      &trueVal,
			IncludeConfig:     &trueVal,
			IncludePlugins:    &trueVal,
			IncludeMods:       &trueVal,
			IncludePlayerData: &trueVal,
			IncludeLogs:       &trueVal,
		})
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		if copied.Name != "Copied All" {
			t.Errorf("expected name %q, got %q", "Copied All", copied.Name)
		}
		if copied.HostPort == 25565 {
			t.Errorf("expected new port != 25565, got %d", copied.HostPort)
		}

		copyDir := s.dataPath(copied.ID)

		// Verify files exist in copy
		checkExists := []string{
			"world/level.dat",
			"world/region/r.0.0.mca",
			"world/playerdata/player1.dat",
			"ops.json",
			"whitelist.json",
			"server.properties",
			"config/paper-global.yml",
			"plugins/MyPlugin.jar",
			"mods/MyMod.jar",
			"logs/latest.log",
		}
		for _, f := range checkExists {
			if _, err := os.Stat(filepath.Join(copyDir, f)); err != nil {
				t.Errorf("expected %s to exist in copied server: %v", f, err)
			}
		}

		// session.lock must NEVER be copied
		if _, err := os.Stat(filepath.Join(copyDir, "world/session.lock")); !os.IsNotExist(err) {
			t.Errorf("session.lock should not be copied")
		}
	})

	t.Run("Copy excluding world and player data", func(t *testing.T) {
		falseVal := false
		trueVal := true
		copied, err := s.Copy(context.Background(), srcID, CopyInput{
			Name:              "Exclude World and Players",
			IncludeWorld:      &falseVal,
			IncludePlayerData: &falseVal,
			IncludeConfig:     &trueVal,
			IncludePlugins:    &trueVal,
			IncludeMods:       &trueVal,
			IncludeLogs:       &falseVal,
		})
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		copyDir := s.dataPath(copied.ID)

		// World and logs and player data should NOT exist
		checkNotExists := []string{
			"world",
			"world/level.dat",
			"world/playerdata",
			"ops.json",
			"whitelist.json",
			"logs/latest.log",
			"crash-reports",
		}
		for _, f := range checkNotExists {
			if _, err := os.Stat(filepath.Join(copyDir, f)); !os.IsNotExist(err) {
				t.Errorf("expected %s to NOT exist in copy", f)
			}
		}

		// Configs and plugins and mods SHOULD exist
		checkExists := []string{
			"server.properties",
			"config/paper-global.yml",
			"plugins/MyPlugin.jar",
			"mods/MyMod.jar",
		}
		for _, f := range checkExists {
			if _, err := os.Stat(filepath.Join(copyDir, f)); err != nil {
				t.Errorf("expected %s to exist in copy: %v", f, err)
			}
		}
	})

	t.Run("Copy excluding plugins and configs", func(t *testing.T) {
		falseVal := false
		trueVal := true
		copied, err := s.Copy(context.Background(), srcID, CopyInput{
			Name:           "Exclude Plugins and Configs",
			IncludeWorld:   &trueVal,
			IncludeConfig:  &falseVal,
			IncludePlugins: &falseVal,
			IncludeMods:    &falseVal,
		})
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		copyDir := s.dataPath(copied.ID)

		// Plugins, mods, configs should NOT exist
		checkNotExists := []string{
			"plugins",
			"mods",
			"server.properties",
			"config",
		}
		for _, f := range checkNotExists {
			if _, err := os.Stat(filepath.Join(copyDir, f)); !os.IsNotExist(err) {
				t.Errorf("expected %s to NOT exist in copy", f)
			}
		}

		// World SHOULD exist
		if _, err := os.Stat(filepath.Join(copyDir, "world/level.dat")); err != nil {
			t.Errorf("expected world/level.dat to exist: %v", err)
		}
	})

	t.Run("Copy with custom excludes", func(t *testing.T) {
		copied, err := s.Copy(context.Background(), srcID, CopyInput{
			Name:           "Custom Excludes",
			CustomExcludes: []string{"backups/*", "*.jar"},
		})
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		copyDir := s.dataPath(copied.ID)

		if _, err := os.Stat(filepath.Join(copyDir, "backups/backup1.zip")); !os.IsNotExist(err) {
			t.Errorf("expected backups/backup1.zip to be excluded")
		}
		if _, err := os.Stat(filepath.Join(copyDir, "plugins/MyPlugin.jar")); !os.IsNotExist(err) {
			t.Errorf("expected *.jar to be excluded")
		}
	})

	t.Run("Port conflict check", func(t *testing.T) {
		_, err := s.Copy(context.Background(), srcID, CopyInput{
			Name:     "Conflict Server",
			HostPort: 25565, // in use by source
		})
		if err == nil {
			t.Fatal("expected port conflict error")
		}
		if !errors.Is(err, ErrPortInUse) {
			t.Errorf("expected ErrPortInUse, got: %v", err)
		}
	})
}

func TestMapDockerState(t *testing.T) {
	tests := []struct {
		status   string
		exitCode int
		want     string
	}{
		{"running", 0, StateRunning},
		{"restarting", 0, StateStarting},
		{"paused", 0, StateRunning},
		{"created", 0, StateStopped},
		{"stopped", 0, StateStopped},
		{"exited", 0, StateStopped},
		{"exited", 143, StateStopped}, // SIGTERM / docker stop
		{"exited", 130, StateStopped}, // SIGINT / Ctrl+C
		{"exited", 1, StateError},     // Java crash / unhandled exception
		{"exited", 137, StateError},   // SIGKILL / OOM
		{"dead", 0, StateStopped},
		{"dead", 1, StateError},
		{"unknown", 0, StateError},
	}

	for _, tt := range tests {
		got := mapDockerState(tt.status, tt.exitCode)
		if got != tt.want {
			t.Errorf("mapDockerState(%q, %d) = %q, want %q", tt.status, tt.exitCode, got, tt.want)
		}
	}
}

func TestStatusReconcilesCrashToError(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	store := &Store{db: dbHandle.DB, docker: fake, dataDir: dir, jars: jars.NewResolver()}

	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "", StateStopped)

	ctx := context.Background()
	srv, err := store.Start(ctx, id)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.State != StateRunning {
		t.Fatalf("expected state running, got %s", srv.State)
	}

	// Simulate server process crashing with exit code 1
	fake.mu.Lock()
	fake.inspectState = &docker.ContainerState{
		Status:   "exited",
		ExitCode: 1,
	}
	fake.mu.Unlock()

	status, err := store.Status(ctx, id)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.State != StateError {
		t.Errorf("expected state %q after crash, got %q", StateError, status.State)
	}

	// Verify database was also updated to StateError
	reloaded, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if reloaded.State != StateError {
		t.Errorf("expected reloaded state %q, got %q", StateError, reloaded.State)
	}
}

func TestStopCommandFor(t *testing.T) {
	cases := []struct {
		serverType string
		want       string
	}{
		{"paper", "stop"},
		{"vanilla", "stop"},
		{"fabric", "stop"},
		{"forge", "stop"},
		{"neoforge", "stop"},
		{"spigot", "stop"},
		{"purpur", "stop"},
		{"folia", "stop"},
		{"bungeecord", "end"},
		{"waterfall", "end"},
		{"velocity", "shutdown"},
		{"geysermc", "geyser stop"},
		{"unknown", "stop"},
	}
	for _, tc := range cases {
		got := stopCommandFor(tc.serverType)
		if got != tc.want {
			t.Errorf("stopCommandFor(%q) = %q, want %q", tc.serverType, got, tc.want)
		}
	}
}

func TestStopSendsGracefulConsoleCommand(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	store := &Store{db: dbHandle.DB, docker: fake, dataDir: dir, jars: jars.NewResolver()}

	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "", StateStopped)

	ctx := context.Background()
	srv, err := store.Start(ctx, id)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.State != StateRunning {
		t.Fatalf("expected state running, got %s", srv.State)
	}

	fake.mu.Lock()
	fake.inspectState = &docker.ContainerState{Status: StateRunning}
	fake.mu.Unlock()

	// When SendConsole is called, simulate server shutting down shortly after
	go func() {
		for {
			time.Sleep(50 * time.Millisecond)
			fake.mu.Lock()
			if len(fake.consoleCommands) > 0 {
				fake.inspectState = &docker.ContainerState{Status: "exited", ExitCode: 0}
				fake.mu.Unlock()
				return
			}
			fake.mu.Unlock()
		}
	}()

	srv, err = store.Stop(ctx, id)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if srv.State != StateStopped {
		t.Errorf("expected state %q, got %q", StateStopped, srv.State)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.consoleCommands) == 0 || fake.consoleCommands[0] != "stop" {
		t.Errorf("expected console command 'stop', got %v", fake.consoleCommands)
	}
	// Verify it did not need to fall back to hard docker Stop
	if fake.stopCalled {
		t.Errorf("expected clean exit without fallback docker.Stop")
	}
}

func TestSendConsoleCommandStopSetsStoppingState(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	store := &Store{db: dbHandle.DB, docker: fake, dataDir: dir, jars: jars.NewResolver()}

	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "", StateStopped)

	ctx := context.Background()
	_, err = store.Start(ctx, id)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Sending "stop" to running server sets StateStopping
	if err := store.SendConsoleCommand(ctx, id, "stop"); err != nil {
		t.Fatalf("SendConsoleCommand: %v", err)
	}
	srv, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if srv.State != StateStopping {
		t.Errorf("expected state %q after console stop, got %q", StateStopping, srv.State)
	}

	// Re-set to running and test "/stop" with leading slash
	if err := store.setState(ctx, id, StateRunning); err != nil {
		t.Fatalf("setState: %v", err)
	}
	if err := store.SendConsoleCommand(ctx, id, "/stop"); err != nil {
		t.Fatalf("SendConsoleCommand: %v", err)
	}
	srv, err = store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if srv.State != StateStopping {
		t.Errorf("expected state %q after /stop, got %q", StateStopping, srv.State)
	}
}

func TestStopFallbackWhenConsoleTimesOut(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	store := &Store{db: dbHandle.DB, docker: fake, dataDir: dir, jars: jars.NewResolver()}

	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "", StateStopped)

	ctx := context.Background()
	_, err = store.Start(ctx, id)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	fake.mu.Lock()
	fake.inspectState = &docker.ContainerState{Status: StateRunning}
	fake.mu.Unlock()

	// Call Stop with a 50ms deadline to trigger timeout fallback quickly
	stopCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	// When Stop falls back to docker.Stop, simulate container exit
	fake.mu.Lock()
	fake.inspectState = &docker.ContainerState{Status: StateRunning}
	fake.mu.Unlock()

	srv, err := store.Stop(stopCtx, id)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if srv.State != StateStopped {
		t.Errorf("expected state %q, got %q", StateStopped, srv.State)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !fake.stopCalled {
		t.Errorf("expected fake.stopCalled == true on fallback")
	}
	if fake.lastStopTimeout != 10*time.Second {
		t.Errorf("expected lastStopTimeout 10s, got %v", fake.lastStopTimeout)
	}
}

func TestCreateServerBehindProxyNoHostPort(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	store := NewStore(dbHandle, nil, jars.NewResolver(), 25565, 25570, dir, dir)
	store.docker = fake

	ctx := context.Background()

	// 1. Create first server behind proxy with NoHostPort: true
	srv1, err := store.Create(ctx, CreateInput{
		Name:       "Lobby Backend",
		ServerType: "paper",
		Version:    "1.21.1",
		RAMMB:      2048,
		NoHostPort: true,
	})
	if err != nil {
		t.Fatalf("Create srv1: %v", err)
	}
	if srv1.HostPort != 0 {
		t.Fatalf("expected srv1.HostPort == 0, got %d", srv1.HostPort)
	}

	// 2. Create second server behind proxy with NoHostPort: true (must not conflict on unique constraint)
	srv2, err := store.Create(ctx, CreateInput{
		Name:       "Survival Backend",
		ServerType: "paper",
		Version:    "1.21.1",
		RAMMB:      4096,
		NoHostPort: true,
	})
	if err != nil {
		t.Fatalf("Create srv2 with 0 host port failed (conflict?): %v", err)
	}
	if srv2.HostPort != 0 {
		t.Fatalf("expected srv2.HostPort == 0, got %d", srv2.HostPort)
	}

	// 3. Create a normal server with a port
	srv3, err := store.Create(ctx, CreateInput{
		Name:       "Proxy Velocity",
		ServerType: "velocity",
		Version:    "3.3.0-SNAPSHOT",
		RAMMB:      1024,
		HostPort:   25577,
	})
	if err != nil {
		t.Fatalf("Create proxy: %v", err)
	}
	if srv3.HostPort != 25577 {
		t.Fatalf("expected proxy HostPort == 25577, got %d", srv3.HostPort)
	}

	// 4. Update srv3 to move behind proxy (HostPort -> 0)
	zero := 0
	updatedSrv3, err := store.Update(ctx, srv3.ID, UpdateInput{
		HostPort: &zero,
	})
	if err != nil {
		t.Fatalf("Update srv3 to 0 port failed: %v", err)
	}
	if updatedSrv3.HostPort != 0 {
		t.Fatalf("expected updatedSrv3.HostPort == 0, got %d", updatedSrv3.HostPort)
	}
}

func TestHasForgeJar(t *testing.T) {
	tmp := t.TempDir()

	// Empty dir: no forge jar
	if hasForgeJar(tmp) {
		t.Errorf("expected hasForgeJar to be false for empty dir")
	}

	// Only installer.jar: should not be treated as installed forge jar
	installerPath := filepath.Join(tmp, "installer.jar")
	if err := os.WriteFile(installerPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	if hasForgeJar(tmp) {
		t.Errorf("expected hasForgeJar to be false when only installer.jar is present")
	}

	// Another installer name: forge-1.20.1-47.2.0-installer.jar
	forgeInstallerPath := filepath.Join(tmp, "forge-1.20.1-47.2.0-installer.jar")
	if err := os.WriteFile(forgeInstallerPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	if hasForgeJar(tmp) {
		t.Errorf("expected hasForgeJar to be false when only installer jars are present")
	}

	// Legacy Forge server jar: forge-1.12.2-14.23.5.2860.jar
	legacyJarPath := filepath.Join(tmp, "forge-1.12.2-14.23.5.2860.jar")
	if err := os.WriteFile(legacyJarPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	if !hasForgeJar(tmp) {
		t.Errorf("expected hasForgeJar to be true when forge server jar is present")
	}
}

func TestPendingInitialState(t *testing.T) {
	dir := t.TempDir()
	store := &Store{dataDir: dir}

	// 1. Forge with installer.jar and no server.jar
	forgeID := "srv-forge"
	forgeDir := filepath.Join(dir, "servers", forgeID)
	_ = os.MkdirAll(forgeDir, 0755)
	_ = os.WriteFile(filepath.Join(forgeDir, "installer.jar"), []byte("jar"), 0644)
	if got := store.pendingInitialState(forgeID, Server{ServerType: "forge"}); got != StateInstalling {
		t.Errorf("expected %q for forge with installer, got %q", StateInstalling, got)
	}

	// 2. Spigot with installer.jar and no server.jar
	spigotID := "srv-spigot"
	spigotDir := filepath.Join(dir, "servers", spigotID)
	_ = os.MkdirAll(spigotDir, 0755)
	_ = os.WriteFile(filepath.Join(spigotDir, "installer.jar"), []byte("jar"), 0644)
	if got := store.pendingInitialState(spigotID, Server{ServerType: "spigot"}); got != StateBuilding {
		t.Errorf("expected %q for spigot with installer, got %q", StateBuilding, got)
	}

	// 3. Sponge with missing libraries/ dir
	spongeID := "srv-sponge"
	spongeDir := filepath.Join(dir, "servers", spongeID)
	_ = os.MkdirAll(spongeDir, 0755)
	_ = os.WriteFile(filepath.Join(spongeDir, "server.jar"), []byte("sponge-jar"), 0644)
	if got := store.pendingInitialState(spongeID, Server{ServerType: "sponge"}); got != StateInstalling {
		t.Errorf("expected %q for sponge with no libraries, got %q", StateInstalling, got)
	}

	// 4. Sponge with populated libraries/ dir
	_ = os.MkdirAll(filepath.Join(spongeDir, "libraries", "sponge"), 0755)
	_ = os.WriteFile(filepath.Join(spongeDir, "libraries", "sponge", "lib.jar"), []byte("lib"), 0644)
	if got := store.pendingInitialState(spongeID, Server{ServerType: "sponge"}); got != "" {
		t.Errorf("expected empty string for sponge with populated libraries, got %q", got)
	}

	// 5. Standard paper server with server.jar
	paperID := "srv-paper"
	paperDir := filepath.Join(dir, "servers", paperID)
	_ = os.MkdirAll(paperDir, 0755)
	_ = os.WriteFile(filepath.Join(paperDir, "server.jar"), []byte("paper-jar"), 0644)
	if got := store.pendingInitialState(paperID, Server{ServerType: "paper"}); got != "" {
		t.Errorf("expected empty string for paper with server.jar, got %q", got)
	}
}

func TestStatusReconcilesInstallingAndBuilding(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	store := &Store{db: dbHandle.DB, docker: fake, dataDir: dir, jars: jars.NewResolver()}

	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "", StateStopped)
	serverDir := filepath.Join(dir, "servers", id)
	_ = os.MkdirAll(serverDir, 0755)

	ctx := context.Background()
	srv, err := store.Start(ctx, id)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.State != StateRunning {
		t.Fatalf("expected state running, got %s", srv.State)
	}

	// Container reports running, but writes .mcm_state = "installing"
	_ = os.WriteFile(filepath.Join(serverDir, ".mcm_state"), []byte("installing\n"), 0644)
	fake.mu.Lock()
	fake.inspectState = &docker.ContainerState{Status: StateRunning}
	fake.mu.Unlock()

	status, err := store.Status(ctx, id)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.State != StateInstalling {
		t.Errorf("expected state %q when .mcm_state is installing, got %q", StateInstalling, status.State)
	}

	// Change .mcm_state to "building"
	_ = os.WriteFile(filepath.Join(serverDir, ".mcm_state"), []byte("building\n"), 0644)
	status, err = store.Status(ctx, id)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.State != StateBuilding {
		t.Errorf("expected state %q when .mcm_state is building, got %q", StateBuilding, status.State)
	}

	// Installer completes and removes .mcm_state
	_ = os.Remove(filepath.Join(serverDir, ".mcm_state"))
	status, err = store.Status(ctx, id)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.State != StateRunning {
		t.Errorf("expected state %q after .mcm_state removed, got %q", StateRunning, status.State)
	}
}

func TestStartDetectsInstallerAndSetsState(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	store := &Store{db: dbHandle.DB, docker: fake, dataDir: dir, jars: jars.NewResolver()}

	// Forge server
	forgeID := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		forgeID, "test-forge", "forge", "1.20.1", "47.2.0", 2048, 0, 0, 25566, "[]", "cont-forge", StateStopped, now, now)
	if err != nil {
		t.Fatalf("insert forge server: %v", err)
	}
	forgeDir := filepath.Join(dir, "servers", forgeID)
	_ = os.MkdirAll(forgeDir, 0755)
	_ = os.WriteFile(filepath.Join(forgeDir, "installer.jar"), []byte("installer"), 0644)

	ctx := context.Background()
	srv, err := store.Start(ctx, forgeID)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.State != StateInstalling {
		t.Errorf("expected state %q for forge server with installer.jar, got %q", StateInstalling, srv.State)
	}

	// Spigot server
	spigotID := uuid.NewString()
	_, err = dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		spigotID, "test-spigot", "spigot", "1.20.4", "latest", 2048, 0, 0, 25567, "[]", "cont-spigot", StateStopped, now, now)
	if err != nil {
		t.Fatalf("insert spigot server: %v", err)
	}
	spigotDir := filepath.Join(dir, "servers", spigotID)
	_ = os.MkdirAll(spigotDir, 0755)
	_ = os.WriteFile(filepath.Join(spigotDir, "installer.jar"), []byte("buildtools"), 0644)

	srvSpigot, err := store.Start(ctx, spigotID)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srvSpigot.State != StateBuilding {
		t.Errorf("expected state %q for spigot server with installer.jar, got %q", StateBuilding, srvSpigot.State)
	}
}

func TestStartJarDownloadStateAndContextResilience(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer dbHandle.Close()

	var canceledDuringDownload atomic.Bool
	var stateDuringDownload string
	var cancelFunc context.CancelFunc

	downloadStarted := make(chan struct{})
	srvMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(downloadStarted)
		if cancelFunc != nil {
			cancelFunc()
			canceledDuringDownload.Store(true)
		}
		w.Header().Set("Content-Type", "application/java-archive")
		_, _ = w.Write([]byte("fake-server-jar-content"))
	}))
	defer srvMock.Close()

	resolver := jars.NewResolver()
	resolver.BuildToolsURL = srvMock.URL

	fake := &fakeRuntime{}
	store := &Store{db: dbHandle.DB, docker: fake, dataDir: dir, jars: resolver}

	sid := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sid, "test-resilience", "spigot", "1.20.4", "latest", 2048, 0, 0, 25568, "[]", "", StateStopped, now, now)
	if err != nil {
		t.Fatalf("insert server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelFunc = cancel

	go func() {
		<-downloadStarted
		// Query database state to verify server state is StateInstalling while downloading
		var st string
		_ = dbHandle.DB.QueryRow("SELECT state FROM servers WHERE id = ?", sid).Scan(&st)
		stateDuringDownload = st
	}()

	srv, err := store.Start(ctx, sid)
	if err != nil {
		t.Fatalf("Start should succeed despite client disconnect: %v", err)
	}

	if !canceledDuringDownload.Load() {
		t.Error("expected client context to have been canceled during download")
	}
	if stateDuringDownload != StateInstalling {
		t.Errorf("expected state during download to be %q, got %q", StateInstalling, stateDuringDownload)
	}
	if srv.State != StateBuilding {
		t.Errorf("expected server to be in StateBuilding after spigot install, got %q", srv.State)
	}

	// Test download failure setting StateError
	srvFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srvFail.Close()

	resolver.BuildToolsURL = srvFail.URL
	failID := uuid.NewString()
	_, err = dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		failID, "test-fail", "spigot", "1.20.4", "latest", 2048, 0, 0, 25569, "[]", "", StateStopped, now, now)
	if err != nil {
		t.Fatalf("insert fail server: %v", err)
	}

	_, err = store.Start(context.Background(), failID)
	if err == nil {
		t.Fatal("expected Start to fail when download fails")
	}

	var failState string
	_ = dbHandle.DB.QueryRow("SELECT state FROM servers WHERE id = ?", failID).Scan(&failState)
	if failState != StateError {
		t.Errorf("expected server state to be %q on download failure, got %q", StateError, failState)
	}
}


