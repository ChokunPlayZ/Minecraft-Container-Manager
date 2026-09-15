import type {
  CurseForgeFile,
  CurseForgeMod,
  Mod,
  ModrinthDependency,
  ModrinthProject,
  ModrinthVersion,
} from './types';
import {
  findInstalledMod,
  getProjects,
  getProjectVersions,
  getVersions,
} from './modrinth';
import {
  findInstalledCurseForgeMod,
  getCurseForgeDownloadUrl,
  getCurseForgeFiles,
  getCurseForgeMods,
} from './curseforge';

export interface ResolvedDependency {
  id: string; // project_id or mod_id
  slug: string;
  title: string;
  iconUrl?: string | null;
  description?: string;
  provider: 'modrinth' | 'curseforge';
  dependencyType: 'required' | 'optional';
  isLibrary: boolean;
  alreadyInstalled: boolean;
  installedFile?: string;

  // Target download info if missing
  versionNumber?: string;
  filename?: string;
  downloadUrl?: string;
  fileSize?: number;
}

export interface DependencyResolutionResult {
  primaryTitle: string;
  dependencies: ResolvedDependency[];
  missingCount: number;
  satisfiedCount: number;
}

export interface ResolveModrinthDependenciesOptions {
  version: ModrinthVersion;
  primaryProject?: { id?: string; slug?: string; title: string };
  serverLoaders: string[];
  serverVersion?: string;
  installedMods: Mod[];
  hashProjectMap?: Map<string, string>;
  installedIds?: Set<string>;
  maxDepth?: number;
  signal?: AbortSignal;
}

/**
 * Recursively resolves library mods and other dependencies for a Modrinth mod version.
 */
