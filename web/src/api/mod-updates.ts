import type { Mod, ModProvider, ModrinthVersion, Server } from './types';
import {
  checkModrinthUpdates,
  getProjectVersions,
  getServerLoaders,
  isVersionFileInstalled,
} from './modrinth';
import { getHangarVersions } from './hangar';
import { getSpigetVersions } from './spiget';
import { getCurseForgeApiKey, getCurseForgeFiles, hasCurseForgeApiKey } from './curseforge';

export interface AvailableModJar {
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
 * Checks all installed mods on the server for newer jar versions.
 * Queries Modrinth hash update API, Hangar, Spiget, and CurseForge APIs.
 */
export async function checkModsForUpdates(
  installedMods: Mod[],
  server: Server,
  signal?: AbortSignal,
): Promise<Record<string, ModUpdateInfo>> {
  if (installedMods.length === 0) return {};

  const updates: Record<string, ModUpdateInfo> = {};
  const serverLoaders = getServerLoaders(server.server_type);
  const serverVersion = server.version?.trim();

  // 1. Batch check via Modrinth for all mods with SHA1 hashes
  const modsWithSha1 = installedMods.filter((m) => Boolean(m.sha1 && m.sha1.length > 0));
  if (modsWithSha1.length > 0) {
    const hashes = modsWithSha1.map((m) => m.sha1 as string);
    try {
      const modrinthUpdates = await checkModrinthUpdates(
        {
          hashes,
          algorithm: 'sha1',
          loaders: serverLoaders.length > 0 ? serverLoaders : undefined,
          gameVersions: serverVersion ? [serverVersion] : undefined,
        },
        signal,
      );

      for (const mod of modsWithSha1) {
        if (!mod.sha1) continue;
        const ver: ModrinthVersion | undefined = modrinthUpdates[mod.sha1];
        if (!ver || !ver.files || ver.files.length === 0) continue;

        const primaryFile = ver.files.find((f) => f.primary) || ver.files[0];
        if (!primaryFile) continue;

        const isSameHash =
          primaryFile.hashes?.sha1 &&
          primaryFile.hashes.sha1.toLowerCase() === mod.sha1.toLowerCase();
        const isSameFilename =
          primaryFile.filename.toLowerCase() === mod.file.toLowerCase() ||
          `${primaryFile.filename.toLowerCase()}.disabled` === mod.file.toLowerCase() ||
          primaryFile.filename.toLowerCase() === `${mod.file.toLowerCase()}.disabled`;

        // If the hash is identical and filename is identical, it's up to date
        if (isSameHash && isSameFilename) {
          continue;
        }

        // Newer version detected!
        updates[mod.name] = {
          modName: mod.name,
          modFile: mod.file,
          provider: 'modrinth',
          projectId: ver.project_id,
          projectSlug: mod.project_slug,
          title: mod.title || mod.name,
          currentVersion: mod.version,
          latestVersion: ver.version_number || ver.name,
          latestJar: primaryFile.filename,
          latestDownloadUrl: primaryFile.url,
          latestReleaseType: ver.version_type,
          latestReleaseDate: ver.date_published,
          changelog: ver.changelog,
        };
      }
    } catch {
      // Modrinth batch query failed, fallback per-mod
    }
  }

  // 2. For remaining mods not yet identified, check their specific providers
  for (const mod of installedMods) {
    if (updates[mod.name]) continue;

    try {
      // Hangar provider
      if (mod.provider === 'hangar' && (mod.project_slug || mod.project_id)) {
        const slug = mod.project_slug || mod.project_id;
        if (!slug) continue;
        const vers = await getHangarVersions(slug, { limit: 5 }, signal);
        if (vers.length > 0) {
          const latest = vers[0];
          const dl = latest.downloads.PAPER;
          if (dl?.fileInfo) {
            const isSame =
              dl.fileInfo.name.toLowerCase() === mod.file.toLowerCase() ||
              `${dl.fileInfo.name.toLowerCase()}.disabled` === mod.file.toLowerCase();
            if (!isSame) {
              updates[mod.name] = {
                modName: mod.name,
                modFile: mod.file,
                provider: 'hangar',
                projectSlug: slug,
                title: mod.title || mod.name,
                currentVersion: mod.version,
                latestVersion: latest.name,
                latestJar: dl.fileInfo.name,
                latestDownloadUrl: dl.downloadUrl,
                latestReleaseDate: latest.createdAt,
              };
            }
          }
        }
        continue;
      }

      // Spiget provider
      if (mod.provider === 'spiget' && mod.project_id) {
        const resourceId = parseInt(mod.project_id, 10);
        if (!isNaN(resourceId)) {
          const vers = await getSpigetVersions(resourceId, signal);
          if (vers.length > 0) {
            const latest = vers[0];
            const isSameVersion =
              mod.version && latest.name && mod.version.trim() === latest.name.trim();
            if (!isSameVersion) {
              updates[mod.name] = {
                modName: mod.name,
                modFile: mod.file,
                provider: 'spiget',
                projectId: String(resourceId),
                title: mod.title || mod.name,
                currentVersion: mod.version,
                latestVersion: latest.name,
                latestJar: `${mod.name}.jar`,
                latestDownloadUrl: `https://cdn.spiget.org/file/spiget-resources/${resourceId}.jar`,
                latestReleaseDate: latest.releaseDate
                  ? new Date(latest.releaseDate * 1000).toISOString()
                  : undefined,
              };
            }
          }
        }
        continue;
      }

      // CurseForge provider
      if (mod.provider === 'curseforge' && mod.project_id && hasCurseForgeApiKey()) {
        const modId = parseInt(mod.project_id, 10);
        if (!isNaN(modId)) {
          const files = await getCurseForgeFiles(
            modId,
            getCurseForgeApiKey(),
            { gameVersion: serverVersion },
            signal,
          );
          if (files.length > 0) {
            const latest = files[0];
            const isSame =
              latest.fileName.toLowerCase() === mod.file.toLowerCase() ||
              `${latest.fileName.toLowerCase()}.disabled` === mod.file.toLowerCase();
            if (!isSame && latest.downloadUrl) {
              updates[mod.name] = {
                modName: mod.name,
                modFile: mod.file,
                provider: 'curseforge',
                projectId: String(modId),
                title: mod.title || mod.name,
                currentVersion: mod.version,
                latestVersion: latest.displayName || latest.fileName,
                latestJar: latest.fileName,
                latestDownloadUrl: latest.downloadUrl,
                latestReleaseDate: latest.fileDate,
              };
            }
          }
        }
        continue;
      }

      // Fallback: Modrinth lookup by project_slug / project_id / mod_id
      const slugOrId = mod.project_slug || mod.project_id || mod.mod_id;
      if (slugOrId) {
        const vers = await getProjectVersions(
          slugOrId,
          {
            loaders: serverLoaders.length > 0 ? serverLoaders : undefined,
            gameVersions: serverVersion ? [serverVersion] : undefined,
          },
          signal,
        );
        if (vers.length > 0) {
          const latest = vers[0];
          const primaryFile = latest.files.find((f) => f.primary) || latest.files[0];
          if (primaryFile) {
            const isSameFilename =
              primaryFile.filename.toLowerCase() === mod.file.toLowerCase() ||
              `${primaryFile.filename.toLowerCase()}.disabled` === mod.file.toLowerCase();
            const isSameHash =
              mod.sha1 &&
              primaryFile.hashes?.sha1 &&
              primaryFile.hashes.sha1.toLowerCase() === mod.sha1.toLowerCase();

            if (!isSameFilename && !isSameHash) {
              updates[mod.name] = {
                modName: mod.name,
                modFile: mod.file,
                provider: 'modrinth',
                projectId: latest.project_id,
                projectSlug: mod.project_slug,
                title: mod.title || mod.name,
                currentVersion: mod.version,
                latestVersion: latest.version_number || latest.name,
                latestJar: primaryFile.filename,
                latestDownloadUrl: primaryFile.url,
                latestReleaseType: latest.version_type,
                latestReleaseDate: latest.date_published,
                changelog: latest.changelog,
              };
            }
          }
        }
      }
    } catch {
      // Ignore individual mod check failure
    }
  }

  return updates;
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
