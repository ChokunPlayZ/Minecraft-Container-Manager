package servers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
)

func TestStoreStats(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { dbHandle.Close() })

	fake := &fakeRuntime{
		existing: map[string]bool{"c-running": true},
		statsResult: docker.ContainerStats{
			CPUPercent:       18.5,
			CPUCores:         4,
			MemoryUsageBytes: 1024 * 1024 * 1024, // 1GB
			MemoryLimitBytes: 4096 * 1024 * 1024,
			MemoryPercent:    25.0,
			DiskReadBytes:    5000,
			DiskWriteBytes:   12000,
			NetRxBytes:       100000,
			NetTxBytes:       250000,
		},
	}

	jr := jars.NewResolver()
	s := NewStore(dbHandle, nil, jr, 25565, 25570, dir, "")
	s.docker = fake

	// 1. Not found server
	_, err = s.Stats(context.Background(), "non-existent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// 2. Stopped server with files in dataDir
	srv, err := s.Create(context.Background(), CreateInput{
		Name:       "test-stats-srv",
		ServerType: jars.TypePaper,
		Version:    "1.21.4",
		RAMMB:      2048,
		CPULimit:   2.0,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// Write a test world file in dataPath
	dataDir := s.dataPath(srv.ID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("mkdir dataDir: %v", err)
	}
	testContent := make([]byte, 1024*64) // 64KB
	if err := os.WriteFile(filepath.Join(dataDir, "world.dat"), testContent, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	stats, err := s.Stats(context.Background(), srv.ID)
	if err != nil {
		t.Fatalf("Stats error: %v", err)
	}
	if stats.Online {
		t.Errorf("expected stopped server to have Online=false")
	}
	if stats.DiskBytes < 65536 {
		t.Errorf("expected DiskBytes >= 65536, got %d", stats.DiskBytes)
	}
	if stats.CPUPercent != 0 {
		t.Errorf("expected CPUPercent 0 for stopped server, got %f", stats.CPUPercent)
	}
	if stats.MemoryUsageBytes != 0 {
		t.Errorf("expected MemoryUsageBytes 0 for stopped server, got %d", stats.MemoryUsageBytes)
	}
	if stats.CPULimit != 2.0 {
		t.Errorf("expected CPULimit 2.0, got %f", stats.CPULimit)
	}

	// 3. Running server
	_, err = dbHandle.DB.Exec(`UPDATE servers SET container_id='c-running', state='running' WHERE id=?`, srv.ID)
	if err != nil {
		t.Fatalf("update server to running: %v", err)
	}

	rStats, err := s.Stats(context.Background(), srv.ID)
	if err != nil {
		t.Fatalf("Stats running error: %v", err)
	}
	if !rStats.Online {
		t.Errorf("expected Online=true for running server")
	}
	if rStats.CPUPercent != 18.5 {
		t.Errorf("expected CPUPercent 18.5, got %f", rStats.CPUPercent)
	}
	if rStats.CPUCores != 4 {
		t.Errorf("expected CPUCores 4, got %d", rStats.CPUCores)
	}
	if rStats.MemoryUsageBytes != 1024*1024*1024 {
		t.Errorf("expected MemoryUsageBytes 1GB, got %d", rStats.MemoryUsageBytes)
	}
	if rStats.NetRxBytes != 100000 || rStats.NetTxBytes != 250000 {
		t.Errorf("expected network stats mismatch: rx=%d, tx=%d", rStats.NetRxBytes, rStats.NetTxBytes)
	}
}
