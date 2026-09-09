import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Mod, ModrinthSearchHit, ModrinthVersion, Server } from '../src/api/types';
import { ModrinthBrowser } from '../src/components/modrinth-browser';
import { ModsPanel } from '../src/components/mods-panel';

const mockServer: Server = {
  id: 'server-1',
  name: 'Mega SMP',
  server_type: 'paper',
  version: '1.21.4',
  build: '145',
  ram_mb: 4096,
  cpu_limit: 2,
  memory_limit_mb: 4096,
  host_port: 25565,
  extra_ports: [],
  container_id: 'container-1',
  state: 'running',
  backup_enabled: false,
  backup_interval_minutes: 60,
  spin_down_enabled: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const mockHit1: ModrinthSearchHit = {
  project_id: 'hit-1',
  project_type: 'mod',
  slug: 'luckperms',
  author: 'Luck',
  title: 'LuckPerms',
  description: 'An advanced permissions plugin for Minecraft servers.',
  categories: ['paper', 'spigot', 'management'],
  versions: ['1.21.4'],
  downloads: 12500000,
  follows: 45000,
  icon_url: 'https://cdn.modrinth.com/luckperms.png',
  date_created: '2021-01-01T00:00:00Z',
  date_modified: '2026-01-01T00:00:00Z',
  latest_version: 'v1.0',
  license: 'GPL-3.0',
  client_side: 'optional',
  server_side: 'required',
};

const mockHit2: ModrinthSearchHit = {
  project_id: 'hit-2',
  project_type: 'mod',
  slug: 'chunky',
  author: 'pop4959',
  title: 'Chunky',
  description: 'Pre-generates chunks quickly and efficiently.',
  categories: ['paper', 'optimization'],
  versions: ['1.21.4'],
  downloads: 850000,
  follows: 12000,
  icon_url: null,
  date_created: '2021-01-01T00:00:00Z',
  date_modified: '2026-01-01T00:00:00Z',
  latest_version: 'v2.0',
  license: 'GPL-3.0',
  client_side: 'optional',
  server_side: 'required',
};

const mockVersions: ModrinthVersion[] = [
  {
    id: 'ver-1',
    project_id: 'hit-2',
    author_id: 'author-1',
    name: 'Chunky 1.4.28',
    version_number: '1.4.28',
    game_versions: ['1.21.4'],
    version_type: 'release',
    loaders: ['paper', 'spigot'],
    featured: true,
    status: 'listed',
    date_published: '2026-01-01T00:00:00Z',
    downloads: 50000,
    files: [
      {
        filename: 'Chunky-1.4.28.jar',
        url: 'https://cdn.modrinth.com/data/chunky.jar',
        primary: true,
        size: 512000,
      },
    ],
  },
];

describe('ModrinthBrowser component', () => {
  beforeEach(() => {
    vi.restoreAllMocks();

    vi.spyOn(global, 'fetch').mockImplementation(async (url) => {
      const urlStr = String(url);
      if (urlStr.includes('/search')) {
        return {
          ok: true,
          json: async () => ({
            hits: [mockHit1, mockHit2],
            offset: 0,
            limit: 20,
            total_hits: 2,
          }),
        } as Response;
      }
      if (urlStr.includes('/version')) {
        return {
          ok: true,
          json: async () => mockVersions,
        } as Response;
      }
      return { ok: false, status: 404 } as Response;
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders server matching banner and mod cards', async () => {
    const installed: Mod[] = [
      { name: 'LuckPerms', file: 'LuckPerms-5.4.jar', enabled: true },
    ];

    render(
      <ModrinthBrowser
        server={mockServer}
        installedMods={installed}
        onModInstalled={vi.fn()}
      />,
    );

    // Matching banner displays server software
    expect(screen.getByText('Matching Server Software')).toBeInTheDocument();
    expect(screen.getByText('Paper / Spigot')).toBeInTheDocument();
    expect(screen.getAllByText(/MC 1.21.4/i).length).toBeGreaterThan(0);

    // Cards render
    await waitFor(() => {
      expect(screen.getByText('LuckPerms')).toBeInTheDocument();
      expect(screen.getByText('Chunky')).toBeInTheDocument();
    });

    // LuckPerms is installed
    expect(screen.getByRole('button', { name: /Installed/i })).toBeInTheDocument();

    // Chunky has Install button
    expect(screen.getByRole('button', { name: /^Install$/i })).toBeInTheDocument();

    // Descriptions & authors
    expect(screen.getByText(/An advanced permissions plugin/i)).toBeInTheDocument();
    expect(screen.getByText(/Pre-generates chunks quickly/i)).toBeInTheDocument();
  });

  it('performs one-click download when clicking Install', async () => {
    const user = userEvent.setup();
    const onModInstalled = vi.fn();

    vi.spyOn(api, 'downloadMod').mockResolvedValue({
      name: 'Chunky-1.4.28',
      file: 'Chunky-1.4.28.jar',
      enabled: true,
    });

    render(
      <ModrinthBrowser
        server={mockServer}
        installedMods={[]}
        onModInstalled={onModInstalled}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText('Chunky')).toBeInTheDocument();
    });

    const installButtons = screen.getAllByRole('button', { name: /^Install$/i });
    // Click install on Chunky (the second card)
    await user.click(installButtons[1]);

    await waitFor(() => {
      expect(api.downloadMod).toHaveBeenCalledWith(
        'server-1',
        'https://cdn.modrinth.com/data/chunky.jar',
        'Chunky-1.4.28.jar',
      );
      expect(onModInstalled).toHaveBeenCalled();
    });
  });

  it('opens details modal and displays installed jar on server and marks version as installed', async () => {
    const user = userEvent.setup();
    const installed: Mod[] = [
      { name: 'Chunky', file: 'Chunky-1.4.28.jar', enabled: true },
    ];

    render(
      <ModrinthBrowser
        server={mockServer}
        installedMods={installed}
        onModInstalled={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText('Chunky')).toBeInTheDocument();
    });

    // Card should also show the installed jar
    expect(screen.getByText(/Installed: Chunky-1.4.28.jar/i)).toBeInTheDocument();

    const detailButtons = screen.getAllByRole('button', { name: /Details/i });
    await user.click(detailButtons[1]);

    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument();
      // Verifies installed jar banner is shown
      expect(screen.getByText('Installed Jar on Server:')).toBeInTheDocument();
      expect(screen.getAllByText('Chunky-1.4.28.jar').length).toBeGreaterThan(0);
      // Verifies the version row is marked with "Installed Jar" badge
      expect(screen.getByText('Installed Jar')).toBeInTheDocument();
    });
  });
});

describe('ModsPanel with tabs', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(api, 'mods').mockResolvedValue({
      type: 'plugins',
      items: [{ name: 'EssentialsX', file: 'EssentialsX.jar', enabled: true }],
    });
  });

  it('switches between Installed and Browse Modrinth tabs', async () => {
    const user = userEvent.setup();

    render(<ModsPanel server={mockServer} />);

    // Shows Installed tab by default
    await waitFor(() => {
      expect(screen.getAllByText(/Installed Plugins/i).length).toBeGreaterThan(0);
      expect(screen.getByText('EssentialsX')).toBeInTheDocument();
    });

    // Switch to Browse Modrinth
    const browseTabButton = screen.getByRole('button', { name: /Browse Modrinth/i });
    await user.click(browseTabButton);

    // Now shows Modrinth browser
    await waitFor(() => {
      expect(screen.getByText('Matching Server Software')).toBeInTheDocument();
    });

    // Switch back to Installed
    const installedTabButton = screen.getByRole('button', { name: /Installed Plugins/i });
    await user.click(installedTabButton);

    await waitFor(() => {
      expect(screen.getAllByText(/Installed Plugins/i).length).toBeGreaterThan(0);
    });
  });
});
