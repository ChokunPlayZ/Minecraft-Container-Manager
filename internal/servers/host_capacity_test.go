package servers

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
)

type hostUsageRuntime struct {
	fakeRuntime
	capacity    docker.HostCapacity
	capacityErr error
	samples     map[string]docker.ContainerStats
	failID      string
	callsMu     sync.Mutex
	calls       []string
}

func (f *hostUsageRuntime) HostCapacity(context.Context) (docker.HostCapacity, error) {
	return f.capacity, f.capacityErr
}

func (f *hostUsageRuntime) Stats(ctx context.Context, id string) (docker.ContainerStats, error) {
	f.callsMu.Lock()
	f.calls = append(f.calls, id)
	f.callsMu.Unlock()
	if err := ctx.Err(); err != nil {
		return docker.ContainerStats{}, err
	}
	if id == f.failID {
		return docker.ContainerStats{}, errors.New("stats unavailable")
	}
	return f.samples[id], nil
}

func newHostUsageStore(t *testing.T) (*Store, *hostUsageRuntime) {
	t.Helper()
	dir := t.TempDir()
	handle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	store := NewStore(handle, nil, nil, 25565, 25665, dir, "/a/host/path/not/visible/in/the/panel")
	t.Cleanup(func() { store.WaitProvisioning(context.Background()) })
	runtime := &hostUsageRuntime{capacity: docker.HostCapacity{Runtime: "docker", CPUCores: 4, MemoryTotalBytes: 8 << 30}, samples: map[string]docker.ContainerStats{}}
	store.docker = runtime
	return store, runtime
}

func insertHostUsageServer(t *testing.T, store *Store, id, containerID, state string, port int) {
	t.Helper()
	_, err := store.db.Exec(`INSERT INTO servers (id,name,server_type,version,ram_mb,host_port,container_id,state,created_at,updated_at)
		VALUES (?,?,'paper','1.21.4',8192,?,?,?,datetime('now'),datetime('now'))`, id, id, port, containerID, state)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHostUsageMeasuresContainersAndDataFilesystem(t *testing.T) {
	store, runtime := newHostUsageStore(t)
	insertHostUsageServer(t, store, "one", "c-one", StateRunning, 25565)
	insertHostUsageServer(t, store, "two", "c-two", StateInstalling, 25566)
	insertHostUsageServer(t, store, "stopped", "c-stopped", StateStopped, 25567)
	insertHostUsageServer(t, store, "error", "c-error", StateError, 25568)
	insertHostUsageServer(t, store, "no-container", "", StateStarting, 25569)
	runtime.samples["c-one"] = docker.ContainerStats{CPUPercent: 150, MemoryUsageBytes: 1 << 30, NetRxBytes: 1000, NetTxBytes: 2000}
	runtime.samples["c-two"] = docker.ContainerStats{CPUPercent: 50, MemoryUsageBytes: 2 << 30, NetRxBytes: 3000, NetTxBytes: 4000}
	result := store.HostCapacity(context.Background())
	if result.Usage == nil || result.Usage.CPUPercent == nil {
		t.Fatalf("missing usage: %+v", result)
	}
	if *result.Usage.CPUPercent != 50 || result.Usage.MemoryBytes != 3<<30 || result.Usage.ActiveServers != 2 {
		t.Fatalf("expected measured usage, not configured heap: %+v", result.Usage)
	}
	if result.Usage.NetRxBytes != 4000 || result.Usage.NetTxBytes != 6000 || len(runtime.calls) != 2 {
		t.Fatalf("unexpected aggregate: %+v, calls=%v", result.Usage, runtime.calls)
	}
	if result.Disk == nil || result.Disk.Path != store.dataDir || result.Disk.TotalBytes == 0 || result.Disk.AvailableBytes > result.Disk.TotalBytes-result.Disk.UsedBytes {
		t.Fatalf("expected the panel's data filesystem and available space: %+v", result.Disk)
	}
}

func TestHostUsageDoesNotReportFailedSamplesAsZero(t *testing.T) {
	store, runtime := newHostUsageStore(t)
	insertHostUsageServer(t, store, "one", "c-one", StateRunning, 25565)
	insertHostUsageServer(t, store, "two", "c-two", StateRunning, 25566)
	runtime.failID = "c-two"
	result := store.HostCapacity(context.Background())
	if result.Usage != nil || !strings.Contains(result.UsageError, "1 of 2") || result.Disk == nil || result.CPUCores != 4 {
		t.Fatalf("expected unavailable usage with independent capacity and disk: %+v", result)
	}
}

func TestHostUsageIndependentFailures(t *testing.T) {
	t.Run("runtime down", func(t *testing.T) {
		store, runtime := newHostUsageStore(t)
		runtime.capacityErr = errors.New("offline")
		result := store.HostCapacity(context.Background())
		if result.Disk == nil || result.RuntimeError == "" || result.Usage != nil {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
	t.Run("runtime missing", func(t *testing.T) {
		store, _ := newHostUsageStore(t)
		store.docker = nil
		result := store.HostCapacity(context.Background())
		if result.Disk == nil || result.RuntimeError == "" || result.Usage != nil {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
	t.Run("data directory missing", func(t *testing.T) {
		store, _ := newHostUsageStore(t)
		store.dataDir = filepath.Join(t.TempDir(), "missing")
		result := store.HostCapacity(context.Background())
		if result.Disk != nil || result.DiskError == "" || result.Usage == nil {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
	t.Run("unknown CPU total", func(t *testing.T) {
		store, runtime := newHostUsageStore(t)
		runtime.capacity.CPUCores = 0
		result := store.HostCapacity(context.Background())
		if result.Usage == nil || result.Usage.CPUPercent != nil {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
	t.Run("no active containers", func(t *testing.T) {
		store, _ := newHostUsageStore(t)
		result := store.HostCapacity(context.Background())
		if result.Usage == nil || *result.Usage.CPUPercent != 0 || result.Usage.MemoryBytes != 0 || result.Usage.ActiveServers != 0 {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
	t.Run("cancelled request", func(t *testing.T) {
		store, _ := newHostUsageStore(t)
		insertHostUsageServer(t, store, "one", "c-one", StateRunning, 25565)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result := store.HostCapacity(ctx)
		if result.Usage != nil || result.UsageError == "" {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
}
