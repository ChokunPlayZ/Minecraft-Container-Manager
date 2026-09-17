import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Server, Mod, VersionUpdateReport } from '../src/api/types';
import { VersionUpdateHelper } from '../src/components/version-update-helper';

const mockServer: Server = {
  id: 'srv-1',
  name: 'My SMP Server',
  server_type: 'fabric',
  version: '1.20.1',
  build: '0.15.11',
  status: 'running',
  state: 'running',
  host_port: 25565,
  ram_mb: 4096,
  cpu_limit: 2,
  memory_limit_mb: 4096,
  autostart: true,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const mockMods: Mod[] = [
  {
    name: 'lithium-fabric-0.12.0',
    file: 'lithium-fabric-0.12.0.jar',
    enabled: true,
    title: 'Lithium',
    version: '0.12.0',
    provider: 'modrinth',
  },
  {
    name: 'fabric-api-0.90.0',
    file: 'fabric-api-0.90.0.jar',
    enabled: true,
    title: 'Fabric API',
    version: '0.90.0',
    provider: 'modrinth',
  },
  {
    name: 'old-mod-1.0',
    file: 'old-mod-1.0.jar',
    enabled: true,
    title: 'Old Mod',
    version: '1.0',
    provider: 'modrinth',
  },
];

const mockReport: VersionUpdateReport = {
  current_version: '1.20.1',
  target_version: '1.21.1',
  server_type: 'fabric',
  total_mods: 3,
  compatible_count: 2,
  update_count: 1,
  not_available_count: 1,
  untracked_count: 0,
  ready_to_update: false,
  last_checked: new Date().toISOString(),
  mods: {
    'lithium-fabric-0.12.0': {
      mod_name: 'lithium-fabric-0.12.0',
      mod_file: 'lithium-fabric-0.12.0.jar',
      title: 'Lithium',
      provider: 'modrinth',
      current_version: '0.12.0',
      status: 'update_available',
      compatible_version: '0.14.0',
      compatible_jar: 'lithium-fabric-0.14.0.jar',
      download_url: 'https://cdn.modrinth.com/data/lithium-fabric-0.14.0.jar',
      release_type: 'release',
    },
    'fabric-api-0.90.0': {
      mod_name: 'fabric-api-0.90.0',
      mod_file: 'fabric-api-0.90.0.jar',
      title: 'Fabric API',
      provider: 'modrinth',
      current_version: '0.90.0',
      status: 'already_compatible',
    },
    'old-mod-1.0': {
      mod_name: 'old-mod-1.0',
      mod_file: 'old-mod-1.0.jar',
      title: 'Old Mod',
      provider: 'modrinth',
      current_version: '1.0',
      status: 'not_available',
    },
  },
};

describe('VersionUpdateHelper', () => {
  beforeEach(() => {
    vi.spyOn(api, 'jarVersions').mockResolvedValue([
      { name: '1.21.1', latest: '1.21.1' },
      { name: '1.20.1', latest: '1.20.1' },
    ]);
    vi.spyOn(api, 'checkVersionUpgradeCompatibility').mockResolvedValue(mockReport);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders header, current version and fetches target versions', async () => {
    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    expect(screen.getByText('Version Update Helper')).toBeInTheDocument();
    expect(screen.getByText(/Current:/i)).toBeInTheDocument();
    expect(screen.getByText(/fabric 1.20.1/i)).toBeInTheDocument();

    await waitFor(() => {
      expect(api.jarVersions).toHaveBeenCalledWith('fabric');
    });
  });

  it('automatically scans and displays compatibility report', async () => {
    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    await waitFor(() => {
      expect(api.checkVersionUpgradeCompatibility).toHaveBeenCalledWith(
        'srv-1',
        '1.21.1',
        false,
      );
    });

    // Verify readiness score
    await waitFor(() => {
      expect(screen.getByText(/67% \(2\/3 mods ready\)/i)).toBeInTheDocument();
    });

    // Check verdict banner
    expect(screen.getByText(/1 mod is not yet ready for Minecraft 1.21.1/i)).toBeInTheDocument();

    // Verify individual mods rendered
    expect(screen.getByText('Lithium')).toBeInTheDocument();
    expect(screen.getByText('Update Ready')).toBeInTheDocument();
    expect(screen.getByText('lithium-fabric-0.14.0.jar')).toBeInTheDocument();

    expect(screen.getByText('Fabric API')).toBeInTheDocument();
    expect(screen.getAllByText('Already Compatible').length).toBeGreaterThanOrEqual(1);

    expect(screen.getByText('Old Mod')).toBeInTheDocument();
    expect(screen.getByText(/No 1.21.1 Build/i)).toBeInTheDocument();
  });

  it('filters mods by status tab', async () => {
    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    await waitFor(() => {
      expect(screen.getByText('Lithium')).toBeInTheDocument();
    });

    // Click Missing filter
    const missingTab = screen.getByRole('button', { name: /Missing \(1\)/i });
    fireEvent.click(missingTab);

    // Old Mod should be shown, Lithium and Fabric API filtered out
    expect(screen.getByText('Old Mod')).toBeInTheDocument();
    expect(screen.queryByText('Lithium')).not.toBeInTheDocument();
    expect(screen.queryByText('Fabric API')).not.toBeInTheDocument();
  });

  it('copies compatibility markdown report to clipboard', async () => {
    const writeTextSpy = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, {
      clipboard: { writeText: writeTextSpy },
    });

    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    await waitFor(() => {
      expect(screen.getByText('Lithium')).toBeInTheDocument();
    });

    const copyBtn = screen.getByRole('button', { name: /Copy Report/i });
    await act(async () => {
      fireEvent.click(copyBtn);
    });

    expect(writeTextSpy).toHaveBeenCalled();
    const copiedContent = writeTextSpy.mock.calls[0][0] as string;
    expect(copiedContent).toContain('# Minecraft 1.21.1 Upgrade Report');
    expect(copiedContent).toContain('Lithium');
    expect(copiedContent).toContain('Fabric API');
    expect(copiedContent).toContain('Old Mod');
  });

  it('creates pre-upgrade backup when button is clicked', async () => {
    const backupSpy = vi.spyOn(api, 'createBackup').mockResolvedValue({
      id: 'bk-1',
      name: 'Pre-Upgrade-1.21.1',
      server_id: 'srv-1',
      size_bytes: 1000,
      storage: 'local',
      created_at: new Date().toISOString(),
    });

    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    await waitFor(() => {
      expect(screen.getByText('Lithium')).toBeInTheDocument();
    });

    const backupBtn = screen.getByRole('button', { name: /Create Backup/i });
    await act(async () => {
      fireEvent.click(backupBtn);
    });

    expect(backupSpy).toHaveBeenCalledWith('srv-1', expect.stringContaining('Pre-Upgrade-1.21.1'));
    expect(screen.getByText(/created successfully!/i)).toBeInTheDocument();
  });
});
