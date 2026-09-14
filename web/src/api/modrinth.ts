import type {
  Mod,
  ModrinthProject,
  ModrinthSearchHit,
  ModrinthSearchResult,
  ModrinthVersion,
  ModrinthVersionFile,
  ServerType,
} from './types';
import { rateLimitedFetchJson } from './rate-limited-fetch';

const MODRINTH_API_BASE = 'https://api.modrinth.com/v2';

export interface ModrinthSearchParams {
  query?: string;
  loaders?: string[];
  gameVersion?: string;
  category?: string;
  projectType?: 'mod' | 'modpack' | 'plugin';
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
 * Extracts canonical mod identifier from a jar filename or display name,
 * stripping version, loader, and Minecraft version suffixes.
 */
export function extractModId(filename: string): string {
  let base = filename.replace(/\.jar(\.disabled)?$/i, '').toLowerCase();

  // 1. Remove loader + version/remaining: -fabric-..., _forge_...
  const loaderPattern = /[-_](fabric|forge|neoforge|quilt|bukkit|spigot|paper|folia|purpur|sponge|bungee|velocity|mod)([-_].*)?$/i;
  base = base.replace(loaderPattern, '');

  // 2. Remove mc version prefix if any: -mc1.20..., _mc1.21...
  base = base.replace(/[-_]mc\d+.*$/i, '');

  // 3. Remove version suffix starting with - or _: -1.0.0, _v2.3, +1.0, -build.45
  base = base.replace(/[-_](v?\d|build).*$/i, '');

  return base.trim();
}

export interface ModrinthVersionFileLookup {
  id: string;
  project_id: string;
  author_id: string;
  name: string;
  version_number: string;
  files: ModrinthVersionFile[];
}

/**
 * Looks up multiple version files from Modrinth by their hash.
 * Endpoint: POST /v2/version_files
 */
export async function lookupModrinthVersionFiles(
  hashes: string[],
  algorithm: 'sha1' | 'sha512' = 'sha1',
  signal?: AbortSignal,
): Promise<Record<string, ModrinthVersionFileLookup>> {
  if (hashes.length === 0) return {};
  try {
    return await rateLimitedFetchJson<Record<string, ModrinthVersionFileLookup>>(
      `${MODRINTH_API_BASE}/version_files`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'User-Agent': 'mcm-panel/1.0 (https://github.com/mcm-panel/mcm)',
        },
        body: JSON.stringify({
          hashes,
          algorithm,
        }),
      },
      {
        cacheTtlMs: 600000, // 10 minutes cache for immutable hashes
        signal,
      },
    );
  } catch {
    return {};
  }
}

/**
 * Checks if a Modrinth project is already installed in the server.
 */
export function isModInstalled(
  project: Pick<ModrinthSearchHit, 'slug' | 'title'> & { project_id?: string },
  installedMods: Mod[],
  hashProjectMap?: Map<string, string>,
): boolean {
  return findInstalledMod(project, installedMods, hashProjectMap) !== undefined;
}

/**
 * Finds the installed mod on the server that matches a Modrinth project precisely.
 */