export async function resolveModrinthDependencies(
  options: ResolveModrinthDependenciesOptions,
): Promise<DependencyResolutionResult> {
  const {
    version,
    primaryProject,
    serverLoaders,
    serverVersion,
    installedMods,
    hashProjectMap,
    installedIds,
    maxDepth = 2,
    signal,
  } = options;

  const resolvedMap = new Map<string, ResolvedDependency>();
  const visitedProjects = new Set<string>();
  const visitedVersions = new Set<string>();

  if (primaryProject?.id) visitedProjects.add(primaryProject.id);
  if (primaryProject?.slug) visitedProjects.add(primaryProject.slug.toLowerCase());
  visitedVersions.add(version.id);

  // Queue of dependencies to resolve
  let currentQueue: Array<{ dep: ModrinthDependency; depth: number }> = (version.dependencies || [])
    .filter((d) => d.dependency_type === 'required' || d.dependency_type === 'optional')
    .map((d) => ({ dep: d, depth: 1 }));

  while (currentQueue.length > 0) {
    // Collect all project IDs and version IDs needed in this pass
    const neededProjectIds = new Set<string>();
    const neededVersionIds = new Set<string>();

    for (const item of currentQueue) {
      if (item.dep.project_id && !visitedProjects.has(item.dep.project_id)) {
        neededProjectIds.add(item.dep.project_id);
      }
      if (item.dep.version_id && !visitedVersions.has(item.dep.version_id)) {
        neededVersionIds.add(item.dep.version_id);
      }
    }

    // Batch fetch projects and pinned versions in parallel
    const [fetchedProjects, fetchedVersions] = await Promise.all([
      neededProjectIds.size > 0 ? getProjects(Array.from(neededProjectIds), signal) : Promise.resolve([]),
      neededVersionIds.size > 0 ? getVersions(Array.from(neededVersionIds), signal) : Promise.resolve([]),
    ]);

    const projectLookup = new Map<string, ModrinthProject>();
    for (const p of fetchedProjects) {
      projectLookup.set(p.id, p);
      projectLookup.set(p.slug.toLowerCase(), p);
    }

    const versionLookup = new Map<string, ModrinthVersion>();
    for (const v of fetchedVersions) {
      versionLookup.set(v.id, v);
      visitedVersions.add(v.id);
      if (v.project_id && !projectLookup.has(v.project_id) && !visitedProjects.has(v.project_id)) {
        neededProjectIds.add(v.project_id);
      }
    }

    // If any versions referenced projects we don't have yet, fetch them
    const missingProjectIds = Array.from(neededProjectIds).filter((id) => !projectLookup.has(id));
    if (missingProjectIds.length > 0) {
      const moreProjects = await getProjects(missingProjectIds, signal);
      for (const p of moreProjects) {
        projectLookup.set(p.id, p);
        projectLookup.set(p.slug.toLowerCase(), p);
      }
    }

    const nextQueue: Array<{ dep: ModrinthDependency; depth: number }> = [];

    // Process each dependency in current queue
    for (const item of currentQueue) {
      const dep = item.dep;
      let project: ModrinthProject | undefined;

      if (dep.project_id) {
        project = projectLookup.get(dep.project_id);
      } else if (dep.version_id) {
        const v = versionLookup.get(dep.version_id);
        if (v?.project_id) {
          project = projectLookup.get(v.project_id);
        }
      }

      if (!project) continue;
      const pKey = project.id;
      if (visitedProjects.has(pKey) || visitedProjects.has(project.slug.toLowerCase())) {
        continue;
      }
      visitedProjects.add(pKey);
      visitedProjects.add(project.slug.toLowerCase());

      // Check if project is already installed
      const existingMod = findInstalledMod(project, installedMods, hashProjectMap);
      const isAlreadyInstalled =
        Boolean(existingMod) ||
        Boolean(installedIds?.has(project.id)) ||
        Boolean(installedIds?.has(project.slug.toLowerCase()));

      const isLibrary = project.categories?.includes('library') ?? false;

      if (isAlreadyInstalled) {
        resolvedMap.set(pKey, {
          id: project.id,
          slug: project.slug,
          title: project.title,
          iconUrl: project.icon_url,
          description: project.description,
          provider: 'modrinth',
          dependencyType: dep.dependency_type as 'required' | 'optional',
          isLibrary,
          alreadyInstalled: true,
          installedFile: existingMod?.file,
        });
        // Mod is already installed; no need to follow its transitive dependencies
        continue;
      }

      // Find compatible downloadable file
      let targetVersion: ModrinthVersion | undefined;
      if (dep.version_id && versionLookup.has(dep.version_id)) {
        targetVersion = versionLookup.get(dep.version_id);
      } else {
        // Query compatible versions for the server
        try {
          const versions = await getProjectVersions(
            project.slug,
            {
              loaders: serverLoaders.length > 0 ? serverLoaders : undefined,
              gameVersions: serverVersion ? [serverVersion] : undefined,
            },
            signal,
          );
          if (versions.length > 0) {
            targetVersion = versions[0];
          }
        } catch {
          // If query fails, continue with partial metadata
        }
      }

      const primaryFile = targetVersion
        ? targetVersion.files.find((f) => f.primary) || targetVersion.files[0]
        : undefined;

      resolvedMap.set(pKey, {
        id: project.id,
        slug: project.slug,
        title: project.title,
        iconUrl: project.icon_url,
        description: project.description,
        provider: 'modrinth',
        dependencyType: dep.dependency_type as 'required' | 'optional',
        isLibrary,
        alreadyInstalled: false,
        versionNumber: targetVersion?.version_number,
        filename: primaryFile?.filename,
        downloadUrl: primaryFile?.url,
        fileSize: primaryFile?.size,
      });

      // Recurse transitive dependencies if within maxDepth
      if (item.depth < maxDepth && targetVersion?.dependencies && targetVersion.dependencies.length > 0) {
        for (const subDep of targetVersion.dependencies) {
          if (subDep.dependency_type === 'required' || subDep.dependency_type === 'optional') {
            nextQueue.push({ dep: subDep, depth: item.depth + 1 });
          }
        }
      }
    }

    currentQueue = nextQueue;
  }

  const allDependencies = Array.from(resolvedMap.values());

  // Sort order:
  // 1. Missing required dependencies first
  // 2. Missing optional dependencies next
  // 3. Satisfied dependencies last
  allDependencies.sort((a, b) => {
    if (a.alreadyInstalled !== b.alreadyInstalled) {
      return a.alreadyInstalled ? 1 : -1;
    }
    if (a.dependencyType !== b.dependencyType) {
      return a.dependencyType === 'required' ? -1 : 1;
    }
    if (a.isLibrary !== b.isLibrary) {
      return a.isLibrary ? -1 : 1;
    }
    return a.title.localeCompare(b.title);
  });

  const missingCount = allDependencies.filter((d) => !d.alreadyInstalled).length;
  const satisfiedCount = allDependencies.filter((d) => d.alreadyInstalled).length;

  return {
    primaryTitle: primaryProject?.title || version.name,
    dependencies: allDependencies,
    missingCount,
    satisfiedCount,
  };
}

export interface ResolveCurseForgeDependenciesOptions {
  file: CurseForgeFile;
  primaryMod?: { id?: number; slug?: string; name: string };
  apiKey: string;
  gameVersion?: string;
  modLoaderType?: number;
  installedMods: Mod[];
  installedSlugs?: Set<string>;
  signal?: AbortSignal;
}

