import { renderWithAdmin as render } from '../src/test/auth';
import { act, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { fetchModAvailableJars } from '../src/api/mod-updates';
import { clearRateLimitCache } from '../src/api/rate-limited-fetch';
import type { BatchUpdateModsResult, Mod, Server, TaskProgress } from '../src/api/types';
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

  describe('server update planner contract', () => {
    it('uses the server update decision without issuing browser catalog checks', async () => {
      const decision = {
        updates: {
          'fabric-api': {
            mod_name: 'fabric-api',
            latest_version: '0.105.0',
            latest_jar: 'fabric-api-0.105.0.jar',
            provider: 'modrinth',
          },
        },
        last_checked: '2026-10-28T00:00:00Z',
      };
      const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(decision), {
        status: 200, headers: { 'Content-Type': 'application/json' },
      }));
      global.fetch = fetchMock;

      await expect(api.checkModUpdates(mockServer.id)).resolves.toEqual(decision);
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(fetchMock.mock.calls[0][0]).toBe(`/api/servers/${mockServer.id}/mods/updates?cached=true`);
    });

    it('does not forward legacy credentials and surfaces a server planning failure', async () => {
      localStorage.setItem('mcm_curseforge_api_key', ' provider-key ');
      const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
        error: { code: 'upstream_error', message: 'Provider unavailable' },
      }), { status: 502, headers: { 'Content-Type': 'application/json' } }));
      global.fetch = fetchMock;
      try {
        await expect(api.checkModUpdates(mockServer.id, true)).rejects.toThrow('Provider unavailable');
        expect(fetchMock.mock.calls[0][0]).toBe(`/api/servers/${mockServer.id}/mods/updates?force=true`);
        expect(new Headers(fetchMock.mock.calls[0][1].headers).has('x-curseforge-api-key')).toBe(false);
      } finally {
        localStorage.removeItem('mcm_curseforge_api_key');
      }
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
          'fabric-api-0.100.0.jar',
          expect.objectContaining({ releaseType: 'release', allowChannelSwitch: false }),
        );
        expect(onUpdated).toHaveBeenCalled();
        expect(onClose).toHaveBeenCalled();
      });
    });

    it('offers a more stable channel without changing the installed channel automatically', async () => {
      const user = userEvent.setup();
      const mod: Mod = { name: 'example', file: 'example-beta.jar.disabled', provider: 'modrinth', enabled: false, release_type: 'beta' };
      vi.spyOn(api, 'getModAvailableVersions').mockResolvedValue([
        { version_id: 'stable', version_name: 'Example Stable', version_number: '2.0', filename: 'example-release.jar', download_url: 'https://cdn.modrinth.com/stable.jar', release_type: 'release' },
        { version_id: 'beta', version_name: 'Example Beta', version_number: '2.1-beta', filename: 'example-beta.jar', download_url: 'https://cdn.modrinth.com/beta.jar', release_type: 'beta', is_current: true },
      ]);
      const download = vi.spyOn(api, 'downloadMod').mockResolvedValue({ ...mod, release_type: 'release' });

      render(<ModJarPickerDialog isOpen server={mockServer} mod={mod} onClose={vi.fn()} onUpdated={vi.fn()} />);
      const showReleases = await screen.findByRole('button', { name: 'View Release builds' });
      expect(screen.getByText('Example Beta')).toBeVisible();
      expect(screen.queryByText('Example Stable')).toBeNull();
      await user.click(showReleases);
      expect(screen.getByText('Example Stable')).toBeVisible();
      expect(download).not.toHaveBeenCalled();
      await user.click(screen.getByRole('button', { name: 'Switch to Release' }));
      expect(download).toHaveBeenCalledWith(mockServer.id, 'https://cdn.modrinth.com/stable.jar', 'example-release.jar', mod.file, expect.objectContaining({
        allowChannelSwitch: true, releaseType: 'release', versionId: 'stable',
      }));
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

    it('supports 1-click batch updates with progress sized to stay inside the banner', async () => {
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

      let progressCb: ((p: TaskProgress) => void) | null = null;
      vi.spyOn(api, 'subscribeTaskEvents').mockImplementation((_sid, cb) => {
        progressCb = cb;
        return () => {};
      });

      const batchResult: BatchUpdateModsResult = {
        updated: 1,
        failed: 0,
        target_version: '1.21.1',
        total: 1,
        items: [
          {
            mod_name: 'fabric-api',
            success: true,
            original: 'fabric-api-0.100.0.jar',
            new_jar: 'fabric-api-0.105.0.jar',
          },
        ],
      };
      let finishBatch!: (result: BatchUpdateModsResult) => void;
      const batchUpdateSpy = vi.spyOn(api, 'batchUpdateMods').mockImplementation(
        () => new Promise((resolve) => { finishBatch = resolve; }),
      );

      render(<ModsPanel server={mockServer} />);

      // Wait for updates banner to appear
      await waitFor(() => {
        expect(screen.getByText(/1 update is available/i)).toBeInTheDocument();
      });

      // Click "Update All (1)" button in quick banner
      const updateAllBannerBtn = screen.getByRole('button', { name: /Update All \(1\)/i });
      await user.click(updateAllBannerBtn);

      // Confirm modal opens
      const confirmBtn = await screen.findByRole('button', { name: /Update All 1/i });
      expect(confirmBtn).toBeInTheDocument();

      await user.click(confirmBtn);

      expect(batchUpdateSpy).toHaveBeenCalledWith('srv-update-test');

      // Only the track has a fixed height; the wrapper must also fit the label.
      const bar = screen.getByRole('progressbar');
      expect(bar).toHaveClass('h-2');
      expect(bar.parentElement).not.toHaveClass('h-2');
      expect(bar.parentElement).toContainElement(screen.getByText('Preparing updates...'));
      expect(bar).toHaveAttribute('aria-valuenow', '0');

      // Simulate SSE event
      await act(async () => {
        progressCb?.({
          server_id: 'srv-update-test',
          operation: 'mod_update_batch',
          stage: 'downloading',
          current: 0,
          total: 1,
          percent: 50,
          message: 'Downloading Fabric API',
        });
      });
      expect(bar).toHaveAttribute('aria-valuenow', '50');
      expect(screen.getByText('Downloading Fabric API')).toBeInTheDocument();

      await act(async () => {
        finishBatch(batchResult);
      });

      // Verify success banner appears
      await waitFor(() => {
        expect(
          screen.getByText(/Successfully updated all 1 mods to their latest versions!/i),
        ).toBeInTheDocument();
      });
    });
  });
});
