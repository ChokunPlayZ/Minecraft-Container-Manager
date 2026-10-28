import type { Mod, ModProvider, Server } from './types';
import {
  getProjectVersions,
  getServerLoaders,
  isVersionFileInstalled,
} from './modrinth';
import { getHangarVersions } from './hangar';
import { getSpigetVersions } from './spiget';
import { getCurseForgeApiKey, getCurseForgeFiles, hasCurseForgeApiKey } from './curseforge';

export interface AvailableModJar {
  hashes?: Record<string, string>;
  versionId: string;
  versionName: string;
  versionNumber: string;
  filename: string;
  downloadUrl: string;
  sizeBytes?: number;
  releaseType?: 'release' | 'beta' | 'alpha' | string;
  gameVersions?: string[];
  loaders?: string[];
  datePublished?: string;
  isCurrent?: boolean;
}

export interface ModUpdateInfo {
  modName: string;
  modFile: string;
  provider: ModProvider;
  projectId?: string;
  projectSlug?: string;
  title: string;
  currentVersion?: string;
  latestVersion: string;
  latestJar: string;
  latestDownloadUrl?: string;
  latestReleaseType?: string;
  latestReleaseDate?: string;
  changelog?: string;
  availableJars?: AvailableModJar[];
}

/**
 * Fetches all available jar releases for a specific mod so the user can pick which jar to install.
 */
export async function fetchModAvailableJars(
  mod: Mod,
  updateInfo: ModUpdateInfo | undefined,
  server: Server,
  signal?: AbortSignal,
): Promise<AvailableModJar[]> {
  const provider = updateInfo?.provider || mod.provider || 'modrinth';
  const serverLoaders = getServerLoaders(server.server_type);
  const serverVersion = server.version?.trim();

  // 1. Modrinth versions
  if (provider === 'modrinth') {
    const slugOrId =
      updateInfo?.projectId ||
      updateInfo?.projectSlug ||
      mod.project_id ||
      mod.project_slug ||
      mod.mod_id ||
      mod.name.toLowerCase();

    try {
      const vers = await getProjectVersions(
        slugOrId,
        {
          loaders: serverLoaders.length > 0 ? serverLoaders : undefined,
          gameVersions: serverVersion ? [serverVersion] : undefined,
        },
        signal,
      );

      const jars: AvailableModJar[] = [];
      for (const ver of vers) {
        for (const file of ver.files) {
          if (!file.filename.toLowerCase().endsWith('.jar')) continue;
          const isCurrent =
            file.filename.toLowerCase() === mod.file.toLowerCase() ||
            `${file.filename.toLowerCase()}.disabled` === mod.file.toLowerCase() ||
            isVersionFileInstalled(file, [mod]);

          jars.push({
            versionId: ver.id,
            versionName: ver.name || ver.version_number,
            versionNumber: ver.version_number,
            filename: file.filename,
            downloadUrl: file.url,
            hashes: file.hashes,
            sizeBytes: file.size,
            releaseType: ver.version_type,
            gameVersions: ver.game_versions,
            loaders: ver.loaders,
            datePublished: ver.date_published,
            isCurrent,
          });
        }
      }
      return jars;
    } catch {
      return [];
    }
  }

  // 2. Hangar versions
  if (provider === 'hangar') {
    const slug = updateInfo?.projectSlug || mod.project_slug || mod.project_id || mod.name;
    try {
      const vers = await getHangarVersions(slug, { limit: 25 }, signal);
      const jars: AvailableModJar[] = [];
      for (const ver of vers) {
        const dl = ver.downloads.PAPER;
        if (dl && dl.downloadUrl && dl.fileInfo) {
          const isCurrent =
            dl.fileInfo.name.toLowerCase() === mod.file.toLowerCase() ||
            `${dl.fileInfo.name.toLowerCase()}.disabled` === mod.file.toLowerCase();

          jars.push({
            versionId: String(ver.id),
            versionName: ver.name,
            versionNumber: ver.name,
            filename: dl.fileInfo.name,
            downloadUrl: dl.downloadUrl,
            sizeBytes: dl.fileInfo.sizeBytes,
            releaseType: ver.channel?.name?.toLowerCase(),
            gameVersions: ver.platformDependencies?.PAPER,
            loaders: ['paper'],
            datePublished: ver.createdAt,
            isCurrent,
          });
        }
      }
      return jars;
    } catch {
      return [];
    }
  }

  // 3. Spiget versions
  if (provider === 'spiget') {
    const resourceId = parseInt(updateInfo?.projectId || mod.project_id || '', 10);
    if (!isNaN(resourceId)) {
      try {
        const vers = await getSpigetVersions(resourceId, signal);
        return vers.map((ver) => {
          const isCurrent =
            mod.version && ver.name && mod.version.trim() === ver.name.trim();
          return {
            versionId: String(ver.id),
            versionName: ver.name,
            versionNumber: ver.name,
            filename: `${mod.name}-${ver.name}.jar`,
            downloadUrl: `https://cdn.spiget.org/file/spiget-resources/${resourceId}.jar`,
            releaseType: 'release',
            loaders: ['spigot', 'paper'],
            datePublished: ver.releaseDate
              ? new Date(ver.releaseDate * 1000).toISOString()
              : undefined,
            isCurrent: Boolean(isCurrent),
          };
        });
      } catch {
        return [];
      }
    }
  }

  // 4. CurseForge files
  if (provider === 'curseforge' && hasCurseForgeApiKey()) {
    const modId = parseInt(updateInfo?.projectId || mod.project_id || '', 10);
    if (!isNaN(modId)) {
      try {
        const files = await getCurseForgeFiles(
          modId,
          getCurseForgeApiKey(),
          { gameVersion: serverVersion },
          signal,
        );
        return files
          .filter((f) => f.downloadUrl && f.fileName.toLowerCase().endsWith('.jar'))
          .map((f) => {
            const isCurrent =
              f.fileName.toLowerCase() === mod.file.toLowerCase() ||
              `${f.fileName.toLowerCase()}.disabled` === mod.file.toLowerCase();
            return {
              versionId: String(f.id),
              versionName: f.displayName || f.fileName,
              versionNumber: f.displayName,
              filename: f.fileName,
              downloadUrl: f.downloadUrl as string,
              sizeBytes: f.fileLength,
              releaseType: f.releaseType === 1 ? 'release' : f.releaseType === 2 ? 'beta' : 'alpha',
              gameVersions: f.gameVersions,
              datePublished: f.fileDate,
              isCurrent,
            };
          });
      } catch {
        return [];
      }
    }
  }

  return [];
}
