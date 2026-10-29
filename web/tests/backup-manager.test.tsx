import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Server } from '../src/api/types';
import { BackupManager } from '../src/routes/backups';

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: ({ children, to, params, search }: { children: React.ReactNode; to: string; params?: { id: string }; search?: { tab: string } }) => <a href={to.replace('$id', params?.id ?? '') + (search?.tab ? `?tab=${search.tab}` : '')}>{children}</a>,
}));
vi.mock('../src/components/require-auth', () => ({ RequireAuth: ({ children }: { children: React.ReactNode }) => <>{children}</> }));
vi.mock('../src/components/app-shell', () => ({ AppShell: ({ children }: { children: React.ReactNode }) => <>{children}</> }));

const server: Server = {
  id: 'survival', name: 'Survival', server_type: 'paper', version: '1.21.4', build: '145',
  ram_mb: 4096, cpu_limit: 2, memory_limit_mb: 4096, host_port: 25565, extra_ports: [],
  container_id: 'container', state: 'running',
  created_at: '2026-10-27T00:00:00Z', updated_at: '2026-10-27T00:00:00Z',
};
afterEach(() => vi.restoreAllMocks());

describe('dedicated backup manager', () => {
  it('loads the server selected by a dashboard link and resets the panel when switching servers', async () => {
    vi.spyOn(api, 'listServers').mockResolvedValue([server, { ...server, id: 'creative', name: 'Creative' }]);
    const list = vi.spyOn(api, 'listBackups').mockResolvedValue({ backups: [] });
    vi.spyOn(api, 'getBackupProgress').mockResolvedValue({ active: false });
    const unsubscribe = vi.fn();
    vi.spyOn(api, 'subscribeBackupEvents').mockReturnValue(unsubscribe);
    const onSelect = vi.fn();
    const view = render(<BackupManager serverId="creative" onSelect={onSelect} />);
    await screen.findByText('No backups yet');
    expect(list).toHaveBeenCalledWith('creative');
    await userEvent.type(screen.getByLabelText('Backup name (optional)'), 'creative-only');
    await userEvent.click(screen.getByLabelText('Server'));
    await userEvent.click(screen.getByRole('option', { name: 'Survival' }));
    expect(onSelect).toHaveBeenCalledWith('survival');
    view.rerender(<BackupManager serverId="survival" onSelect={onSelect} />);
    await waitFor(() => expect(list).toHaveBeenCalledWith('survival'));
    expect(screen.getByLabelText('Backup name (optional)')).toHaveValue('');
    expect(unsubscribe).toHaveBeenCalledTimes(1);
  });

  it('does not silently operate on another server when a bookmarked server is missing', async () => {
    vi.spyOn(api, 'listServers').mockResolvedValue([server]);
    const list = vi.spyOn(api, 'listBackups');
    render(<BackupManager serverId="deleted-server" onSelect={vi.fn()} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('This server is unavailable.');
    expect(list).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Create backup' })).not.toBeInTheDocument();
  });

  it('points automatic backups to the server schedule system', async () => {
    vi.spyOn(api, 'listServers').mockResolvedValue([server]);
    vi.spyOn(api, 'listBackups').mockResolvedValue({ backups: [] });
    vi.spyOn(api, 'getBackupProgress').mockResolvedValue({ active: false });
    vi.spyOn(api, 'subscribeBackupEvents').mockReturnValue(vi.fn());
    const update = vi.spyOn(api, 'updateServer');
    render(<BackupManager serverId="survival" onSelect={vi.fn()} />);
    expect(await screen.findByText('Manage recurring backups in this server’s Schedules tab.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Manage schedules' })).toHaveAttribute('href', '/servers/survival?tab=schedules');
    expect(screen.queryByLabelText('Interval (minutes)')).not.toBeInTheDocument();
    expect(update).not.toHaveBeenCalled();
  });
});
