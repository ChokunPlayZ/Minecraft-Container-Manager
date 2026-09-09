import type {
  Mod,
  SpigetResource,
  SpigetVersion,
} from './types';
import { extractModId } from './modrinth';

export const SPIGET_API_BASE = 'https://api.spiget.org/v2';

export const SPIGET_SORT_OPTIONS: {
  id: '-downloads' | '-rating' | '-releaseDate' | '-updateDate';
  label: string;
}[] = [
  { id: '-downloads', label: 'Most Downloaded' },
  { id: '-rating', label: 'Highest Rated' },
  { id: '-updateDate', label: 'Recently Updated' },
  { id: '-releaseDate', label: 'Newest' },
];

export interface SpigetSearchParams {
  query?: string;
  sort?: '-downloads' | '-rating' | '-releaseDate' | '-updateDate';
  limit?: number;
  page?: number;
}

/**
 * Searches SpigotMC resources or lists popular resources if query is empty.
 */
export async function searchSpiget(
  params: SpigetSearchParams,
  signal?: AbortSignal,
): Promise<SpigetResource[]> {
  const {
    query = '',
    sort = '-downloads',
    limit = 20,
    page = 1,
  } = params;

  const searchParams = new URLSearchParams();
  searchParams.set('size', String(limit));
  searchParams.set('page', String(page));
  searchParams.set('sort', sort);

  const cleanQuery = query.trim();
  let url: string;
  if (cleanQuery) {
    url = `${SPIGET_API_BASE}/search/resources/${encodeURIComponent(cleanQuery)}?${searchParams.toString()}`;
  } else {
    url = `${SPIGET_API_BASE}/resources?${searchParams.toString()}`;
  }

  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
      'User-Agent': 'mcm-panel/1.0',
    },
    signal,
  });

  if (!resp.ok) {
    throw new Error(`Spigot search failed with status ${resp.status}`);
  }

  const data = (await resp.json()) as SpigetResource[];
  return Array.isArray(data) ? data : [];
}

/**
 * Retrieves release versions for a Spigot resource.
 */
export async function getSpigetVersions(
  resourceId: number,
  signal?: AbortSignal,
): Promise<SpigetVersion[]> {
  const url = `${SPIGET_API_BASE}/resources/${resourceId}/versions?size=25&sort=-releaseDate`;

  const resp = await fetch(url, {
    headers: {
      Accept: 'application/json',
      'User-Agent': 'mcm-panel/1.0',
    },
    signal,
  });

  if (!resp.ok) {
    throw new Error(`Failed to fetch Spigot versions for ${resourceId} (${resp.status})`);
  }

  const data = (await resp.json()) as SpigetVersion[];
  return Array.isArray(data) ? data : [];
}

/**
 * Resolves the display icon image URL or data URI for a Spigot resource.
 */
export function getSpigetIconUrl(resource: SpigetResource): string | null {
  if (resource.icon?.data) {
    return `data:image/png;base64,${resource.icon.data}`;
  }
  if (resource.icon?.url) {
    if (resource.icon.url.startsWith('http')) {
      return resource.icon.url;
    }
    return `https://static.spigotmc.org/${resource.icon.url.replace(/^\/+/, '')}`;
  }
  return null;
}

/**
 * Constructs direct download URL for a Spigot resource jar.
 */
export function getSpigetDownloadUrl(resourceId: number): string {
  return `https://cdn.spiget.org/file/spiget-resources/${resourceId}.jar`;
}

/**
 * Returns a filesystem-safe jar filename for a Spigot resource.
 */
export function getSpigetSafeFilename(resourceName: string, versionName?: string): string {
  let base = resourceName.trim().replace(/[^a-zA-Z0-9._-]/g, '_');
  if (versionName && versionName.trim()) {
    const cleanVer = versionName.trim().replace(/[^a-zA-Z0-9._-]/g, '_');
    base = `${base}-${cleanVer}`;
  }
  if (!base.toLowerCase().endsWith('.jar')) {
    base = `${base}.jar`;
  }
  return base;
}

/**
 * Checks if a Spigot resource matches an installed mod.
 */
export function findInstalledSpigetResource(
  resource: Pick<SpigetResource, 'name' | 'id'>,
  installedMods: Mod[],
): Mod | undefined {
  const targetName = resource.name.toLowerCase().trim().replace(/[\s_]+/g, '-');

  return installedMods.find((m) => {
    const rawFile = m.file.replace(/\.jar(\.disabled)?$/i, '').toLowerCase();
    const rawName = m.name.toLowerCase();
    if (rawFile === targetName || rawName === targetName) {
      return true;
    }

    const fileModId = extractModId(m.file).replace(/_/g, '-');
    const nameModId = extractModId(m.name).replace(/_/g, '-');

    return fileModId === targetName || nameModId === targetName;
  });
}

export function isSpigetResourceInstalled(
  resource: Pick<SpigetResource, 'name' | 'id'>,
  installedMods: Mod[],
): boolean {
  return findInstalledSpigetResource(resource, installedMods) !== undefined;
}
