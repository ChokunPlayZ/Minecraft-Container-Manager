import { renderWithAdmin as render } from '../src/test/auth';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, FileConflictError } from '../src/api/client';
import type { FileContent, Server } from '../src/api/types';
import { FileManager } from '../src/components/file-manager';
import { FileConflictDialog } from '../src/components/file-conflict-dialog';

const server = { id: 'server-1' } as Server;
const entry = { name: 'config.yml', is_directory: false, size_bytes: 10, modified_at: '2026-10-27T00:00:00Z' };
const snapshot = (content: string, revision = 'opened', exists = true): FileContent => ({ path: 'config.yml', content, revision, exists });
const conflict = (content: string, revision = 'external', exists = true) => new FileConflictError('File changed', snapshot(content, revision, exists));

beforeEach(() => {
  vi.spyOn(api, 'listFiles').mockResolvedValue({ path: '', entries: [entry] });
  vi.spyOn(api, 'subscribeTaskEvents').mockReturnValue(() => {});
  vi.spyOn(api, 'readFileContent').mockResolvedValue(snapshot('pvp: true\nplayers: 20'));
});
afterEach(() => vi.restoreAllMocks());

async function openAndEdit() {
  const user = userEvent.setup();
  render(<FileManager server={server} />);
  await user.click(await screen.findByRole('button', { name: 'config.yml' }));
  const editor = await screen.findByRole('textbox', { name: 'File content' });
  await waitFor(() => expect(editor).toHaveValue('pvp: true\nplayers: 20'));
  fireEvent.change(editor, { target: { value: 'pvp: false\nplayers: 20' } });
  return { user, editor };
}

