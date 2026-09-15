import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Check,
  Download,
  ExternalLink,
  Filter,
  Info,
  Layers,
  Loader2,
  Package,
  RefreshCw,
  Search,
  Server as ServerIcon,
  SlidersHorizontal,
  Sparkles,
  Trash2,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import { isPluginServerType } from '../api/curseforge';
import {
  extractModId,
  findInstalledMod,
  formatCount,
  formatFileSize,
  getLoaderLabel,
  getServerLoaders,
  getProjectVersions,
  isModInstalled,
  isVersionFileInstalled,
  lookupModrinthVersionFiles,
  searchModrinth,
} from '../api/modrinth';
import type {
  Mod,
  ModUpdateInfo,
  ModrinthSearchHit,
  ModrinthVersion,
  ModrinthVersionFile,
  Server,
} from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { ModUpdateDialog } from './mod-update-dialog';
import { ModDependenciesDialog } from './mod-dependencies-dialog';
import {
  resolveModrinthDependencies,
  type ResolvedDependency,
} from '../api/mod-dependencies';
import { PluginCard, PluginEmptyState, PluginGridSkeleton } from './plugin-card';
import { useModal } from './ui/modal';

const CATEGORIES = [
  { id: 'all', label: 'All' },
  { id: 'optimization', label: 'Optimization' },
  { id: 'management', label: 'Administration' },
  { id: 'utility', label: 'Utility' },
  { id: 'game-mechanics', label: 'Gameplay' },
  { id: 'economy', label: 'Economy' },
  { id: 'worldgen', label: 'World Gen' },
  { id: 'adventure', label: 'Adventure' },
  { id: 'social', label: 'Social' },
  { id: 'library', label: 'Library' },
];

const SORT_OPTIONS: { id: 'downloads' | 'relevance' | 'follows' | 'updated' | 'newest'; label: string }[] = [
  { id: 'downloads', label: 'Most Downloaded' },
  { id: 'relevance', label: 'Relevance' },
  { id: 'follows', label: 'Most Followed' },
  { id: 'updated', label: 'Recently Updated' },
  { id: 'newest', label: 'Newest' },
];

interface ModrinthBrowserProps {
  server: Server;
  installedMods: Mod[];
  updates?: Record<string, ModUpdateInfo>;
  onOpenPicker?: (mod: Mod, updateInfo?: ModUpdateInfo) => void;
  onModInstalled: (mod: Mod) => void;
  onModDeleted?: (modName: string) => void;
}

