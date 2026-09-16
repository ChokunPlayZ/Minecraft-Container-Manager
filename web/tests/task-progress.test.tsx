import { describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { ModpackInstallDialog } from '../src/components/modpack-install-dialog';
import { FileManager } from '../src/components/file-manager';
import { api } from '../src/api/client';
import type { ModpackManifest, Server, TaskProgress } from '../src/api/types';

const mockServer: Server = {
  id: 'srv-test-1',
  name: 'Test Server',
  server_type: 'fabric',
  version: '1.21.1',
  build: '0.16.0',
  ram_mb: 2048,
  cpu_limit: 1,
  memory_limit_mb: 2048,
  host_port: 25565,
  extra_ports: [],
  container_id: null,
  state: 'stopped',
  backup_enabled: false,
  backup_interval_minutes: 0,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const mockManifest: ModpackManifest = {
  format: 'modrinth',
  name: 'Cobblemon Official',
  version: '1.5.0',
  summary: 'Pokemon in Minecraft',
  minecraft_version: '1.21.1',
  loader: 'fabric',
  loader_version: '0.16.0',
  total_files: 50,
  server_files: 45,
  client_only_files: 5,
};

describe('Task Progress Display', () => {
  it('subscribes to task events and displays stage progress in ModpackInstallDialog', async () => {
    let subscriberCallback: ((p: TaskProgress) => void) | null = null;
    vi.spyOn(api, 'subscribeTaskEvents').mockImplementation((serverId, onProgress) => {
      subscriberCallback = onProgress;
      return () => {};
    });

    let resolveRemote: () => void;
    vi.spyOn(api, 'installModpackRemote').mockImplementation(
      () => new Promise((resolve) => { resolveRemote = resolve; })
    );

    render(
      <ModpackInstallDialog
        server={mockServer}
        manifest={mockManifest}
        installOptions={{ source: 'modrinth', url: 'https://example.com/pack.mrpack' }}
        onClose={() => {}}
        onInstalled={() => {}}
      />
    );

    // Click install button
    const installBtn = screen.getByRole('button', { name: /install modpack/i });
    await act(async () => {
      installBtn.click();
    });

    expect(api.subscribeTaskEvents).toHaveBeenCalledWith('srv-test-1', expect.any(Function));
    expect(subscriberCallback).not.toBeNull();

    // Simulate stage 2: Extracting Files
    await act(async () => {
      subscriberCallback?.({
        server_id: 'srv-test-1',
        operation: 'modpack_install',
        stage: 'extracting',
        stage_title: 'Extracting Files (10/50)',
        stage_index: 2,
        stage_total: 4,
        percent: 40,
        message: 'config/cobblemon.json',
      });
    });

    expect(screen.getByText('Stage 2 of 4')).toBeInTheDocument();
    expect(screen.getAllByText('Extracting Files (10/50)').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('config/cobblemon.json')).toBeInTheDocument();
    expect(screen.getByText('40%')).toBeInTheDocument();

    // Simulate stage 3: Downloading Mods
    await act(async () => {
      subscriberCallback?.({
        server_id: 'srv-test-1',
        operation: 'modpack_install',
        stage: 'downloading_mods',
        stage_title: 'Downloading Mods (25/45)',
        stage_index: 3,
        stage_total: 4,
        percent: 75,
        message: 'cobblemon-fabric-1.5.jar',
      });
    });

    expect(screen.getByText('Stage 3 of 4')).toBeInTheDocument();
    expect(screen.getAllByText('Downloading Mods (25/45)').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('cobblemon-fabric-1.5.jar')).toBeInTheDocument();
    expect(screen.getByText('75%')).toBeInTheDocument();

    // Finish remote install
    await act(async () => {
      resolveRemote!();
    });
  });

  it('displays real progress percentage and filename in FileManager when archive task runs', async () => {
    let subscriberCallback: ((p: TaskProgress) => void) | null = null;
    vi.spyOn(api, 'subscribeTaskEvents').mockImplementation((serverId, onProgress) => {
      subscriberCallback = onProgress;
      return () => {};
    });

    vi.spyOn(api, 'listFiles').mockResolvedValue({
      entries: [
        { name: 'world', is_dir: true, size: 0, updated_at: new Date().toISOString() },
      ],
    });

    render(<FileManager server={mockServer} />);

    expect(api.subscribeTaskEvents).toHaveBeenCalledWith('srv-test-1', expect.any(Function));
    expect(subscriberCallback).not.toBeNull();

    // Simulate archive compression event from backend
    await act(async () => {
      subscriberCallback?.({
        server_id: 'srv-test-1',
        operation: 'archive',
        stage: 'compressing',
        stage_title: 'Compressing Files',
        stage_index: 2,
        stage_total: 2,
        percent: 62,
        message: 'world/level.dat',
        bytes_done: 65000000,
        bytes_total: 100000000,
      });
    });

    expect(screen.getByText('Compressing Files')).toBeInTheDocument();
    expect(screen.getByText('62%')).toBeInTheDocument();
    expect(screen.getByText(/world\/level\.dat/)).toBeInTheDocument();
  });
});
