package docker

import (
	"context"
	"fmt"
)

// HostCapacity describes the resources reported by the container daemon. Query
// the daemon rather than the panel process so remote hosts and VMs are accurate.
type HostCapacity struct {
	Runtime          string `json:"runtime"`
	CPUCores         int    `json:"cpu_cores"`
	MemoryTotalBytes uint64 `json:"memory_total_bytes"`
}

func (m *Manager) HostCapacity(ctx context.Context) (HostCapacity, error) {
	capacity := HostCapacity{Runtime: m.runtimeName()}
	info, err := m.client.Info(ctx)
	if err != nil {
		return capacity, fmt.Errorf("inspect %s host capacity: %w", m.runtimeName(), err)
	}
	if info.NCPU > 0 {
		capacity.CPUCores = info.NCPU
	}
	if info.MemTotal > 0 {
		capacity.MemoryTotalBytes = uint64(info.MemTotal)
	}
	return capacity, nil
}