export function ModrinthBrowser({
  server,
  installedMods,
  updates,
  onOpenPicker,
  onModInstalled,
  onModDeleted,
}: ModrinthBrowserProps) {
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('all');
  const [sort, setSort] = useState<'downloads' | 'relevance' | 'follows' | 'updated' | 'newest'>('downloads');

  // Matching toggles
  const [matchLoader, setMatchLoader] = useState(true);
  const [matchVersion, setMatchVersion] = useState(true);
  const [serverSideOnly, setServerSideOnly] = useState(true);
  const [onlyServerSide, setOnlyServerSide] = useState(false);

  // Results & pagination
  const [results, setResults] = useState<ModrinthSearchHit[]>([]);
  const [totalHits, setTotalHits] = useState(0);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Installation state
  const [installingId, setInstallingId] = useState<string | null>(null);
  const [installedIds, setInstalledIds] = useState<Set<string>>(new Set());
  const [installedFiles, setInstalledFiles] = useState<Set<string>>(new Set());
  const [hashProjectMap, setHashProjectMap] = useState<Map<string, string>>(new Map());
  const [notification, setNotification] = useState<{ text: string; type: 'success' | 'error' } | null>(null);

  // Batch query Modrinth for installed mod SHA1 hashes
  useEffect(() => {
    const hashes = installedMods
      .map((m) => m.sha1)
      .filter((h): h is string => Boolean(h && h.length > 0));

    if (hashes.length === 0) return;

    let isMounted = true;
    const controller = new AbortController();

    lookupModrinthVersionFiles(hashes, 'sha1', controller.signal)
      .then((data) => {
        if (!isMounted) return;
        const newMap = new Map<string, string>();
        const newProjectIds = new Set<string>();
        for (const [hash, ver] of Object.entries(data)) {
          if (ver.project_id) {
            newMap.set(hash, ver.project_id);
            newProjectIds.add(ver.project_id);
          }
        }
        setHashProjectMap(newMap);
        setInstalledIds((prev) => new Set([...prev, ...newProjectIds]));
      })
      .catch(() => {
        // network error / offline - fallback works seamlessly
      });

    return () => {
      isMounted = false;
      controller.abort();
    };
  }, [installedMods]);

  // Project details & versions dialog
  const [selectedProject, setSelectedProject] = useState<ModrinthSearchHit | null>(null);
  const [versions, setVersions] = useState<ModrinthVersion[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [versionError, setVersionError] = useState<string | null>(null);

  const loaderLabel = getLoaderLabel(server.server_type);
  const serverLoaders = useMemo(() => getServerLoaders(server.server_type), [server.server_type]);
  const activeLoaders = useMemo(
    () => (matchLoader ? serverLoaders : []),
    [matchLoader, serverLoaders],
  );
  const activeVersion = matchVersion ? server.version : undefined;

  const abortControllerRef = useRef<AbortController | null>(null);
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const isFetchingRef = useRef(false);
  const offsetRef = useRef(0);

  const performSearch = useCallback(
    async (resetOffset = true) => {
      if (isFetchingRef.current && !resetOffset) return;
      isFetchingRef.current = true;

      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
      const controller = new AbortController();
      abortControllerRef.current = controller;

      const currentOffset = resetOffset ? 0 : offsetRef.current;
      if (resetOffset) {
        setLoading(true);
        setError(null);
        offsetRef.current = 0;
      } else {
        setLoadingMore(true);
      }

      try {
        const data = await searchModrinth(
          {
            query,
            loaders: activeLoaders,
            gameVersion: activeVersion,
            category,
            serverSideOnly,
            onlyServerSide,
            sort,
            offset: currentOffset,
            limit: 20,
          },
          controller.signal,
        );

        if (resetOffset) {
          setResults(data.hits);
          offsetRef.current = data.hits.length;
        } else {
          setResults((prev) => [...prev, ...data.hits]);
          offsetRef.current = currentOffset + data.hits.length;
        }
        setTotalHits(data.total_hits);
      } catch (err: unknown) {
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : 'Failed to search Modrinth');
      } finally {
        isFetchingRef.current = false;
        setLoading(false);
        setLoadingMore(false);
      }
    },
    [query, activeLoaders, activeVersion, category, serverSideOnly, onlyServerSide, sort],
  );

  // Trigger search on filter changes with debounce for query
  useEffect(() => {
    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }
    searchTimeoutRef.current = setTimeout(() => {
      void performSearch(true);
    }, 250);

    return () => {
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    };
  }, [performSearch]);

  const hasMore = results.length < totalHits && results.length > 0;
  const supportsIntersectionObserver =
    typeof window !== 'undefined' && 'IntersectionObserver' in window;

  // Infinite scroll lazy loading
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel || !hasMore || loading || loadingMore || !supportsIntersectionObserver) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (entry.isIntersecting && !isFetchingRef.current) {
          void performSearch(false);
        }
      },
      {
        rootMargin: '400px',
        threshold: 0,
      },
    );

    observer.observe(sentinel);
    return () => {
      observer.disconnect();
    };
  }, [hasMore, loading, loadingMore, supportsIntersectionObserver, performSearch]);


  // Dismiss notification after 4s
  useEffect(() => {
    if (!notification) return;
    const t = setTimeout(() => setNotification(null), 4000);
    return () => clearTimeout(t);
  }, [notification]);

  // Load versions when a project is selected
  useEffect(() => {
    if (!selectedProject) {
      setVersions([]);
      setVersionError(null);
      return;
    }

    let isMounted = true;
    setLoadingVersions(true);
    setVersionError(null);

    getProjectVersions(selectedProject.slug, {
      loaders: activeLoaders.length > 0 ? activeLoaders : undefined,
      gameVersions: activeVersion ? [activeVersion] : undefined,
    })
      .then((data) => {
        if (!isMounted) return;
        setVersions(data);
      })
      .catch((err) => {
        if (!isMounted) return;
        setVersionError(err instanceof Error ? err.message : 'Failed to load versions');
      })
      .finally(() => {
        if (isMounted) setLoadingVersions(false);
      });

    return () => {
      isMounted = false;
    };
  }, [selectedProject, activeLoaders, activeVersion]);

  const { confirm, dialog } = useModal();
  const [updatePrompt, setUpdatePrompt] = useState<{
    projectTitle: string;
    existingMod: Mod;
    targetUrl: string;
    targetFilename: string;
    projectId?: string;
    projectSlug?: string;
  } | null>(null);

  const [dependencyPrompt, setDependencyPrompt] = useState<{
    primaryMod: {
      title: string;
      versionNumber?: string;
      filename: string;
      iconUrl?: string | null;
      url: string;
      projectId?: string;
      projectSlug?: string;
    };
    dependencies: ResolvedDependency[];
  } | null>(null);

  const [isBatchInstalling, setIsBatchInstalling] = useState(false);
  const [installProgress, setInstallProgress] = useState<{
    current: number;
    total: number;
    currentName: string;
  } | null>(null);

  function findExistingOldJar(
    targetFilename: string,
    project?: Pick<ModrinthSearchHit, 'slug' | 'title'> & { project_id?: string },
  ): Mod | undefined {
    let existingMod = project ? findInstalledMod(project, installedMods, hashProjectMap) : undefined;
    if (!existingMod) {
      const baseId = extractModId(targetFilename).replace(/_/g, '-');
      if (baseId.length > 0) {
        existingMod = installedMods.find((m) => {
          if (m.mod_id) {
            return m.mod_id.toLowerCase().replace(/_/g, '-') === baseId;
          }
          const mId = extractModId(m.file).replace(/_/g, '-');
          return mId === baseId;
        });
      }
    }
    if (!existingMod) return undefined;

    const isSameFile =
      existingMod.file.toLowerCase() === targetFilename.toLowerCase() ||
      existingMod.file.toLowerCase() === `${targetFilename.toLowerCase()}.disabled`;
    if (isSameFile) return undefined;

    return existingMod;
  }

  async function executeInstall(
    url: string,
    filename: string,
    title: string,
    projectId?: string,
    projectSlug?: string,
    deleteOldMod?: Mod,
  ) {
    setInstallingId(projectId || url);
    setError(null);

    try {
      const mod = deleteOldMod
        ? await api.downloadMod(
            server.id,
            url,
            filename,
            deleteOldMod.name,
            { projectId, projectSlug, provider: 'modrinth' },
          )
        : await api.downloadMod(
            server.id,
            url,
            filename,
            undefined,
            { projectId, projectSlug, provider: 'modrinth' },
          );
      onModInstalled(mod);
      if (deleteOldMod) {
        onModDeleted?.(deleteOldMod.name);
        setInstalledFiles((prev) => {
          const next = new Set(prev);
          next.delete(deleteOldMod.file.toLowerCase());
          next.delete(`${deleteOldMod.file.toLowerCase()}.disabled`);
          return next;
        });
      }

      if (projectId || projectSlug) {
        setInstalledIds(
          (prev) =>
            new Set([
              ...prev,
              ...(projectId ? [projectId] : []),
              ...(projectSlug ? [projectSlug] : []),
            ]),
        );
      }
      setInstalledFiles((prev) => new Set([...prev, filename.toLowerCase()]));
      setNotification({
        type: 'success',
        text: deleteOldMod
          ? `Installed ${title} (${filename}) and deleted old jar`
          : `Installed ${title} (${filename})`,
      });
      setUpdatePrompt(null);
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Download failed',
      });
    } finally {
      setInstallingId(null);
    }
  }

  async function handleDeleteMod(mod: Mod) {
    if (
      !(await confirm(`Delete ${mod.file} from the server? This cannot be undone.`, {
        title: 'Delete Installed Jar',
        confirmLabel: 'Delete',
        destructive: true,
      }))
    ) {
      return;
    }

    try {
      await api.deleteMod(server.id, mod.name);
      onModDeleted?.(mod.name);
      setInstalledFiles((prev) => {
        const next = new Set(prev);
        next.delete(mod.file.toLowerCase());
        next.delete(`${mod.file.toLowerCase()}.disabled`);
        return next;
      });
      if (selectedProject) {
        setInstalledIds((prev) => {
          const next = new Set(prev);
          next.delete(selectedProject.project_id);
          next.delete(selectedProject.slug);
          return next;
        });
      }
      setNotification({
        type: 'success',
        text: `Deleted ${mod.file} from server`,
      });
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Delete failed',
      });
    }
  }

  async function handleConfirmInstallWithDependencies(selectedDeps: ResolvedDependency[]) {
    if (!dependencyPrompt) return;
    const { primaryMod } = dependencyPrompt;
    setIsBatchInstalling(true);
    setError(null);

    const totalCount = 1 + selectedDeps.length;

    try {
      // 1. Install primary mod
      setInstallProgress({
        current: 1,
        total: totalCount,
        currentName: primaryMod.title,
      });

      const primaryExistingMod = findExistingOldJar(primaryMod.filename, {
        slug: primaryMod.projectSlug || '',
        title: primaryMod.title,
        project_id: primaryMod.projectId,
      });

      const downloadedPrimary = primaryExistingMod
        ? await api.downloadMod(
            server.id,
            primaryMod.url,
            primaryMod.filename,
            primaryExistingMod.name,
            { projectId: primaryMod.projectId, projectSlug: primaryMod.projectSlug, provider: 'modrinth' },
          )
        : await api.downloadMod(
            server.id,
            primaryMod.url,
            primaryMod.filename,
            undefined,
            { projectId: primaryMod.projectId, projectSlug: primaryMod.projectSlug, provider: 'modrinth' },
          );

      onModInstalled(downloadedPrimary);
      if (primaryExistingMod) {
        onModDeleted?.(primaryExistingMod.name);
        setInstalledFiles((prev) => {
          const next = new Set(prev);
          next.delete(primaryExistingMod.file.toLowerCase());
          next.delete(`${primaryExistingMod.file.toLowerCase()}.disabled`);
          return next;
        });
      }

      setInstalledFiles((prev) => new Set([...prev, primaryMod.filename.toLowerCase()]));
      if (primaryMod.projectId || primaryMod.projectSlug) {
        setInstalledIds((prev) => {
          const next = new Set(prev);
          if (primaryMod.projectId) next.add(primaryMod.projectId);
          if (primaryMod.projectSlug) next.add(primaryMod.projectSlug);
          return next;
        });
      }

      // 2. Install each selected dependency
      for (let i = 0; i < selectedDeps.length; i++) {
        const dep = selectedDeps[i];
        if (!dep.downloadUrl || !dep.filename) continue;

        setInstallProgress({
          current: i + 2,
          total: totalCount,
          currentName: dep.title,
        });

        const depExistingMod = findExistingOldJar(dep.filename, {
          slug: dep.slug,
          title: dep.title,
          project_id: dep.id,
        });

        const downloadedDep = depExistingMod
          ? await api.downloadMod(
              server.id,
              dep.downloadUrl,
              dep.filename,
              depExistingMod.name,
              { projectId: dep.id, projectSlug: dep.slug, provider: 'modrinth' },
            )
          : await api.downloadMod(
              server.id,
              dep.downloadUrl,
              dep.filename,
              undefined,
              { projectId: dep.id, projectSlug: dep.slug, provider: 'modrinth' },
            );

        onModInstalled(downloadedDep);
        if (depExistingMod) {
          onModDeleted?.(depExistingMod.name);
          setInstalledFiles((prev) => {
            const next = new Set(prev);
            next.delete(depExistingMod.file.toLowerCase());
            next.delete(`${depExistingMod.file.toLowerCase()}.disabled`);
            return next;
          });
        }

        setInstalledFiles((prev) => new Set([...prev, dep.filename!.toLowerCase()]));
        setInstalledIds((prev) => {
          const next = new Set(prev);
          next.add(dep.id);
          next.add(dep.slug);
          return next;
        });
      }

      setNotification({
        type: 'success',
        text:
          selectedDeps.length > 0
            ? `Successfully installed ${primaryMod.title} and ${selectedDeps.length} dependencies`
            : `Installed ${primaryMod.title}`,
      });
      setDependencyPrompt(null);
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Batch installation failed',
      });
    } finally {
      setIsBatchInstalling(false);
      setInstallProgress(null);
      setInstallingId(null);
    }
  }

  // One-click install for a search hit
  async function handleOneClickInstall(project: ModrinthSearchHit) {
    setInstallingId(project.project_id);
    setError(null);

    try {
      // Find compatible versions
      const vers = await getProjectVersions(project.slug, {
        loaders: activeLoaders.length > 0 ? activeLoaders : undefined,
        gameVersions: activeVersion ? [activeVersion] : undefined,
      });

      if (vers.length === 0) {
        // If no strictly matching version, open modal to let user pick another version
        setSelectedProject(project);
        setNotification({
          type: 'error',
          text: `No exact build found for ${loaderLabel} ${server.version}. Please select an alternate version below.`,
        });
        setInstallingId(null);
        return;
      }

      const latest = vers[0];
      const primaryFile = latest.files.find((f) => f.primary) || latest.files[0];
      if (!primaryFile) {
        throw new Error('No installable file found in release.');
      }

      // Check for dependencies before proceeding
      if (latest.dependencies && latest.dependencies.length > 0) {
        try {
          const resolution = await resolveModrinthDependencies({
            version: latest,
            primaryProject: { id: project.project_id, slug: project.slug, title: project.title },
            serverLoaders: activeLoaders,
            serverVersion: activeVersion,
            installedMods,
            hashProjectMap,
            installedIds,
          });

          if (resolution.missingCount > 0) {
            setInstallingId(null);
            setDependencyPrompt({
              primaryMod: {
                title: project.title,
                versionNumber: latest.version_number,
                filename: primaryFile.filename,
                iconUrl: project.icon_url,
                url: primaryFile.url,
                projectId: project.project_id,
                projectSlug: project.slug,
              },
              dependencies: resolution.dependencies,
            });
            return;
          }
        } catch {
          // If dependency resolution encounters an error, proceed with standard install
        }
      }

      const existingMod = findExistingOldJar(primaryFile.filename, project);
      if (existingMod) {
        setInstallingId(null);
        setUpdatePrompt({
          projectTitle: project.title,
          existingMod,
          targetUrl: primaryFile.url,
          targetFilename: primaryFile.filename,
          projectId: project.project_id,
          projectSlug: project.slug,
        });
        return;
      }

      await executeInstall(
        primaryFile.url,
        primaryFile.filename,
        project.title,
        project.project_id,
        project.slug,
      );
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Download failed',
      });
      setInstallingId(null);
    }
  }

  // Install a specific version file from the dialog
  async function handleInstallVersion(file: ModrinthVersionFile, projectTitle: string, ver?: ModrinthVersion) {
    if (ver?.dependencies && ver.dependencies.length > 0) {
      try {
        const resolution = await resolveModrinthDependencies({
          version: ver,
          primaryProject: selectedProject
            ? { id: selectedProject.project_id, slug: selectedProject.slug, title: selectedProject.title }
            : { title: projectTitle },
          serverLoaders: activeLoaders,
          serverVersion: activeVersion,
          installedMods,
          hashProjectMap,
          installedIds,
        });

        if (resolution.missingCount > 0) {
          setDependencyPrompt({
            primaryMod: {
              title: projectTitle,
              versionNumber: ver.version_number,
              filename: file.filename,
              iconUrl: selectedProject?.icon_url,
              url: file.url,
              projectId: selectedProject?.project_id,
              projectSlug: selectedProject?.slug,
            },
            dependencies: resolution.dependencies,
          });
          return;
        }
      } catch {
        // Fallback to standard install
      }
    }

    const existingMod = findExistingOldJar(file.filename, selectedProject ?? undefined);
    if (existingMod) {
      setUpdatePrompt({
        projectTitle,
        existingMod,
        targetUrl: file.url,
        targetFilename: file.filename,
        projectId: selectedProject?.project_id,
        projectSlug: selectedProject?.slug,
      });
      return;
    }

    await executeInstall(
      file.url,
      file.filename,
      projectTitle,
      selectedProject?.project_id,
      selectedProject?.slug,
    );
  }

  function checkInstalled(project: ModrinthSearchHit): boolean {
    if (installedIds.has(project.project_id) || installedIds.has(project.slug)) {
      return true;
    }
    return isModInstalled(project, installedMods, hashProjectMap);
  }

  return (
    <div className="space-y-4">
      {dialog}

      {dependencyPrompt && (
        <ModDependenciesDialog
          isOpen={true}
          primaryMod={dependencyPrompt.primaryMod}
          dependencies={dependencyPrompt.dependencies}
          isInstalling={isBatchInstalling}
          installProgress={installProgress}
          onConfirmInstall={handleConfirmInstallWithDependencies}
          onSkipAndInstallPrimaryOnly={() => {
            const { primaryMod } = dependencyPrompt;
            setDependencyPrompt(null);
            const existingMod = findExistingOldJar(primaryMod.filename, {
              slug: primaryMod.projectSlug || '',
              title: primaryMod.title,
              project_id: primaryMod.projectId,
            });
            if (existingMod) {
              setUpdatePrompt({
                projectTitle: primaryMod.title,
                existingMod,
                targetUrl: primaryMod.url,
                targetFilename: primaryMod.filename,
                projectId: primaryMod.projectId,
                projectSlug: primaryMod.projectSlug,
              });
              return;
            }
            void executeInstall(
              primaryMod.url,
              primaryMod.filename,
              primaryMod.title,
              primaryMod.projectId,
              primaryMod.projectSlug,
            );
          }}
          onCancel={() => {
            setDependencyPrompt(null);
            setInstallingId(null);
          }}
        />
      )}

      {updatePrompt && (
        <ModUpdateDialog
          isOpen={true}
          modTitle={updatePrompt.projectTitle}
          installedJar={updatePrompt.existingMod.file}
          newJar={updatePrompt.targetFilename}
          isInstalling={installingId !== null}
          onConfirmDeleteAndInstall={() =>
            executeInstall(
              updatePrompt.targetUrl,
              updatePrompt.targetFilename,
              updatePrompt.projectTitle,
              updatePrompt.projectId,
              updatePrompt.projectSlug,
              updatePrompt.existingMod,
            )
          }
          onConfirmKeepAndInstall={() =>
            executeInstall(
              updatePrompt.targetUrl,
              updatePrompt.targetFilename,
              updatePrompt.projectTitle,
              updatePrompt.projectId,
              updatePrompt.projectSlug,
            )
          }
          onDeleteOldOnly={() => {
            const oldMod = updatePrompt.existingMod;
            setUpdatePrompt(null);
            void handleDeleteMod(oldMod);
          }}
          onCancel={() => setUpdatePrompt(null)}
        />
      )}
      {/* Toast Notification */}
      {notification && (
        <div
          role="status"
          className={`fixed bottom-6 right-6 z-50 flex items-center gap-3 rounded-lg border px-4 py-3 shadow-lg transition-all animate-in fade-in slide-in-from-bottom-2 ${
            notification.type === 'success'
              ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-900 dark:text-emerald-200'
              : 'border-destructive/30 bg-destructive/10 text-destructive'
          }`}
        >
          {notification.type === 'success' ? (
            <Check className="h-4 w-4 shrink-0 text-emerald-500" />
          ) : (
            <Info className="h-4 w-4 shrink-0" />
          )}
          <p className="text-sm font-medium">{notification.text}</p>
          <button
            type="button"
            className="ml-2 text-muted-foreground hover:text-foreground"
            onClick={() => setNotification(null)}
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* Server Compatibility Matching Banner */}
      <div className="rounded-xl border border-primary/20 bg-gradient-to-r from-primary/5 via-secondary/20 to-primary/5 p-4 backdrop-blur-xs">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Sparkles className="h-5 w-5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-semibold text-foreground">Matching Server Software</span>
                <Badge variant="secondary" className="font-mono text-xs">
                  {loaderLabel}
                </Badge>
                {server.version && (
                  <Badge variant="outline" className="font-mono text-xs">
                    MC {server.version}
                  </Badge>
                )}
              </div>
              <p className="text-xs text-muted-foreground">
                Showing {isPluginServerType(server.server_type) ? 'plugins' : 'mods'} verified for your current server environment.
              </p>
            </div>
          </div>

          {/* Filter Matching Toggles */}
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <button
              type="button"
              onClick={() => setMatchLoader(!matchLoader)}
              className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 font-medium transition-colors ${
                matchLoader
                  ? 'border-primary/40 bg-primary/10 text-primary shadow-2xs'
                  : 'border-border bg-background text-muted-foreground hover:text-foreground'
              }`}
            >
              <Layers className="h-3.5 w-3.5" />
              Match {loaderLabel}
            </button>

            {server.version && (
              <button
                type="button"
                onClick={() => setMatchVersion(!matchVersion)}
                className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 font-medium transition-colors ${
                  matchVersion
                    ? 'border-primary/40 bg-primary/10 text-primary shadow-2xs'
                    : 'border-border bg-background text-muted-foreground hover:text-foreground'
                }`}
              >
                <SlidersHorizontal className="h-3.5 w-3.5" />
                MC {server.version}
              </button>
            )}

            <button
              type="button"
              onClick={() => setServerSideOnly(!serverSideOnly)}
              className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 font-medium transition-colors ${
                serverSideOnly
                  ? 'border-primary/40 bg-primary/10 text-primary shadow-2xs'
                  : 'border-border bg-background text-muted-foreground hover:text-foreground'
              }`}
              title="Exclude client-only mods from search results"
            >
              <Filter className="h-3.5 w-3.5" />
              Server-Compatible
            </button>

            <button
              type="button"
              onClick={() => setOnlyServerSide(!onlyServerSide)}
              className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 font-medium transition-colors ${
                onlyServerSide
                  ? 'border-primary/40 bg-primary/10 text-primary shadow-2xs'
                  : 'border-border bg-background text-muted-foreground hover:text-foreground'
              }`}
              title="Show only mods that run exclusively on the server (no client-side mod required)"
            >
              <ServerIcon className="h-3.5 w-3.5" />
              Only Server-Sided
            </button>
          </div>
        </div>
      </div>

      {/* Search Bar & Controls */}
      <div className="space-y-3">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={`Search Modrinth for ${isPluginServerType(server.server_type) ? 'plugins' : 'mods'} (e.g. WorldEdit, Essentials, Chunky)...`}
              className="pl-9 pr-8"
            />
            {query && (
              <button
                type="button"
                aria-label="Clear search"
                onClick={() => setQuery('')}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              >
                <X className="h-4 w-4" />
              </button>
            )}
          </div>

          <div className="flex items-center gap-2">
            {/* Sort Select */}
            <select
              aria-label="Sort search results"
              value={sort}
              onChange={(e) => setSort(e.target.value as typeof sort)}
              className="h-9 rounded-md border border-input bg-background px-3 py-1 text-xs font-medium text-foreground shadow-2xs focus:outline-none focus:ring-1 focus:ring-ring"
            >
              {SORT_OPTIONS.map((opt) => (
                <option key={opt.id} value={opt.id}>
                  {opt.label}
                </option>
              ))}
            </select>

            <Button
              variant="outline"
              size="sm"
              onClick={() => void performSearch(true)}
              disabled={loading}
              title="Refresh results"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </div>

        {/* Category Pills */}
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 scrollbar-none text-xs">
          {CATEGORIES.map((cat) => {
            const active = category === cat.id;
            return (
              <button
                key={cat.id}
                type="button"
                onClick={() => setCategory(cat.id)}
                className={`shrink-0 rounded-full px-3 py-1 font-medium transition-colors ${
                  active
                    ? 'bg-primary text-primary-foreground shadow-2xs'
                    : 'bg-secondary/60 text-secondary-foreground hover:bg-secondary'
                }`}
              >
                {cat.label}
              </button>
            );
          })}
        </div>
      </div>

      {/* Results Header */}
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>
          {loading
            ? 'Searching Modrinth...'
            : totalHits > 0
              ? `Found ${totalHits.toLocaleString()} results`
              : 'No results'}
        </span>
        <span>Powered by Modrinth API</span>
      </div>

      {/* Error state */}
      {error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
          <p className="font-medium">Failed to search Modrinth</p>
          <p className="text-xs opacity-90">{error}</p>
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => void performSearch(true)}
          >
            Retry
          </Button>
        </div>
      )}

      {/* Results Grid */}
      {loading ? (
        <PluginGridSkeleton count={6} />
      ) : results.length === 0 && !error ? (
        <PluginEmptyState
          title="No matching projects found"
          description={`No projects matched your criteria for ${loaderLabel} ${server.version ? `MC ${server.version}` : ''}${onlyServerSide ? ' (only server-sided)' : ''}. Try adjusting search terms or toggling the compatibility filters.`}
          action={
            (matchVersion || matchLoader || serverSideOnly || onlyServerSide || category !== 'all') && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setMatchVersion(false);
                  setMatchLoader(false);
                  setServerSideOnly(false);
                  setOnlyServerSide(false);
                  setCategory('all');
                  setQuery('');
                }}
              >
                Reset All Filters
              </Button>
            )
          }
        />
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {results.map((project) => {
            const installedMod = findInstalledMod(project, installedMods, hashProjectMap);
            const isInstalled = Boolean(installedMod) || checkInstalled(project);
            const isInstalling = installingId === project.project_id;
            const isClientOnly = project.server_side === 'unsupported';
            const isServerOnly =
              project.server_side !== 'unsupported' &&
              (project.client_side === 'unsupported' ||
                project.environment?.includes('server_only') ||
                project.environment?.includes('dedicated_server_only'));
            const updateInfo = installedMod ? updates?.[installedMod.name] : undefined;
            const hasUpdate = Boolean(updateInfo);

            const badges = (
              <>
                {isClientOnly && (
                  <Badge variant="destructive" className="text-[10px] px-1.5 py-0">
                    Client Only
                  </Badge>
                )}
                {isServerOnly && (
                  <Badge
                    variant="outline"
                    className="border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300 text-[10px] px-1.5 py-0"
                  >
                    Server Only
                  </Badge>
                )}
                {project.categories?.slice(0, 2).map((cat) => (
                  <Badge
                    key={cat}
                    variant="secondary"
                    className="text-[10px] px-1.5 py-0 capitalize"
                  >
                    {cat}
                  </Badge>
                ))}
              </>
            );

            return (
              <PluginCard
                key={project.project_id}
                id={project.project_id}
                title={project.title}
                author={project.author}
                description={project.description}
                iconUrl={project.icon_url}
                provider="modrinth"
                isInstalled={isInstalled}
                installedJar={installedMod?.file}
                isInstalling={isInstalling}
                hasUpdate={hasUpdate}
                updateLabel={updateInfo ? `v${updateInfo.latestVersion}` : 'Update'}
                onUpdate={
                  installedMod && onOpenPicker
                    ? () => onOpenPicker(installedMod, updateInfo)
                    : undefined
                }
                downloads={project.downloads}
                follows={project.follows}
                badges={badges}
                externalUrl={`https://modrinth.com/${project.project_type}/${project.slug}`}
                externalLabel="Modrinth"
                installLabel="Install"
                onInstall={() => void handleOneClickInstall(project)}
                onDelete={installedMod ? () => void handleDeleteMod(installedMod) : undefined}
                onViewDetails={() => setSelectedProject(project)}
                detailsTitle="View versions and details"
                detailsLabel="Details"
                onClickTitle={() => setSelectedProject(project)}
              />
            );
          })}
        </div>
      )}

      {/* Infinite Scroll Sentinel & Loading Indicator */}
      {hasMore && (
        <div
          ref={sentinelRef}
          data-testid="infinite-scroll-sentinel"
          className="flex flex-col items-center justify-center py-6 text-xs text-muted-foreground"
        >
          {loadingMore ? (
            <div className="flex items-center gap-2 rounded-full border border-primary/20 bg-secondary/70 px-4 py-2 shadow-2xs backdrop-blur-xs animate-in fade-in">
              <Loader2 className="h-4 w-4 animate-spin text-primary" />
              <span className="font-medium text-foreground">Loading more mods...</span>
            </div>
          ) : !supportsIntersectionObserver ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => void performSearch(false)}
              disabled={loadingMore}
            >
              Load More ({results.length} of {totalHits.toLocaleString()})
            </Button>
          ) : (
            <div className="h-10 w-full" aria-hidden="true" />
          )}
        </div>
      )}

      {/* End of catalog notice */}
      {!hasMore && results.length > 0 && !loading && (
        <div className="border-t border-border/40 py-6 text-center text-xs text-muted-foreground">
          You&apos;ve reached the end of the catalog ({totalHits.toLocaleString()} mods loaded).
        </div>
      )}

      {/* Project Details & Versions Dialog */}
      {selectedProject && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-xs"
          role="presentation"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) setSelectedProject(null);
          }}
        >
          <div
            className="relative flex max-h-[90vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border bg-card text-card-foreground shadow-2xl animate-in fade-in zoom-in-95"
            role="dialog"
            aria-modal="true"
            aria-labelledby="modrinth-modal-title"
          >
            {/* Header */}
            <div className="flex items-start justify-between border-b p-5">
              <div className="flex items-start gap-4">
                <div className="flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-xl border bg-secondary/40 shadow-xs">
                  {selectedProject.icon_url ? (
                    <img
                      src={selectedProject.icon_url}
                      alt={selectedProject.title}
                      className="h-full w-full object-cover"
                    />
                  ) : (
                    <Package className="h-7 w-7 text-muted-foreground" />
                  )}
                </div>

                <div>
                  <div className="flex items-center gap-2">
                    <h3 id="modrinth-modal-title" className="text-lg font-bold text-foreground">
                      {selectedProject.title}
                    </h3>
                    <Badge variant="outline" className="text-xs uppercase">
                      {selectedProject.license || 'Open'}
                    </Badge>
                  </div>

                  <p className="text-xs text-muted-foreground">
                    by <span className="font-semibold text-foreground">{selectedProject.author}</span> •{' '}
                    <span className="font-mono">{formatCount(selectedProject.downloads)} downloads</span>
                  </p>

                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {selectedProject.server_side === 'unsupported' ? (
                      <Badge variant="destructive" className="text-[10px]">
                        Client Only
                      </Badge>
                    ) : selectedProject.client_side === 'unsupported' ||
                      selectedProject.environment?.includes('server_only') ||
                      selectedProject.environment?.includes('dedicated_server_only') ? (
                      <Badge
                        variant="outline"
                        className="border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300 text-[10px]"
                      >
                        Server Only
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-[10px]">
                        Client & Server
                      </Badge>
                    )}
                    {selectedProject.categories?.map((cat) => (
                      <Badge key={cat} variant="secondary" className="text-[10px] capitalize">
                        {cat}
                      </Badge>
                    ))}
                  </div>
                </div>
              </div>

              <div className="flex items-center gap-2">
                <a
                  href={`https://modrinth.com/${selectedProject.project_type}/${selectedProject.slug}`}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1 rounded-md border border-input bg-background px-2.5 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  Modrinth <ExternalLink className="h-3 w-3" />
                </a>

                <button
                  type="button"
                  aria-label="Close dialog"
                  onClick={() => setSelectedProject(null)}
                  className="rounded-md p-1.5 text-muted-foreground hover:bg-secondary hover:text-foreground"
                >
                  <X className="h-5 w-5" />
                </button>
              </div>
            </div>

            {/* Content Body */}
            <div className="flex-1 overflow-y-auto p-5 space-y-5">
              {/* Summary */}
              <div>
                <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  About
                </h4>
                <p className="mt-1 text-sm text-foreground/90 leading-relaxed">
                  {selectedProject.description}
                </p>
              </div>

              {/* Installed jar on server banner if detected */}
              {(() => {
                const selectedInstalledMod = findInstalledMod(selectedProject, installedMods, hashProjectMap);
                if (!selectedInstalledMod) return null;
                return (
                  <div className="flex items-center justify-between gap-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-900 dark:text-emerald-200">
                    <div className="flex items-center gap-2.5 min-w-0">
                      <Check className="h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
                      <div className="min-w-0">
                        <span className="font-semibold text-foreground">Installed Jar on Server:</span>{' '}
                        <span className="font-mono font-medium text-emerald-700 dark:text-emerald-300 truncate">
                          {selectedInstalledMod.file}
                        </span>
                      </div>
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      <Badge
                        variant="outline"
                        className="shrink-0 border-emerald-500/40 bg-background/60 font-mono text-[10px] text-emerald-600 dark:text-emerald-400"
                      >
                        {selectedInstalledMod.enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                      <Button
                        variant="destructive"
                        size="sm"
                        className="h-7 gap-1 px-2.5 text-xs"
                        title={`Delete ${selectedInstalledMod.file} from server`}
                        onClick={() => void handleDeleteMod(selectedInstalledMod)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                        Delete Jar
                      </Button>
                    </div>
                  </div>
                );
              })()}

              {/* Versions List */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    Available Releases ({versions.length})
                  </h4>
                  <span className="text-xs text-muted-foreground">
                    Targeting {loaderLabel} {activeVersion ? `• MC ${activeVersion}` : ''}
                  </span>
                </div>

                {loadingVersions ? (
                  <div className="flex items-center justify-center py-10">
                    <Loader2 className="h-6 w-6 animate-spin text-primary" />
                  </div>
                ) : versionError ? (
                  <p className="text-xs text-destructive">{versionError}</p>
                ) : versions.length === 0 ? (
                  <div className="rounded-lg border border-dashed p-6 text-center text-xs text-muted-foreground">
                    No releases matching {loaderLabel} {activeVersion ? `and Minecraft ${activeVersion}` : ''}.
                  </div>
                ) : (
                  <div className="space-y-2">
                    {versions.map((ver) => {
                      const primaryFile = ver.files.find((f) => f.primary) || ver.files[0];
                      if (!primaryFile) return null;
                      const isDownloadingThis = installingId === primaryFile.url;
                      const isThisFileInstalled =
                        installedFiles.has(primaryFile.filename.toLowerCase()) ||
                        isVersionFileInstalled(primaryFile, installedMods);

                      return (
                        <div
                          key={ver.id}
                          className={`flex items-center justify-between gap-3 rounded-lg border p-3 text-xs transition-colors ${
                            isThisFileInstalled
                              ? 'border-emerald-500/50 bg-emerald-500/10 dark:bg-emerald-950/20 shadow-2xs'
                              : 'border-border bg-background/60 hover:border-primary/40 hover:bg-secondary/20'
                          }`}
                        >
                          <div className="min-w-0 space-y-1">
                            <div className="flex items-center gap-2 flex-wrap">
                              <span className="font-semibold text-foreground truncate">
                                {ver.name || ver.version_number}
                              </span>
                              <Badge
                                variant={ver.version_type === 'release' ? 'default' : 'secondary'}
                                className="text-[10px] px-1.5 py-0 uppercase"
                              >
                                {ver.version_type}
                              </Badge>
                              {isThisFileInstalled && (
                                <Badge
                                  variant="outline"
                                  className="border-emerald-500/40 bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 gap-1 text-[10px] px-1.5 py-0 font-medium"
                                >
                                  <Check className="h-3 w-3 text-emerald-600 dark:text-emerald-400" />
                                  Installed Jar
                                </Badge>
                              )}
                            </div>

                            <p className="truncate text-xs font-mono text-muted-foreground">
                              {primaryFile.filename} · {formatFileSize(primaryFile.size)}
                            </p>

                            <div className="flex flex-wrap gap-1 pt-0.5">
                              {ver.game_versions?.slice(0, 4).map((gv) => (
                                <span
                                  key={gv}
                                  className="rounded bg-secondary px-1.5 py-0.5 text-[10px] text-secondary-foreground"
                                >
                                  {gv}
                                </span>
                              ))}
                              {(ver.game_versions?.length || 0) > 4 && (
                                <span className="text-[10px] text-muted-foreground">
                                  +{(ver.game_versions?.length || 0) - 4} more
                                </span>
                              )}
                            </div>
                          </div>

                          {isThisFileInstalled ? (
                            <div className="flex items-center gap-1 shrink-0">
                              <Button
                                size="sm"
                                variant="outline"
                                disabled={isDownloadingThis}
                                onClick={() => void handleInstallVersion(primaryFile, selectedProject.title, ver)}
                                className="h-8 gap-1.5 border-emerald-500/40 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/20 dark:text-emerald-300 text-xs"
                                title="Reinstall this version"
                              >
                                {isDownloadingThis ? (
                                  <>
                                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                                    Downloading...
                                  </>
                                ) : (
                                  <>
                                    <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" />
                                    Installed
                                  </>
                                )}
                              </Button>
                              {(() => {
                                const matchingMod = installedMods.find(
                                  (m) =>
                                    m.file.toLowerCase() === primaryFile.filename.toLowerCase() ||
                                    m.file.toLowerCase() === `${primaryFile.filename.toLowerCase()}.disabled`,
                                );
                                if (!matchingMod) return null;
                                return (
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    className="h-8 w-8 text-muted-foreground hover:bg-destructive/10 hover:text-destructive transition-colors"
                                    title={`Delete ${matchingMod.file} from server`}
                                    aria-label={`Delete ${matchingMod.file}`}
                                    onClick={() => void handleDeleteMod(matchingMod)}
                                  >
                                    <Trash2 className="h-3.5 w-3.5" />
                                  </Button>
                                );
                              })()}
                            </div>
                          ) : (
                            <Button
                              size="sm"
                              disabled={isDownloadingThis}
                              onClick={() => void handleInstallVersion(primaryFile, selectedProject.title, ver)}
                              className="shrink-0 gap-1.5"
                            >
                              {isDownloadingThis ? (
                                <>
                                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                                  Downloading...
                                </>
                              ) : (
                                <>
                                  <Download className="h-3.5 w-3.5" />
                                  Install
                                </>
                              )}
                            </Button>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>

            {/* Footer */}
            <div className="flex justify-end border-t bg-secondary/10 px-5 py-3">
              <Button variant="outline" size="sm" onClick={() => setSelectedProject(null)}>
                Close
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
