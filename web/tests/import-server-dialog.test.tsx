import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '../src/api/client';
import type { Server } from '../src/api/types';
import { ImportServerDialog } from '../src/components/import-server-dialog';

const imported = { id: 'imported', name: 'My server', state: 'stopped' } as Server;

describe('ImportServerDialog', () => {
  beforeEach(() => {
    vi.spyOn(api, 'jarVersions').mockResolvedValue([{ name: '1.21.1' }, { name: '1.20.1' }]);
    vi.spyOn(api, 'jarBuilds').mockImplementation(async (_type, version) => [{ version, build: '123', display: 'Build 123' }]);
  });

  it('uploads an export, reports progress, and waits for extraction before closing', async () => {
    const user = userEvent.setup();
    let finish!: (server: Server) => void;
    const upload = vi.spyOn(api, 'importServer').mockImplementation((_file, _settings, progress) => {
      progress?.(100, 100);
      return new Promise((resolve) => { finish = resolve; });
    });
    const onImported = vi.fn();
    render(<ImportServerDialog onImported={onImported} />);
    await user.click(screen.getByRole('button', { name: 'Import server' }));
    const dialog = screen.getByRole('dialog', { name: 'Import server' });
    expect(within(dialog).getByRole('button', { name: 'Import server' })).toBeDisabled();
    await user.upload(screen.getByLabelText('Server ZIP file'), new File(['zip'], 'first-archive.zip', { type: 'application/zip' }));
    const file = new File(['zip'], 'renamed-archive.zip', { type: 'application/zip' });
    await user.upload(screen.getByLabelText('Server ZIP file'), file);
    expect(screen.getByLabelText('Server name (optional)')).toHaveValue('');
    expect(within(dialog).getByRole('button', { name: 'Import server' })).toBeEnabled();
    await user.click(within(dialog).getByRole('button', { name: 'Import server' }));
    expect(upload).toHaveBeenCalledWith(file, expect.objectContaining({ name: undefined, server_type: undefined, no_host_port: false }), expect.any(Function));
    expect(screen.getByText('Importing server files…')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Change ZIP' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Remove selected ZIP' })).toBeDisabled();
    fireEvent.drop(screen.getByRole('group', { name: 'ZIP upload area' }), { dataTransfer: { files: [new File(['zip'], 'replacement.zip')], types: ['Files'] } });
    expect(screen.getByText('renamed-archive.zip')).toBeInTheDocument();
    expect(screen.queryByText('replacement.zip')).not.toBeInTheDocument();
    await user.keyboard('{Escape}');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(onImported).not.toHaveBeenCalled();
    await act(async () => { finish(imported); });
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(onImported).toHaveBeenCalledWith(imported);
  });

  it('retains the archive after an error and accepts legacy settings on retry', async () => {
    const user = userEvent.setup();
    const upload = vi.spyOn(api, 'importServer').mockRejectedValueOnce(new ApiError(400, 'Choose the server type and enter its version.')).mockResolvedValue(imported);
    render(<ImportServerDialog onImported={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Import server' }));
    let file = new File(['zip'], 'Legacy-export.zip', { type: 'application/zip' });
    await user.upload(screen.getByLabelText('Server ZIP file'), file);
    fireEvent.submit(screen.getByRole('dialog').querySelector('form')!);
    expect(await screen.findByRole('alert')).toHaveTextContent('Choose the server type and enter its version.');
    await user.type(screen.getByLabelText('Server name (optional)'), 'Legacy server');
    file = new File(['zip'], 'replacement-archive.zip', { type: 'application/zip' });
    await user.upload(screen.getByLabelText('Server ZIP file'), file);
    expect(screen.getByLabelText('Server name (optional)')).toHaveValue('Legacy server');
    await user.click(screen.getByText('Server settings', { selector: 'summary' }));
    await user.click(screen.getByRole('combobox', { name: 'Server type' }));
    await user.click(screen.getByRole('option', { name: 'Fabric' }));
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Version' })).toBeEnabled());
    await user.click(screen.getByRole('combobox', { name: 'Version' }));
    await user.click(screen.getByRole('option', { name: '1.20.1' }));
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Build' })).toBeEnabled());
    await user.click(screen.getByRole('combobox', { name: 'Build' }));
    await user.click(screen.getByRole('option', { name: '123' }));
    await user.type(screen.getByLabelText('RAM (MB)'), '4096');
    await user.type(screen.getByLabelText('Java version'), '17');
    await user.click(screen.getByRole('checkbox', { name: /keep server private/i }));
    fireEvent.submit(screen.getByRole('dialog').querySelector('form')!);
    await waitFor(() => expect(upload).toHaveBeenLastCalledWith(file, expect.objectContaining({ name: 'Legacy server', server_type: 'fabric', version: '1.20.1', build: '123', ram_mb: 4096, java_version: 17, no_host_port: true }), expect.any(Function)));
  });

  it('rejects empty, oversized and non-ZIP files before uploading', async () => {
    const user = userEvent.setup();
    const upload = vi.spyOn(api, 'importServer').mockResolvedValue(imported);
    render(<ImportServerDialog onImported={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Import server' }));
    const oversized = new File(['zip'], 'large.zip');
    Object.defineProperty(oversized, 'size', { value: 2 * 1024 ** 3 + 1 });
    for (const file of [new File([], 'empty.zip'), oversized]) {
      fireEvent.change(screen.getByLabelText('Server ZIP file'), { target: { files: [file] } });
      expect(screen.getByRole('alert')).toHaveTextContent('Choose a non-empty ZIP file of at most 2 GiB.');
      expect(within(screen.getByRole('dialog')).getByRole('button', { name: 'Import server' })).toBeDisabled();
    }
    fireEvent.change(screen.getByLabelText('Server ZIP file'), { target: { files: [new File(['bad'], 'world.tar')] } });
    expect(screen.getByRole('alert')).toHaveTextContent('Choose a .zip file exported from MCM.');
    expect(upload).not.toHaveBeenCalled();
  });

  it('accepts a dropped ZIP and keeps drag feedback while crossing child elements', async () => {
    const user = userEvent.setup();
    const upload = vi.spyOn(api, 'importServer').mockResolvedValue(imported);
    render(<ImportServerDialog onImported={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Import server' }));
    const area = screen.getByRole('group', { name: 'ZIP upload area' });
    const file = new File(['zip'], 'Dropped-server.zip', { type: 'application/zip' });
    const dataTransfer = { files: [file], types: ['Files'] };
    fireEvent.dragEnter(area, { dataTransfer });
    fireEvent.dragEnter(screen.getByRole('button', { name: 'Choose ZIP' }), { dataTransfer });
    fireEvent.dragLeave(screen.getByRole('button', { name: 'Choose ZIP' }), { dataTransfer });
    expect(screen.getByText('Drop your ZIP to select it')).toBeInTheDocument();
    fireEvent.drop(area, { dataTransfer });
    expect(screen.getByText(file.name)).toBeInTheDocument();
    expect(screen.getByText('3 B · ZIP archive')).toBeInTheDocument();
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Import server' }));
    await waitFor(() => expect(upload).toHaveBeenCalledWith(file, expect.objectContaining({ name: undefined }), expect.any(Function)));
  });

  it('retains a valid ZIP after invalid replacements and allows removal and reselection', async () => {
    const user = userEvent.setup();
    render(<ImportServerDialog onImported={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Import server' }));
    const file = new File(['zip'], 'Keep-me.zip', { type: 'application/zip' });
    await user.upload(screen.getByLabelText('Server ZIP file'), file);
    const area = screen.getByRole('group', { name: 'ZIP upload area' });
    fireEvent.drop(area, { dataTransfer: { files: [file, file], types: ['Files'] } });
    expect(screen.getByRole('alert')).toHaveTextContent('Choose one server ZIP at a time. Your selected ZIP has been kept.');
    fireEvent.drop(area, { dataTransfer: { files: [new File(['tar'], 'wrong.tar')], types: ['Files'] } });
    expect(screen.getByRole('alert')).toHaveTextContent('Choose a .zip file');
    expect(screen.getByText('Keep-me.zip')).toBeInTheDocument();
    await user.type(screen.getByLabelText('Server name (optional)'), 'Keep this name');
    await user.click(screen.getByRole('button', { name: 'Remove selected ZIP' }));
    expect(screen.getByText('Drop your server ZIP here')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(within(screen.getByRole('dialog')).getByRole('button', { name: 'Import server' })).toBeDisabled();
    await user.upload(screen.getByLabelText('Server ZIP file'), file);
    expect(screen.getByText('Keep-me.zip')).toBeInTheDocument();
    expect(screen.getByLabelText('Server name (optional)')).toHaveValue('Keep this name');
  });

  it('uses the same stable-first version and build ordering as new server creation', async () => {
    const user = userEvent.setup();
    vi.mocked(api.jarVersions).mockResolvedValue([{ name: '25w01a' }, { name: '1.21.2' }, { name: '1.22-pre1' }, { name: '1.21.11' }]);
    vi.mocked(api.jarBuilds).mockImplementation(async (_type, version) => [
      { version, build: '0.17.0-beta.1', display: '' }, { version, build: '0.16.9', display: '' }, { version, build: '0.16.10', display: '' },
    ]);
    render(<ImportServerDialog onImported={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Import server' }));
    await user.click(screen.getByText('Server settings', { selector: 'summary' }));
    expect(screen.getByRole('combobox', { name: 'Version' })).toBeDisabled();
    expect(api.jarVersions).not.toHaveBeenCalled();
    await user.click(screen.getByRole('combobox', { name: 'Server type' }));
    await user.click(screen.getByRole('option', { name: 'Paper' }));
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Version' })).toBeEnabled());
    expect(screen.getByRole('combobox', { name: 'Version' })).toHaveTextContent('Use exported setting');
    await user.click(screen.getByRole('combobox', { name: 'Version' }));
    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual(['Use exported setting', '1.21.11', '1.21.2', '25w01a', '1.22-pre1', 'Enter version manually…']);
    await user.click(screen.getByRole('option', { name: '1.21.11' }));
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Build' })).toBeEnabled());
    expect(api.jarBuilds).toHaveBeenLastCalledWith('paper', '1.21.11');
    await user.click(screen.getByRole('combobox', { name: 'Build' }));
    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual(['Use exported setting', '0.16.10', '0.16.9', '0.17.0-beta.1', 'Enter build manually…']);
    await user.click(screen.getByRole('option', { name: '0.16.9' }));
    await user.click(screen.getByRole('combobox', { name: 'Version' }));
    await user.click(screen.getByRole('option', { name: '1.22-pre1' }));
    await waitFor(() => expect(api.jarBuilds).toHaveBeenLastCalledWith('paper', '1.22-pre1'));
    expect(screen.getByRole('combobox', { name: 'Build' })).toHaveTextContent('Use exported setting');
  });

  it('allows an older export to use manual version and build values when catalogs fail', async () => {
    const user = userEvent.setup();
    vi.mocked(api.jarVersions).mockRejectedValue(new Error('offline'));
    const upload = vi.spyOn(api, 'importServer').mockResolvedValue(imported);
    render(<ImportServerDialog onImported={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Import server' }));
    const file = new File(['zip'], 'older.zip', { type: 'application/zip' });
    await user.upload(screen.getByLabelText('Server ZIP file'), file);
    await user.type(screen.getByLabelText('Server name (optional)'), 'Older server');
    await user.click(screen.getByText('Server settings', { selector: 'summary' }));
    await user.click(screen.getByRole('combobox', { name: 'Server type' }));
    await user.click(screen.getByRole('option', { name: 'Forge' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load versions.');
    await user.click(screen.getByRole('combobox', { name: 'Version' }));
    await user.click(screen.getByRole('option', { name: 'Enter version manually…' }));
    await user.type(screen.getByRole('textbox', { name: 'Version number' }), '1.12.2');
    await user.click(screen.getByRole('combobox', { name: 'Build' }));
    await user.click(screen.getByRole('option', { name: 'Enter build manually…' }));
    await user.type(screen.getByRole('textbox', { name: 'Build number' }), '14.23.5.2860');
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Import server' }));
    await waitFor(() => expect(upload).toHaveBeenCalledWith(file, expect.objectContaining({ version: '1.12.2', build: '14.23.5.2860' }), expect.any(Function)));
    expect(api.jarBuilds).not.toHaveBeenCalled();
  });

  it('ignores stale catalog responses after changing the server type or version', async () => {
    const user = userEvent.setup();
    let resolvePaper!: (versions: Array<{ name: string }>) => void;
    let resolveOldBuilds!: (builds: Array<{ version: string; build: string; display: string }>) => void;
    vi.mocked(api.jarVersions).mockImplementation((type) => type === 'paper'
      ? new Promise((resolve) => { resolvePaper = resolve; })
      : Promise.resolve([{ name: '1.21.1' }, { name: '1.20.1' }]));
    vi.mocked(api.jarBuilds).mockImplementation((_type, version) => version === '1.20.1'
      ? new Promise((resolve) => { resolveOldBuilds = resolve; })
      : Promise.resolve([{ version, build: 'new-build', display: '' }]));
    render(<ImportServerDialog onImported={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Import server' }));
    await user.click(screen.getByText('Server settings', { selector: 'summary' }));
    await user.click(screen.getByRole('combobox', { name: 'Server type' }));
    await user.click(screen.getByRole('option', { name: 'Paper' }));
    expect(screen.getByRole('combobox', { name: 'Version' })).toBeDisabled();
    await user.click(screen.getByRole('combobox', { name: 'Server type' }));
    await user.click(screen.getByRole('option', { name: 'Fabric' }));
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Version' })).toBeEnabled());
    await act(async () => { resolvePaper([{ name: 'stale-version' }]); });
    await user.click(screen.getByRole('combobox', { name: 'Version' }));
    expect(screen.queryByRole('option', { name: 'stale-version' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('option', { name: '1.20.1' }));
    expect(screen.getByRole('combobox', { name: 'Build' })).toBeDisabled();
    await user.click(screen.getByRole('combobox', { name: 'Version' }));
    await user.click(screen.getByRole('option', { name: '1.21.1' }));
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Build' })).toBeEnabled());
    await act(async () => { resolveOldBuilds([{ version: '1.20.1', build: 'stale-build', display: '' }]); });
    await user.click(screen.getByRole('combobox', { name: 'Build' }));
    expect(screen.getByRole('option', { name: 'new-build' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: 'stale-build' })).not.toBeInTheDocument();
  });

  it('recovers from version and build catalog errors using the retry actions', async () => {
    const user = userEvent.setup();
    vi.mocked(api.jarVersions).mockRejectedValueOnce(new Error('offline')).mockResolvedValue([{ name: '1.21.1' }]);
    vi.mocked(api.jarBuilds).mockRejectedValueOnce(new Error('offline')).mockResolvedValue([{ version: '1.21.1', build: '456', display: '' }]);
    render(<ImportServerDialog onImported={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Import server' }));
    await user.click(screen.getByText('Server settings', { selector: 'summary' }));
    await user.click(screen.getByRole('combobox', { name: 'Server type' }));
    await user.click(screen.getByRole('option', { name: 'Paper' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load versions.');
    await user.click(screen.getByRole('button', { name: 'retry' }));
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Version' })).toBeEnabled());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    await user.click(screen.getByRole('combobox', { name: 'Version' }));
    await user.click(screen.getByRole('option', { name: '1.21.1' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load builds.');
    await user.click(screen.getByRole('button', { name: 'retry' }));
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Build' })).toBeEnabled());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    await user.click(screen.getByRole('combobox', { name: 'Build' }));
    expect(screen.getByRole('option', { name: '456' })).toBeInTheDocument();
  });
});
