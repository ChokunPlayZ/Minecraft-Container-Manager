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

  it('renders with no installed modpack and displays informative empty state with no install hub', async () => {
    vi.spyOn(api, 'getInstalledModpack').mockResolvedValue({
      installed: false,
      modpack: null,
    });

    render(<ModpacksPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText(/No Modpack Installed/i)).toBeInTheDocument();
    });

    // Check that install catalog tabs are NOT present on existing server
    expect(screen.queryByRole('button', { name: /Modrinth/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /CurseForge/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Upload File/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /From URL/i })).not.toBeInTheDocument();
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
      expect(screen.getAllByText(/Better MC/i).length).toBeGreaterThan(0);
    });

    expect(screen.getByText(/Active Modpack/i)).toBeInTheDocument();
    expect(screen.getAllByText(/MODRINTH/i).length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: /Uninstall/i })).toBeInTheDocument();
  });

  it('locks modpack when created_with_modpack is true, hides uninstall, and displays modpack updates', async () => {
    const lockedModpack: InstalledModpack = {
      name: 'Cobblemon Official',
      version: 'v1.5.0',
      summary: 'Gotta catch em all',
      author: 'Cobblemon Team',
      format: 'modrinth',
      minecraft_version: '1.20.1',
      loader: 'fabric',
      loader_version: '0.15.11',
      installed_at: '2026-09-14T10:00:00Z',
      source: 'modrinth',
      project_id: 'cobblemon-id',
      project_slug: 'cobblemon',
      installed_files: ['mods/cobblemon.jar'],
      created_with_modpack: true,
    };

    vi.spyOn(api, 'getInstalledModpack').mockResolvedValue({
      installed: true,
      modpack: lockedModpack,
    });

    vi.spyOn(global, 'fetch').mockImplementation(async (url) => {
      const urlStr = String(url);
      if (urlStr.includes('/version')) {
        return {
          ok: true,
          json: async () => [
            {
              id: 'ver-150',
              name: 'Cobblemon 1.5.0',
              version_number: 'v1.5.0',
              game_versions: ['1.20.1'],
              loaders: ['fabric'],
              files: [{ url: 'https://cdn.example.com/cobblemon-1.5.0.mrpack', filename: 'cobblemon-1.5.0.mrpack', primary: true }],
            },
            {
              id: 'ver-160',
              name: 'Cobblemon 1.6.0',
              version_number: 'v1.6.0',
              game_versions: ['1.20.1'],
              loaders: ['fabric'],
              files: [{ url: 'https://cdn.example.com/cobblemon-1.6.0.mrpack', filename: 'cobblemon-1.6.0.mrpack', primary: true }],
            },
          ],
        } as Response;
      }
      return {
        ok: true,
        json: async () => ({ hits: [], total_hits: 0 }),
      } as Response;
    });

    render(<ModpacksPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getAllByText(/Cobblemon Official/i).length).toBeGreaterThan(0);
    });

    // Check locked badge is present
    expect(screen.getByText(/Modpack Server \(Locked\)/i)).toBeInTheDocument();

    // Check Uninstall button is NOT present
    expect(screen.queryByRole('button', { name: /Uninstall/i })).not.toBeInTheDocument();

    // Check Modpack Updates section is shown
    expect(screen.getByText(/Modpack Updates/i)).toBeInTheDocument();
    expect(screen.getByText(/Locked to Cobblemon Official/i)).toBeInTheDocument();

    // Check version list
    await waitFor(() => {
      expect(screen.getByText(/Current Active Version/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /^Update$/i })).toBeInTheDocument();
    });
  });

  it('displays manual update archive dropzone for installed modpack', async () => {
    const lockedModpack: InstalledModpack = {
      name: 'Cobblemon Official',
      version: 'v1.5.0',
      summary: 'Gotta catch em all',
      author: 'Cobblemon Team',
      format: 'modrinth',
      minecraft_version: '1.20.1',
      loader: 'fabric',
      loader_version: '0.15.11',
      installed_at: '2026-09-14T10:00:00Z',
      source: 'modrinth',
      installed_files: ['mods/cobblemon.jar'],
      created_with_modpack: true,
    };

    vi.spyOn(api, 'getInstalledModpack').mockResolvedValue({
      installed: true,
      modpack: lockedModpack,
    });

    vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => [],
    } as Response);

    render(<ModpacksPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText(/Upload Updated Modpack Archive/i)).toBeInTheDocument();
    });

    expect(screen.getByText(/Choose Update File/i)).toBeInTheDocument();
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

  it('supports searching modpacks, selecting a pack, and creating a server from it', async () => {
    const user = userEvent.setup();

    const mockCreateServer = vi.spyOn(api, 'createServer').mockResolvedValue({
      ...mockServer,
      id: 'srv-created-1',
      name: 'Cobblemon Official',
    });

    const mockInstallRemote = vi.spyOn(api, 'installModpackRemote').mockResolvedValue({
      name: 'Cobblemon Official',
      version: 'v1.5.0',
      format: 'modrinth',
      minecraft_version: '1.20.1',
      loader: 'fabric',
      loader_version: 'ver-150',
      installed_at: '2026-09-14T10:00:00Z',
      source: 'modrinth',
      created_with_modpack: true,
      installed_files: [],
    });

    vi.spyOn(global, 'fetch').mockImplementation(async (url) => {
      const urlStr = String(url);
      if (urlStr.includes('/search')) {
        return {
          ok: true,
          json: async () => ({
            hits: [
              {
                project_id: 'cobblemon-proj',
                slug: 'cobblemon',
                title: 'Cobblemon Official',
                description: 'Pokemon modpack for Fabric',
                author: 'CobblemonTeam',
                downloads: 500000,
                follows: 10000,
                categories: ['adventure'],
                loaders: ['fabric'],
                game_versions: ['1.20.1'],
              },
            ],
            total_hits: 1,
          }),
        } as Response;
      }
      if (urlStr.includes('/version')) {
        return {
          ok: true,
          json: async () => [
            {
              id: 'ver-150',
              name: 'Cobblemon 1.5.0',
              version_number: '1.5.0',
              game_versions: ['1.20.1'],
              loaders: ['fabric'],
              files: [{ url: 'https://cdn.example.com/cobblemon.mrpack', filename: 'cobblemon.mrpack', primary: true }],
            },
          ],
        } as Response;
      }
      return { ok: false } as Response;
    });

    const onCreated = vi.fn();
    render(<CreateServerDialog onCreated={onCreated} />);

    await user.click(screen.getByRole('button', { name: /Create server/i }));

    const searchTabBtn = screen.getByRole('button', { name: /Search Modpacks/i });
    await user.click(searchTabBtn);

    // Wait for search result card to appear
    await waitFor(() => {
      expect(screen.getAllByText(/Cobblemon Official/i).length).toBeGreaterThan(0);
    });

    // Click Select button on card
    const selectBtn = screen.getByRole('button', { name: /^Select$/i });
    await user.click(selectBtn);

    // Verify selected pack card appears and server name is populated
    await waitFor(() => {
      expect(screen.getByText(/Dedicated Modpack Server/i)).toBeInTheDocument();
    });

    const createBtn = screen.getByRole('button', { name: /Create Server from Modpack/i });
    await user.click(createBtn);

    await waitFor(() => {
      expect(mockCreateServer).toHaveBeenCalled();
      expect(mockInstallRemote).toHaveBeenCalledWith('srv-created-1', expect.objectContaining({
        created_with_modpack: true,
        source: 'modrinth',
        project_id: 'cobblemon-proj',
        project_slug: 'cobblemon',
      }));
      expect(onCreated).toHaveBeenCalled();
    });
  });
});
