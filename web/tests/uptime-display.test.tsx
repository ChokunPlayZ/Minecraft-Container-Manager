import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Server } from '../src/api/types';
import { DashboardRoute } from '../src/routes/dashboard';
import { ServerDetailRoute } from '../src/routes/servers.$id';

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({
    component: () => null,
    useParams: () => ({ id: 'srv-running' }),
  }),
  Link: ({ children, ...props }: any) => <a {...props}>{children}</a>,
  useNavigate: () => vi.fn(),
}));

vi.mock('../src/components/require-auth', () => ({
  RequireAuth: ({ children }: any) => <div data-testid="require-auth">{children}</div>,
}));

vi.mock('../src/components/app-shell', () => ({
  AppShell: ({ children }: any) => <div data-testid="app-shell">{children}</div>,
}));

vi.mock('../src/components/create-server-dialog', () => ({
  CreateServerDialog: () => <button type="button">Create Server</button>,
}));

vi.mock('../src/components/console-viewer', () => ({
  ConsoleViewer: () => <div data-testid="console-viewer">Console</div>,
}));

const runningServer: Server = {
  id: 'srv-running',
  name: 'Survival SMP',
  server_type: 'paper',
  version: '1.21.4',
  build: '145',
  ram_mb: 8192,
  cpu_limit: 4,
  memory_limit_mb: 8192,
  host_port: 25565,
  extra_ports: [],
  container_id: 'container-1',
  state: 'running',
  backup_enabled: true,
  backup_interval_minutes: 60,
  started_at: new Date(Date.now() - 3665 * 1000).toISOString(),
  uptime_seconds: 3665,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const stoppedServer: Server = {
  id: 'srv-stopped',
  name: 'Creative World',
  server_type: 'vanilla',
  version: '1.21.4',
  build: '',
  ram_mb: 4096,
  cpu_limit: 2,
  memory_limit_mb: 4096,
  host_port: 25566,
  extra_ports: [],
  container_id: null,
  state: 'stopped',
  backup_enabled: false,
  backup_interval_minutes: 720,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('Server Uptime Display', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('DashboardRoute', () => {
    it('displays uptime row on running server cards but omits on stopped servers', async () => {
      vi.spyOn(api, 'listServers').mockResolvedValue([runningServer, stoppedServer]);

      render(<DashboardRoute />);

      await waitFor(() => {
        expect(screen.getByText('Survival SMP')).toBeInTheDocument();
        expect(screen.getByText('Creative World')).toBeInTheDocument();
      });

      const uptimeBadges = screen.getAllByTestId('dashboard-server-uptime');
      expect(uptimeBadges.length).toBe(1);
      expect(uptimeBadges[0]).toHaveTextContent(/Uptime/i);
      expect(uptimeBadges[0]).toHaveTextContent(/1h 1m/);
    });
  });

  describe('ServerDetailRoute', () => {
    it('displays live uptime badge in hero header when server is running', async () => {
      vi.spyOn(api, 'getServer').mockResolvedValue(runningServer);
      vi.spyOn(api, 'serverStatus').mockResolvedValue({
        id: runningServer.id,
        state: 'running',
        ram_mb: runningServer.ram_mb,
        host_port: runningServer.host_port,
        container_id: runningServer.container_id,
        started_at: runningServer.started_at,
        uptime_seconds: runningServer.uptime_seconds,
      });
      vi.spyOn(api, 'getServerDNS').mockResolvedValue({
        configured: false,
        domain: '',
        server_id: runningServer.id,
        server_name: runningServer.name,
        host_port: runningServer.host_port,
      });

      render(<ServerDetailRoute />);

      await waitFor(() => {
        expect(screen.getByTestId('server-uptime')).toBeInTheDocument();
      });

      const badge = screen.getByTestId('server-uptime');
      expect(badge).toHaveTextContent(/Uptime: 1h 1m/);
    });

    it('does not display uptime badge when server is stopped', async () => {
      vi.spyOn(api, 'getServer').mockResolvedValue(stoppedServer);
      vi.spyOn(api, 'serverStatus').mockResolvedValue({
        id: stoppedServer.id,
        state: 'stopped',
        ram_mb: stoppedServer.ram_mb,
        host_port: stoppedServer.host_port,
        container_id: null,
      });
      vi.spyOn(api, 'getServerDNS').mockResolvedValue({
        configured: false,
        domain: '',
        server_id: stoppedServer.id,
        server_name: stoppedServer.name,
        host_port: stoppedServer.host_port,
      });

      render(<ServerDetailRoute />);

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'Creative World' })).toBeInTheDocument();
      });

      expect(screen.queryByTestId('server-uptime')).not.toBeInTheDocument();
    });
  });
});
