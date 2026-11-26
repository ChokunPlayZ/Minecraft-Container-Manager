import { curseForgeChannel, matchesInstalledArtifact, normalizeModChannel, spigetChannel } from './mod-channels';
import type { Mod, ModProvider, Server } from './types';
import {
  getProjectVersions,
  getServerLoaders,
} from './modrinth';
import { getHangarVersions } from './hangar';
import { getSpigetDownloadUrl, getSpigetVersions } from './spiget';
import { getCurseForgeFiles } from './curseforge';

export interface AvailableModJar {
  hashes?: Record<string, string>;
  versionId: string;
  versionName: string;
  versionNumber: string;
  filename: string;
  downloadUrl: string;
  sizeBytes?: number;
  releaseType?: string;
  provider?: Mod['provider'];
  projectId?: string;
  projectSlug?: string;
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
          const isCurrent = matchesInstalledArtifact(mod, file.filename, file.hashes?.sha1);

          jars.push({
            versionId: ver.id,
            provider,
            projectId: ver.project_id,
            versionName: ver.name || ver.version_number,
            versionNumber: ver.version_number,
            filename: file.filename,
            downloadUrl: file.url,
            hashes: file.hashes,
            sizeBytes: file.size,
            releaseType: normalizeModChannel(ver.version_type),
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
            releaseType: normalizeModChannel(ver.channel?.name),
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
          const isCurrent = mod.catalog_version_id
            ? mod.catalog_version_id === String(ver.id)
            : mod.version && ver.name && mod.version.trim() === ver.name.trim();
          return {
            versionId: String(ver.id),
            versionName: ver.name,
            versionNumber: ver.name,
            filename: mod.file.replace(/\.disabled$/, ''),
            downloadUrl: getSpigetDownloadUrl(resourceId, ver.id),
            releaseType: spigetChannel(ver.name),
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
  if (provider === 'curseforge') {
    const modId = parseInt(updateInfo?.projectId || mod.project_id || '', 10);
    if (!isNaN(modId)) {
      try {
        const files = await getCurseForgeFiles(
          modId,
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
              releaseType: curseForgeChannel(f.releaseType),
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
