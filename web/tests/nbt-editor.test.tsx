import { renderWithAdmin as render } from '../src/test/auth';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, api } from '../src/api/client';
import { nbtApi, type NBTFile } from '../src/api/nbt';
import type { Server } from '../src/api/types';
import { NBTEditor } from '../src/components/nbt-editor';
import { FileManager } from '../src/components/file-manager';
import { PlayersPanel } from '../src/components/players-panel';
import { PlayerDataPanel } from '../src/components/player-data-panel';
import { documentError, isNBTFile } from '../src/lib/nbt';

const path = 'world/playerdata/8667ba71-b85a-4004-af54-457a9734eed7.dat';
const fixture = (): NBTFile => ({
  path,
  revision: 'opened',
  compression: 'gzip',
  editable: true,
  root: {
    type: 10,
    name: '',
    children: [
      { type: 5, name: 'Health', value: '20' },
      { type: 3, name: 'foodLevel', value: '20' },
      { type: 3, name: 'XpLevel', value: '12' },
      { type: 4, name: 'LastDeathTime', value: '9223372036854775807' },
      {
        type: 9,
        name: 'Inventory',
        list_type: 10,
        children: [
          {
            type: 10,
            name: '',
            children: [
              { type: 8, name: 'id', value: 'minecraft:diamond' },
              { type: 1, name: 'Slot', value: '0' },
            ],
          },
        ],
      },
      { type: 12, name: 'Longs', values: ['-9223372036854775808', '9223372036854775807'] },
    ],
  },
});
const server = { id: 'one', state: 'stopped' } as Server;
beforeEach(() => {
  vi.spyOn(nbtApi, 'read').mockResolvedValue(fixture());
});
afterEach(() => vi.restoreAllMocks());
const open = (props = {}) =>
  render(<NBTEditor serverId="one" path={path} state="stopped" onClose={vi.fn()} {...props} />);

