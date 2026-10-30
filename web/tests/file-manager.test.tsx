import { adminUser, renderWithAdmin as render, renderWithUser } from '../src/test/auth';
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

describe('FileManager context menus', () => {
  it('opens only the targeted file menu and edits the file', async () => {
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    const file = await screen.findByRole('button', { name: 'config.yml' });
    await user.pointer({ target: file, keys: '[MouseRight]' });
    const menu = await screen.findByRole('menu', { name: 'Actions for config.yml' });
    expect(screen.getAllByRole('menu')).toHaveLength(1);
    expect(api.readFileContent).not.toHaveBeenCalled();
    await user.click(within(menu).getByRole('menuitem', { name: 'Edit' }));
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'File content' })).toHaveValue('pvp: true\nplayers: 20'));
    expect(api.readFileContent).toHaveBeenCalledWith(server.id, 'config.yml');
  });

  it('opens with Shift+F10 and restores focus when dismissed with Escape', async () => {
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    const file = await screen.findByRole('button', { name: 'config.yml' });
    file.focus();
    await user.keyboard('{Shift>}{F10}{/Shift}');
    expect(await screen.findByRole('menu', { name: 'Actions for config.yml' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument());
    expect(file).toHaveFocus();
  });

  it('opens only the file menu after a touch long press', async () => {
    render(<FileManager server={server} />);
    const file = await screen.findByRole('button', { name: 'config.yml' });
    fireEvent.pointerDown(file, { pointerType: 'touch', pointerId: 1 });
    expect(await screen.findByRole('menu', { name: 'Actions for config.yml' })).toBeInTheDocument();
    expect(screen.getAllByRole('menu')).toHaveLength(1);
    fireEvent.pointerUp(file, { pointerType: 'touch', pointerId: 1 });
  });

  it('keeps deletion behind its existing confirmation', async () => {
    const remove = vi.spyOn(api, 'deleteFile').mockResolvedValue({ ok: true });
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    await user.pointer({ target: await screen.findByRole('button', { name: 'config.yml' }), keys: '[MouseRight]' });
    await user.click(screen.getByRole('menuitem', { name: 'Delete' }));
    const dialog = await screen.findByRole('dialog', { name: 'Delete file' });
    expect(remove).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(remove).not.toHaveBeenCalled();
  });

  it('disables write and delete operations for read-only users', async () => {
    const user = userEvent.setup();
    renderWithUser(<FileManager server={server} />, { ...adminUser, role: 'viewer', permissions: ['files.read'] });
    await user.pointer({ target: await screen.findByRole('button', { name: 'config.yml' }), keys: '[MouseRight]' });
    for (const name of ['Archive as zip', 'Rename', 'Delete']) {
      expect(screen.getByRole('menuitem', { name })).toHaveAttribute('aria-disabled', 'true');
    }
    expect(screen.getByRole('menuitem', { name: 'Download' })).not.toHaveAttribute('aria-disabled');
  });

  it('provides creation and refresh actions in the folder background', async () => {
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    await screen.findByRole('button', { name: 'config.yml' });
    await user.pointer({ target: screen.getByLabelText('File browser'), keys: '[MouseRight]' });
    const menu = await screen.findByRole('menu', { name: 'Folder actions' });
    expect(within(menu).getByRole('menuitem', { name: 'New file' })).toBeInTheDocument();
    expect(within(menu).getByRole('menuitem', { name: 'Upload files' })).toBeInTheDocument();
    await user.click(within(menu).getByRole('menuitem', { name: 'New folder' }));
    expect(await screen.findByRole('dialog', { name: 'New folder' })).toBeInTheDocument();
  });

  it('opens folders and uses the current folder when renaming a file', async () => {
    vi.mocked(api.listFiles).mockImplementation(async (_id, path) => ({ path: path ?? '', entries: path ? [entry] : [{ ...entry, name: 'plugins', is_directory: true }] }));
    const rename = vi.spyOn(api, 'renameFile').mockResolvedValue({ ...entry, name: 'settings.yml' });
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    await user.pointer({ target: await screen.findByRole('button', { name: 'plugins' }), keys: '[MouseRight]' });
    await user.click(screen.getByRole('menuitem', { name: 'Open folder' }));
    await user.pointer({ target: await screen.findByRole('button', { name: 'config.yml' }), keys: '[MouseRight]' });
    await user.click(screen.getByRole('menuitem', { name: 'Rename' }));
    const dialog = await screen.findByRole('dialog', { name: 'Rename item' });
    const name = within(dialog).getByRole('textbox');
    await user.clear(name);
    await user.type(name, 'settings.yml{Enter}');
    await waitFor(() => expect(rename).toHaveBeenCalledWith(server.id, 'plugins/config.yml', 'settings.yml'));
  });
});

