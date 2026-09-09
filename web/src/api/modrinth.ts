import type {
  Mod,
  ModrinthProject,
  ModrinthSearchHit,
  ModrinthSearchResult,
  ModrinthVersion,
  ServerType,
} from './types';

const MODRINTH_API_BASE = 'https://api.modrinth.com/v2';

export interface ModrinthSearchParams {
  query?: string;
  loaders?: string[];
  gameVersion?: string;
  category?: string;
  serverSideOnly?: boolean;
  sort?: 'relevance' | 'downloads' | 'follows' | 'newest' | 'updated';
  offset?: number;
  limit?: number;
}

/**
 * Maps a server's type to compatible Modrinth loader categories.
 */
export function getServerLoaders(serverType: ServerType): string[] {
  switch (serverType) {
    case 'paper':
      return ['paper', 'purpur', 'spigot', 'bukkit', 'folia'];
    case 'spigot':
      return ['spigot', 'bukkit', 'paper'];
    case 'fabric':
      return ['fabric', 'quilt'];
    case 'forge':
      return ['forge'];
    case 'neoforge':
      return ['neoforge', 'forge'];
    default:
      return [];
  }
}

/**
 * Returns human-readable label for the server software loader.
 */
export function getLoaderLabel(serverType: ServerType): string {
  switch (serverType) {
    case 'paper':
      return 'Paper / Spigot';
    case 'spigot':
      return 'Spigot / Bukkit';
    case 'fabric':
      return 'Fabric';
    case 'forge':
      return 'Forge';
    case 'neoforge':
      return 'NeoForge';
    case 'vanilla':
      return 'Vanilla';
    default:
      return serverType;
  }
}

/**
 * Formats large download/follower counts into readable strings (e.g. 1.2M, 45.3K).
 */
export function formatCount(count: number): string {
  if (count >= 1_000_000) {
    return `${(count / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`;
  }
  if (count >= 1_000) {
    return `${(count / 1_000).toFixed(1).replace(/\.0$/, '')}K`;
  }
  return count.toLocaleString();
}

/**
 * Formats file size in bytes to human-readable string.
 */
export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1).replace(/\.0$/, '')} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1).replace(/\.0$/, '')} MB`;
}

/**
 * Checks if a Modrinth project is already installed in the server.
 */
export function isModInstalled(
  project: Pick<ModrinthSearchHit, 'slug' | 'title'>,
  installedMods: Mod[],
): boolean {
  const normSlug = project.slug.toLowerCase().replace(/[-_]/g, '');
  const normTitle = project.title.toLowerCase().replace(/[^a-z0-9]/g, '');

  return installedMods.some((m) => {
    const fn = m.file.toLowerCase().replace(/[-_.]/g, '');
    const mn = m.name.toLowerCase().replace(/[^a-z0-9]/g, '');

    return (
      fn.includes(normSlug) ||
      mn.includes(normSlug) ||
      fn.includes(normTitle) ||
      mn.includes(normTitle)
    );
  });
}

/**
 * Searches Modrinth projects matching given filters.
 */
export async function searchModrinth(
  params: ModrinthSearchParams,
  signal?: AbortSignal,
): Promise<ModrinthSearchResult> {
  const {
    query = '',
    loaders = [],
    gameVersion,
    category,
    serverSideOnly = true,
    sort = 'downloads',
    offset = 0,
    limit = 20,
  } = params;

  const facets: string[][] = [];

  // Loader category matching
  if (loaders.length > 0) {
    facets.push(loaders.map((l) => `categories:${l}`));
  }

  // Version matching
  if (gameVersion && gameVersion.trim()) {
    facets.push([`versions:${gameVersion.trim()}`]);
  }

  // Category filter
  if (category && category !== 'all') {
    facets.push([`categories:${category}`]);
  }

  // Exclude client-only mods by default for servers
  if (serverSideOnly) {
    facets.push(['server_side!=unsupported']);
  }

  const searchParams = new URLSearchParams();
  if (query.trim()) {
    searchParams.set('query', query.trim());
  }
  if (facets.length > 0) {
    searchParams.set('facets', JSON.stringify(facets));
  }
  searchParams.set('index', sort);
  searchParams.set('offset', String(offset));
  searchParams.set('limit', String(limit));

  const url = `${MODRINTH_API_BASE}/search?${searchParams.toString()}`;
  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
    },
    signal,
  });

  if (!resp.ok) {
    throw new Error(`Modrinth search failed with status ${resp.status}`);
  }

  return (await resp.json()) as ModrinthSearchResult;
}

/**
 * Retrieves versions for a Modrinth project, optionally filtered by loaders and game versions.
 */
export async function getProjectVersions(
  projectSlugOrId: string,
  options?: {
    loaders?: string[];
    gameVersions?: string[];
  },
  signal?: AbortSignal,
): Promise<ModrinthVersion[]> {
  const searchParams = new URLSearchParams();
  if (options?.loaders && options.loaders.length > 0) {
    searchParams.set('loaders', JSON.stringify(options.loaders));
  }
  if (options?.gameVersions && options.gameVersions.length > 0) {
    searchParams.set('game_versions', JSON.stringify(options.gameVersions));
  }

  const query = searchParams.toString();
  const url = `${MODRINTH_API_BASE}/project/${encodeURIComponent(projectSlugOrId)}/version${query ? `?${query}` : ''}`;

  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
    },
    signal,
  });

  if (!resp.ok) {
    throw new Error(`Failed to fetch versions for ${projectSlugOrId} (${resp.status})`);
  }

  return (await resp.json()) as ModrinthVersion[];
}

/**
 * Retrieves full details for a Modrinth project.
 */
export async function getProject(
  projectSlugOrId: string,
  signal?: AbortSignal,
): Promise<ModrinthProject> {
  const url = `${MODRINTH_API_BASE}/project/${encodeURIComponent(projectSlugOrId)}`;
  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
    },
    signal,
  });

  if (!resp.ok) {
    throw new Error(`Failed to fetch project ${projectSlugOrId} (${resp.status})`);
  }

  return (await resp.json()) as ModrinthProject;
}
