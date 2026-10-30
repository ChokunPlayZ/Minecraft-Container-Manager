import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '../src/api/client';
import type { Server } from '../src/api/types';
import { ImportServerDialog } from '../src/components/import-server-dialog';

const imported = { id: 'imported', name: 'My server', state: 'stopped' } as Server;

describe('ImportServerDialog', () => {
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
    // jsdom does not update required-file validity after userEvent.upload.
    expect(within(dialog).getByRole('button', { name: 'Import server' })).toBeEnabled();
    fireEvent.submit(dialog.querySelector('form')!);
    expect(upload).toHaveBeenCalledWith(file, expect.objectContaining({ name: undefined, server_type: undefined, no_host_port: false }), expect.any(Function));
    expect(screen.getByText('Importing server files…')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
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
    await user.type(screen.getByLabelText('Version'), '1.20.1');
    await user.type(screen.getByLabelText('RAM (MB)'), '4096');
    await user.type(screen.getByLabelText('Java version'), '17');
    await user.click(screen.getByRole('checkbox', { name: /keep server private/i }));
    fireEvent.submit(screen.getByRole('dialog').querySelector('form')!);
    await waitFor(() => expect(upload).toHaveBeenLastCalledWith(file, expect.objectContaining({ name: 'Legacy server', server_type: 'fabric', version: '1.20.1', ram_mb: 4096, java_version: 17, no_host_port: true }), expect.any(Function)));
  });

  it('rejects empty, oversized and non-ZIP files before uploading', async () => {
    const user = userEvent.setup();
    const upload = vi.spyOn(api, 'importServer').mockResolvedValue(imported);
    render(<ImportServerDialog onImported={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Import server' }));
    const oversized = new File(['zip'], 'large.zip');
    Object.defineProperty(oversized, 'size', { value: 2 * 1024 ** 3 + 1 });
    for (const file of [new File([], 'empty.zip'), oversized, new File(['bad'], 'world.tar')]) {
      fireEvent.change(screen.getByLabelText('Server ZIP file'), { target: { files: [file] } });
      expect(screen.getByRole('alert')).toHaveTextContent('Choose a non-empty ZIP file of at most 2 GiB.');
      expect(within(screen.getByRole('dialog')).getByRole('button', { name: 'Import server' })).toBeDisabled();
    }
    expect(upload).not.toHaveBeenCalled();
  });
});
