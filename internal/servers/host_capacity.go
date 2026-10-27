package servers

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/mcm-panel/mcm/internal/docker"
)

type HostResources struct {
	docker.HostCapacity
	Usage        *HostUsage `json:"usage"`
	Disk         *DataDisk  `json:"disk"`
	RuntimeError string     `json:"runtime_error,omitempty"`
	UsageError   string     `json:"usage_error,omitempty"`
	DiskError    string     `json:"disk_error,omitempty"`
}

// HostUsage sums measured usage for managed containers. CPU is normalized to
// the daemon's total cores (100% means the entire host), rather than one core.
type HostUsage struct {
	CPUPercent    *float64 `json:"cpu_percent"`
	MemoryBytes   uint64   `json:"memory_bytes"`
	NetRxBytes    uint64   `json:"net_rx_bytes"`
	NetTxBytes    uint64   `json:"net_tx_bytes"`
	ActiveServers int      `json:"active_servers"`
}

// DataDisk describes the filesystem containing MCM_DATA_DIR, including free
// blocks available to the panel's user, which may exclude reserved space.
type DataDisk struct {
	Path           string `json:"path"`
	TotalBytes     uint64 `json:"total_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

// HostCapacity keeps data-directory disk reporting independent of the runtime:
// an unreachable daemon must not hide the remaining space on the data disk.
func (s *Store) HostCapacity(ctx context.Context) HostResources {
	result := HostResources{}
	disk, err := dataDiskSpace(s.dataDir)
	if err != nil {
		result.DiskError = "Data directory disk space is unavailable. Check the data directory and its permissions."
	} else {
		result.Disk = &disk
	}
	if isDockerNil(s.docker) {
		result.RuntimeError = "Container metrics are unavailable. Check the container runtime and refresh."
		return result
	}
	result.HostCapacity, err = s.docker.HostCapacity(ctx)
	if err != nil {
		result.RuntimeError = "Container metrics are unavailable. Check the container runtime and refresh."
		return result
	}
	servers, err := s.List(ctx)
	if err != nil {
		result.UsageError = "Server usage is unavailable. Refresh to try again."
		return result
	}
	var ids []string
	for _, server := range servers {
		if server.ContainerID != "" && server.State != StateStopped && server.State != StateError {
			ids = append(ids, server.ContainerID)
		}
	}
	type sample struct {
		stats docker.ContainerStats
		err   error
	}
	samples := make([]sample, len(ids))
	jobs := make(chan int, len(ids))
	for i := range ids {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	for range min(8, len(ids)) {
		workers.Go(func() {
			for i := range jobs {
				if err := ctx.Err(); err != nil {
					samples[i].err = err
					continue
				}
				samples[i].stats, samples[i].err = s.docker.Stats(ctx, ids[i])
			}
		})
	}
	workers.Wait()
	usage := &HostUsage{ActiveServers: len(ids)}
	var cpu float64
	var failed int
	for _, sample := range samples {
		if sample.err != nil {
			failed++
			continue
		}
		cpu += sample.stats.CPUPercent
		usage.MemoryBytes += sample.stats.MemoryUsageBytes
		usage.NetRxBytes += sample.stats.NetRxBytes
		usage.NetTxBytes += sample.stats.NetTxBytes
	}
	if failed > 0 {
		result.UsageError = fmt.Sprintf("Usage unavailable for %d of %d active servers. Refresh to try again.", failed, len(ids))
		return result
	}
	if result.CPUCores > 0 {
		percent := math.Min(100, math.Max(0, cpu/float64(result.CPUCores)))
		usage.CPUPercent = &percent
	}
	result.Usage = usage
	return result
}
