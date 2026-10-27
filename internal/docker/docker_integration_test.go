//go:build integration

package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/config"
)

// openIntegrationManager uses the same Docker/Podman configuration as the app,
// skipping when the selected API is unavailable.
func openIntegrationManager(t *testing.T) *Manager {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := NewWithRuntime(cfg.ContainerRuntime, cfg.DockerHost, cfg.ServerImage)
	if err != nil {
		t.Skipf("%s client unavailable: %v", cfg.ContainerRuntime, err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := mgr.client.Ping(ctx); err != nil {
		t.Skipf("%s API unreachable: %v", cfg.ContainerRuntime, err)
	}
	return mgr
}

func TestContainerLifecycle(t *testing.T) {
	mgr := openIntegrationManager(t)
	ctx := context.Background()

	dataDir := filepath.Join(t.TempDir(), "servers", "itest")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}

	id := "itest-" + time.Now().Format("150405")
	cid, err := mgr.Create(ctx, CreateOpts{
		ID:         id,
		HostPort:   25699,
		DataDir:    dataDir,
		ServerType: "paper",
		Version:    "1.21.1",
		Build:      "120",
		RAMMB:      2048,
	})
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	t.Cleanup(func() {
		_ = mgr.Remove(context.Background(), cid)
	})

	if Name(id) != "mcm-"+id {
		t.Fatalf("unexpected container name %s", Name(id))
	}

	if err := mgr.Start(ctx, cid); err != nil {
		t.Fatalf("start container: %v", err)
	}
	status, err := mgr.Status(ctx, cid)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status != "running" {
		t.Fatalf("expected running, got %q", status)
	}

	rc, err := mgr.Logs(ctx, cid, false)
	if err == nil {
		rc.Close()
	}

	if err := mgr.Stop(ctx, cid, 10*time.Second); err != nil {
		t.Fatalf("stop container: %v", err)
	}
	status, err = mgr.Status(ctx, cid)
	if err != nil {
		t.Fatalf("status after stop: %v", err)
	}
	if status == "running" {
		t.Fatal("container still running after stop")
	}

	if err := mgr.Remove(ctx, cid); err != nil {
		t.Fatalf("remove container: %v", err)
	}
}
