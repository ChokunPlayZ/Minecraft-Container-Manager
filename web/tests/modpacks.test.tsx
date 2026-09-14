import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { InstalledModpack, ModpackManifest, Server } from '../src/api/types';
import { ModpacksPanel } from '../src/components/modpacks-panel';
import { ModpackInstallDialog } from '../src/components/modpack-install-dialog';
import { CreateServerDialog } from '../src/components/create-server-dialog';

const mockServer: Server = {
  id: 'srv-modpack-1',
  name: 'Test Fabric Server',
  server_type: 'fabric',
  version: '1.20.1',
  build: '0.15.11',
  ram_mb: 4096,
  cpu_limit: 2,
  memory_limit_mb: 4096,
  host_port: 25565,
  extra_ports: [],
  container_id: 'container-1',
  state: 'stopped',
  backup_enabled: false,
  backup_interval_minutes: 60,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('ModpacksPanel Component', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders with no installed modpack and displays 4 source tabs', async () => {
    vi.spyOn(api, 'getInstalledModpack').mockResolvedValue({
      installed: false,
      modpack: null,
    });

    render(<ModpacksPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText(/No Modpack Installed/i)).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /Modrinth/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /CurseForge/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Upload File/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /From URL/i })).toBeInTheDocument();
  });

  it('renders active installed modpack hero card', async () => {
    const installed: InstalledModpack = {
      name: 'Better MC [FABRIC]',
      version: 'v25',
      summary: 'The ultimate Minecraft modpack experience',
      author: 'SHXRKIE',
      format: 'modrinth',
      minecraft_version: '1.20.1',
      loader: 'fabric',
      loader_version: '0.15.11',
      installed_at: '2026-09-14T10:00:00Z',
      source: 'upload',
      installed_files: ['mods/mod1.jar', 'mods/mod2.jar'],
    };

    vi.spyOn(api, 'getInstalledModpack').mockResolvedValue({
      installed: true,
      modpack: installed,
    });

    render(<ModpacksPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Better MC [FABRIC]')).toBeInTheDocument();
    });

    expect(screen.getByText(/Active Modpack/i)).toBeInTheDocument();
    expect(screen.getAllByText(/MODRINTH/i).length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: /Uninstall/i })).toBeInTheDocument();
  });

  it('switches to Upload tab and shows dropzone', async () => {
    const user = userEvent.setup();

    vi.spyOn(api, 'getInstalledModpack').mockResolvedValue({
      installed: false,
      modpack: null,
    });

    render(<ModpacksPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText(/No Modpack Installed/i)).toBeInTheDocument();
    });

    const uploadTab = screen.getByRole('button', { name: /Upload File/i });
    await user.click(uploadTab);

    expect(
      screen.getByText(/Choose or drop a modpack archive/i),
    ).toBeInTheDocument();
  });

  it('switches to From URL tab and displays input', async () => {
    const user = userEvent.setup();

    vi.spyOn(api, 'getInstalledModpack').mockResolvedValue({
      installed: false,
      modpack: null,
    });

    render(<ModpacksPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText(/No Modpack Installed/i)).toBeInTheDocument();
    });

    const urlTab = screen.getByRole('button', { name: /From URL/i });
    await user.click(urlTab);

    expect(screen.getByPlaceholderText(/cdn\.modrinth\.com/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Inspect URL/i })).toBeInTheDocument();
  });
});

describe('ModpackInstallDialog Component', () => {
  it('renders software compatibility details and allows installing', async () => {
    const user = userEvent.setup();

    const manifest: ModpackManifest = {
      format: 'modrinth',
      name: 'Cobblemon Official Pack',
      version: '1.5.2',
      summary: 'Gotta catch them all!',
      author: 'Cobblemon Team',
      minecraft_version: '1.20.1',
      loader: 'fabric',
      loader_version: '0.15.11',
      total_files: 45,
      server_files: 40,
      client_only_files: 5,
    };

    const mockInstall = vi.spyOn(api, 'installModpackRemote').mockResolvedValue({
      name: manifest.name,
      version: manifest.version,
      format: manifest.format,
      minecraft_version: manifest.minecraft_version,
      loader: manifest.loader,
      loader_version: manifest.loader_version,
      installed_at: '2026-09-14T12:00:00Z',
      source: 'modrinth',
      installed_files: [],
    });

    const onInstalled = vi.fn();
    const onClose = vi.fn();

    render(
      <ModpackInstallDialog
        server={mockServer}
        manifest={manifest}
        installOptions={{ source: 'modrinth', url: 'https://example.com/pack.mrpack' }}
        onClose={onClose}
        onInstalled={onInstalled}
      />,
    );

    expect(screen.getByText('Cobblemon Official Pack')).toBeInTheDocument();
    expect(screen.getByText(/Compatible with this server software/i)).toBeInTheDocument();

    const installBtn = screen.getByRole('button', { name: /Install Modpack/i });
    await user.click(installBtn);

    expect(mockInstall).toHaveBeenCalled();
  });

  it('shows software mismatch alert when server version does not match modpack', () => {
    const manifest: ModpackManifest = {
      format: 'curseforge',
      name: 'All The Forge 9',
      version: 'v1.0',
      minecraft_version: '1.19.2',
      loader: 'forge',
      loader_version: '43.2.0',
      total_files: 100,
      server_files: 100,
      client_only_files: 0,
    };

    render(
      <ModpackInstallDialog
        server={mockServer} // mockServer is fabric 1.20.1
        manifest={manifest}
        installOptions={{ source: 'curseforge', url: 'https://example.com/pack.zip' }}
        onClose={() => {}}
        onInstalled={() => {}}
      />,
    );

    expect(screen.getByText(/Server Software Mismatch Detected/i)).toBeInTheDocument();
    expect(screen.getByText(/Automatically reconfigure server to/i)).toBeInTheDocument();
  });
});

describe('CreateServerDialog Modpack Integration', () => {
  it('supports switching to From Modpack mode', async () => {
    const user = userEvent.setup();

    render(<CreateServerDialog onCreated={() => {}} />);

    const openBtn = screen.getByRole('button', { name: /Create server/i });
    await user.click(openBtn);

    const modpackModeBtn = screen.getByRole('button', { name: /From Modpack/i });
    expect(modpackModeBtn).toBeInTheDocument();

    await user.click(modpackModeBtn);

    expect(screen.getByText(/Choose or drop modpack archive/i)).toBeInTheDocument();
  });
});
