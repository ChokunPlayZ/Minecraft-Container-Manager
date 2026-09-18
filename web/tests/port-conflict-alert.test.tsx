import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Server } from '../src/api/types';
import { CreateServerDialog } from '../src/components/create-server-dialog';
import { ServerSettings } from '../src/components/server-settings';
import { CopyServerDialog } from '../src/components/copy-server-dialog';

vi.mock('../src/api/client', () => ({
  api: {
    listServers: vi.fn(),
    availablePorts: vi.fn(),
    javaVersions: vi.fn(),
    jarVersions: vi.fn(),
    jarBuilds: vi.fn(),
    createServer: vi.fn(),
    updateServer: vi.fn(),
    copyServer: vi.fn(),
  },
  ApiError: class ApiError extends Error {
    detail: string;
    constructor(detail: string) {
      super(detail);
      this.detail = detail;
    }
  },
}));

const mockServers: Server[] = [
  {
    id: 'srv-1',
    name: 'Paper Survival',
    server_type: 'paper',
    version: '1.21.4',
    build: '145',
    ram_mb: 4096,
    host_port: 25565,
    extra_ports: [
      {
        id: 'ep-1',
        description: 'Geyser Bedrock Adapter',
        host_port: 19132,
        container_port: 19132,
        protocol: 'udp',
      },
    ],
    container_id: 'mcm-srv-1',
    state: 'running',
    backup_enabled: true,
    backup_interval_minutes: 720,
    needs_rebuild: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 'srv-2',
    name: 'Creative Plot',
    server_type: 'paper',
    version: '1.21.4',
    build: '145',
    ram_mb: 2048,
    host_port: 25566,
    extra_ports: [],
    container_id: 'mcm-srv-2',
    state: 'stopped',
    backup_enabled: true,
    backup_interval_minutes: 720,
    needs_rebuild: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
];

describe('Port Conflict Alerting across Frontend Components', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listServers).mockResolvedValue(mockServers);
    vi.mocked(api.availablePorts).mockResolvedValue({
      available: [25567, 25568],
      used: [
        {
          port: 25565,
          server_id: 'srv-1',
          server_name: 'Paper Survival',
          type: 'host_port',
          description: 'Primary game port',
        },
        {
          port: 19132,
          server_id: 'srv-1',
          server_name: 'Paper Survival',
          type: 'extra_port',
          description: 'Geyser Bedrock Adapter',
          protocol: 'udp',
        },
        {
          port: 25566,
          server_id: 'srv-2',
          server_name: 'Creative Plot',
          type: 'host_port',
          description: 'Primary game port',
        },
      ],
    });
    vi.mocked(api.javaVersions).mockResolvedValue([
      { version: 21, is_lts: true, name: 'Java 21 (LTS)' },
    ]);
    vi.mocked(api.jarVersions).mockResolvedValue([{ name: '2.11.3' }]);
    vi.mocked(api.jarBuilds).mockResolvedValue([{ version: '2.11.3', build: 'latest', display: 'latest' }]);
  });

  describe('CreateServerDialog', () => {
    it('displays warning alert when entering an extra port already used by another server', async () => {
      render(
        <CreateServerDialog
          onCreated={vi.fn()}
        />
      );

      // Click trigger button to open modal
      fireEvent.click(screen.getByRole('button', { name: /create server/i }));

      await waitFor(() => {
        expect(api.listServers).toHaveBeenCalled();
      });

      // Target port input
      const portInput = screen.getByLabelText(/server port/i) as HTMLInputElement;
      fireEvent.change(portInput, { target: { value: '19132' } });

      await waitFor(() => {
        expect(screen.getByTestId('port-conflict-alert')).toBeInTheDocument();
      });

      expect(screen.getByText(/Port 19132 is already in use/i)).toBeInTheDocument();
      expect(screen.getByText(/Paper Survival/i)).toBeInTheDocument();
      expect(screen.getByText(/Geyser Bedrock Adapter/i)).toBeInTheDocument();
    });
  });

  describe('ServerSettings', () => {
    it('displays conflict alert when hostPort matches another server extra port', async () => {
      render(
        <ServerSettings
          server={mockServers[1]}
          onSaved={vi.fn()}
        />
      );

      await waitFor(() => {
        expect(api.listServers).toHaveBeenCalled();
      });

      const portInput = screen.getByLabelText(/game port \(host\)/i) as HTMLInputElement;
      fireEvent.change(portInput, { target: { value: '19132' } });

      await waitFor(() => {
        expect(screen.getByTestId('host-port-conflict-alert')).toBeInTheDocument();
      });

      expect(screen.getByText(/already in use by server "Paper Survival"/i)).toBeInTheDocument();
    });

    it('displays conflict alert on extra port card when conflicting with another server', async () => {
      render(
        <ServerSettings
          server={mockServers[1]}
          onSaved={vi.fn()}
        />
      );

      await waitFor(() => {
        expect(api.listServers).toHaveBeenCalled();
      });

      // Click "Add port"
      const addPortBtn = screen.getByRole('button', { name: /add port/i });
      fireEvent.click(addPortBtn);

      const extraHostPortInputs = screen.getAllByLabelText(/host port/i);
      const extraHostPort = extraHostPortInputs[extraHostPortInputs.length - 1];
      fireEvent.change(extraHostPort, { target: { value: '19132' } });

      await waitFor(() => {
        expect(screen.getByText(/already in use by server "Paper Survival"/i)).toBeInTheDocument();
      });
    });
  });

  describe('CopyServerDialog', () => {
    it('displays conflict alert when custom port matches existing server extra port', async () => {
      render(
        <CopyServerDialog
          server={mockServers[1]}
          open={true}
          onClose={vi.fn()}
        />
      );

      await waitFor(() => {
        expect(api.listServers).toHaveBeenCalled();
      });

      // Switch to custom port
      const customPortBtn = screen.getByRole('button', { name: /custom port/i });
      fireEvent.click(customPortBtn);

      const customInput = screen.getByPlaceholderText(/e\.g\. 25566/i);
      fireEvent.change(customInput, { target: { value: '19132' } });

      await waitFor(() => {
        expect(screen.getByTestId('copy-port-conflict-alert')).toBeInTheDocument();
      });

      expect(screen.getByText(/already in use by server "Paper Survival"/i)).toBeInTheDocument();
    });
  });
});
