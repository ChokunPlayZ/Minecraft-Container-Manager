import type {
  CurseForgeFile,
  CurseForgeMod,
  CurseForgeSearchResult,
  Mod,
  ServerType,
} from './types';
import { extractModId } from './modrinth';

export const CURSEFORGE_API_BASE = 'https://api.curseforge.com/v1';
export const MINECRAFT_GAME_ID = 432;

export const CF_STORAGE_KEY = 'mcm_curseforge_api_key';

export function getCurseForgeApiKey(): string {
  if (typeof window === 'undefined') return '';
  return localStorage.getItem(CF_STORAGE_KEY) || '';
}

export function setCurseForgeApiKey(key: string): void {
  if (typeof window === 'undefined') return;
  if (!key.trim()) {
    localStorage.removeItem(CF_STORAGE_KEY);
  } else {
    localStorage.setItem(CF_STORAGE_KEY, key.trim());
  }
}

export function hasCurseForgeApiKey(): boolean {
  return Boolean(getCurseForgeApiKey());
}

/**
 * Maps server software type to CurseForge loader type ID.
 * 1 = Forge, 4 = Fabric, 5 = Quilt, 6 = NeoForge
 */
export function getCurseForgeLoaderType(serverType: ServerType): number | undefined {
  switch (serverType) {
    case 'forge':
      return 1;
    case 'fabric':
      return 4;
    case 'neoforge':
      return 6;
    default:
      return undefined;
  }
}

/**
 * Returns appropriate CurseForge classId: 6 for mods, 5 for plugins.
 */
export function getCurseForgeClassId(serverType: ServerType): number {
  if (serverType === 'paper' || serverType === 'spigot') {
    return 5; // Bukkit Plugins
  }
  return 6; // Mods
}

export const CF_SORT_OPTIONS: {
  id: number;
  label: string;
}[] = [
  { id: 2, label: 'Popularity' },
  { id: 6, label: 'Most Downloaded' },
  { id: 3, label: 'Recently Updated' },
  { id: 1, label: 'Featured' },
];

export interface CurseForgeSearchParams {
  query?: string;
  classId?: number;
  modLoaderType?: number;
  gameVersion?: string;
  sortField?: number;
  pageSize?: number;
  index?: number;
}

/**
 * Searches CurseForge mods or plugins.
 */
export async function searchCurseForge(
  params: CurseForgeSearchParams,
  apiKey: string,
  signal?: AbortSignal,
): Promise<CurseForgeSearchResult> {
  const {
    query = '',
    classId = 6,
    modLoaderType,
    gameVersion,
    sortField = 6,
    pageSize = 20,
    index = 0,
  } = params;

  const searchParams = new URLSearchParams();
  searchParams.set('gameId', String(MINECRAFT_GAME_ID));
  searchParams.set('classId', String(classId));
  if (query.trim()) {
    searchParams.set('searchFilter', query.trim());
  }
  if (modLoaderType !== undefined) {
    searchParams.set('modLoaderType', String(modLoaderType));
  }
  if (gameVersion && gameVersion.trim()) {
    searchParams.set('gameVersion', gameVersion.trim());
  }
  searchParams.set('sortField', String(sortField));
  searchParams.set('sortOrder', 'desc');
  searchParams.set('pageSize', String(pageSize));
  searchParams.set('index', String(index));

  const url = `${CURSEFORGE_API_BASE}/mods/search?${searchParams.toString()}`;
  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
      'x-api-key': apiKey.trim(),
    },
    signal,
  });

  if (!resp.ok) {
    if (resp.status === 403 || resp.status === 401) {
      throw new Error('Invalid CurseForge API key or access denied.');
    }
    throw new Error(`CurseForge search failed with status ${resp.status}`);
  }

  return (await resp.json()) as CurseForgeSearchResult;
}

/**
 * Retrieves release files for a CurseForge mod.
 */