const selectionEntries = ['config.yml', 'server.properties', 'level.dat', 'plugins'].map((name) => ({
  ...entry, name, is_directory: name === 'plugins',
}));

function expectSelected(...names: string[]) {
  for (const checkbox of screen.getAllByRole('checkbox', { name: /^Select / })) {
    const label = checkbox.getAttribute('aria-label');
    if (!label) continue;
    const name = label.slice('Select '.length);
    if (names.includes(name)) expect(checkbox).toBeChecked();
    else expect(checkbox).not.toBeChecked();
  }
}

describe('FileManager multi-selection', () => {
  beforeEach(() => {
    vi.mocked(api.listFiles).mockResolvedValue({ path: '', entries: selectionEntries });
  });

  it.each(['ctrlKey', 'metaKey'] as const)('toggles files and folders with %s without opening them', async (modifier) => {
    const download = vi.spyOn(api, 'downloadFile');
    render(<FileManager server={server} />);
    const first = await screen.findByRole('button', { name: 'config.yml' });
    fireEvent.click(first, { [modifier]: true });
    fireEvent.click(screen.getByRole('button', { name: 'plugins' }), { [modifier]: true });
    expectSelected('config.yml', 'plugins');
    fireEvent.click(first, { [modifier]: true });
    expectSelected('plugins');
    expect(api.readFileContent).not.toHaveBeenCalled();
    expect(download).not.toHaveBeenCalled();
    expect(api.listFiles).toHaveBeenCalledTimes(1);
  });

  it('extends and shrinks ranges in both directions from the original anchor', async () => {
    render(<FileManager server={server} />);
    const first = await screen.findByRole('button', { name: 'config.yml' });
    const third = screen.getByRole('button', { name: 'level.dat' });
    fireEvent.click(first, { ctrlKey: true });
    fireEvent.click(third, { shiftKey: true });
    expectSelected('config.yml', 'server.properties', 'level.dat');
    fireEvent.click(screen.getByRole('button', { name: 'server.properties' }), { shiftKey: true });
    expectSelected('config.yml', 'server.properties');
    fireEvent.click(screen.getByLabelText('level.dat', { selector: 'div' }));
    fireEvent.click(first, { shiftKey: true });
    expectSelected('config.yml', 'server.properties', 'level.dat');
    expect(api.readFileContent).not.toHaveBeenCalled();
  });

  it('adds a range to an existing selection with Ctrl+Shift and supports row clicks', async () => {
    render(<FileManager server={server} />);
    await screen.findByRole('button', { name: 'config.yml' });
    fireEvent.click(screen.getByLabelText('config.yml', { selector: 'div' }), { ctrlKey: true });
    fireEvent.click(screen.getByLabelText('plugins', { selector: 'div' }), { ctrlKey: true });
    fireEvent.click(screen.getByRole('button', { name: 'level.dat' }), { ctrlKey: true, shiftKey: true });
    expectSelected('config.yml', 'level.dat', 'plugins');
    expect(screen.getByRole('checkbox', { name: 'Select all (4)' })).toBePartiallyChecked();
    fireEvent.click(screen.getByLabelText('server.properties', { selector: 'div' }));
    expectSelected('server.properties');
    expect(api.readFileContent).not.toHaveBeenCalled();
  });

  it('selects a range when Shift-clicking checkboxes', async () => {
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    await user.click(await screen.findByRole('checkbox', { name: 'Select config.yml' }));
    await user.keyboard('{Shift>}');
    await user.click(screen.getByRole('checkbox', { name: 'Select plugins' }));
    await user.keyboard('{/Shift}');
    expectSelected(...selectionEntries.map((entry) => entry.name));
    expect(api.readFileContent).not.toHaveBeenCalled();
  });

  it('preserves a selection on right click and replaces it when another file is targeted', async () => {
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    const first = await screen.findByRole('button', { name: 'config.yml' });
    fireEvent.click(first, { ctrlKey: true });
    fireEvent.click(screen.getByRole('button', { name: 'server.properties' }), { ctrlKey: true });
    await user.pointer({ target: first, keys: '[MouseRight]' });
    const menu = await screen.findByRole('menu', { name: 'Actions for 2 selected items' });
    expect(within(menu).getByRole('menuitem', { name: 'Zip selected (2)' })).toBeInTheDocument();
    expect(within(menu).getByRole('menuitem', { name: 'Delete selected (2)' })).toBeInTheDocument();
    expect(within(menu).queryByRole('menuitem', { name: 'Rename' })).not.toBeInTheDocument();
    await user.keyboard('{Escape}');
    expectSelected('config.yml', 'server.properties');
    await user.pointer({ target: screen.getByRole('button', { name: 'level.dat' }), keys: '[MouseRight]' });
    expect(await screen.findByRole('menu', { name: 'Actions for level.dat' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    expectSelected('level.dat');
  });

  it('offers group actions from the keyboard and the folder background', async () => {
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    const first = await screen.findByRole('button', { name: 'config.yml' });
    await user.click(screen.getByRole('checkbox', { name: 'Select all (4)' }));
    first.focus();
    await user.keyboard('{Shift>}{F10}{/Shift}');
    expect(await screen.findByRole('menu', { name: 'Actions for 4 selected items' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await user.pointer({ target: screen.getByLabelText('File browser'), keys: '[MouseRight]' });
    const menu = await screen.findByRole('menu', { name: 'Folder actions' });
    expect(within(menu).getByRole('menuitem', { name: 'Delete selected (4)' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    expectSelected(...selectionEntries.map((entry) => entry.name));
  });

  it('prunes missing selections on refresh and clears the anchor when changing folders', async () => {
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    fireEvent.click(await screen.findByRole('button', { name: 'config.yml' }), { ctrlKey: true });
    fireEvent.click(screen.getByRole('button', { name: 'server.properties' }), { ctrlKey: true });
    vi.mocked(api.listFiles).mockResolvedValue({ path: '', entries: selectionEntries.slice(1) });
    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'config.yml' })).not.toBeInTheDocument());
    expectSelected('server.properties');
    vi.mocked(api.listFiles).mockResolvedValue({ path: 'plugins', entries: selectionEntries.slice(0, 3) });
    await user.click(screen.getByRole('button', { name: 'plugins' }));
    await waitFor(() => expect(api.listFiles).toHaveBeenLastCalledWith(server.id, 'plugins'));
    expectSelected();
    fireEvent.click(await screen.findByRole('button', { name: 'level.dat' }), { shiftKey: true });
    expectSelected('level.dat');
  });
});

describe('FileManager batch deletion', () => {
  beforeEach(() => {
    vi.mocked(api.listFiles).mockResolvedValue({ path: '', entries: selectionEntries });
  });

  it('confirms the selected files once and keeps the selection when canceled', async () => {
    const remove = vi.spyOn(api, 'deleteFile').mockResolvedValue({ ok: true });
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    const first = await screen.findByRole('button', { name: 'config.yml' });
    fireEvent.click(first, { ctrlKey: true });
    fireEvent.click(screen.getByRole('button', { name: 'server.properties' }), { ctrlKey: true });
    await user.pointer({ target: first, keys: '[MouseRight]' });
    await user.click(screen.getByRole('menuitem', { name: 'Delete selected (2)' }));
    const dialog = await screen.findByRole('dialog', { name: 'Delete selected items' });
    expect(dialog).toHaveTextContent('config.yml');
    expect(dialog).toHaveTextContent('server.properties');
    expect(remove).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expectSelected('config.yml', 'server.properties');
    expect(remove).not.toHaveBeenCalled();
  });

  it('deletes selected files in the current directory and refreshes once', async () => {
    const remove = vi.spyOn(api, 'deleteFile').mockResolvedValue({ ok: true });
    vi.mocked(api.listFiles).mockImplementation(async (_id, path) => ({ path: path ?? '', entries: path ? selectionEntries.slice(0, 2) : [selectionEntries[3]] }));
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    await user.click(await screen.findByRole('button', { name: 'plugins' }));
    await screen.findByRole('button', { name: 'config.yml' });
    await user.click(screen.getByRole('checkbox', { name: 'Select all (2)' }));
    await user.click(screen.getByRole('button', { name: 'Delete selected (2)' }));
    const dialog = await screen.findByRole('dialog', { name: 'Delete selected items' });
    vi.mocked(api.listFiles).mockResolvedValue({ path: 'plugins', entries: [] });
    const loads = vi.mocked(api.listFiles).mock.calls.length;
    await user.click(within(dialog).getByRole('button', { name: 'Delete 2 items' }));
    await screen.findByText('Empty folder');
    expect(remove.mock.calls).toEqual([[server.id, 'plugins/config.yml'], [server.id, 'plugins/server.properties']]);
    expect(api.listFiles).toHaveBeenCalledTimes(loads + 1);
    expect(screen.getByRole('button', { name: 'Delete selected (0)' })).toBeDisabled();
  });

  it('continues after failures and keeps only failed items selected for retry', async () => {
    const remove = vi.spyOn(api, 'deleteFile').mockImplementation(async (_id, path) => {
      if (path === 'server.properties') throw new ApiError(403, 'Permission denied');
      return { ok: true };
    });
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    await screen.findByRole('button', { name: 'config.yml' });
    await user.click(screen.getByRole('checkbox', { name: 'Select all (4)' }));
    await user.click(screen.getByRole('button', { name: 'Delete selected (4)' }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Delete 4 items' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not delete 1 of 4 items. server.properties: Permission denied');
    expect(remove).toHaveBeenCalledTimes(4);
    expect(remove).toHaveBeenCalledWith(server.id, 'plugins');
    expectSelected('server.properties');
    expect(screen.getByRole('button', { name: 'Delete selected (1)' })).toBeEnabled();
  });

  it('keeps the directory and selection stable while a batch is being deleted', async () => {
    let finishDelete!: (result: { ok: boolean }) => void;
    vi.spyOn(api, 'deleteFile').mockImplementationOnce(() => new Promise((resolve) => { finishDelete = resolve; })).mockResolvedValue({ ok: true });
    const user = userEvent.setup();
    render(<FileManager server={server} />);
    await screen.findByRole('button', { name: 'config.yml' });
    await user.click(screen.getByRole('checkbox', { name: 'Select all (4)' }));
    await user.click(screen.getByRole('button', { name: 'Delete selected (4)' }));
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Delete 4 items' }));
    for (const name of ['plugins', 'config.yml', 'server root', 'Refresh', 'Delete selected (4)']) {
      expect(screen.getByRole('button', { name })).toBeDisabled();
    }
    expect(screen.getByRole('checkbox', { name: 'Select all (4)' })).toBeDisabled();
    finishDelete({ ok: true });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled());
  });

  it.each([false, true])('preserves an open editor unless its deletion succeeds (success: %s)', async (succeeds) => {
    const remove = vi.spyOn(api, 'deleteFile').mockImplementation(async () => {
      if (!succeeds) throw new ApiError(403, 'Permission denied');
      return { ok: true };
    });
    const { user, editor } = await openAndEdit();
    fireEvent.click(screen.getByRole('button', { name: 'server.properties' }), { ctrlKey: true });
    await user.click(screen.getByRole('button', { name: 'Delete selected (2)' }));
    const dialog = await screen.findByRole('dialog', { name: 'Delete selected items' });
    expect(dialog).toHaveTextContent('Unsaved changes in "config.yml" will also be discarded.');
    await user.click(within(dialog).getByRole('button', { name: 'Delete 2 items' }));
    await waitFor(() => expect(remove).toHaveBeenCalledTimes(2));
    if (succeeds) {
      await waitFor(() => expect(screen.queryByRole('textbox', { name: 'File content' })).not.toBeInTheDocument());
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    } else {
      expect(await screen.findByRole('alert')).toHaveTextContent('Could not delete 2 of 2 items');
      expect(editor).toHaveValue('pvp: false\nplayers: 20');
      expectSelected('config.yml', 'server.properties');
    }
  });

  it('uses write permission for ZIP and delete permission for batch removal', async () => {
    const user = userEvent.setup();
    renderWithUser(<FileManager server={server} />, { ...adminUser, role: 'custom', permissions: ['servers.view', 'files.read', 'files.write'] });
    const first = await screen.findByRole('button', { name: 'config.yml' });
    await user.click(screen.getByRole('checkbox', { name: 'Select all (4)' }));
    expect(screen.getByRole('button', { name: 'Zip (4)' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Delete selected (4)' })).toBeDisabled();
    await user.pointer({ target: first, keys: '[MouseRight]' });
    expect(screen.getByRole('menuitem', { name: 'Zip selected (4)' })).not.toHaveAttribute('aria-disabled');
    expect(screen.getByRole('menuitem', { name: 'Delete selected (4)' })).toHaveAttribute('aria-disabled', 'true');
  });
});

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
