package docker

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

func TestCalculateStats(t *testing.T) {
	mgr := &Manager{
		cpuCache: make(map[string]cpuSample),
	}

	raw := &container.StatsResponse{
		Stats: container.Stats{
			CPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 500000000, // 0.5s CPU
				},
				SystemUsage: 2000000000,
				OnlineCPUs:  2,
			},
			PreCPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 400000000, // 0.4s CPU => delta 0.1s
				},
				SystemUsage: 1000000000, // delta 1.0s
			},
			MemoryStats: container.MemoryStats{
				Usage: 2 * 1024 * 1024 * 1024, // 2GB
				Limit: 4 * 1024 * 1024 * 1024, // 4GB
				Stats: map[string]uint64{
					"inactive_file": 512 * 1024 * 1024, // 512MB inactive cache
				},
			},
			BlkioStats: container.BlkioStats{
				IoServiceBytesRecursive: []container.BlkioStatEntry{
					{Op: "read", Value: 1048576},
					{Op: "write", Value: 2097152},
				},
			},
		},
		Networks: map[string]container.NetworkStats{
			"eth0": {
				RxBytes: 50000,
				TxBytes: 150000,
			},
		},
	}

	stats := mgr.CalculateStats("c-123", raw)

	// CPU percent: (0.1 / 1.0) * 2 cores * 100 = 20.00%
	if stats.CPUPercent != 20.0 {
		t.Errorf("expected CPUPercent 20.0, got %f", stats.CPUPercent)
	}
	if stats.CPUCores != 2 {
		t.Errorf("expected CPUCores 2, got %d", stats.CPUCores)
	}

	// Memory usage: 2GB - 512MB = 1.5GB
	expectedMem := uint64(1.5 * 1024 * 1024 * 1024)
	if stats.MemoryUsageBytes != expectedMem {
		t.Errorf("expected MemoryUsageBytes %d, got %d", expectedMem, stats.MemoryUsageBytes)
	}
	// Memory percent: 1.5GB / 4GB = 37.5%
	if stats.MemoryPercent != 37.5 {
		t.Errorf("expected MemoryPercent 37.5, got %f", stats.MemoryPercent)
	}

	// Disk I/O
	if stats.DiskReadBytes != 1048576 {
		t.Errorf("expected DiskReadBytes 1048576, got %d", stats.DiskReadBytes)
	}
	if stats.DiskWriteBytes != 2097152 {
		t.Errorf("expected DiskWriteBytes 2097152, got %d", stats.DiskWriteBytes)
	}

	// Network
	if stats.NetRxBytes != 50000 {
		t.Errorf("expected NetRxBytes 50000, got %d", stats.NetRxBytes)
	}
	if stats.NetTxBytes != 150000 {
		t.Errorf("expected NetTxBytes 150000, got %d", stats.NetTxBytes)
	}
}

func TestCalculateStatsFallbackCache(t *testing.T) {
	mgr := &Manager{
		cpuCache: make(map[string]cpuSample),
	}

	containerID := "c-fallback"
	// Populate cache with sample 1 second ago
	mgr.cpuCache[containerID] = cpuSample{
		readAt:      time.Now().Add(-1 * time.Second),
		totalUsage:  1000000000,
		systemUsage: 1000000000,
	}

	// Stats where PreCPUStats has no system usage (e.g. single-shot docker response)
	raw := &container.StatsResponse{
		Stats: container.Stats{
			CPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 1200000000, // delta 200ms
				},
				SystemUsage: 2000000000, // delta 1000ms
				OnlineCPUs:  1,
			},
		},
	}

	stats := mgr.CalculateStats(containerID, raw)

	// CPU percent: (200ms / 1000ms) * 1 core * 100 = 20.00%
	if stats.CPUPercent != 20.0 {
		t.Errorf("expected CPUPercent 20.0, got %f", stats.CPUPercent)
	}
}