export function findInstalledMod(
  project: Pick<ModrinthSearchHit, 'slug' | 'title'> & { project_id?: string },
  installedMods: Mod[],
  hashProjectMap?: Map<string, string>, // sha1 -> project_id or slug
): Mod | undefined {
  const targetSlug = project.slug.toLowerCase().trim().replace(/_/g, '-');
  const targetTitle = project.title.toLowerCase().trim().replace(/[\s_]+/g, '-');
  const targetId = project.project_id;

  // 1. Exact catalog / hash-based project ID match
  for (const m of installedMods) {
    if (targetId && m.project_id && m.project_id === targetId) {
      return m;
    }
    if (m.project_slug && m.project_slug.toLowerCase() === targetSlug) {
      return m;
    }
    if (m.sha1 && hashProjectMap && targetId) {
      const mappedId = hashProjectMap.get(m.sha1);
      if (mappedId && (mappedId === targetId || mappedId === targetSlug)) {
        return m;
      }
    }
  }

  // 2. Canonical mod identifier match from jar manifest (e.g. fabric.mod.json, plugin.yml)
  for (const m of installedMods) {
    if (m.mod_id) {
      const cleanModId = m.mod_id.toLowerCase().trim().replace(/_/g, '-');
      if (cleanModId === targetSlug || cleanModId === targetTitle) {
        return m;
      }
      if (m.title) {
        const cleanModTitle = m.title.toLowerCase().trim().replace(/[\s_]+/g, '-');
        if (cleanModTitle === targetSlug || cleanModTitle === targetTitle) {
          return m;
        }
      }
      // If the mod_id is explicitly known from jar manifest, NEVER allow loose filename
      // matching to hijack this jar for an unrelated project (e.g. graves jar for ly-graves)
      continue;
    }

    // 3. Exact match on raw filename or name
    const rawFile = m.file.replace(/\.jar(\.disabled)?$/i, '').toLowerCase();
    const rawName = m.name.toLowerCase();
    if (rawFile === targetSlug || rawName === targetSlug) {
      return m;
    }
    if (rawFile === targetTitle || rawName === targetTitle) {
      // Only match on targetTitle if targetTitle is specific (matches targetSlug or contains hyphen)
      if (targetTitle === targetSlug || targetTitle.includes('-')) {
        return m;
      }
    }

    // 4. Canonical mod identifier match from filename
    const fileModId = extractModId(m.file).replace(/_/g, '-');
    const nameModId = extractModId(m.name).replace(/_/g, '-');

    if (fileModId === targetSlug || nameModId === targetSlug) {
      return m;
    }

    // Match on targetTitle, but NEVER if targetSlug has a prefix before targetTitle
    // (e.g. fileModId "graves" must NOT match project with slug "ly-graves" and title "Graves")
    if (fileModId === targetTitle || nameModId === targetTitle) {
      if (!targetSlug.endsWith(`-${targetTitle}`)) {
        return m;
      }
    }
  }

  return undefined;
}

/**
 * Checks if a specific version file matches any installed mod jar on the server.
 */
export function isVersionFileInstalled(
  file: Pick<ModrinthVersionFile, 'filename'>,
  installedMods: Mod[],
): boolean {
  const targetName = file.filename.toLowerCase();
  return installedMods.some((m) => {
    const fn = m.file.toLowerCase();
    return fn === targetName || fn === `${targetName}.disabled` || targetName === `${fn}.disabled`;
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
    projectType,
    serverSideOnly = true,
    sort = 'downloads',
    offset = 0,
    limit = 20,
  } = params;

  const facets: string[][] = [];

  // Project type (mod, modpack, plugin)
  if (projectType) {
    facets.push([`project_type:${projectType}`]);
  }

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

  // Exclude client-only mods by default for servers, except for modpacks which are bundles
  if (serverSideOnly && projectType !== 'modpack') {
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
  return rateLimitedFetchJson<ModrinthSearchResult>(
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

  return rateLimitedFetchJson<ModrinthVersion[]>(
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
 * Retrieves full details for a Modrinth project.
 */
export async function getProject(
  projectSlugOrId: string,
  signal?: AbortSignal,
): Promise<ModrinthProject> {
  const url = `${MODRINTH_API_BASE}/project/${encodeURIComponent(projectSlugOrId)}`;
  return rateLimitedFetchJson<ModrinthProject>(
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

export interface ModrinthVersionUpdateParams {
  hashes: string[];
  algorithm?: 'sha1' | 'sha512';
  loaders?: string[];
  gameVersions?: string[];
}

/**
 * Checks for updates on Modrinth given file hashes.
 * Endpoint: POST /v2/version_files/update
 */
export async function checkModrinthUpdates(
  params: ModrinthVersionUpdateParams,
  signal?: AbortSignal,
): Promise<Record<string, ModrinthVersion>> {
  if (params.hashes.length === 0) return {};
  try {
    return await rateLimitedFetchJson<Record<string, ModrinthVersion>>(
      `${MODRINTH_API_BASE}/version_files/update`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'User-Agent': 'mcm-panel/1.0 (https://github.com/mcm-panel/mcm)',
        },
        body: JSON.stringify({
          hashes: params.hashes,
          algorithm: params.algorithm ?? 'sha1',
          loaders: params.loaders,
          game_versions: params.gameVersions,
        }),
      },
      {
        cacheTtlMs: 300000, // 5 minutes
        signal,
      },
    );
  } catch {
    return {};
  }
}
