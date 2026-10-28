import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Me, Server, User } from '../src/api/types';
import { ServerSettings } from '../src/components/server-settings';
import { DashboardRoute } from '../src/routes/dashboard';
import { UsersPage } from '../src/routes/users';

const auth = vi.hoisted(() => ({ me: { id: 'admin-1', email: 'admin@example.test', role: 'admin' } as Me }));

vi.mock('../src/lib/auth', () => ({
  useAuth: () => ({ user: auth.me }),
  useIsAdmin: () => auth.me.role !== 'member',
}));
vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}));
vi.mock('../src/components/require-auth', () => ({ RequireAuth: ({ children }: { children: React.ReactNode }) => <>{children}</> }));
vi.mock('../src/components/app-shell', () => ({ AppShell: ({ children }: { children: React.ReactNode }) => <>{children}</> }));
vi.mock('../src/components/create-server-dialog', () => ({ CreateServerDialog: () => <button type="button">Create server</button> }));
vi.mock('../src/components/host-capacity', () => ({ HostCapacity: () => <div>Host resources</div> }));

const base: Server = {
  id: 'survival', name: 'Survival', server_type: 'paper', version: '1.21.4', build: '145', container_id: 'c1', ram_mb: 4096, cpu_limit: 0,
  memory_limit_mb: 0, host_port: 25565, extra_ports: [], state: 'stopped', backup_enabled: true,
  backup_interval_minutes: 720, created_at: '2026-10-27T00:00:00Z', updated_at: '2026-10-27T00:00:00Z',
};
const servers = [base, { ...base, id: 'creative', name: 'Creative', host_port: 25566 }];
const user = (patch: Partial<User>): User => ({
  id: 'u', email: 'u@example.test', role: 'member', server_ids: [], totp_enabled: false,
  created_at: '2026-10-27T00:00:00Z', updated_at: '2026-10-27T00:00:00Z', ...patch,
});
const admin = user({ id: 'admin-1', email: 'admin@example.test', role: 'admin' });

beforeEach(() => {
  auth.me = { id: 'admin-1', email: 'admin@example.test', role: 'admin' };
  vi.spyOn(api, 'listServers').mockResolvedValue(servers);
  vi.spyOn(api, 'getUpdateChecker').mockResolvedValue({ enabled: false, running: false, timezone: 'UTC', servers: {} });
});
afterEach(() => vi.restoreAllMocks());

describe('Users page access controls', () => {
  it('creates a member limited to the selected servers', async () => {
    vi.spyOn(api, 'listUsers').mockResolvedValue([admin]);
    const create = vi.spyOn(api, 'createUser').mockResolvedValue(user({ server_ids: ['creative'] }));
    render(<UsersPage />);

    await userEvent.type(screen.getByLabelText('Email'), 'builder@example.test');
    await userEvent.type(screen.getByLabelText('Password'), 'password123');
    expect(screen.getByRole('radio', { name: /Member/ })).toBeChecked();
    await userEvent.click(await screen.findByRole('checkbox', { name: /Creative/ }));
    expect(screen.getByText('1 of 2 selected')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Create user' }));

    expect(create).toHaveBeenCalledWith({ email: 'builder@example.test', password: 'password123', role: 'member', server_ids: ['creative'] });
  });

  it('does not send server assignments for a new admin', async () => {
    vi.spyOn(api, 'listUsers').mockResolvedValue([admin]);
    const create = vi.spyOn(api, 'createUser').mockResolvedValue(admin);
    render(<UsersPage />);

    await userEvent.type(screen.getByLabelText('Email'), 'second@example.test');
    await userEvent.type(screen.getByLabelText('Password'), 'password123');
    await userEvent.click(screen.getByRole('radio', { name: /Admin/ }));
    expect(screen.queryByText('Server access')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Create user' }));

    expect(create).toHaveBeenCalledWith(expect.objectContaining({ role: 'admin', server_ids: [] }));
  });

  it('summarizes member access and updates assignments', async () => {
    const member = user({ id: 'm1', email: 'builder@example.test', server_ids: ['survival'] });
    vi.spyOn(api, 'listUsers').mockResolvedValue([admin, member, user({ id: 'm2', email: 'idle@example.test' })]);
    const update = vi.spyOn(api, 'updateUser').mockResolvedValue(member);
    render(<UsersPage />);

    expect((await screen.findByText(/Can manage:/)).closest('p')).toHaveTextContent('Can manage: Survival');
    expect(screen.getByText('No servers assigned')).toBeInTheDocument();

    await userEvent.click(screen.getAllByRole('button', { name: 'Edit' })[1]);
    // The Add user form has its own picker; the edit form renders after it.
    await userEvent.click(screen.getAllByRole('checkbox', { name: /Creative/ }).at(-1)!);
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(update).toHaveBeenCalledWith('m1', { server_ids: ['survival', 'creative'] });
  });

  it('locks the role of the signed-in admin', async () => {
    vi.spyOn(api, 'listUsers').mockResolvedValue([admin]);
    render(<UsersPage />);

    await userEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    const roles = screen.getAllByRole('radio', { name: /Member|Admin/ }).filter((r) => (r as HTMLInputElement).name === 'edit-admin-1-role');
    expect(roles).toHaveLength(2);
    roles.forEach((r) => expect(r).toBeDisabled());
    expect(screen.getByText('You cannot change your own role.')).toBeInTheDocument();
  });

  it('tells members that only administrators manage accounts', () => {
    auth.me = { id: 'm1', email: 'builder@example.test', role: 'member' };
    const list = vi.spyOn(api, 'listUsers');
    render(<UsersPage />);
    expect(screen.getByText('Only administrators can manage panel accounts.')).toBeInTheDocument();
    expect(list).not.toHaveBeenCalled();
  });
});

describe('member view', () => {
  beforeEach(() => {
    auth.me = { id: 'm1', email: 'builder@example.test', role: 'member' };
  });

  it('hides server creation and host resources on the dashboard', async () => {
    vi.spyOn(api, 'listServers').mockResolvedValue([]);
    render(<DashboardRoute />);
    expect(await screen.findByText('No servers assigned')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Create server' })).not.toBeInTheDocument();
    expect(screen.queryByText('Host resources')).not.toBeInTheDocument();
  });

  it('keeps resources and ports read-only in server settings', async () => {
    vi.spyOn(api, 'javaVersions').mockResolvedValue([]);
    const ports = vi.spyOn(api, 'availablePorts');
    const update = vi.spyOn(api, 'updateServer').mockResolvedValue(base);
    render(<ServerSettings server={base} onSaved={vi.fn()} />);

    expect(screen.getByText('Only administrators can change memory, CPU, and ports.')).toBeInTheDocument();
    expect(screen.getByLabelText('RAM (MB)')).toBeDisabled();
    expect(screen.getByLabelText('Game port (host)')).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Add port' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: /DNS/ })).not.toBeInTheDocument();

    await userEvent.clear(screen.getByLabelText('Name'));
    await userEvent.type(screen.getByLabelText('Name'), 'Renamed');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith('survival', expect.objectContaining({ name: 'Renamed', ram_mb: 4096, host_port: 25565 })));
    expect(ports).not.toHaveBeenCalled();
  });
});
