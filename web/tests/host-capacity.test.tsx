import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { HostCapacity as Capacity } from '../src/api/types';
import { HostCapacity } from '../src/components/host-capacity';

const capacity: Capacity = {
  runtime: 'docker', cpu_cores: 8, memory_total_bytes: 32 * 1024 ** 3,
  usage: { cpu_percent: 14.8, memory_bytes: 3.2 * 1024 ** 3, net_rx_bytes: 12 * 1024 ** 2, net_tx_bytes: 33 * 1024 ** 2, active_servers: 2 },
  disk: { path: '/data/minecraft', total_bytes: 500 * 1024 ** 3, used_bytes: 180 * 1024 ** 3, available_bytes: 310 * 1024 ** 3 },
};

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers(); });

describe('Host resources', () => {
  it('shows measured server usage with host totals and available data filesystem space', async () => {
    vi.spyOn(api, 'hostCapacity').mockResolvedValue(capacity);
    render(<HostCapacity />);

    expect(await screen.findByText('14.8%')).toBeInTheDocument();
    expect(screen.getByText('8 cores total')).toBeInTheDocument();
    expect(screen.getByText('3.2 GB')).toBeInTheDocument();
    expect(screen.getByText('/ 32 GB')).toBeInTheDocument();
    expect(screen.getByText('310 GB')).toBeInTheDocument();
    expect(screen.getByText('180 GB / 500 GB used')).toBeInTheDocument();
    expect(screen.getByText('/data/minecraft')).toBeInTheDocument();
    expect(screen.getByRole('meter', { name: 'Server CPU usage' })).toHaveAttribute('aria-valuenow', '14.8');
    expect(screen.getByRole('meter', { name: 'Server memory usage' })).toHaveAttribute('aria-valuenow', '10');
    expect(screen.getByRole('meter', { name: 'Data disk usage' })).toHaveAttribute('aria-valuenow', '36');
    const network = within(screen.getByTestId('host-network'));
    expect(network.getByLabelText('Received')).toHaveTextContent('12 MB');
    expect(network.getByLabelText('Sent')).toHaveTextContent('33 MB');
  });

  it('keeps disk space visible when runtime metrics are unavailable', async () => {
    vi.spyOn(api, 'hostCapacity').mockResolvedValue({ ...capacity, usage: null, runtime_error: 'Container metrics are unavailable.' });
    render(<HostCapacity />);

    expect(await screen.findByRole('status')).toHaveTextContent('Container metrics are unavailable');
    expect(within(screen.getByTestId('host-cpu')).getByText('Unavailable')).toBeInTheDocument();
    expect(within(screen.getByTestId('host-disk')).getByText('310 GB')).toBeInTheDocument();
    expect(within(screen.getByTestId('host-disk')).getByText('Live')).toBeInTheDocument();
    expect(screen.queryByRole('meter', { name: 'Server CPU usage' })).not.toBeInTheDocument();
  });

  it('does not invent disk space when its filesystem cannot be read', async () => {
    vi.spyOn(api, 'hostCapacity').mockResolvedValue({ ...capacity, disk: null, disk_error: 'Data directory disk space is unavailable.' });
    render(<HostCapacity />);

    expect(await screen.findByRole('status')).toHaveTextContent('Data directory disk space is unavailable');
    expect(within(screen.getByTestId('host-disk')).getByText('—')).toBeInTheDocument();
    expect(screen.queryByRole('meter', { name: 'Data disk usage' })).not.toBeInTheDocument();
    expect(screen.getByText('14.8%')).toBeInTheDocument();
  });

  it('preserves known usage when host capacity is not reported', async () => {
    vi.spyOn(api, 'hostCapacity').mockResolvedValue({ ...capacity, runtime: 'podman', cpu_cores: 0, memory_total_bytes: 0, usage: { ...capacity.usage!, cpu_percent: null } });
    render(<HostCapacity />);

    expect(await screen.findByText('Podman host')).toBeInTheDocument();
    expect(screen.getByText('3.2 GB')).toBeInTheDocument();
    expect(screen.queryByRole('meter', { name: 'Server CPU usage' })).not.toBeInTheDocument();
    expect(screen.queryByRole('meter', { name: 'Server memory usage' })).not.toBeInTheDocument();
  });

  it('shows a skeleton while loading and retries failed requests', async () => {
    vi.spyOn(api, 'hostCapacity').mockRejectedValueOnce(new Error('offline')).mockResolvedValue(capacity);
    render(<HostCapacity />);
    expect(screen.getByLabelText('Loading host resources')).toBeInTheDocument();
    expect(await screen.findByRole('status')).toHaveTextContent('Host resources could not be refreshed');
    fireEvent.click(screen.getByRole('button', { name: 'Refresh host resources' }));
    expect(await screen.findByText('310 GB')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('stops labeling stale readings Live when a refresh fails', async () => {
    vi.spyOn(api, 'hostCapacity').mockResolvedValueOnce(capacity).mockRejectedValue(new Error('offline'));
    render(<HostCapacity />);
    expect(await screen.findByText('310 GB')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Refresh host resources' }));
    expect(await screen.findByRole('status')).toBeInTheDocument();
    expect(screen.queryByText('Live')).not.toBeInTheDocument();
    expect(screen.queryByText('310 GB')).not.toBeInTheDocument();
  });

  it('refreshes every five seconds and stops polling when unmounted', async () => {
    vi.useFakeTimers();
    const request = vi.spyOn(api, 'hostCapacity').mockResolvedValue(capacity);
    const view = render(<HostCapacity />);
    await act(async () => {});
    expect(request).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(request).toHaveBeenCalledTimes(2);
    view.unmount();
    await vi.advanceTimersByTimeAsync(5000);
    expect(request).toHaveBeenCalledTimes(2);
  });

  it('keeps slow requests from overlapping', async () => {
    vi.useFakeTimers();
    const request = vi.spyOn(api, 'hostCapacity').mockReturnValue(new Promise(() => {}));
    const view = render(<HostCapacity />);
    await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
    expect(request).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Refresh host resources' })).toBeDisabled();
    view.unmount();
  });
});
