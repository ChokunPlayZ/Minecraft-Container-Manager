import type {
  HangarProject,
  HangarSearchResult,
  HangarVersion,
  HangarVersionsResult,
  Mod,
} from './types';
import { extractModId } from './modrinth';
import { rateLimitedFetchJson } from './rate-limited-fetch';

export const HANGAR_API_BASE = 'https://hangar.papermc.io/api/v1';

export const HANGAR_CATEGORIES = [
  { id: 'all', label: 'All' },
  { id: 'admin_tools', label: 'Administration' },
  { id: 'gameplay', label: 'Gameplay' },
  { id: 'protection', label: 'Protection' },
  { id: 'economy', label: 'Economy' },
  { id: 'chat', label: 'Chat' },
  { id: 'world_management', label: 'World Management' },
  { id: 'dev_tools', label: 'Developer Tools' },
  { id: 'games', label: 'Minigames' },
  { id: 'role_play', label: 'Roleplay' },
  { id: 'misc', label: 'Miscellaneous' },
];

export const HANGAR_SORT_OPTIONS: {
  id: '-downloads' | '-stars' | '-views' | '-updated' | '-newest';
  label: string;
}[] = [
  { id: '-downloads', label: 'Most Downloaded' },
  { id: '-stars', label: 'Most Stars' },
  { id: '-views', label: 'Most Views' },
  { id: '-updated', label: 'Recently Updated' },
  { id: '-newest', label: 'Newest' },
];

export interface HangarSearchParams {
  query?: string;
  category?: string;
  sort?: '-downloads' | '-stars' | '-views' | '-updated' | '-newest';
  platform?: string;
  limit?: number;
  offset?: number;
}

/**
 * Searches Hangar projects matching given filters.
 */
export async function searchHangar(
  params: HangarSearchParams,
  signal?: AbortSignal,
): Promise<HangarSearchResult> {
  const {
    query = '',
    category,
    sort = '-downloads',
    platform = 'PAPER',
    limit = 20,
    offset = 0,
  } = params;

  const searchParams = new URLSearchParams();
  if (query.trim()) {
    searchParams.set('q', query.trim());
  }
  if (category && category !== 'all') {
    searchParams.set('category', category);
  }
  if (sort) {
    searchParams.set('sort', sort);
  }
  if (platform) {
    searchParams.set('platform', platform);
  }
  searchParams.set('limit', String(limit));
  searchParams.set('offset', String(offset));

  const url = `${HANGAR_API_BASE}/projects?${searchParams.toString()}`;
  return rateLimitedFetchJson<HangarSearchResult>(
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
 * Retrieves release versions for a Hangar project.
 */
export async function getHangarVersions(
  projectSlug: string,
  options?: {
    limit?: number;
    offset?: number;
  },
  signal?: AbortSignal,
): Promise<HangarVersion[]> {
  const searchParams = new URLSearchParams();
  searchParams.set('limit', String(options?.limit ?? 25));
  if (options?.offset) {
    searchParams.set('offset', String(options.offset));
  }

  const query = searchParams.toString();
  const url = `${HANGAR_API_BASE}/projects/${encodeURIComponent(projectSlug)}/versions?${query}`;

  const data = await rateLimitedFetchJson<HangarVersionsResult>(
    url,
    {
      headers: {
        Accept: 'application/json',
      },
    },
    {
      cacheTtlMs: 300000, // 5 minutes
      signal,
    },
  );

  return data.result ?? [];
}

/**
 * Retrieves full details for a Hangar project.
 */
export async function getHangarProject(
  projectSlug: string,
  signal?: AbortSignal,
): Promise<HangarProject> {
  const url = `${HANGAR_API_BASE}/projects/${encodeURIComponent(projectSlug)}`;
  return rateLimitedFetchJson<HangarProject>(
    url,
    {
      headers: {
        Accept: 'application/json',
      },
    },
    {
      cacheTtlMs: 300000, // 5 minutes
      signal,
    },
  );
}

/**
 * Checks if a Hangar project matches any installed mod/plugin.
 */
export function findInstalledHangarProject(
  project: Pick<HangarProject, 'name' | 'namespace'>,
  installedMods: Mod[],
): Mod | undefined {
  const targetSlug = project.namespace.slug.toLowerCase().trim().replace(/_/g, '-');
  const targetName = project.name.toLowerCase().trim().replace(/[\s_]+/g, '-');

  // 1. Exact catalog project match
  for (const m of installedMods) {
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

export function isHangarProjectInstalled(
  project: Pick<HangarProject, 'name' | 'namespace'>,
  installedMods: Mod[],
): boolean {
  return findInstalledHangarProject(project, installedMods) !== undefined;
}

export function isHangarVersionInstalled(
  version: HangarVersion,
  installedMods: Mod[],
): boolean {
  const download = version.downloads?.PAPER || Object.values(version.downloads || {})[0];
  const filename = download?.fileInfo?.name?.toLowerCase();
  if (!filename) return false;

  return installedMods.some((m) => {
    const fn = m.file.toLowerCase();
    return fn === filename || fn === `${filename}.disabled` || filename === `${fn}.disabled`;
  });
}
