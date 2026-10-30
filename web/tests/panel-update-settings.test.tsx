import { act, fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { PanelUpdateStatus } from '../src/api/types';
import { PanelUpdateSettings } from '../src/components/panel-update-settings';

const commit = '1234567890abcdef1234567890abcdef12345678';
const base: PanelUpdateStatus = { build: { commit, dirty: false }, supported: true, available: false };
const available = { ...base, latest_commit: 'abcdef1234567890abcdef1234567890abcdef12', checked_at: '2026-10-28T10:00:00Z', available: true };

describe('Panel software updates', () => {
  beforeEach(() => { vi.restoreAllMocks(); vi.spyOn(api, 'getPanelUpdate').mockResolvedValue(base); });
  afterEach(() => { vi.useRealTimers(); });

  it('shows the full build commit and explains unsupported deployments', async () => {
    vi.mocked(api.getPanelUpdate).mockResolvedValue({ ...base, supported: false, reason: 'Rebuild the container image.' });
    render(<PanelUpdateSettings />);
    expect(await screen.findByText(commit)).toBeVisible();
    expect(screen.getByText('Rebuild the container image.')).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Check for panel updates' })).not.toBeInTheDocument();
  });

  it('checks, confirms, and sends the reviewed commit to the updater', async () => {
    const user = userEvent.setup();
    vi.spyOn(api, 'checkPanelUpdate').mockResolvedValue(available);
    const start = vi.spyOn(api, 'startPanelUpdate').mockResolvedValue({ ...available, job: { state: 'succeeded', started_at: '', message: 'Panel update complete.' }, log: 'Readiness passed.' });
    render(<PanelUpdateSettings />);
    await user.click(await screen.findByRole('button', { name: 'Check for panel updates' }));
    expect(await screen.findByText(available.latest_commit)).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Install panel update' }));
    expect(start).not.toHaveBeenCalled();
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveTextContent('backs up the panel database and settings');
    await user.click(within(dialog).getByRole('button', { name: 'Install update' }));
    expect(start).toHaveBeenCalledWith(available.latest_commit);
    expect(await screen.findByRole('button', { name: 'Reload updated panel' })).toBeVisible();
    expect(screen.getByRole('progressbar', { name: 'Panel update progress' })).toHaveAttribute('aria-valuenow', '100');
    await user.click(screen.getByText('Update log'));
    expect(screen.getByLabelText('Panel update log')).toHaveTextContent('Readiness passed.');
    await user.tab();
    expect(screen.getByRole('button', { name: 'Pause auto-scroll' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('region', { name: 'Panel update log' })).toHaveFocus();
  });

  it('cancels without starting the update and keeps check errors visible', async () => {
    const user = userEvent.setup();
    vi.mocked(api.getPanelUpdate).mockResolvedValue(available);
    const start = vi.spyOn(api, 'startPanelUpdate');
    vi.spyOn(api, 'checkPanelUpdate').mockRejectedValue(new Error('Commit or stash local changes.'));
    render(<PanelUpdateSettings />);
    await user.click(await screen.findByRole('button', { name: 'Install panel update' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(start).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Check for panel updates' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Commit or stash local changes.');
  });

  it('recovers polling through a restart and exposes a persisted failure log', async () => {
    vi.useFakeTimers();
    const running: PanelUpdateStatus = { ...base, job: { state: 'running', started_at: '', message: 'Building the panel.' } };
    vi.mocked(api.getPanelUpdate).mockResolvedValue(running);
    render(<PanelUpdateSettings />);
    await act(async () => { await Promise.resolve(); });
    expect(screen.getByRole('progressbar', { name: 'Panel update progress' })).not.toHaveAttribute('aria-valuenow');
    expect(screen.getByRole('region', { name: 'Panel update log' })).toHaveTextContent('Waiting for installer output');
    expect(screen.getByRole('button', { name: 'Check for panel updates' })).toBeDisabled();
    vi.mocked(api.getPanelUpdate).mockRejectedValue(new Error('Disconnected'));
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(screen.getByText(/The panel is reconnecting/)).toBeVisible();
    vi.mocked(api.getPanelUpdate).mockResolvedValue({ ...base, job: { state: 'failed', started_at: '', message: 'The previous version was restored.' }, log: 'Readiness failed; rollback completed.' });
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(screen.getByRole('alert')).toHaveTextContent('The previous version was restored.');
    expect(screen.getByLabelText('Panel update log')).toHaveTextContent('rollback completed');
    expect(screen.getByRole('button', { name: 'Check for panel updates' })).toBeEnabled();
  });

  it('shows installer milestones and preserves progress while reconnecting', async () => {
    vi.useFakeTimers();
    const running: PanelUpdateStatus = {
      ...base,
      job: { state: 'running', started_at: 'update-1', message: 'Building the panel.', progress: { stage: 'build', title: 'Building panel and web interface', step: 3, total: 6 } },
      log: 'Compiling web interface…',
    };
    vi.mocked(api.getPanelUpdate).mockResolvedValue(running);
    render(<PanelUpdateSettings />);
    await act(async () => { await Promise.resolve(); });
    const bar = screen.getByRole('progressbar', { name: 'Panel update progress' });
    expect(bar).toHaveAttribute('aria-valuetext', 'Building panel and web interface, Step 3 of 6');
    expect(screen.getByText('Step 3 of 6')).toBeVisible();
    const value = bar.getAttribute('aria-valuenow');
    vi.mocked(api.getPanelUpdate).mockRejectedValue(new Error('Disconnected'));
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(bar).toHaveAttribute('aria-valuenow', value);
    expect(bar).toHaveAttribute('aria-valuetext', expect.stringContaining('reconnecting'));
    expect(screen.getByRole('region', { name: 'Panel update log' })).toHaveTextContent('Compiling web interface');
    vi.mocked(api.getPanelUpdate).mockResolvedValue({ ...running, job: { ...running.job!, progress: { stage: 'verify', title: 'Checking panel readiness', step: 6, total: 6 } } });
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(screen.getByText('Step 6 of 6')).toBeVisible();
    expect(Number(bar.getAttribute('aria-valuenow'))).toBeLessThan(100);
  });

  it('lets readers scroll back without new output pulling them down and resume following', async () => {
    vi.useFakeTimers();
    const running: PanelUpdateStatus = { ...base, job: { state: 'running', started_at: 'update-1', message: 'Building panel.' }, log: 'First output' };
    vi.mocked(api.getPanelUpdate).mockResolvedValue(running);
    render(<PanelUpdateSettings />);
    await act(async () => { await Promise.resolve(); });
    const viewer = screen.getByRole('region', { name: 'Panel update log' });
    Object.defineProperties(viewer, {
      scrollHeight: { configurable: true, value: 1000 },
      clientHeight: { configurable: true, value: 300 },
    });
    viewer.scrollTop = 200;
    fireEvent.scroll(viewer);
    expect(screen.getByRole('button', { name: 'Resume auto-scroll' })).toHaveAttribute('aria-pressed', 'false');
    vi.mocked(api.getPanelUpdate).mockResolvedValue({ ...running, log: 'First output\nNew output' });
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(viewer).toHaveTextContent('New output');
    expect(viewer.scrollTop).toBe(200);
    fireEvent.click(screen.getByRole('button', { name: 'Jump to latest' }));
    expect(viewer.scrollTop).toBe(1000);
    expect(screen.getByRole('button', { name: 'Pause auto-scroll' })).toHaveAttribute('aria-pressed', 'true');
    Object.defineProperty(viewer, 'scrollHeight', { configurable: true, value: 1200 });
    vi.mocked(api.getPanelUpdate).mockResolvedValue({ ...running, log: 'Even newer output' });
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(viewer.scrollTop).toBe(1200);
    fireEvent.click(screen.getByRole('button', { name: 'Pause auto-scroll' }));
    Object.defineProperty(viewer, 'scrollHeight', { configurable: true, value: 1400 });
    vi.mocked(api.getPanelUpdate).mockResolvedValue({ ...running, log: 'Output while paused' });
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(viewer.scrollTop).toBe(1200);
    fireEvent.click(screen.getByRole('button', { name: 'Resume auto-scroll' }));
    expect(viewer.scrollTop).toBe(1400);
    fireEvent.click(screen.getByRole('button', { name: 'Update log' }));
    expect(screen.queryByRole('region', { name: 'Panel update log' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Update log' }));
    expect(screen.getByRole('region', { name: 'Panel update log' })).toBeVisible();
  });

  it('opens failure logs and identifies rollback without claiming completion', async () => {
    vi.mocked(api.getPanelUpdate).mockResolvedValue({ ...base, job: { state: 'failed', started_at: '', message: 'Previous version restored.', progress: { stage: 'rollback', title: 'Restoring the previous version', step: 0, total: 6 } }, log: 'Readiness failed.' });
    render(<PanelUpdateSettings />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Previous version restored.');
    expect(screen.getByRole('region', { name: 'Panel update log' })).toBeVisible();
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0');
    expect(screen.queryByRole('button', { name: 'Reload updated panel' })).not.toBeInTheDocument();
  });
});
