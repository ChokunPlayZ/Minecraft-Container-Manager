import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '../src/api/client';
import type { Server } from '../src/api/types';
import { ServerQuickActions } from '../src/components/server-quick-actions';

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}));

const server: Server = {
  id: 'survival', name: 'Survival', server_type: 'paper', version: '1.21.4', build: '145',
  ram_mb: 4096, cpu_limit: 2, memory_limit_mb: 4096, host_port: 25565, extra_ports: [],
  container_id: 'container', state: 'running',
  created_at: '2026-10-27T00:00:00Z', updated_at: '2026-10-27T00:00:00Z',
};
afterEach(() => vi.restoreAllMocks());

describe('dashboard server actions', () => {
  it('starts a stopped server and applies the API response to the dashboard', async () => {
    const start = vi.spyOn(api, 'startServer').mockResolvedValue(server);
    const onUpdated = vi.fn();
    render(<ServerQuickActions server={{ ...server, state: 'stopped' }} onUpdated={onUpdated} onCopy={vi.fn()} />);
    await userEvent.click(screen.getByRole('button', { name: 'Start Survival' }));
    await waitFor(() => expect(onUpdated).toHaveBeenCalledWith(server));
    expect(start).toHaveBeenCalledWith('survival');
  });

  it('blocks duplicate actions during a graceful stop and recovers after failure', async () => {
    let rejectStop!: (reason: unknown) => void;
    const stop = vi.spyOn(api, 'stopServer').mockReturnValue(new Promise((_resolve, reject) => { rejectStop = reject; }));
    const onUpdated = vi.fn();
    render(<ServerQuickActions server={server} onUpdated={onUpdated} onCopy={vi.fn()} />);
    await userEvent.click(screen.getByRole('button', { name: 'Stop Survival' }));
    expect(screen.getByRole('button', { name: 'Stop Survival' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'More actions for Survival' })).toBeDisabled();
    rejectStop(new ApiError(503, 'Unavailable', 'The container runtime is unavailable.'));
    expect(await screen.findByRole('alert')).toHaveTextContent('The container runtime is unavailable.');
    expect(screen.getByRole('button', { name: 'Stop Survival' })).toBeEnabled();
    expect(stop).toHaveBeenCalledTimes(1);
    expect(onUpdated).not.toHaveBeenCalled();
  });

  it('requires confirmation before force killing, including during a stuck stop', async () => {
    const kill = vi.spyOn(api, 'killServer').mockResolvedValue({ ...server, state: 'stopped' });
    const onUpdated = vi.fn();
    render(<ServerQuickActions server={{ ...server, state: 'stopping' }} onUpdated={onUpdated} onCopy={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'Stop Survival' })).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'More actions for Survival' }));
    await userEvent.click(screen.getByRole('menuitem', { name: 'Force kill' }));
    const dialog = await screen.findByRole('dialog', { name: 'Force kill server' });
    expect(kill).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(kill).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'More actions for Survival' }));
    await userEvent.click(screen.getByRole('menuitem', { name: 'Force kill' }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Force kill' }));
    await waitFor(() => expect(kill).toHaveBeenCalledWith('survival'));
    expect(onUpdated).toHaveBeenCalledWith(expect.objectContaining({ state: 'stopped' }));
  });

  it('restarts a running server through the row menu', async () => {
    const restart = vi.spyOn(api, 'restartServer').mockResolvedValue({ ...server, state: 'starting' });
    render(<ServerQuickActions server={server} onUpdated={vi.fn()} onCopy={vi.fn()} />);
    await userEvent.click(screen.getByRole('button', { name: 'More actions for Survival' }));
    await userEvent.click(screen.getByRole('menuitem', { name: 'Restart' }));
    await waitFor(() => expect(restart).toHaveBeenCalledWith(server.id));
  });
});
