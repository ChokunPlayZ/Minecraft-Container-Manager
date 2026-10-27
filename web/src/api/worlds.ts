import { ApiError, request } from './client';

export interface WorldInput { name: string; seed: string; level_type: string; generate_structures: boolean }
export interface WorldDimension { id: string; name: string; regions: number }
export interface World {
  name: string; active: boolean; generated: boolean; seed: string | null; level_type: string;
  generate_structures: boolean; size_bytes: number; modified_at: string; version?: string;
  metadata_error: boolean; dimensions: WorldDimension[];
}
export interface WorldList { worlds: World[]; active_world: string; editable: boolean }
export interface WorldRegion { x: number; z: number; chunks: number; size_bytes: number }
export interface WorldRegions { regions: WorldRegion[]; total: number; offset: number }
export interface WorldChunk { x: number; z: number; timestamp: number }
export interface ChunkSelection { dimension: string; region_x: number; region_z: number; chunks: number[] }
const base = (id: string) => `/api/servers/${encodeURIComponent(id)}/worlds`;
const worldPath = (id: string, name: string) => `${base(id)}/${encodeURIComponent(name)}`;
const demo = () => typeof window !== 'undefined' && (window.location.search.includes('mock=true') || window.localStorage.getItem('mcm-mock') === 'true');
const demoWorlds: World[] = [
  { name: 'world', active: true, generated: true, seed: '-4172144997902289642', level_type: 'minecraft:normal', generate_structures: true, size_bytes: 1843200000, modified_at: '2026-10-27T12:00:00Z', version: '1.21.4', metadata_error: false, dimensions: [{ id: 'world', name: 'Overworld', regions: 12 }, { id: 'world_nether/DIM-1', name: 'Nether', regions: 4 }, { id: 'world_the_end/DIM1', name: 'The End', regions: 2 }] },
  { name: 'creative', active: false, generated: true, seed: '7042838102957781601', level_type: 'minecraft:flat', generate_structures: false, size_bytes: 314572800, modified_at: '2026-10-24T08:00:00Z', version: '1.21.4', metadata_error: false, dimensions: [{ id: 'creative', name: 'Overworld', regions: 3 }] },
];
async function mutate<T>(path: string, init: RequestInit): Promise<T> {
  if (demo()) throw new ApiError(409, 'Demo worlds are read-only.');
  return request<T>(path, init);
}
export const worldApi = {
  list: (id: string): Promise<WorldList> => demo() ? Promise.resolve({ worlds: structuredClone(demoWorlds), active_world: 'world', editable: false }) : request(base(id)),
  create: (id: string, input: WorldInput) => mutate<World>(base(id), { method: 'POST', body: JSON.stringify(input) }),
  save: (id: string, input: WorldInput) => mutate<World>(worldPath(id, input.name), { method: 'PUT', body: JSON.stringify(input) }),
  activate: (id: string, name: string) => mutate<World>(`${worldPath(id, name)}/activate`, { method: 'POST' }),
  remove: (id: string, name: string) => mutate<{ ok: boolean; recovery_path: string }>(worldPath(id, name), { method: 'DELETE' }),
  import: (id: string, name: string, file: File) => {
    const form = new FormData(); form.set('name', name); form.set('file', file);
    return mutate<World>(`${base(id)}/import`, { method: 'POST', body: form });
  },
  export: async (id: string, name: string) => {
    if (demo()) throw new ApiError(409, 'Demo worlds are read-only.');
    const response = await fetch(`${worldPath(id, name)}/export`, { credentials: 'include' });
    if (!response.ok) { const body = await response.json().catch(() => null); throw new ApiError(response.status, body?.error?.message ?? 'Could not export this world. Try again.'); }
    const url = URL.createObjectURL(await response.blob());
    const anchor = document.createElement('a'); anchor.href = url; anchor.download = `${name}.zip`; document.body.appendChild(anchor); anchor.click(); anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 60000);
  },
  regions: (id: string, name: string, dimension: string, offset = 0): Promise<WorldRegions> => demo() ? Promise.resolve({ regions: [{ x: 0, z: 0, chunks: 768, size_bytes: 67108864 }, { x: -1, z: 0, chunks: 512, size_bytes: 48234496 }, { x: 0, z: -1, chunks: 394, size_bytes: 31457280 }], total: 3, offset: 0 }) : request(`${worldPath(id, name)}/regions?${new URLSearchParams({ dimension, offset: String(offset) })}`),
  chunks: (id: string, name: string, dimension: string, x: number, z: number): Promise<{ chunks: WorldChunk[] }> => demo() ? Promise.resolve({ chunks: Array.from({ length: 1024 }, (_, i) => i).filter((i) => i % 32 > 2 && i % 32 < 29 && Math.floor(i / 32) > 1 && Math.floor(i / 32) < 30 && (i * 7) % 23 > 1).map((i) => ({ x: x * 32 + i % 32, z: z * 32 + Math.floor(i / 32), timestamp: 1793102400 })) }) : request(`${worldPath(id, name)}/chunks?${new URLSearchParams({ dimension, x: String(x), z: String(z) })}`),
  reset: (id: string, name: string, selection: ChunkSelection) => mutate<{ reset: number; recovery_path: string }>(`${worldPath(id, name)}/chunks/reset`, { method: 'POST', body: JSON.stringify(selection) }),
};
