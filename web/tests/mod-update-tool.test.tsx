import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { checkModsForUpdates, fetchModAvailableJars } from '../src/api/mod-updates';
import { clearRateLimitCache } from '../src/api/rate-limited-fetch';
import type { Mod, Server } from '../src/api/types';
import { ModsPanel } from '../src/components/mods-panel';
import { ModJarPickerDialog } from '../src/components/mod-jar-picker-dialog';

const mockServer: Server = {
  id: 'srv-update-test',
  name: 'Fabric Survival',
  server_type: 'fabric',
  version: '1.21.1',
  build: '1',
  ram_mb: 4096,
  cpu_limit: 2,
  memory_limit_mb: 4096,
  host_port: 25565,
  extra_ports: [],
  container_id: 'c-123',
  state: 'running',
  backup_enabled: false,
  backup_interval_minutes: 60,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('Mod Update Tool & Jar Picker', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.restoreAllMocks();
    clearRateLimitCache();
  });

  afterEach(() => {
    global.fetch = originalFetch;
    clearRateLimitCache();
  });

  describe('checkModsForUpdates API', () => {
    it('detects available updates for mods via Modrinth batch hashes', async () => {
      const installedMods: Mod[] = [
        {
          name: 'fabric-api',
          file: 'fabric-api-0.100.0.jar',
          enabled: true,
          sha1: 'oldhash123',
          version: '0.100.0',
        },
        {
          name: 'sodium',
          file: 'sodium-fabric-0.5.8.jar',
          enabled: true,
          sha1: 'uptodatehash456',
          version: '0.5.8',
        },
      ];

      const mockBatchResponse = {
        oldhash123: {
          id: 'ver-new',
          project_id: 'P7dR8mSH',
          name: 'Fabric API 0.105.0',
          version_number: '0.105.0',
          version_type: 'release',
          date_published: '2026-09-01T12:00:00Z',
          files: [
            {
              url: 'https://cdn.modrinth.com/data/fabric-api-0.105.0.jar',
              filename: 'fabric-api-0.105.0.jar',
              primary: true,
              size: 2048576,
              hashes: { sha1: 'newhash789' },
            },
          ],
        },
        uptodatehash456: {
          id: 'ver-current',
          project_id: 'AANobbMI',
          name: 'Sodium 0.5.8',
          version_number: '0.5.8',
          version_type: 'release',
          files: [
            {
              url: 'https://cdn.modrinth.com/data/sodium-fabric-0.5.8.jar',
              filename: 'sodium-fabric-0.5.8.jar',
              primary: true,
              size: 1048576,
              hashes: { sha1: 'uptodatehash456' },
            },
          ],
        },
      };

      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockBatchResponse,
      });

      const updates = await checkModsForUpdates(installedMods, mockServer);

      // Only fabric-api should have an update detected
      expect(Object.keys(updates)).toHaveLength(1);
      expect(updates['fabric-api']).toBeDefined();
      expect(updates['fabric-api'].latestJar).toBe('fabric-api-0.105.0.jar');
      expect(updates['fabric-api'].latestVersion).toBe('0.105.0');
      expect(updates['sodium']).toBeUndefined();
    });

    it('detects available updates for Hangar provider mods', async () => {
      const installedMods: Mod[] = [
        {
          name: 'Chunky',
          file: 'Chunky-1.4.9.jar',
          enabled: true,
          provider: 'hangar',
          project_slug: 'Chunky',
          version: '1.4.9',
        },
      ];

      const mockHangarVersions = {
        pagination: { count: 1, limit: 5, offset: 0 },
        result: [
          {
            id: 200,
            name: '1.4.11',
            channel: { name: 'Release', color: '#00cc88' },
            downloads: {
              PAPER: {
                fileInfo: {
                  name: 'Chunky-1.4.11.jar',
                  sizeBytes: 950000,
                  sha256Hash: 'hash256',
                },
                downloadUrl: 'https://hangarcdn.papermc.io/plugins/Chunky-1.4.11.jar',
              },
            },
            createdAt: '2026-09-05T00:00:00Z',
          },
        ],
      };

      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockHangarVersions,
      });

      const paperServer = { ...mockServer, server_type: 'paper' as const };
      const updates = await checkModsForUpdates(installedMods, paperServer);

      expect(updates['Chunky']).toBeDefined();
      expect(updates['Chunky'].latestJar).toBe('Chunky-1.4.11.jar');
      expect(updates['Chunky'].latestVersion).toBe('1.4.11');
    });
  });

  describe('fetchModAvailableJars', () => {
    it('fetches compatible releases and marks the currently installed jar', async () => {
      const mod: Mod = {
        name: 'fabric-api',
        file: 'fabric-api-0.100.0.jar',
        enabled: true,
        project_slug: 'fabric-api',
        provider: 'modrinth',
      };

      const mockVersions = [
        {
          id: 'v2',
          name: 'Fabric API 0.105.0',
          version_number: '0.105.0',
          version_type: 'release',
          game_versions: ['1.21.1'],
          loaders: ['fabric'],
          date_published: '2026-09-01T12:00:00Z',
          files: [
            {
              url: 'https://cdn.modrinth.com/data/fabric-api-0.105.0.jar',
              filename: 'fabric-api-0.105.0.jar',
              primary: true,
              size: 2048576,
            },
          ],
        },
        {
          id: 'v1',
          name: 'Fabric API 0.100.0',
          version_number: '0.100.0',
          version_type: 'release',
          game_versions: ['1.21.1'],
          loaders: ['fabric'],
          date_published: '2026-08-01T12:00:00Z',
          files: [
            {
              url: 'https://cdn.modrinth.com/data/fabric-api-0.100.0.jar',
              filename: 'fabric-api-0.100.0.jar',
              primary: true,
              size: 2000000,
            },
          ],
        },
      ];

      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockVersions,
      });

      const jars = await fetchModAvailableJars(mod, undefined, mockServer);

      expect(jars).toHaveLength(2);
      expect(jars[0].filename).toBe('fabric-api-0.105.0.jar');
      expect(jars[0].isCurrent).toBe(false);

      expect(jars[1].filename).toBe('fabric-api-0.100.0.jar');
      expect(jars[1].isCurrent).toBe(true);
    });
  });

  describe('ModJarPickerDialog component', () => {
    it('renders dialog, lists jar releases, and allows picking a new jar to update', async () => {
      const user = userEvent.setup();

      const mod: Mod = {
        name: 'fabric-api',
        file: 'fabric-api-0.100.0.jar',
        enabled: true,
        version: '0.100.0',
        project_slug: 'fabric-api',
        provider: 'modrinth',
      };

      const mockVersions = [
        {
          id: 'v2',
          name: 'Fabric API 0.105.0',
          version_number: '0.105.0',
          version_type: 'release',
          game_versions: ['1.21.1'],
          loaders: ['fabric'],
          date_published: '2026-09-01T12:00:00Z',
          files: [
            {
              url: 'https://cdn.modrinth.com/data/fabric-api-0.105.0.jar',
              filename: 'fabric-api-0.105.0.jar',
              primary: true,
              size: 2048576,
            },
          ],
        },
      ];

      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockVersions,
      });

      vi.spyOn(api, 'downloadMod').mockResolvedValue({
        name: 'fabric-api',
        file: 'fabric-api-0.105.0.jar',
        enabled: true,
      });

      const onUpdated = vi.fn();
      const onClose = vi.fn();

      render(
        <ModJarPickerDialog
          isOpen={true}
          server={mockServer}
          mod={mod}
          onClose={onClose}
          onUpdated={onUpdated}
        />,
      );

      // Verify header & current jar banner
      expect(screen.getByRole('heading', { name: /Update fabric-api/i })).toBeInTheDocument();
      expect(screen.getAllByText('fabric-api-0.100.0.jar').length).toBeGreaterThan(0);

      // Wait for versions to load
      await waitFor(() => {
        expect(screen.getByText('Fabric API 0.105.0')).toBeInTheDocument();
        expect(screen.getByText(/fabric-api-0.105.0.jar/i)).toBeInTheDocument();
      });

      // Click "Pick & Update"
      const pickButton = screen.getByRole('button', { name: /Pick & Update/i });
      await user.click(pickButton);

      await waitFor(() => {
        expect(api.downloadMod).toHaveBeenCalledWith(
          mockServer.id,
          'https://cdn.modrinth.com/data/fabric-api-0.105.0.jar',
          'fabric-api-0.105.0.jar',
          'fabric-api', // deletes old jar by default
          expect.anything(),
        );
        expect(onUpdated).toHaveBeenCalled();
        expect(onClose).toHaveBeenCalled();
      });
    });

    it('supports uploading a replacement jar file directly from the Upload tab', async () => {
      const user = userEvent.setup();

      const mod: Mod = {
        name: 'custom-mod',
        file: 'custom-mod-1.0.jar',
        enabled: true,
      };

      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => [],
      });

      vi.spyOn(api, 'uploadMod').mockResolvedValue({
        name: 'custom-mod',
        file: 'custom-mod-2.0.jar',
        enabled: true,
      });

      const onUpdated = vi.fn();
      const onClose = vi.fn();

      render(
        <ModJarPickerDialog
          isOpen={true}
          server={mockServer}
          mod={mod}
          onClose={onClose}
          onUpdated={onUpdated}
        />,
      );

      // Switch to upload tab
      const uploadTab = screen.getByRole('button', { name: /Upload Local Jar/i });
      await user.click(uploadTab);

      const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
      expect(fileInput).toBeInTheDocument();

      const newJarFile = new File(['fake-jar-data'], 'custom-mod-2.0.jar', {
        type: 'application/java-archive',
      });
      await user.upload(fileInput, newJarFile);

      await waitFor(() => {
        expect(api.uploadMod).toHaveBeenCalledWith(
          mockServer.id,
          newJarFile,
          expect.any(Function),
          'custom-mod', // deletes old jar by default
          expect.anything(),
        );
        expect(onUpdated).toHaveBeenCalled();
        expect(onClose).toHaveBeenCalled();
      });
    });
  });

  describe('ModsPanel Mod Update Tool View', () => {
    it('displays update badge, update button, and dedicated update tab', async () => {
      const user = userEvent.setup();

      const installedMods: Mod[] = [
        {
          name: 'fabric-api',
          title: 'Fabric API',
          file: 'fabric-api-0.100.0.jar',
          enabled: true,
          sha1: 'oldhash123',
          version: '0.100.0',
        },
      ];

      vi.spyOn(api, 'mods').mockResolvedValue({
        type: 'mods',
        items: installedMods,
      });

      const mockBatchResponse = {
        oldhash123: {
          id: 'ver-new',
          project_id: 'P7dR8mSH',
          name: 'Fabric API 0.105.0',
          version_number: '0.105.0',
          version_type: 'release',
          date_published: '2026-09-01T12:00:00Z',
          files: [
            {
              url: 'https://cdn.modrinth.com/data/fabric-api-0.105.0.jar',
              filename: 'fabric-api-0.105.0.jar',
              primary: true,
              size: 2048576,
              hashes: { sha1: 'newhash789' },
            },
          ],
        },
      };

      vi.spyOn(api, 'checkModUpdates').mockResolvedValue({
        updates: {
          'fabric-api': {
            mod_name: 'fabric-api',
            title: 'Fabric API',
            current_version: '0.100.0',
            latest_version: '0.105.0',
            latest_jar: 'fabric-api-0.105.0.jar',
            latest_download_url: 'https://cdn.modrinth.com/data/fabric-api-0.105.0.jar',
            latest_release_date: '2026-09-01T12:00:00Z',
            provider: 'modrinth',
            project_id: 'P7dR8mSH',
          } as any,
        },
        last_checked: new Date().toISOString(),
      });

      global.fetch = vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('/version_files/update')) {
          return { ok: true, json: async () => mockBatchResponse };
        }
        if (url.includes('/version_files')) {
          return { ok: true, json: async () => ({}) };
        }
        return {
          ok: true,
          json: async () => [
            {
              id: 'ver-new',
              name: 'Fabric API 0.105.0',
              version_number: '0.105.0',
              files: [
                {
                  url: 'https://cdn.modrinth.com/data/fabric-api-0.105.0.jar',
                  filename: 'fabric-api-0.105.0.jar',
                  primary: true,
                  size: 2048576,
                },
              ],
            },
          ],
        };
      });

      render(<ModsPanel server={mockServer} />);

      // Wait for mods and updates to load
      await waitFor(() => {
        expect(screen.getByText('fabric-api')).toBeInTheDocument();
        expect(screen.getByText(/Update available: 0.105.0/i)).toBeInTheDocument();
      });

      // Verify the "Update" button is displayed on the installed mod row
      const updateRowButton = screen.getByRole('button', { name: /^Update$/i });
      expect(updateRowButton).toBeInTheDocument();

      // Verify the "Mod Update Tool" tab is rendered with count badge
      const updateTabButton = screen.getByRole('button', { name: /Mod Update Tool/i });
      expect(updateTabButton).toBeInTheDocument();

      // Click the Mod Update Tool tab
      await user.click(updateTabButton);

      // Verify the Mod Update Tool view displays the old vs new jar comparison
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'Mod Update Tool' })).toBeInTheDocument();
        expect(screen.getByText('fabric-api-0.100.0.jar')).toBeInTheDocument();
        expect(screen.getByText('fabric-api-0.105.0.jar')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /Update & Pick Jar/i })).toBeInTheDocument();
      });

      // Click "Update & Pick Jar"
      const pickJarButton = screen.getByRole('button', { name: /Update & Pick Jar/i });
      await user.click(pickJarButton);

      // Verify the Jar Picker Modal opens
      await waitFor(() => {
        expect(screen.getByRole('dialog')).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: /Update Fabric API/i })).toBeInTheDocument();
      });
    });

    it('does not repeatedly spam update check queries on mount or re-render', async () => {
      const checkSpy = vi.spyOn(api, 'checkModUpdates').mockResolvedValue({
        updates: {},
        last_checked: new Date().toISOString(),
      });

      const installedMods: Mod[] = [
        {
          name: 'fabric-api',
          title: 'Fabric API',
          file: 'fabric-api-0.100.0.jar',
          enabled: true,
          version: '0.100.0',
        },
      ];

      // Simulate network request returning fresh array reference each time
      vi.spyOn(api, 'mods').mockImplementation(async () => ({
        type: 'mods',
        items: [...installedMods],
      }));

      render(<ModsPanel server={mockServer} />);

      await waitFor(() => {
        expect(screen.getByText('fabric-api')).toBeInTheDocument();
      });

      // Allow any microtasks / effects to settle
      await new Promise((r) => setTimeout(r, 150));

      // Must be called exactly once, NOT spammed in a render loop
      expect(checkSpy).toHaveBeenCalledTimes(1);
    });
  });
});
