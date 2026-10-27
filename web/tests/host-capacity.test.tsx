import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Server } from '../src/api/types';
import { HostCapacity } from '../src/components/host-capacity';

const capacity = { runtime: 'docker' as const, cpu_cores: 8, memory_total_bytes: 32 * 1024 ** 3 };
const server = (overrides: Partial<Server> = {}) => ({ ram_mb: 4096, memory_limit_mb: 0, cpu_limit: 2, state: 'stopped', ...overrides }) as Server;

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers(); });

describe('Host capacity', () => {
  it('compares daemon totals with all server limits, including default overhead and uncapped CPUs', async () => {
    vi.spyOn(api, 'hostCapacity').mockResolvedValue(capacity);
    render(<HostCapacity servers={[server(), server({ memory_limit_mb: 8192, cpu_limit: 0 })]} />);

    expect(await screen.findByText('32 GB')).toBeInTheDocument();
    expect(screen.getByText('8 cores')).toBeInTheDocument();
    expect(screen.getByText('2 cores in configured limits · 1 server uncapped')).toBeInTheDocument();
    expect(screen.getByText('14 GB in configured limits')).toBeInTheDocument();
    expect(screen.getByRole('meter', { name: 'Configured memory limits' })).toHaveAttribute('aria-valuenow', '43.75');
    expect(screen.getByRole('meter', { name: 'Configured CPU limits' })).toHaveAttribute('aria-valuenow', '25');
  });

  it('shows overcommit amounts while keeping meters within their accessible range', async () => {
    vi.spyOn(api, 'hostCapacity').mockResolvedValue(capacity);
    render(<HostCapacity servers={[server({ memory_limit_mb: 40960, cpu_limit: 10 })]} />);

    expect(await screen.findByText('8 GB over host capacity')).toBeInTheDocument();
    expect(screen.getByText('2 cores over host capacity')).toBeInTheDocument();
    for (const meter of screen.getAllByRole('meter')) expect(meter).toHaveAttribute('aria-valuenow', '100');
    expect(screen.getByRole('meter', { name: 'Configured memory limits' })).toHaveAttribute('aria-valuetext', '40 GB configured out of 32 GB capacity');
  });

  it('does not invent totals when the runtime omits them', async () => {
    vi.spyOn(api, 'hostCapacity').mockResolvedValue({ runtime: 'podman', cpu_cores: 0, memory_total_bytes: 0 });
    render(<HostCapacity servers={[]} />);

    expect(await screen.findByText('Podman host')).toBeInTheDocument();
    expect(screen.getAllByText('Not reported')).toHaveLength(2);
    expect(screen.queryByRole('meter')).not.toBeInTheDocument();
  });

  it('shows a skeleton while loading and waits for server data before showing allocations', async () => {
    let resolve!: (value: typeof capacity) => void;
    vi.spyOn(api, 'hostCapacity').mockReturnValue(new Promise((done) => { resolve = done; }));
    const view = render(<HostCapacity servers={null} />);
    expect(screen.getByLabelText('Loading host capacity')).toBeInTheDocument();
    await act(async () => { resolve(capacity); });
    expect(screen.queryByRole('meter')).not.toBeInTheDocument();
    view.rerender(<HostCapacity servers={[]} />);
    expect(screen.getAllByRole('meter')).toHaveLength(2);
    expect(screen.getByText('0 B in configured limits')).toBeInTheDocument();
  });

  it('recovers from runtime failures using the refresh control', async () => {
    vi.spyOn(api, 'hostCapacity').mockRejectedValueOnce(new Error('offline')).mockResolvedValue(capacity);
    render(<HostCapacity servers={[]} />);

    expect(await screen.findByRole('status')).toHaveTextContent('Host capacity is unavailable');
    fireEvent.click(screen.getByRole('button', { name: 'Refresh host capacity' }));
    expect(await screen.findByText('32 GB')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('refreshes capacity periodically and stops polling when unmounted', async () => {
    vi.useFakeTimers();
    const request = vi.spyOn(api, 'hostCapacity').mockResolvedValue(capacity);
    const view = render(<HostCapacity servers={[]} />);
    await act(async () => {});
    expect(request).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
    expect(request).toHaveBeenCalledTimes(2);
    view.unmount();
    await vi.advanceTimersByTimeAsync(30_000);
    expect(request).toHaveBeenCalledTimes(2);
  });

  it('keeps a slow request from overlapping with the next poll', async () => {
    vi.useFakeTimers();
    const request = vi.spyOn(api, 'hostCapacity').mockReturnValue(new Promise(() => {}));
    const view = render(<HostCapacity servers={[]} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    expect(request).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Refresh host capacity' })).toBeDisabled();
    view.unmount();
  });
});