describe('FileManager save conflicts', () => {
  it('saves with the opened revision, then uses the saved revision on the next save', async () => {
    const write = vi.spyOn(api, 'writeFileContent').mockResolvedValue({ ...entry, revision: 'saved' });
    const { user } = await openAndEdit();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(screen.queryByText('unsaved')).not.toBeInTheDocument());
    expect(write).toHaveBeenCalledWith(server.id, 'config.yml', 'pvp: false\nplayers: 20', 'opened');
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(write).toHaveBeenLastCalledWith(server.id, 'config.yml', 'pvp: false\nplayers: 20', 'saved'));
  });

  it('compares both versions and keeps edits when dismissed with Escape', async () => {
    const write = vi.spyOn(api, 'writeFileContent').mockRejectedValue(conflict('pvp: true\nplayers: 50'));
    const { user, editor } = await openAndEdit();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    const dialog = await screen.findByRole('dialog', { name: 'File changed on server' });
    expect(within(dialog).getByText('pvp: false')).toBeInTheDocument();
    expect(within(dialog).getByText('players: 50')).toBeInTheDocument();
    expect(write).toHaveBeenCalledTimes(1);
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(editor).toHaveValue('pvp: false\nplayers: 20');
    expect(screen.getByText('unsaved')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save' })).toHaveFocus();
  });

  it('loads the reviewed server version without saving over it', async () => {
    const write = vi.spyOn(api, 'writeFileContent').mockRejectedValueOnce(conflict('server edits')).mockResolvedValue({ ...entry, revision: 'next' });
    const { user, editor } = await openAndEdit();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await user.click(await screen.findByRole('button', { name: 'Use server version' }));
    expect(editor).toHaveValue('server edits');
    expect(screen.queryByText('unsaved')).not.toBeInTheDocument();
    expect(write).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(write).toHaveBeenLastCalledWith(server.id, 'config.yml', 'server edits', 'external'));
  });

  it('checks the reviewed revision on overwrite and shows a newer conflict again', async () => {
    const write = vi.spyOn(api, 'writeFileContent')
      .mockRejectedValueOnce(conflict('first external version', 'external-1'))
      .mockRejectedValueOnce(conflict('second external version', 'external-2'))
      .mockResolvedValue({ ...entry, revision: 'saved' });
    const { user, editor } = await openAndEdit();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await user.click(await screen.findByRole('button', { name: 'Overwrite server file' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('The file changed again.');
    expect(screen.getByText('second external version')).toBeInTheDocument();
    expect(write).toHaveBeenLastCalledWith(server.id, 'config.yml', 'pvp: false\nplayers: 20', 'external-1');
    await user.click(screen.getByRole('button', { name: 'Overwrite server file' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(write).toHaveBeenLastCalledWith(server.id, 'config.yml', 'pvp: false\nplayers: 20', 'external-2');
    expect(editor).toHaveValue('pvp: false\nplayers: 20');
    expect(screen.queryByText('unsaved')).not.toBeInTheDocument();
  });

  it('preserves the comparison and local edits when an overwrite fails', async () => {
    vi.spyOn(api, 'writeFileContent').mockRejectedValueOnce(conflict('external')).mockRejectedValue(new ApiError(403, 'Permission denied'));
    const { user, editor } = await openAndEdit();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await user.click(await screen.findByRole('button', { name: 'Overwrite server file' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Permission denied');
    await user.click(screen.getByRole('button', { name: 'Keep editing' }));
    expect(editor).toHaveValue('pvp: false\nplayers: 20');
    expect(screen.getByText('unsaved')).toBeInTheDocument();
  });

  it('requires review before recreating a deleted file', async () => {
    const write = vi.spyOn(api, 'writeFileContent').mockRejectedValueOnce(conflict('', '', false)).mockResolvedValue({ ...entry, revision: 'restored' });
    const { user } = await openAndEdit();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText(/This file was deleted/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Use server version' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Recreate file' }));
    await waitFor(() => expect(write).toHaveBeenLastCalledWith(server.id, 'config.yml', 'pvp: false\nplayers: 20', ''));
  });

  it('checks that a new file does not already exist', async () => {
    const write = vi.spyOn(api, 'writeFileContent').mockRejectedValue(conflict('existing file'));
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    await user.click(screen.getByRole('button', { name: 'New file' }));
    const name = within(screen.getByRole('dialog')).getByRole('textbox');
    await user.clear(name);
    await user.type(name, 'config.yml{Enter}');
    fireEvent.change(screen.getByRole('textbox', { name: 'File content' }), { target: { value: 'new content' } });
    await user.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByRole('dialog', { name: 'File changed on server' })).toBeInTheDocument();
    expect(write).toHaveBeenCalledWith(server.id, 'config.yml', 'new content', '');
  });

  it('does not allow saving when opening the file failed', async () => {
    vi.mocked(api.readFileContent).mockRejectedValue(new ApiError(403, 'Cannot read file'));
    render(<FileManager server={server} />);
    fireEvent.click(await screen.findByRole('button', { name: 'config.yml' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Cannot read file');
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
    expect(screen.getByRole('textbox', { name: 'File content' })).toBeDisabled();
  });

  it('keeps unsaved edits when the same file is selected again and discard is canceled', async () => {
    const { user, editor } = await openAndEdit();
    await user.click(screen.getByRole('button', { name: 'config.yml' }));
    const dialog = await screen.findByRole('dialog', { name: 'Discard changes' });
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(editor).toHaveValue('pvp: false\nplayers: 20');
    expect(api.readFileContent).toHaveBeenCalledTimes(1);
  });

  it('pages large comparisons so every line can be reviewed', async () => {
    const lines = Array.from({ length: 300 }, (_, i) => `Setting ${i + 1}`).join('\n');
    const user = userEvent.setup();
    render(<FileConflictDialog current={snapshot(lines)} content="local edits" busy={false} error={null} onCancel={vi.fn()} onReload={vi.fn()} onOverwrite={vi.fn()} />);
    expect(screen.getByText('Setting 200')).toBeInTheDocument();
    expect(screen.queryByText('Setting 201')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Next comparison page' }));
    expect(screen.getByText('Comparison page 2 of 2')).toBeInTheDocument();
    expect(screen.getByText('Setting 300')).toBeInTheDocument();
    expect(screen.queryByText('Setting 200')).not.toBeInTheDocument();
  });
});
