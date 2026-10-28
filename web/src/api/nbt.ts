import { ApiError, request } from './client';

export interface NBTTag {
  type: number;
  name: string;
  value?: string;
  children?: NBTTag[];
  list_type?: number;
  values?: string[];
}
export interface NBTFile {
  path: string;
  revision: string;
  compression: 'gzip' | 'zlib' | 'none';
  root: NBTTag;
  editable: boolean;
  backup_path?: string;
}
export interface PlayerDataFile {
  uuid: string;
  name: string;
  world: string;
  path: string;
  modified_at: string;
}
const base = (id: string) => `/api/servers/${encodeURIComponent(id)}`;
const demo = () =>
  typeof window !== 'undefined' &&
  (window.location.search.includes('mock=true') || window.localStorage.getItem('mcm-mock') === 'true');
const demoPath = 'world/playerdata/8667ba71-b85a-4004-af54-457a9734eed7.dat';
const demoRoot: NBTTag = {
  type: 10,
  name: '',
  children: [
    { type: 5, name: 'Health', value: '20' },
    { type: 3, name: 'foodLevel', value: '20' },
    { type: 5, name: 'foodSaturationLevel', value: '5' },
    { type: 3, name: 'XpLevel', value: '12' },
    { type: 3, name: 'XpTotal', value: '216' },
    { type: 5, name: 'XpP', value: '0.5' },
    { type: 3, name: 'playerGameType', value: '0' },
    { type: 8, name: 'Dimension', value: 'minecraft:overworld' },
    {
      type: 9,
      name: 'Pos',
      list_type: 6,
      children: ['128.5', '64', '-302.5'].map((value) => ({ type: 6, name: '', value })),
    },
    {
      type: 9,
      name: 'Inventory',
      list_type: 10,
      children: [
        {
          type: 10,
          name: '',
          children: [
            { type: 1, name: 'Slot', value: '0' },
            { type: 8, name: 'id', value: 'minecraft:diamond_sword' },
            { type: 3, name: 'count', value: '1' },
            { type: 10, name: 'components', children: [{ type: 3, name: 'minecraft:damage', value: '0' }] },
          ],
        },
      ],
    },
    { type: 9, name: 'EnderItems', list_type: 10, children: [] },
    {
      type: 10,
      name: 'abilities',
      children: [
        { type: 1, name: 'flying', value: '0' },
        { type: 5, name: 'walkSpeed', value: '0.1' },
      ],
    },
    { type: 4, name: 'LastDeathTime', value: '9223372036854775807' },
    { type: 3, name: 'DataVersion', value: '4189' },
  ],
};

export const nbtApi = {
  read: (id: string, path: string): Promise<NBTFile> =>
    demo()
      ? Promise.resolve({
          path,
          revision: 'demo',
          compression: 'gzip',
          root: structuredClone(demoRoot),
          editable: false,
        })
      : request(`${base(id)}/files/nbt?${new URLSearchParams({ path })}`),
  save: (id: string, file: NBTFile): Promise<NBTFile> => {
    if (demo()) return Promise.reject(new ApiError(409, 'Demo NBT data is read-only.'));
    return request(`${base(id)}/files/nbt`, {
      method: 'PUT',
      body: JSON.stringify({ path: file.path, expected_revision: file.revision, root: file.root }),
    });
  },
  players: (id: string): Promise<{ players: PlayerDataFile[] }> =>
    demo()
      ? Promise.resolve({
          players: [
            {
              uuid: '8667ba71-b85a-4004-af54-457a9734eed7',
              name: 'Steve',
              world: 'world',
              path: demoPath,
              modified_at: '2026-10-28T08:00:00Z',
            },
          ],
        })
      : request(`${base(id)}/players/data`),
};
