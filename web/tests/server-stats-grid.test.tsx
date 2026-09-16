import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { ServerStats } from '../src/api/types';
import { ServerStatsGrid } from '../src/components/server-stats-grid';

const mockOnlineStats: ServerStats = {
  server_id: 'srv-1',
  online: true,
  cpu_percent: 24.5,
  cpu_cores: 4,
  cpu_limit: 2,
  memory_bytes: 2147483648, // 2 GB
  memory_limit_bytes: 4294967296, // 4 GB
  memory_percent: 50.0,
  disk_bytes: 838860800, // 800 MB
  disk_read_bytes: 10485760, // 10 MB
  disk_write_bytes: 20971520, // 20 MB
  net_rx_bytes: 5242880, // 5 MB
  net_tx_bytes: 15728640, // 15 MB
};

const mockStoppedStats: ServerStats = {
  server_id: 'srv-1',
  online: false,
  cpu_percent: 0,
  cpu_cores: 4,
  cpu_limit: 2,
  memory_bytes: 0,
  memory_limit_bytes: 4294967296,
  memory_percent: 0,
  disk_bytes: 838860800, // 800 MB (world size on disk)
  disk_read_bytes: 0,
  disk_write_bytes: 0,
  net_rx_bytes: 0,
  net_tx_bytes: 0,
};

describe('ServerStatsGrid Component', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders all 4 stat cards (CPU, MEM, Disk, Network)', async () => {
    vi.spyOn(api, 'serverStats').mockResolvedValue(mockOnlineStats);

    render(<ServerStatsGrid serverId="srv-1" isRunning={true} />);

    await waitFor(() => {
      expect(screen.getByTestId('stat-cpu')).toBeInTheDocument();
      expect(screen.getByTestId('stat-mem')).toBeInTheDocument();
      expect(screen.getByTestId('stat-disk')).toBeInTheDocument();
      expect(screen.getByTestId('stat-network')).toBeInTheDocument();
    });
  });

  it('displays accurate online stats for CPU, Memory, Disk, and Network with Live badges', async () => {
    vi.spyOn(api, 'serverStats').mockResolvedValue(mockOnlineStats);

    render(<ServerStatsGrid serverId="srv-1" isRunning={true} />);

    await waitFor(() => {
      expect(screen.getByTestId('stat-cpu-value')).toHaveTextContent('24.5%');
      expect(screen.getByTestId('stat-mem-value')).toHaveTextContent('2 GB');
      expect(screen.getByText(/\/ 4 GB/)).toBeInTheDocument();
      expect(screen.getByTestId('stat-disk-value')).toHaveTextContent('800 MB');
      expect(screen.getByTestId('stat-network-rx')).toHaveTextContent('5 MB');
      expect(screen.getByTestId('stat-network-tx')).toHaveTextContent('15 MB');
    });

    // Check Live indicators
    const liveIndicators = screen.getAllByText(/Live/i);
    expect(liveIndicators.length).toBeGreaterThan(0);
  });

  it('displays stopped/offline state with zero CPU/MEM while preserving disk usage', async () => {
    vi.spyOn(api, 'serverStats').mockResolvedValue(mockStoppedStats);

    render(<ServerStatsGrid serverId="srv-1" isRunning={false} />);

    await waitFor(() => {
      expect(screen.getByTestId('stat-cpu-value')).toHaveTextContent('0.0%');
      expect(screen.getByTestId('stat-mem-value')).toHaveTextContent('0 B');
      expect(screen.getByTestId('stat-disk-value')).toHaveTextContent('800 MB');
      expect(screen.getByTestId('stat-network-rx')).toHaveTextContent('0 B');
      expect(screen.getByTestId('stat-network-tx')).toHaveTextContent('0 B');
    });

    // Verify Offline label is shown
    expect(screen.getAllByText(/Offline/i).length).toBeGreaterThan(0);
  });

  it('handles API failure gracefully without crashing', async () => {
    vi.spyOn(api, 'serverStats').mockRejectedValue(new Error('Network error'));

    render(<ServerStatsGrid serverId="srv-1" isRunning={true} />);

    await waitFor(() => {
      expect(screen.getByTestId('server-stats-grid')).toBeInTheDocument();
      expect(screen.getByTestId('stat-cpu')).toBeInTheDocument();
    });
  });
});
