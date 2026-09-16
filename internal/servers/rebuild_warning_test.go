package servers

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
)

func TestRebuildWarningLifecycle(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	store := &Store{db: dbHandle.DB, docker: fake, dataDir: dir}

	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "", StateStopped)

	ctx := context.Background()

	// 1. Fresh server before container creation: needs_rebuild must be false
	srv, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == false before container creation, got true")
	}

	// 2. Start server to create container: needs_rebuild must be false
	srv, err = store.Start(ctx, id)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.ContainerID == "" {
		t.Fatalf("expected ContainerID to be non-empty after start")
	}
	if srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == false after starting container, got true")
	}

	// 3. Update RAM from 2048 to 4096: container is not detached, so needs_rebuild must become true
	newRAM := 4096
	srv, err = store.Update(ctx, id, UpdateInput{RAMMB: &newRAM})
	if err != nil {
		t.Fatalf("Update RAM: %v", err)
	}
	if !srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == true after changing RAM without rebuild")
	}
	if len(srv.RebuildReasons) == 0 {
		t.Errorf("expected non-empty RebuildReasons after changing RAM")
	}

	// Verify List also returns NeedsRebuild == true
	all, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) == 0 || !all[0].NeedsRebuild {
		t.Errorf("expected List to report NeedsRebuild == true for server")
	}

	// 4. Update CPU and Memory limit: reasons should include them
	newCPU := 2.5
	newMemLimit := 6144
	srv, err = store.Update(ctx, id, UpdateInput{
		CPULimit:      &newCPU,
		MemoryLimitMB: &newMemLimit,
	})
	if err != nil {
		t.Fatalf("Update limits: %v", err)
	}
	if !srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == true")
	}
	if len(srv.RebuildReasons) < 3 {
		t.Errorf("expected at least 3 rebuild reasons (RAM, CPU, memory limit), got %v", srv.RebuildReasons)
	}

	// 5. Reverting values back to applied container settings clears NeedsRebuild
	origRAM := 2048
	origCPU := 0.0
	origMemLimit := 0
	srv, err = store.Update(ctx, id, UpdateInput{
		RAMMB:         &origRAM,
		CPULimit:      &origCPU,
		MemoryLimitMB: &origMemLimit,
	})
	if err != nil {
		t.Fatalf("Revert limits: %v", err)
	}
	if srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == false after reverting to container settings, got true: %v", srv.RebuildReasons)
	}

	// 6. Update ExtraPorts: should trigger NeedsRebuild
	ports := []ExtraPort{
		{
			ID:            "webui",
			Description:   "Web UI",
			HostPort:      8080,
			ContainerPort: 8080,
			Protocol:      "tcp",
		},
	}
	srv, err = store.Update(ctx, id, UpdateInput{ExtraPorts: &ports})
	if err != nil {
		t.Fatalf("Update ExtraPorts: %v", err)
	}
	if !srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == true after adding ExtraPort")
	}

	// 7. Calling Recreate detaches container and clears NeedsRebuild
	srv, err = store.Recreate(ctx, id)
	if err != nil {
		t.Fatalf("Recreate: %v", err)
	}
	if srv.ContainerID != "" {
		t.Errorf("expected ContainerID == '' after Recreate, got %q", srv.ContainerID)
	}
	if srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == false after Recreate")
	}

	// 8. Next Start provisions fresh container with the extra ports, needs_rebuild stays false
	srv, err = store.Start(ctx, id)
	if err != nil {
		t.Fatalf("Start after recreate: %v", err)
	}
	if srv.ContainerID == "" {
		t.Fatalf("expected new container after Start")
	}
	if srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == false after Start with newly provisioned container")
	}
}

func TestRebuildWarningExtraPortsUnchanged(t *testing.T) {
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

	// Update with identical empty extra ports should not trigger rebuild
	emptyPorts := []ExtraPort{}
	srv, err := store.Update(ctx, id, UpdateInput{ExtraPorts: &emptyPorts})
	if err != nil {
		t.Fatalf("Update empty ports: %v", err)
	}
	if srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == false for unchanged empty extra ports")
	}
}

func TestRebuildWarningJavaVersion(t *testing.T) {
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
	if srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == false after Start")
	}

	// Update JavaVersion from 21 to 17
	newJava := 17
	srv, err = store.Update(ctx, id, UpdateInput{JavaVersion: &newJava})
	if err != nil {
		t.Fatalf("Update JavaVersion: %v", err)
	}
	if !srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == true after changing JavaVersion")
	}
	found := false
	for _, r := range srv.RebuildReasons {
		if r == "Java version changed from 21 to 17" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Java version change reason in %v", srv.RebuildReasons)
	}
}

func TestRebuildWarningEntrypointVersion(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fake := &fakeRuntime{}
	store := &Store{db: dbHandle.DB, docker: fake, dataDir: dir, jars: jars.NewResolver()}

	id := uuid.NewString()
	// Insert server with an older container_config that has EntrypointVersion 0 / missing
	oldConfig := `{"server_type":"paper","version":"1.21.1","build":"120","ram_mb":2048,"cpu_limit":0,"memory_limit_mb":0,"host_port":25565,"java_version":21,"extra_ports":[]}`
	_, err = dbHandle.DB.Exec(`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at, container_config, java_version) VALUES (?, 'legacy', 'paper', '1.21.1', '120', 2048, 0, 0, 25565, '[]', 'container-legacy', 'stopped', datetime('now'), datetime('now'), ?, 21)`, id, oldConfig)
	if err != nil {
		t.Fatalf("insert legacy server: %v", err)
	}

	ctx := context.Background()
	srv, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !srv.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == true for container with legacy entrypoint version")
	}
	found := false
	for _, r := range srv.RebuildReasons {
		if r == "Container runtime script updated to support clean process exit on crash" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected entrypoint update rebuild reason in %v", srv.RebuildReasons)
	}

	// Insert server with EntrypointVersion 1 (legacy before graceful stop trap)
	id2 := uuid.NewString()
	v1Config := `{"server_type":"paper","version":"1.21.1","build":"120","ram_mb":2048,"cpu_limit":0,"memory_limit_mb":0,"host_port":25566,"java_version":21,"extra_ports":[],"entrypoint_version":1}`
	_, err = dbHandle.DB.Exec(`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at, container_config, java_version) VALUES (?, 'v1server', 'paper', '1.21.1', '120', 2048, 0, 0, 25566, '[]', 'container-v1', 'stopped', datetime('now'), datetime('now'), ?, 21)`, id2, v1Config)
	if err != nil {
		t.Fatalf("insert v1 server: %v", err)
	}

	srv2, err := store.Get(ctx, id2)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !srv2.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == true for container with entrypoint version 1")
	}

	// Insert server with EntrypointVersion 2 (latest)
	id3 := uuid.NewString()
	v2Config := `{"server_type":"paper","version":"1.21.1","build":"120","ram_mb":2048,"cpu_limit":0,"memory_limit_mb":0,"host_port":25567,"java_version":21,"extra_ports":[],"entrypoint_version":2}`
	_, err = dbHandle.DB.Exec(`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at, container_config, java_version) VALUES (?, 'v2server', 'paper', '1.21.1', '120', 2048, 0, 0, 25567, '[]', 'container-v2', 'stopped', datetime('now'), datetime('now'), ?, 21)`, id3, v2Config)
	if err != nil {
		t.Fatalf("insert v2 server: %v", err)
	}

	srv3, err := store.Get(ctx, id3)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if srv3.NeedsRebuild {
		t.Errorf("expected NeedsRebuild == false for container with entrypoint version 2, got reasons: %v", srv3.RebuildReasons)
	}
}

