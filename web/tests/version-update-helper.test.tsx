import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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
  state: 'running',
  host_port: 25565,
  ram_mb: 4096,
  cpu_limit: 2,
  memory_limit_mb: 4096,
  extra_ports: [],
  container_id: null,
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

  it('defaults to the latest stable target and keeps preview versions selectable', async () => {
    const user = userEvent.setup();
    vi.mocked(api.jarVersions).mockResolvedValue([
      { name: '25w01a' }, { name: '1.22-rc1' }, { name: '1.20.1' }, { name: '1.21.1' },
    ]);
    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);
    const selector = screen.getByRole('combobox', { name: 'Target Minecraft version' });
    await waitFor(() => expect(selector).toHaveTextContent('MC 1.21.1'));
    await user.click(selector);
    await user.click(screen.getByRole('option', { name: 'MC 1.22-rc1' }));
    expect(selector).toHaveTextContent('MC 1.22-rc1');
    await user.click(screen.getByRole('button', { name: /Check Compatibility Now/i }));
    await waitFor(() => expect(api.checkVersionUpgradeCompatibility).toHaveBeenCalledWith('srv-1', '1.22-rc1', true));
  });

  it('keeps the current release as the target when it is already the latest stable version', async () => {
    vi.mocked(api.jarVersions).mockResolvedValue([
      { name: '1.22-beta.1' }, { name: '1.20.1' }, { name: '1.21.1' },
    ]);
    render(<VersionUpdateHelper server={{ ...mockServer, version: '1.21.1' }} installedMods={mockMods} />);
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Target Minecraft version' })).toHaveTextContent('MC 1.21.1 (Current)'));
  });

  it('renders header, current version and does not auto-scan until user initiates', async () => {
    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    expect(screen.getByText('Version Update Helper')).toBeInTheDocument();
    expect(screen.queryByText('SMP Upgrade Tool')).not.toBeInTheDocument();
    expect(screen.getByText(/Current:/i)).toBeInTheDocument();
    expect(screen.getByText(/fabric 1.20.1/i)).toBeInTheDocument();

    await waitFor(() => {
      expect(api.jarVersions).toHaveBeenCalledWith('fabric');
    });

    // Should NOT have called compatibility check automatically
    expect(api.checkVersionUpgradeCompatibility).not.toHaveBeenCalled();
    expect(screen.getByText('Ready to check version compatibility?')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Check Compatibility Now/i })).toBeInTheDocument();
  });

  it('scans on user request, shows progress bar, and displays compatibility report', async () => {
    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    const checkBtn = await screen.findByRole('button', { name: /Check Compatibility Now/i });
    await waitFor(() => {
      expect(checkBtn).not.toBeDisabled();
    });
    fireEvent.click(checkBtn);

    // Progress bar should be displayed while scanning
    expect(screen.getByRole('progressbar')).toBeInTheDocument();

    await waitFor(() => {
      expect(api.checkVersionUpgradeCompatibility).toHaveBeenCalledWith(
        'srv-1',
        '1.21.1',
        true,
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

  it('filters mods by status tab after scanning', async () => {
    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    const checkBtn = await screen.findByRole('button', { name: /Check Compatibility Now/i });
    await waitFor(() => {
      expect(checkBtn).not.toBeDisabled();
    });
    fireEvent.click(checkBtn);

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
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: writeTextSpy },
    });

    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    const checkBtn = await screen.findByRole('button', { name: /Check Compatibility Now/i });
    await waitFor(() => {
      expect(checkBtn).not.toBeDisabled();
    });
    fireEvent.click(checkBtn);

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
      location: 'local',
      status: 'completed',
      created_at: new Date().toISOString(),
    });

    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    const checkBtn = await screen.findByRole('button', { name: /Check Compatibility Now/i });
    await waitFor(() => {
      expect(checkBtn).not.toBeDisabled();
    });
    fireEvent.click(checkBtn);

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

  it('performs batch update of compatible mods with real-time progress and silent report refresh', async () => {
    let progressCb: ((p: any) => void) | null = null;
    vi.spyOn(api, 'subscribeTaskEvents').mockImplementation((_sid, cb) => {
      progressCb = cb;
      return () => {};
    });

    const applySpy = vi.spyOn(api, 'applyVersionUpgradeMods').mockResolvedValue({
      updated: 1,
      failed: 0,
      target_version: '1.21.1',
      total: 1,
      items: [
        {
          mod_name: 'lithium-fabric-0.12.0',
          success: true,
          original: 'lithium-fabric-0.12.0.jar',
          new_jar: 'lithium-fabric-0.14.0.jar',
        },
      ],
    });

    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods} />);

    const checkBtn = await screen.findByRole('button', { name: /Check Compatibility Now/i });
    await waitFor(() => expect(checkBtn).toBeEnabled());
    fireEvent.click(checkBtn);

    await waitFor(() => {
      expect(screen.getByText('Lithium')).toBeInTheDocument();
    });

    // Find and click "Update All Compatible Mods (1)"
    const updateAllBtn = screen.getByRole('button', {
      name: /Update All Compatible Mods \(1\)/i,
    });
    fireEvent.click(updateAllBtn);

    // Confirm dialog should appear
    const confirmBtn = await screen.findByRole('button', { name: /Update 1 Mods/i });
    expect(confirmBtn).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(confirmBtn);
    });

    expect(applySpy).toHaveBeenCalledWith('srv-1', '1.21.1');

    // Simulate real-time progress event from backend
    await act(async () => {
      progressCb?.({
        server_id: 'srv-1',
        operation: 'mod_upgrade_batch',
        current: 1,
        total: 1,
        percent: 100,
        message: 'Updated lithium-fabric-0.12.0',
      });
    });

    // Verify success banner is shown
    await waitFor(() => {
      expect(
        screen.getByText(/Successfully updated all 1 compatible mods for Minecraft 1.21.1!/i),
      ).toBeInTheDocument();
    });

    // Verify silent refresh of compatibility report was triggered
    expect(api.checkVersionUpgradeCompatibility).toHaveBeenCalledTimes(2);
  });

  it('upgrades the server and mods sequentially (backup -> update mods -> server jar upgrade)', async () => {
    const readyReport: VersionUpdateReport = {
      ...mockReport,
      ready_to_update: true,
      not_available_count: 0,
      update_count: 1,
    };
    vi.spyOn(api, 'checkVersionUpgradeCompatibility').mockResolvedValue(readyReport);

    const backupSpy = vi.spyOn(api, 'createBackup').mockResolvedValue({
      id: 'bk-full',
      server_id: 'srv-1',
      name: 'Pre-Upgrade-1.21.1',
      size_bytes: 5000,
      location: 'local',
      status: 'completed',
      created_at: new Date().toISOString(),
    });

    const applySpy = vi.spyOn(api, 'applyVersionUpgradeMods').mockResolvedValue({
      updated: 1,
      failed: 0,
      target_version: '1.21.1',
      total: 1,
      items: [
        {
          mod_name: 'lithium-fabric-0.12.0',
          success: true,
          original: 'lithium-fabric-0.12.0.jar',
          new_jar: 'lithium-fabric-0.14.0.jar',
        },
      ],
    });

    const buildsSpy = vi.spyOn(api, 'jarBuilds').mockResolvedValue([
      { version: '1.21.1', build: '0.17.0-beta.1', display: '0.17.0-beta.1' },
      { version: '1.21.1', build: '0.16.0', display: '0.16.0' },
    ]);

    const installSpy = vi.spyOn(api, 'installServer').mockResolvedValue({
      server_id: mockServer.id,
      installed: true,
      version: '1.21.1',
      build: '0.16.0',
    });

    const refreshSpy = vi.fn();

    render(
      <VersionUpdateHelper
        server={mockServer}
        installedMods={mockMods}
        onRefreshInstalled={refreshSpy}
      />,
    );

    const checkBtn = await screen.findByRole('button', { name: /Check Compatibility Now/i });
    await waitFor(() => expect(checkBtn).toBeEnabled());
    fireEvent.click(checkBtn);

    // Wait for the ready report and upgrade button
    const oneClickBtn = await screen.findByRole('button', {
      name: /Upgrade server & mods to MC 1.21.1/i,
    });
    expect(oneClickBtn).toBeInTheDocument();

    fireEvent.click(oneClickBtn);

    // Modal confirmation dialog
    const confirmBtn = await screen.findByRole('button', {
      name: /^Upgrade server & mods$/i,
    });
    expect(confirmBtn).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(confirmBtn);
    });

    // Verify all steps executed in order
    expect(backupSpy).toHaveBeenCalledWith('srv-1', expect.stringContaining('Pre-Upgrade-1.21.1'));
    expect(applySpy).toHaveBeenCalledWith('srv-1', '1.21.1');
    expect(buildsSpy).toHaveBeenCalledWith('fabric', '1.21.1');
    expect(installSpy).toHaveBeenCalledWith('srv-1', { version: '1.21.1', build: '0.16.0' });
    expect(refreshSpy).toHaveBeenCalled();

    // Verify success banner
    await waitFor(() => {
      expect(
        screen.getByText(/Server and mods successfully upgraded to fabric 1.21.1/i),
      ).toBeInTheDocument();
    });
  });
  it('invalidates all readiness actions when the selected target or installed artifacts change',async()=>{
    const user=userEvent.setup();vi.mocked(api.checkVersionUpgradeCompatibility).mockResolvedValue({...mockReport,ready_to_update:true,not_available_count:0});
    const view=render(<VersionUpdateHelper server={mockServer} installedMods={mockMods}/>);
    const scan=screen.getByRole('button',{name:/Check Compatibility Now/i});await waitFor(()=>expect(scan).toBeEnabled());await user.click(scan);
    await screen.findByRole('button',{name:/Upgrade server & mods to MC 1.21.1/i});
    const selector=screen.getByRole('combobox',{name:'Target Minecraft version'});await user.click(selector);await user.click(screen.getByRole('option',{name:/MC 1.20.1/}));
    expect(screen.queryByRole('button',{name:/Upgrade server & mods to MC/i})).not.toBeInTheDocument();
    expect(screen.queryByRole('button',{name:/Update All Compatible Mods/i})).not.toBeInTheDocument();
    await user.click(selector);await user.click(screen.getByRole('option',{name:'MC 1.21.1'}));
    await screen.findByRole('button',{name:/Upgrade server & mods to MC 1.21.1/i});
    view.rerender(<VersionUpdateHelper server={mockServer} installedMods={[...mockMods,{name:'new',file:'new.jar',enabled:true}]}/>);
    expect(screen.queryByRole('button',{name:/Upgrade server & mods to MC/i})).not.toBeInTheDocument();
  });
  it('stops a one-click upgrade before switching software after a mod failure',async()=>{
    vi.mocked(api.checkVersionUpgradeCompatibility).mockResolvedValue({...mockReport,ready_to_update:true,not_available_count:0});
    vi.spyOn(api,'createBackup').mockResolvedValue({id:'recovery',server_id:mockServer.id,name:'recovery',status:'completed',size_bytes:1,location:'local',created_at:new Date().toISOString()});
    vi.spyOn(api,'applyVersionUpgradeMods').mockResolvedValue({updated:0,failed:1,target_version:'1.21.1',total:1,items:[]});
    const install=vi.spyOn(api,'installServer');const builds=vi.spyOn(api,'jarBuilds');
    render(<VersionUpdateHelper server={mockServer} installedMods={mockMods}/>);
    const scan=screen.getByRole('button',{name:/Check Compatibility Now/i});await waitFor(()=>expect(scan).toBeEnabled());fireEvent.click(scan);
    fireEvent.click(await screen.findByRole('button',{name:/Upgrade server & mods to MC 1.21.1/i}));
    fireEvent.click(await screen.findByRole('button',{name:/^Upgrade server & mods$/i}));
    await screen.findByText(/Server software was left at its current version/i);
    expect(install).not.toHaveBeenCalled();expect(builds).not.toHaveBeenCalled();expect(screen.queryByText(/Server and mods successfully upgraded/i)).not.toBeInTheDocument();
  });

});
