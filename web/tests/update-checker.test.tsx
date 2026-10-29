import { act, render, renderHook, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { UpdateCheckerSettings } from '../src/components/update-checker-settings';
import { UpdateAvailableBadge } from '../src/components/update-available-badge';
import type { UpdateCheckerStatus } from '../src/api/types';
import { useUpdateChecker } from '../src/lib/use-update-checker';
import { announceUpdateDiscovery } from '../src/lib/update-discovery';

const disabled: UpdateCheckerStatus = { enabled: false, running: false, timezone: 'Asia/Bangkok', servers: {} };
const enabled: UpdateCheckerStatus = { ...disabled, enabled: true, next_check_at: '2026-10-28T17:00:00Z' };

describe('Update checker', () => {
  beforeEach(() => { vi.restoreAllMocks(); vi.spyOn(api, 'getUpdateChecker').mockResolvedValue(disabled); });
  afterEach(() => vi.unstubAllGlobals());

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

  it.each(['versions', 'builds'])('refreshes badges immediately after querying software %s with nightly checks off', async (catalog) => {
    const discovered: UpdateCheckerStatus = { ...disabled, servers: { paper: { checked_at: '', software: { supported: true, available: true, latest_version: '1.21.4', latest_build: '227' } } } };
    vi.mocked(api.getUpdateChecker).mockResolvedValueOnce(disabled).mockResolvedValue(discovered);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json([])));
    const { result } = renderHook(() => useUpdateChecker());
    await waitFor(() => expect(result.current.status).toEqual(disabled));
    await act(async () => {
      if (catalog === 'versions') await api.jarVersions('paper');
      else await api.jarBuilds('paper', '1.21.4');
    });
    await waitFor(() => expect(result.current.status).toEqual(discovered));
    expect(api.getUpdateChecker).toHaveBeenCalledTimes(2);
  });

  it('reloads discoveries that arrive while an older status request is still pending', async () => {
    let resolveInitial!: (value: UpdateCheckerStatus) => void;
    const initial = new Promise<UpdateCheckerStatus>((resolve) => { resolveInitial = resolve; });
    vi.mocked(api.getUpdateChecker).mockReturnValueOnce(initial).mockResolvedValue(enabled);
    const { result } = renderHook(() => useUpdateChecker());
    act(() => { announceUpdateDiscovery([]); });
    await act(async () => { resolveInitial(disabled); });
    await waitFor(() => expect(result.current.status).toEqual(enabled));
    expect(api.getUpdateChecker).toHaveBeenCalledTimes(2);
  });
});