describe('NBT editor', () => {
  it('saves quick player edits with original types, exact longs and revision, then exposes backup', async () => {
    const save = vi
      .spyOn(nbtApi, 'save')
      .mockImplementation(async (_id, file) => ({ ...file, revision: 'saved', backup_path: `${path}.bak` }));
    open();
    const health = await screen.findByLabelText('Health');
    fireEvent.change(health, { target: { value: '10' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save NBT' }));
    expect(await screen.findByRole('status')).toHaveTextContent('NBT changes saved.');
    expect(save).toHaveBeenCalledWith(
      'one',
      expect.objectContaining({
        revision: 'opened',
        compression: 'gzip',
        root: expect.objectContaining({
          children: expect.arrayContaining([
            { type: 5, name: 'Health', value: '10' },
            { type: 4, name: 'LastDeathTime', value: '9223372036854775807' },
          ]),
        }),
      }),
    );
    expect(screen.getByRole('link', { name: /Download previous version/ })).toHaveAttribute(
      'href',
      expect.stringContaining('files/download?path='),
    );
    expect(screen.getByRole('button', { name: 'Save NBT' })).toBeDisabled();
    fireEvent.change(health, { target: { value: '12' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save NBT' }));
    expect(save).toHaveBeenLastCalledWith('one', expect.objectContaining({ revision: 'saved' }));
  });

  it('searches nested item data, edits a string and preserves typed inventory structure', async () => {
    const save = vi
      .spyOn(nbtApi, 'save')
      .mockImplementation(async (_id, file) => ({ ...file, revision: 'saved' }));
    open();
    fireEvent.change(await screen.findByLabelText('Search NBT tags'), {
      target: { value: 'minecraft:diamond' },
    });
    await userEvent.click(screen.getByRole('button', { name: /root\/Inventory\/\[0\]\/id/ }));
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'minecraft:netherite_ingot' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save NBT' }));
    await waitFor(() => expect(save).toHaveBeenCalled());
    expect(save.mock.calls[0][1].root.children![4].children![0].children![0]).toEqual({
      type: 8,
      name: 'id',
      value: 'minecraft:netherite_ingot',
    });
  });

  it('blocks integer overflow and edits signed long arrays without numeric coercion', async () => {
    const save = vi
      .spyOn(nbtApi, 'save')
      .mockImplementation(async (_id, file) => ({ ...file, revision: 'saved' }));
    open();
    await userEvent.click(await screen.findByRole('button', { name: 'Longs — Long array' }));
    fireEvent.change(screen.getByLabelText('Array values'), { target: { value: '9223372036854775808' } });
    expect(screen.getByRole('alert')).toHaveTextContent('signed 64-bit integer');
    expect(screen.getByRole('button', { name: 'Save NBT' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Array values'), {
      target: { value: '-9223372036854775808, 42' },
    });
    await userEvent.click(screen.getByRole('button', { name: 'Save NBT' }));
    await waitFor(() => expect(save).toHaveBeenCalled());
    expect(save.mock.calls[0][1].root.children![5].values).toEqual(['-9223372036854775808', '42']);
  });

  it('adds homogeneous list compounds and removes a tag before saving', async () => {
    const save = vi
      .spyOn(nbtApi, 'save')
      .mockImplementation(async (_id, file) => ({ ...file, revision: 'saved' }));
    open();
    await userEvent.click(await screen.findByRole('button', { name: 'Inventory — List' }));
    await userEvent.click(screen.getByRole('button', { name: 'Add item' }));
    await userEvent.click(screen.getByRole('button', { name: 'Save NBT' }));
    await waitFor(() => expect(save).toHaveBeenCalled());
    expect(save.mock.calls[0][1].root.children![4]).toMatchObject({
      type: 9,
      list_type: 10,
      children: [expect.anything(), { type: 10, name: '', children: [] }],
    });
    await userEvent.click(screen.getByRole('button', { name: 'LastDeathTime — Long' }));
    await userEvent.click(screen.getByRole('button', { name: 'Remove tag' }));
    expect(screen.queryByRole('button', { name: 'LastDeathTime — Long' })).not.toBeInTheDocument();
  });

  it('keeps edits after a conflict and requires confirmation before reloading or leaving', async () => {
    const close = vi.fn();
    vi.spyOn(nbtApi, 'save').mockRejectedValue(
      new ApiError(409, 'File changed. Reload before saving.', undefined, 'nbt_changed'),
    );
    open({ onClose: close });
    fireEvent.change(await screen.findByLabelText('Health'), { target: { value: '10' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save NBT' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('File changed');
    expect(screen.getByLabelText('Health')).toHaveValue('10');
    expect(screen.getByRole('button', { name: 'Save NBT' })).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'Back to browser' }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(close).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Reload saved file' }));
    await userEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Reload and discard' }),
    );
    await waitFor(() => expect(screen.getByLabelText('Health')).toHaveValue('20'));
  });

  it('blocks a rename that would collide with a sibling tag', async () => {
    const save = vi.spyOn(nbtApi, 'save');
    open();
    await userEvent.click(await screen.findByRole('button', { name: 'Health — Float' }));
    fireEvent.change(screen.getByLabelText('Tag name'), { target: { value: 'foodLevel' } });
    expect(screen.getByRole('alert')).toHaveTextContent('Duplicate tag name: foodLevel');
    expect(screen.getByRole('button', { name: 'Save NBT' })).toBeDisabled();
    expect(save).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('Tag name'), { target: { value: 'Health' } });
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save NBT' })).toBeEnabled();
  });

  it('shows a retryable read error and prevents writes while the server is running', async () => {
    vi.mocked(nbtApi.read)
      .mockRejectedValueOnce(new ApiError(400, 'Invalid NBT file.'))
      .mockResolvedValueOnce(fixture());
    open({ state: 'running' });
    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid NBT file.');
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByLabelText('Health')).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Save NBT' })).toBeDisabled();
  });
});

describe('NBT entry points', () => {
  it('opens .dat files in the NBT editor and offers arbitrary files as NBT', async () => {
    vi.spyOn(api, 'listFiles').mockResolvedValue({
      path: '',
      entries: [
        { name: 'level.dat', is_directory: false, size_bytes: 100, modified_at: '' },
        { name: 'custom.bin', is_directory: false, size_bytes: 100, modified_at: '' },
      ],
    });
    vi.spyOn(api, 'subscribeTaskEvents').mockReturnValue(() => {});
    const textRead = vi.spyOn(api, 'readFileContent');
    render(<FileManager server={server} />);
    await userEvent.click(await screen.findByRole('button', { name: 'level.dat' }));
    expect(await screen.findByRole('region', { name: 'NBT editor' })).toBeInTheDocument();
    expect(nbtApi.read).toHaveBeenCalledWith('one', 'level.dat');
    expect(textRead).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Back to browser' }));
    await userEvent.click(screen.getByRole('button', { name: 'Open custom.bin as NBT' }));
    await waitFor(() => expect(nbtApi.read).toHaveBeenLastCalledWith('one', 'custom.bin'));
  });

  it('finds offline player data by name and opens quick edit from Players', async () => {
    vi.spyOn(nbtApi, 'players').mockResolvedValue({
      players: [
        {
          uuid: '8667ba71-b85a-4004-af54-457a9734eed7',
          name: 'Steve',
          world: 'world',
          path,
          modified_at: '',
        },
      ],
    });
    render(<PlayersPanel server={server} />);
    await userEvent.click(screen.getByRole('button', { name: 'Player data' }));
    fireEvent.change(await screen.findByLabelText('Search saved players'), { target: { value: 'steve' } });
    await userEvent.click(await screen.findByRole('button', { name: /Steve.*Open player data/ }));
    expect(await screen.findByLabelText('Health')).toHaveValue('20');
  });

  it('guards unsaved player data when switching sub-tabs', async () => {
    vi.spyOn(nbtApi, 'players').mockResolvedValue({
      players: [
        {
          uuid: '8667ba71-b85a-4004-af54-457a9734eed7',
          name: 'Steve',
          world: 'world',
          path,
          modified_at: '',
        },
      ],
    });
    const dirty = vi.fn();
    render(<PlayersPanel server={server} onNBTDirtyChange={dirty} />);
    await userEvent.click(screen.getByRole('button', { name: 'Player data' }));
    await userEvent.click(await screen.findByRole('button', { name: /Steve.*Open player data/ }));
    fireEvent.change(await screen.findByLabelText('Health'), { target: { value: '10' } });
    expect(dirty).toHaveBeenLastCalledWith(true);
    await userEvent.click(screen.getByRole('button', { name: /Online Players/ }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(screen.getByLabelText('Health')).toHaveValue('10');
    await userEvent.click(screen.getByRole('button', { name: /Online Players/ }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Discard' }));
    await waitFor(() => expect(screen.queryByRole('region', { name: 'NBT editor' })).not.toBeInTheDocument());
    expect(dirty).toHaveBeenLastCalledWith(false);
  });

  it('does not report an empty player library when discovery failed', async () => {
    vi.spyOn(nbtApi, 'players').mockRejectedValue(new ApiError(500, 'Could not read player folders.'));
    render(<PlayerDataPanel serverId="one" state="stopped" />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not read player folders.');
    expect(screen.queryByText('No saved player data')).not.toBeInTheDocument();
    expect(screen.queryByText('0 player files')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Refresh player data' })).toBeEnabled();
  });

  it('recognizes other Java NBT formats and keeps invalid edits out of save requests', () => {
    for (const name of [
      'level.dat',
      'level.dat_old',
      'structure.nbt',
      'base.schem',
      'base.schematic',
      'base.litematic',
      'structure.nbt.gz',
    ])
      expect(isNBTFile(name)).toBe(true);
    expect(isNBTFile('region.mca')).toBe(false);
    expect(documentError({ type: 4, name: 'long', value: '9223372036854775808' })).toContain('64-bit');
    expect(
      documentError({
        type: 10,
        name: '',
        children: [
          { type: 1, name: 'same', value: '1' },
          { type: 1, name: 'same', value: '2' },
        ],
      }),
    ).toContain('Duplicate tag name');
  });
});
