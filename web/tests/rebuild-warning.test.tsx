import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Server } from '../src/api/types';
import { ServerSettings } from '../src/components/server-settings';
import { DashboardRoute } from '../src/routes/dashboard';
import { ServerDetailRoute } from '../src/routes/servers.$id';

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({
    component: () => null,
    useParams: () => ({ id: 'srv-rebuild' }),
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

vi.mock('../src/components/ui/modal', () => ({
  useModal: () => ({
    confirm: vi.fn().mockResolvedValue(true),
  }),
}));

const serverNeedingRebuild: Server = {
  id: 'srv-rebuild',
  name: 'SMP Modded',
  server_type: 'paper',
  version: '1.21.4',
  build: '145',
  ram_mb: 8192,
  cpu_limit: 4,
  memory_limit_mb: 8192,
  host_port: 25565,
  extra_ports: [],
  container_id: 'container-abc',
  state: 'running',
  backup_enabled: true,
  backup_interval_minutes: 60,
  needs_rebuild: true,
  rebuild_reasons: ['RAM changed from 4096 MB to 8192 MB', 'CPU limit changed from 2.0 to 4.0 cores'],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const serverClean: Server = {
  id: 'srv-clean',
  name: 'Clean Server',
  server_type: 'paper',
  version: '1.21.4',
  build: '145',
  ram_mb: 4096,
  cpu_limit: 0,
  memory_limit_mb: 0,
  host_port: 25566,
  extra_ports: [],
  container_id: 'container-def',
  state: 'running',
  backup_enabled: true,
  backup_interval_minutes: 60,
  needs_rebuild: false,
  rebuild_reasons: [],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('Container Rebuild Warnings', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('ServerDetailRoute', () => {
    it('displays rebuild warning banner and header badge when server needs rebuild', async () => {
      vi.spyOn(api, 'getServer').mockResolvedValue(serverNeedingRebuild);
      vi.spyOn(api, 'serverStatus').mockResolvedValue(serverNeedingRebuild);
      vi.spyOn(api, 'getServerDNS').mockResolvedValue({
        configured: false,
        domain: '',
        server_id: serverNeedingRebuild.id,
        server_name: serverNeedingRebuild.name,
        host_port: serverNeedingRebuild.host_port,
        record: null,
        join_address: '',
      });
      const recreateSpy = vi.spyOn(api, 'recreateServer').mockResolvedValue({
        ...serverNeedingRebuild,
        needs_rebuild: false,
        rebuild_reasons: [],
      });

      render(<ServerDetailRoute />);

      await waitFor(() => {
        expect(screen.getByTestId('rebuild-warning-banner')).toBeInTheDocument();
        expect(screen.getByTestId('header-rebuild-badge')).toBeInTheDocument();
      });

      expect(screen.getByText(/Container Rebuild Required/i)).toBeInTheDocument();
      expect(screen.getByText('RAM changed from 4096 MB to 8192 MB')).toBeInTheDocument();
      expect(screen.getByText('CPU limit changed from 2.0 to 4.0 cores')).toBeInTheDocument();

      // Click the "Rebuild container" button in the warning banner
      const banner = screen.getByTestId('rebuild-warning-banner');
      const rebuildButton = banner.querySelector('button');
      expect(rebuildButton).not.toBeNull();
      fireEvent.click(rebuildButton!);

      await waitFor(() => {
        expect(recreateSpy).toHaveBeenCalledWith(serverNeedingRebuild.id);
      });
    });

    it('does not display rebuild banner or header badge when clean', async () => {
      vi.spyOn(api, 'getServer').mockResolvedValue(serverClean);
      vi.spyOn(api, 'serverStatus').mockResolvedValue(serverClean);
      vi.spyOn(api, 'getServerDNS').mockResolvedValue({
        configured: false,
        domain: '',
        server_id: serverClean.id,
        server_name: serverClean.name,
        host_port: serverClean.host_port,
        record: null,
        join_address: '',
      });

      render(<ServerDetailRoute />);

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'Clean Server' })).toBeInTheDocument();
      });

      expect(screen.queryByTestId('rebuild-warning-banner')).not.toBeInTheDocument();
      expect(screen.queryByTestId('header-rebuild-badge')).not.toBeInTheDocument();
    });
  });

  describe('ServerSettings component', () => {
    it('renders rebuild alert and invokes onRebuild when clicked', () => {
      const onSaved = vi.fn();
      const onRebuild = vi.fn();

      render(<ServerSettings server={serverNeedingRebuild} onSaved={onSaved} onRebuild={onRebuild} />);

      expect(screen.getByTestId('settings-rebuild-alert')).toBeInTheDocument();
      expect(screen.getByText('RAM changed from 4096 MB to 8192 MB')).toBeInTheDocument();

      const rebuildBtn = screen.getByTestId('settings-rebuild-alert').querySelector('button');
      expect(rebuildBtn).not.toBeNull();
      fireEvent.click(rebuildBtn!);
      expect(onRebuild).toHaveBeenCalledTimes(1);
    });

    it('shows saved notice informing that container rebuild is required after update', async () => {
      const onSaved = vi.fn();
      vi.spyOn(api, 'updateServer').mockResolvedValue(serverNeedingRebuild);

      render(<ServerSettings server={serverNeedingRebuild} onSaved={onSaved} />);

      const saveBtn = screen.getByRole('button', { name: 'Save' });
      fireEvent.click(saveBtn);

      await waitFor(() => {
        expect(screen.getByText(/Container rebuild required for memory and container settings/i)).toBeInTheDocument();
      });
    });
  });

  describe('DashboardRoute', () => {
    it('displays rebuild required badge only on servers that need rebuild', async () => {
      vi.spyOn(api, 'listServers').mockResolvedValue([serverNeedingRebuild, serverClean]);

      render(<DashboardRoute />);

      await waitFor(() => {
        expect(screen.getByText('SMP Modded')).toBeInTheDocument();
        expect(screen.getByText('Clean Server')).toBeInTheDocument();
      });

      const badges = screen.getAllByTestId('dashboard-rebuild-badge');
      expect(badges.length).toBe(1);
      expect(badges[0]).toHaveTextContent('Rebuild required');
    });
  });
});
