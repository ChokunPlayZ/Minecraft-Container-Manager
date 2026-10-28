import { act, render, screen, within } from '@testing-library/react';
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
    await user.click(screen.getByText('Update log'));
    expect(screen.getByLabelText('Panel update log')).toHaveTextContent('Readiness passed.');
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
    expect(screen.getByText('Update in progress')).toBeVisible();
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
});