/**
 * Resolves dependencies for a CurseForge file.
 */
export async function resolveCurseForgeDependencies(
  options: ResolveCurseForgeDependenciesOptions,
): Promise<DependencyResolutionResult> {
  const {
    file,
    primaryMod,
    apiKey,
    gameVersion,
    modLoaderType,
    installedMods,
    installedSlugs,
    signal,
  } = options;

  if (!file.dependencies || file.dependencies.length === 0 || !apiKey.trim()) {
    return {
      primaryTitle: primaryMod?.name || file.displayName,
      dependencies: [],
      missingCount: 0,
      satisfiedCount: 0,
    };
  }

  // Filter dependencies: 3 = Required, 2 = Optional
  const validDeps = file.dependencies.filter(
    (d) => d.relationType === 3 || d.relationType === 2,
  );

  if (validDeps.length === 0) {
    return {
      primaryTitle: primaryMod?.name || file.displayName,
      dependencies: [],
      missingCount: 0,
      satisfiedCount: 0,
    };
  }

  const modIds = validDeps.map((d) => d.modId);
  const fetchedMods = await getCurseForgeMods(modIds, apiKey, signal);
  const modMap = new Map<number, CurseForgeMod>();
  for (const m of fetchedMods) {
    modMap.set(m.id, m);
  }

  const resolved: ResolvedDependency[] = [];

  for (const dep of validDeps) {
    const cfMod = modMap.get(dep.modId);
    if (!cfMod) continue;

    const existingMod = findInstalledCurseForgeMod(cfMod, installedMods);
    const isAlreadyInstalled =
      Boolean(existingMod) ||
      Boolean(installedSlugs?.has(cfMod.slug)) ||
      Boolean(installedSlugs?.has(cfMod.name.toLowerCase()));

    const depType: 'required' | 'optional' = dep.relationType === 3 ? 'required' : 'optional';
    // Classify library: CurseForge categories often include 'library' or classId 6
    const isLibrary = cfMod.categories?.some((c) =>
      c.name.toLowerCase().includes('library') || c.slug.toLowerCase().includes('library') || c.slug.toLowerCase().includes('api'),
    ) ?? false;

    if (isAlreadyInstalled) {
      resolved.push({
        id: String(cfMod.id),
        slug: cfMod.slug,
        title: cfMod.name,
        iconUrl: cfMod.logo?.thumbnailUrl || cfMod.logo?.url,
        description: cfMod.summary,
        provider: 'curseforge',
        dependencyType: depType,
        isLibrary,
        alreadyInstalled: true,
        installedFile: existingMod?.file,
      });
      continue;
    }

    // Fetch compatible file
    let targetFile: CurseForgeFile | undefined;
    try {
      const fls = await getCurseForgeFiles(cfMod.id, apiKey, {
        gameVersion,
        modLoaderType,
      }, signal);
      targetFile = fls.find((f) => f.fileName.toLowerCase().endsWith('.jar')) || fls[0];
    } catch {
      // ignore
    }

    let downloadUrl = targetFile?.downloadUrl ?? undefined;
    if (!downloadUrl && targetFile) {
      try {
        downloadUrl = await getCurseForgeDownloadUrl(cfMod.id, targetFile.id, apiKey, signal);
      } catch {
        // ignore
      }
    }

    resolved.push({
      id: String(cfMod.id),
      slug: cfMod.slug,
      title: cfMod.name,
      iconUrl: cfMod.logo?.thumbnailUrl || cfMod.logo?.url,
      description: cfMod.summary,
      provider: 'curseforge',
      dependencyType: depType,
      isLibrary,
      alreadyInstalled: false,
      versionNumber: targetFile?.displayName || targetFile?.fileName,
      filename: targetFile?.fileName,
      downloadUrl,
      fileSize: targetFile?.fileLength,
    });
  }

  resolved.sort((a, b) => {
    if (a.alreadyInstalled !== b.alreadyInstalled) {
      return a.alreadyInstalled ? 1 : -1;
    }
    if (a.dependencyType !== b.dependencyType) {
      return a.dependencyType === 'required' ? -1 : 1;
    }
    return a.title.localeCompare(b.title);
  });

  return {
    primaryTitle: primaryMod?.name || file.displayName,
    dependencies: resolved,
    missingCount: resolved.filter((d) => !d.alreadyInstalled).length,
    satisfiedCount: resolved.filter((d) => d.alreadyInstalled).length,
  };
}