export async function getCurseForgeFiles(
  modId: number,
  apiKey: string,
  options?: {
    gameVersion?: string;
    modLoaderType?: number;
    pageSize?: number;
  },
  signal?: AbortSignal,
): Promise<CurseForgeFile[]> {
  const searchParams = new URLSearchParams();
  if (options?.gameVersion) {
    searchParams.set('gameVersion', options.gameVersion);
  }
  if (options?.modLoaderType !== undefined) {
    searchParams.set('modLoaderType', String(options.modLoaderType));
  }
  searchParams.set('pageSize', String(options?.pageSize ?? 25));

  const query = searchParams.toString();
  const url = `${CURSEFORGE_API_BASE}/mods/${modId}/files?${query}`;

  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
      'x-api-key': apiKey.trim(),
    },
    signal,
  });

  if (!resp.ok) {
    throw new Error(`Failed to fetch files for mod ${modId} (${resp.status})`);
  }

  const data = (await resp.json()) as { data: CurseForgeFile[] };
  return data.data ?? [];
}

/**
 * Gets direct download URL for a CurseForge file, resolving edge URL if needed.
 */
export async function getCurseForgeDownloadUrl(
  modId: number,
  fileId: number,
  apiKey: string,
  signal?: AbortSignal,
): Promise<string> {
  const url = `${CURSEFORGE_API_BASE}/mods/${modId}/files/${fileId}/download-url`;
  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
      'x-api-key': apiKey.trim(),
    },
    signal,
  });

  if (!resp.ok) {
    throw new Error(`Failed to obtain download URL for file ${fileId} (${resp.status})`);
  }

  const data = (await resp.json()) as { data: string };
  return data.data;
}

/**
 * Checks if a CurseForge mod matches an installed mod.
 */
export function findInstalledCurseForgeMod(
  mod: Pick<CurseForgeMod, 'name' | 'slug'> & { id?: number },
  installedMods: Mod[],
): Mod | undefined {
  const targetSlug = mod.slug.toLowerCase().trim().replace(/_/g, '-');
  const targetName = mod.name.toLowerCase().trim().replace(/[\s_]+/g, '-');
  const targetIdStr = mod.id ? String(mod.id) : undefined;

  // 1. Exact catalog project match
  for (const m of installedMods) {
    if (targetIdStr && m.project_id && m.project_id === targetIdStr) {
      return m;
    }
    if (m.project_slug && m.project_slug.toLowerCase() === targetSlug) {
      return m;
    }
  }

  // 2. Canonical mod identifier from jar manifest
  for (const m of installedMods) {
    if (m.mod_id) {
      const cleanModId = m.mod_id.toLowerCase().trim().replace(/_/g, '-');
      if (cleanModId === targetSlug || cleanModId === targetName) {
        return m;
      }
      if (m.title) {
        const cleanModTitle = m.title.toLowerCase().trim().replace(/[\s_]+/g, '-');
        if (cleanModTitle === targetSlug || cleanModTitle === targetName) {
          return m;
        }
      }
      continue;
    }

    // 3. Exact match on raw filename or name
    const rawFile = m.file.replace(/\.jar(\.disabled)?$/i, '').toLowerCase();
    const rawName = m.name.toLowerCase();
    if (rawFile === targetSlug || rawName === targetSlug) {
      return m;
    }
    if (rawFile === targetName || rawName === targetName) {
      if (targetName === targetSlug || targetName.includes('-')) {
        return m;
      }
    }

    // 4. Filename mod identifier
    const fileModId = extractModId(m.file).replace(/_/g, '-');
    const nameModId = extractModId(m.name).replace(/_/g, '-');

    if (fileModId === targetSlug || nameModId === targetSlug) {
      return m;
    }

    if (targetName === targetSlug && fileModId === targetName) {
      return m;
    }
  }

  return undefined;
}

export function isCurseForgeModInstalled(
  mod: Pick<CurseForgeMod, 'name' | 'slug'>,
  installedMods: Mod[],
): boolean {
  return findInstalledCurseForgeMod(mod, installedMods) !== undefined;
}
