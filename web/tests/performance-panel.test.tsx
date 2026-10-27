import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '../src/api/client';
import type { PerformanceHistory, SparkReport } from '../src/api/types';
import { PerformancePanel } from '../src/components/performance-panel';

const history = (overrides: Partial<PerformanceHistory> = {}): PerformanceHistory => ({
  start: '2026-10-27T10:00:00Z', end: '2026-10-27T11:00:00Z',
  interval_seconds: 30, resolution_seconds: 30, retention_days: 7,
  settings: { collection_mode: 'off' },
  spark: { available: true, source: 'bundled' }, report: null,
  samples: [
    { timestamp: '2026-10-27T10:59:00Z', online: true, cpu_percent: 150, memory_bytes: 1024 ** 3, players: 2, tps: 19, tick_ms: 35 },
    { timestamp: '2026-10-27T10:59:30Z', online: true, cpu_percent: 50, memory_bytes: 2 * 1024 ** 3, players: 0, tps: null, tick_ms: null },
  ], ...overrides,
});

describe('PerformancePanel', () => {
  beforeEach(() => { vi.restoreAllMocks(); });
  afterEach(() => { vi.useRealTimers(); });

  it('defaults polling off, saves each transport, and can turn it off again', async () => {
    let current = history();
    vi.spyOn(api, 'performanceHistory').mockImplementation(async () => current);
    const save = vi.spyOn(api, 'setPerformanceCollection').mockImplementation(async (_id, mode) => {
      current = { ...current, settings: { collection_mode: mode } };
      return current.settings;
    });
    const user = userEvent.setup();
    render(<PerformancePanel serverId="one" isRunning />);
    const select = await screen.findByRole('combobox', { name: 'Player and tick metrics' });
    await waitFor(() => expect(select).toBeEnabled());
    expect(select).toHaveTextContent('Off (default)');
    expect(save).not.toHaveBeenCalled();
    for (const [mode, label] of [['console', 'Console commands'], ['rcon', 'RCON polling'], ['off', 'Off (default)']] as const) {
      await user.click(select);
      await user.click(screen.getByRole('option', { name: label }));
      await waitFor(() => expect(save).toHaveBeenLastCalledWith('one', mode));
      await waitFor(() => expect(select).toBeEnabled());
      expect(select).toHaveTextContent(label);
      if (mode === 'console') expect(screen.getByText(/Their responses appear in your server console/)).toBeInTheDocument();
      if (mode === 'rcon') expect(screen.getByText(/There is no console fallback/)).toBeInTheDocument();
    }
    expect(screen.getByRole('button', { name: 'Capture spark report' })).toBeEnabled();
  });

  it('preserves the saved selection when saving fails and displays polling failures', async () => {
    const load = vi.spyOn(api, 'performanceHistory').mockResolvedValue(history());
    vi.spyOn(api, 'setPerformanceCollection').mockRejectedValue(new Error('offline'));
    const user = userEvent.setup();
    const { unmount } = render(<PerformancePanel serverId="one" isRunning />);
    const select = await screen.findByRole('combobox', { name: 'Player and tick metrics' });
    await waitFor(() => expect(select).toBeEnabled());
    await user.click(select); await user.click(screen.getByRole('option', { name: 'RCON polling' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not save the collection mode');
    expect(select).toHaveTextContent('Off (default)');
    unmount();
    load.mockResolvedValue(history({ settings: { collection_mode: 'rcon', last_error: 'RCON is unavailable. Console polling was not used.' } }));
    render(<PerformancePanel serverId="one" isRunning />);
    expect(await screen.findByRole('alert')).toHaveTextContent('RCON is unavailable');
  });

  it('renders five charts, keeps missing ticks empty, and inspects history by keyboard', async () => {
    vi.spyOn(api, 'performanceHistory').mockResolvedValue(history());
    render(<PerformancePanel serverId="one" isRunning />);
    expect(await screen.findByRole('heading', { name: 'Performance history' })).toBeInTheDocument();
    const charts = await screen.findAllByRole('img', { name: /history\./ });
    expect(charts).toHaveLength(5);
    expect(screen.getByText('50.0%')).toBeInTheDocument();
    expect(screen.getAllByText('—')).toHaveLength(2);
    fireEvent.keyDown(charts[0], { key: 'Home' });
    expect(screen.getByText('150.0%')).toBeInTheDocument();
    expect(screen.getByText('35.0 ms')).toBeInTheDocument();
    expect(screen.getByText(/^Inspecting/)).toBeInTheDocument();
    fireEvent.keyDown(charts[0], { key: 'End' });
    expect(screen.getByText('50.0%')).toBeInTheDocument();
    fireEvent.keyDown(charts[0], { key: 'Escape' });
    expect(screen.queryByText(/^Inspecting/)).not.toBeInTheDocument();
  });

  it('loads another range and recovers from failed loading', async () => {
    const load = vi.spyOn(api, 'performanceHistory').mockRejectedValueOnce(new Error('offline')).mockResolvedValue(history());
    const user = userEvent.setup();
    render(<PerformancePanel serverId="one" isRunning />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load performance history');
    await user.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByRole('img', { name: /^CPU usage history/ })).toBeInTheDocument();
    await user.click(screen.getByRole('combobox', { name: 'History time range' }));
    await user.click(screen.getByRole('option', { name: 'Last 7 days' }));
    await waitFor(() => expect(load).toHaveBeenLastCalledWith('one', '7d'));
  });

  it('shows empty history, explains missing spark, and prevents offline captures', async () => {
    vi.spyOn(api, 'performanceHistory').mockResolvedValue(history({ samples: [], spark: { available: false, source: 'unavailable' } }));
    const capture = vi.spyOn(api, 'captureSparkReport');
    render(<PerformancePanel serverId="one" isRunning={false} />);
    expect(await screen.findByText('No performance history yet')).toBeInTheDocument();
    expect(screen.getByText(/Spark was not detected/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Capture spark report' })).toBeDisabled();
    expect(capture).not.toHaveBeenCalled();
  });

  it('captures once, polls for the result, and shows the report link', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const report: SparkReport = { id: 'report', status: 'capturing', started_at: '2026-10-27T11:00:00Z', existing: false };
    const load = vi.spyOn(api, 'performanceHistory').mockResolvedValue(history());
    vi.spyOn(api, 'captureSparkReport').mockResolvedValue(report);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<PerformancePanel serverId="one" isRunning />);
    await screen.findByText('Spark is bundled with this Paper server.');
    load.mockResolvedValue(history({ report }));
    await user.click(screen.getByRole('button', { name: 'Capture spark report' }));
    expect(await screen.findByRole('button', { name: 'Capturing report…' })).toBeDisabled();
    expect(api.captureSparkReport).toHaveBeenCalledExactlyOnceWith('one');
    load.mockResolvedValue(history({ report: { ...report, status: 'completed', url: 'https://spark.lucko.me/result' } }));
    await act(async () => { await vi.advanceTimersByTimeAsync(3100); });
    expect(await screen.findByRole('link', { name: 'Open spark report' })).toHaveAttribute('href', 'https://spark.lucko.me/result');
    expect(screen.getByRole('button', { name: 'Capture spark report' })).toBeEnabled();
  });

  it('shows actionable capture errors and rejects untrusted result links', async () => {
    vi.spyOn(api, 'performanceHistory').mockResolvedValue(history({ report: { id: 'bad', status: 'completed', started_at: '', existing: false, url: 'javascript:alert(1)' } }));
    vi.spyOn(api, 'captureSparkReport').mockRejectedValue(new ApiError(409, 'busy', 'A capture is already running.'));
    const user = userEvent.setup();
    render(<PerformancePanel serverId="one" isRunning />);
    await screen.findByText('Spark is bundled with this Paper server.');
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Capture spark report' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('A capture is already running.');
  });
});
