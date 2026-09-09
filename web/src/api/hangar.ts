import type {
  HangarProject,
  HangarSearchResult,
  HangarVersion,
  HangarVersionsResult,
  Mod,
} from './types';
import { extractModId } from './modrinth';

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
  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
    },
    signal,
  });

  if (!resp.ok) {
    throw new Error(`Hangar search failed with status ${resp.status}`);
  }

  return (await resp.json()) as HangarSearchResult;
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

  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
    },
    signal,
  });

  if (!resp.ok) {
    throw new Error(`Failed to fetch Hangar versions for ${projectSlug} (${resp.status})`);
  }

  const data = (await resp.json()) as HangarVersionsResult;
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
  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
    },
    signal,
  });

  if (!resp.ok) {
    throw new Error(`Failed to fetch Hangar project ${projectSlug} (${resp.status})`);
  }

  return (await resp.json()) as HangarProject;
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

  return installedMods.find((m) => {
    const rawFile = m.file.replace(/\.jar(\.disabled)?$/i, '').toLowerCase();
    const rawName = m.name.toLowerCase();
    if (
      rawFile === targetSlug ||
      rawFile === targetName ||
      rawName === targetSlug ||
      rawName === targetName
    ) {
      return true;
    }

    const fileModId = extractModId(m.file).replace(/_/g, '-');
    const nameModId = extractModId(m.name).replace(/_/g, '-');

    return (
      fileModId === targetSlug ||
      fileModId === targetName ||
      nameModId === targetSlug ||
      nameModId === targetName
    );
  });
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
