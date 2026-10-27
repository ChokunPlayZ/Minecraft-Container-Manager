import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { InstallPanel } from '../src/components/install-panel';

describe('InstallPanel release selection', () => {
  beforeEach(() => {
    vi.spyOn(api, 'installInfo').mockResolvedValue({
      server_id: 'srv-1', installed: false, version: null, build: null,
    });
    vi.spyOn(api, 'jarVersions').mockResolvedValue([
      { name: '26.1-snapshot-1' }, { name: '1.21.1' }, { name: '1.21.2' },
    ]);
    vi.spyOn(api, 'jarBuilds').mockImplementation(async (_type, version) => [
      { version, build: '0.17.0-beta.1', display: '0.17.0-beta.1' },
      { version, build: '0.16.9', display: '0.16.9' },
      { version, build: '0.16.10', display: '0.16.10' },
    ]);
  });

  afterEach(() => vi.restoreAllMocks());

  it('defaults to stable releases and lets the user choose a snapshot and beta build', async () => {
    const user = userEvent.setup();
    render(<InstallPanel serverId="srv-1" serverType="fabric" onInstalled={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: 'Version' })).toHaveTextContent('1.21.2');
      expect(screen.getByRole('combobox', { name: 'Build' })).toHaveTextContent('0.16.10');
    });
    await user.click(screen.getByRole('combobox', { name: 'Version' }));
    await user.click(screen.getByRole('option', { name: '26.1-snapshot-1' }));
    await waitFor(() => expect(api.jarBuilds).toHaveBeenLastCalledWith('fabric', '26.1-snapshot-1'));
    await user.click(await screen.findByRole('combobox', { name: 'Build' }));
    await user.click(screen.getByRole('option', { name: '0.17.0-beta.1' }));
    expect(screen.getByRole('combobox', { name: 'Build' })).toHaveTextContent('0.17.0-beta.1');
  });

  it('preserves an installed beta build, then defaults to stable after changing versions', async () => {
    const user = userEvent.setup();
    vi.mocked(api.installInfo).mockResolvedValue({
      server_id: 'srv-1', installed: true, version: '1.21.1', build: '0.17.0-beta.1',
    });
    render(<InstallPanel serverId="srv-1" serverType="fabric" onInstalled={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: 'Version' })).toHaveTextContent('1.21.1');
      expect(screen.getByRole('combobox', { name: 'Build' })).toHaveTextContent('0.17.0-beta.1');
    });
    await user.click(screen.getByRole('combobox', { name: 'Version' }));
    await user.click(screen.getByRole('option', { name: '1.21.2' }));
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Build' })).toHaveTextContent('0.16.10'));
  });

  it('keeps preview-only software installable', async () => {
    vi.mocked(api.jarVersions).mockResolvedValue([{ name: '3.4.0-SNAPSHOT' }]);
    vi.mocked(api.jarBuilds).mockResolvedValue([
      { version: '3.4.0-SNAPSHOT', build: '120', display: '120' },
      { version: '3.4.0-SNAPSHOT', build: '99', display: '99' },
    ]);
    render(<InstallPanel serverId="srv-1" serverType="velocity" onInstalled={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: 'Version' })).toHaveTextContent('3.4.0-SNAPSHOT');
      expect(screen.getByRole('combobox', { name: 'Build' })).toHaveTextContent('120');
      expect(screen.getByRole('button', { name: /^Install$/ })).toBeEnabled();
    });
  });
});
