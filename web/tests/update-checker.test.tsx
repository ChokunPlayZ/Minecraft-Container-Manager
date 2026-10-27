import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { UpdateCheckerSettings } from '../src/components/update-checker-settings';
import { UpdateAvailableBadge } from '../src/components/update-available-badge';
import type { UpdateCheckerStatus } from '../src/api/types';

const disabled: UpdateCheckerStatus = { enabled: false, running: false, timezone: 'Asia/Bangkok', servers: {} };
const enabled: UpdateCheckerStatus = { ...disabled, enabled: true, next_check_at: '2026-10-28T17:00:00Z' };

describe('Update checker', () => {
  beforeEach(() => { vi.restoreAllMocks(); vi.spyOn(api, 'getUpdateChecker').mockResolvedValue(disabled); });

  it('starts opted out and saves opt-in before allowing a fleet scan', async () => {
    const user = userEvent.setup();
    const save = vi.spyOn(api, 'saveUpdateChecker').mockResolvedValue(enabled);
    const run = vi.spyOn(api, 'runUpdateCheck').mockResolvedValue({ accepted: true });
    render(<UpdateCheckerSettings />);
    expect(await screen.findByRole('switch', { name: 'Enable nightly update checks' })).not.toBeChecked();
    expect(screen.getByRole('button', { name: 'Check now' })).toBeDisabled();
    expect(run).not.toHaveBeenCalled();
    await user.click(screen.getByRole('switch'));
    expect(save).toHaveBeenCalledWith(true);
    expect(await screen.findByText('Nightly update checks enabled.')).toBeVisible();
    expect(screen.getByText(/00:00 \(Asia\/Bangkok\)/)).toBeVisible();
    expect(screen.getByRole('button', { name: 'Check now' })).toBeEnabled();
    vi.mocked(api.getUpdateChecker).mockResolvedValue({ ...enabled, running: true });
    await user.click(screen.getByRole('button', { name: 'Check now' }));
    expect(run).toHaveBeenCalledOnce();
    expect(await screen.findByRole('button', { name: 'Checking…' })).toBeDisabled();
  });

  it('keeps the saved switch value when saving fails', async () => {
    vi.spyOn(api, 'saveUpdateChecker').mockRejectedValue(new Error('Could not save update checks.'));
    render(<UpdateCheckerSettings />);
    await userEvent.click(await screen.findByRole('switch'));
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not save');
    expect(screen.getByRole('switch')).not.toBeChecked();
  });

  it('offers retry after a load failure', async () => {
    vi.mocked(api.getUpdateChecker).mockRejectedValueOnce(new Error('Connection failed')).mockResolvedValue(enabled);
    render(<UpdateCheckerSettings />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Connection failed');
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByRole('switch')).toBeChecked();
  });

  it('shows badges for software or installed content and hides empty results', () => {
    const { rerender } = render(<UpdateAvailableBadge />);
    expect(screen.queryByText('Update Available')).not.toBeInTheDocument();
    rerender(<UpdateAvailableBadge result={{ checked_at: '', software: { supported: true, available: true, latest_version: '1.21.5' } }} />);
    expect(screen.getByText('Update Available')).toHaveAttribute('title', 'Software: 1.21.5');
    rerender(<UpdateAvailableBadge result={{ checked_at: '', mods: { last_checked: '', updates: { shared: { mod_name: 'shared', mod_file: 'shared.jar', provider: 'hangar', title: 'Shared', latest_version: '2', latest_jar: 'shared-2.jar' } } } }} />);
    expect(screen.getByText('Update Available')).toHaveAttribute('title', '1 plugin/mod updates');
    rerender(<UpdateAvailableBadge result={{ checked_at: '', software: { supported: true, available: false } }} />);
    expect(screen.queryByText('Update Available')).not.toBeInTheDocument();
  });
});
