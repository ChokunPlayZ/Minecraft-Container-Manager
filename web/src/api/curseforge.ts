import { rememberArtifactHashes } from './artifact-hashes';
import { request } from './client';
import { clearRateLimitCache } from './rate-limited-fetch';
import type {
  CurseForgeFile,
  CurseForgeMod,
  CurseForgeSearchResult,
  Mod,
  ServerType,
} from './types';
import { extractModId } from './modrinth';
import { rateLimitedFetchJson } from './rate-limited-fetch';

export const CURSEFORGE_API_BASE = '/api/curseforge';
export const MINECRAFT_GAME_ID = 432;

export interface CurseForgeSettings {
  configured: boolean;
  source: 'database' | 'environment' | '';
}

export function getCurseForgeSettings(): Promise<CurseForgeSettings> {
  return request<CurseForgeSettings>(`${CURSEFORGE_API_BASE}/settings`);
}

export async function saveCurseForgeApiKey(key: string): Promise<CurseForgeSettings> {
  const settings = await request<CurseForgeSettings>(`${CURSEFORGE_API_BASE}/settings`, {
    method: 'PUT',
    body: JSON.stringify({ api_key: key.trim() }),
  });
  clearRateLimitCache();
  // Remove the old browser-only credential after explicitly saving the shared replacement.
  if (typeof window !== 'undefined') {
    localStorage.removeItem('mcm_curseforge_api_key');
    window.dispatchEvent(new Event('mcm:curseforge-settings'));
  }
  return settings;
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
    case 'quilt':
      return 5;
    case 'neoforge':
      return 6;
    default:
      return undefined;
  }
}

export const CF_CLASS_MODS = 6;
export const CF_CLASS_PLUGINS = 5;
export const CF_CLASS_MODPACKS = 4471;

export function isPluginServerType(serverType: ServerType): boolean {
  return [
    'paper',
    'spigot',
    'purpur',
    'pufferfish',
    'leaf',
    'folia',
    'waterfall',
    'velocity',
    'bungeecord',
    'limbo',
    'nanolimbo',
    'geysermc',
  ].includes(serverType);
}

/**
 * Returns appropriate CurseForge classId: 6 for mods, 5 for plugins.
 */
export function getCurseForgeClassId(serverType: ServerType): number {
  if (isPluginServerType(serverType)) {
    return CF_CLASS_PLUGINS; // Bukkit Plugins
  }
  return CF_CLASS_MODS; // Mods
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
  return rateLimitedFetchJson<CurseForgeSearchResult>(
    url,
    {
      headers: {
        Accept: 'application/json',
      },
    },
    {
      cacheTtlMs: 120000, // 2 minutes
      signal,
    },
  );
}

/**
 * Retrieves release files for a CurseForge mod.
 */
export async function getCurseForgeFiles(
  modId: number,
  options?: {
    gameVersion?: string;
    modLoaderType?: number;
    pageSize?: number;
    allPages?: boolean;
    stopWhen?: (build: CurseForgeFile) => boolean;
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
  searchParams.set('pageSize', String(options?.pageSize ?? (options?.allPages ? 50 : 25)));

  const files: CurseForgeFile[] = [];
  for (;;) {
    const url = `${CURSEFORGE_API_BASE}/mods/${modId}/files?${searchParams}`;
    const page = await rateLimitedFetchJson<{ data: CurseForgeFile[]; pagination?: { totalCount: number } }>(
      url, { headers: { Accept: 'application/json' } }, { cacheTtlMs: 300000, signal },
    );
    files.push(...(page.data ?? []));
    if (options?.stopWhen && (page.data ?? []).some(options.stopWhen)) break;
    if (!options?.allPages || !page.data?.length || files.length >= (page.pagination?.totalCount ?? 0)) break;
    searchParams.set('index', String(files.length));
  }
 for(const file of files){const hashes:Record<string,string>={};for(const hash of file.hashes??[]){if(hash.algo===1)hashes.sha1=hash.value;else if(hash.algo===2)hashes.md5=hash.value;}rememberArtifactHashes(file.downloadUrl??undefined,hashes);}
 return files;
}

/**
 * Gets direct download URL for a CurseForge file, resolving edge URL if needed.
 */
export async function getCurseForgeDownloadUrl(
  modId: number,
  fileId: number,
  signal?: AbortSignal,
): Promise<string> {
  const url = `${CURSEFORGE_API_BASE}/mods/${modId}/files/${fileId}/download-url`;
  const data = await rateLimitedFetchJson<{ data: string }>(
    url,
    {
      headers: {
        Accept: 'application/json',
      },
    },
    {
      cacheTtlMs: 900000, // 15 minutes
      signal,
    },
  );
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

/**
 * Retrieves multiple mods from CurseForge by their IDs in a single batch request.
 * Endpoint: POST /v1/mods
 */
export async function getCurseForgeMods(
  modIds: number[],
  signal?: AbortSignal,
): Promise<CurseForgeMod[]> {
  if (modIds.length === 0) return [];
  const url = `${CURSEFORGE_API_BASE}/mods`;
  try {
    const res = await rateLimitedFetchJson<{ data: CurseForgeMod[] }>(
      url,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Accept: 'application/json',
        },
        body: JSON.stringify({ modIds }),
      },
      {
        cacheTtlMs: 300000,
        signal,
      },
    );
    return res.data ?? [];
  } catch {
    return [];
  }
}
