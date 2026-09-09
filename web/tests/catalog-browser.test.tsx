import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Mod, Server } from '../src/api/types';
import { CatalogBrowser } from '../src/components/catalog-browser';

const mockServer: Server = {
  id: 'srv-1',
  name: 'Paper Test Server',
  server_type: 'paper',
  version: '1.21.1',
  port: 25565,
  rcon_port: 25575,
  status: 'running',
  memory: '4G',
  cpu_limit: 2,
  created_at: '2026-01-01T00:00:00Z',
  auto_start: true,
};

const mockInstalledMods: Mod[] = [];

describe('CatalogBrowser multi-provider component', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  afterEach(() => {
    global.fetch = originalFetch;
    localStorage.clear();
  });

  it('renders all 4 provider selector tabs with active indicator', () => {
    render(
      <CatalogBrowser
        server={mockServer}
        installedMods={mockInstalledMods}
        onModInstalled={() => {}}
      />,
    );

    expect(screen.getByRole('button', { name: /Modrinth/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /PaperMC Hangar/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /SpigotMC/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /CurseForge/i })).toBeInTheDocument();
  });

  it('switches to Hangar tab and allows one-click install', async () => {
    const user = userEvent.setup();

    const mockHangarSearch = {
      pagination: { count: 1, limit: 20, offset: 0 },
      result: [
        {
          id: 101,
          name: 'Chunky',
          namespace: { owner: 'pop4959', slug: 'Chunky' },
          stats: { downloads: 50000, stars: 100, views: 100000, recentDownloads: 500, recentViews: 1000, watchers: 5 },
          category: 'world_management',
          description: 'Pre-generates chunks quickly.',
          lastUpdated: '2026-08-01T00:00:00Z',
          visibility: 'public',
          avatarUrl: '',
          supportedPlatforms: { PAPER: ['1.21'] },
        },
      ],
    };

    const mockHangarVersions = {
      pagination: { count: 1, limit: 25, offset: 0 },
      result: [
        {
          id: 1,
          projectId: 101,
          name: '1.4.10',
          visibility: 'public',
          stats: { totalDownloads: 50000, platformDownloads: { PAPER: 50000 } },
          author: 'pop4959',
          channel: { name: 'Release', color: '#00cc88' },
          downloads: {
            PAPER: {
              fileInfo: {
                name: 'Chunky-1.4.10.jar',
                sizeBytes: 900000,
                sha256Hash: 'hash',
              },
              downloadUrl: 'https://hangarcdn.papermc.io/plugins/pop4959/Chunky/versions/1.4.10/PAPER/Chunky-1.4.10.jar',
              externalUrl: null,
            },
          },
          platformDependencies: { PAPER: ['1.21'] },
          createdAt: '2026-08-01T00:00:00Z',
        },
      ],
    };

    global.fetch = vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/versions')) {
        return { ok: true, json: async () => mockHangarVersions };
      }
      return { ok: true, json: async () => mockHangarSearch };
    });

    vi.spyOn(api, 'downloadMod').mockResolvedValue({
      name: 'Chunky',
      file: 'Chunky-1.4.10.jar',
      enabled: true,
    });

    const onInstalled = vi.fn();

    render(
      <CatalogBrowser
        server={mockServer}
        installedMods={mockInstalledMods}
        onModInstalled={onInstalled}
      />,
    );

    // Switch to Hangar tab
    const hangarTab = screen.getByRole('button', { name: /PaperMC Hangar/i });
    await user.click(hangarTab);

    // Should see Chunky project
    await waitFor(() => {
      expect(screen.getByText('Pre-generates chunks quickly.')).toBeInTheDocument();
      expect(screen.getByText('Chunky')).toBeInTheDocument();
    });

    // Click Install Latest
    const installButton = screen.getByRole('button', { name: /Install Latest/i });
    await user.click(installButton);

    await waitFor(() => {
      expect(api.downloadMod).toHaveBeenCalledWith(
        mockServer.id,
        'https://hangarcdn.papermc.io/plugins/pop4959/Chunky/versions/1.4.10/PAPER/Chunky-1.4.10.jar',
        'Chunky-1.4.10.jar',
      );
      expect(onInstalled).toHaveBeenCalled();
    });
  });

  it('switches to SpigotMC tab and allows downloading resource jar', async () => {
    const user = userEvent.setup();

    const mockSpigotResources = [
      {
        id: 28140,
        name: 'LuckPerms',
        tag: 'A permissions plugin for Minecraft',
        version: { id: 123 },
        author: { id: 456 },
        category: { id: 1 },
        rating: { count: 50, average: 4.9 },
        downloads: 1000000,
        icon: { url: '', data: '' },
        releaseDate: 123456,
        updateDate: 123456,
      },
    ];

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => mockSpigotResources,
    });

    vi.spyOn(api, 'downloadMod').mockResolvedValue({
      name: 'LuckPerms',
      file: 'LuckPerms.jar',
      enabled: true,
    });

    const onInstalled = vi.fn();

    render(
      <CatalogBrowser
        server={mockServer}
        installedMods={mockInstalledMods}
        onModInstalled={onInstalled}
      />,
    );

    // Switch to SpigotMC tab
    const spigotTab = screen.getByRole('button', { name: /SpigotMC/i });
    await user.click(spigotTab);

    await waitFor(() => {
      expect(screen.getByText(/SpigotMC Catalog/i)).toBeInTheDocument();
      expect(screen.getByText('LuckPerms')).toBeInTheDocument();
    });

    // Click Install Jar
    const installJarButton = screen.getByRole('button', { name: /Install Jar/i });
    await user.click(installJarButton);

    await waitFor(() => {
      expect(api.downloadMod).toHaveBeenCalledWith(
        mockServer.id,
        'https://cdn.spiget.org/file/spiget-resources/28140.jar',
        'LuckPerms.jar',
      );
      expect(onInstalled).toHaveBeenCalled();
    });
  });

  it('switches to CurseForge tab and prompts for API key when not connected', async () => {
    const user = userEvent.setup();

    render(
      <CatalogBrowser
        server={mockServer}
        installedMods={mockInstalledMods}
        onModInstalled={() => {}}
      />,
    );

    // Switch to CurseForge tab
    const cfTab = screen.getByRole('button', { name: /CurseForge/i });
    await user.click(cfTab);

    // Verify setup banner is shown
    expect(screen.getByRole('heading', { name: /CurseForge API Key/i })).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/Paste your CurseForge API key/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Save & Connect/i })).toBeInTheDocument();
  });
});
