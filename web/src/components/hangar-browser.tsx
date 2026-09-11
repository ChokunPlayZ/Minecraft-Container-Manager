import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Check,
  Download,
  ExternalLink,
  Loader2,
  RefreshCw,
  Search,
  Trash2,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import {
  findInstalledHangarProject,
  getHangarVersions,
  HANGAR_CATEGORIES,
  HANGAR_SORT_OPTIONS,
  isHangarProjectInstalled,
  isHangarVersionInstalled,
  searchHangar,
} from '../api/hangar';
import { extractModId, formatFileSize } from '../api/modrinth';
import type {
  HangarProject,
  HangarVersion,
  Mod,
  Server,
} from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent } from './ui/card';
import { Input } from './ui/input';
import { ModUpdateDialog } from './mod-update-dialog';
import { PluginCard, PluginEmptyState, PluginGridSkeleton } from './plugin-card';
import { useModal } from './ui/modal';

interface HangarBrowserProps {
  server: Server;
  installedMods: Mod[];
  onModInstalled: (mod: Mod) => void;
  onModDeleted?: (modName: string) => void;
}

export function HangarBrowser({
  server,
  installedMods,
  onModInstalled,
  onModDeleted,
}: HangarBrowserProps) {
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('all');
  const [sort, setSort] = useState<'-downloads' | '-stars' | '-views' | '-updated' | '-newest'>('-downloads');

  const [results, setResults] = useState<HangarProject[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [installingId, setInstallingId] = useState<string | null>(null);
  const [installedSlugs, setInstalledSlugs] = useState<Set<string>>(new Set());
  const [notification, setNotification] = useState<{ text: string; type: 'success' | 'error' } | null>(null);

  // Version picker modal
  const [selectedProject, setSelectedProject] = useState<HangarProject | null>(null);
  const [versions, setVersions] = useState<HangarVersion[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [versionError, setVersionError] = useState<string | null>(null);

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
        const data = await searchHangar(
          {
            query,
            category,
            sort,
            platform: 'PAPER',
            limit: 20,
            offset: currentOffset,
          },
          controller.signal,
        );

        if (resetOffset) {
          setResults(data.result);
          offsetRef.current = data.result.length;
        } else {
          setResults((prev) => [...prev, ...data.result]);
          offsetRef.current = currentOffset + data.result.length;
        }
        setTotalCount(data.pagination.count);
      } catch (err: unknown) {
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : 'Failed to search Hangar');
      } finally {
        isFetchingRef.current = false;
        setLoading(false);
        setLoadingMore(false);
      }
    },
    [query, category, sort],
  );

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

  const hasMore = results.length < totalCount && results.length > 0;
  const supportsIntersectionObserver =
    typeof window !== 'undefined' && 'IntersectionObserver' in window;

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
      { rootMargin: '400px', threshold: 0 },
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasMore, loading, loadingMore, supportsIntersectionObserver, performSearch]);

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

    getHangarVersions(selectedProject.namespace.slug)
      .then((vers) => {
        if (!isMounted) return;
        setVersions(vers);
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
  }, [selectedProject]);

  const { confirm, dialog } = useModal();
  const [updatePrompt, setUpdatePrompt] = useState<{
    projectTitle: string;
    existingMod: Mod;
    targetUrl: string;
    targetFilename: string;
    projectSlug?: string;
    projectName?: string;
  } | null>(null);

  function findExistingOldJar(
    targetFilename: string,
    project?: Pick<HangarProject, 'name' | 'namespace'>,
  ): Mod | undefined {
    let existingMod = project ? findInstalledHangarProject(project, installedMods) : undefined;
    if (!existingMod) {
      const baseId = extractModId(targetFilename).replace(/_/g, '-');
      if (baseId.length > 0) {
        existingMod = installedMods.find((m) => {
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
    projectSlug?: string,
    projectName?: string,
    deleteOldMod?: Mod,
  ) {
    setInstallingId(projectSlug || filename);
    setError(null);

    try {
      if (deleteOldMod) {
        try {
          await api.deleteMod(server.id, deleteOldMod.name);
        } catch {
          // ignore
        }
      }

      const mod = deleteOldMod
        ? await api.downloadMod(server.id, url, filename, deleteOldMod.name, {
            projectSlug,
            provider: 'hangar',
          })
        : await api.downloadMod(server.id, url, filename, undefined, {
            projectSlug,
            provider: 'hangar',
          });
      onModInstalled(mod);
      if (deleteOldMod) {
        onModDeleted?.(deleteOldMod.name);
      }

      if (projectSlug || projectName) {
        setInstalledSlugs(
          (prev) =>
            new Set([
              ...prev,
              ...(projectSlug ? [projectSlug] : []),
              ...(projectName ? [projectName.toLowerCase()] : []),
            ]),
        );
      }
      setNotification({
        type: 'success',
        text: deleteOldMod
          ? `Installed ${title} (${filename}) and removed ${deleteOldMod.file}`
          : `Installed ${title} (${filename}) from Hangar`,
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
      if (selectedProject) {
        setInstalledSlugs((prev) => {
          const next = new Set(prev);
          next.delete(selectedProject.namespace.slug);
          next.delete(selectedProject.name.toLowerCase());
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

  async function handleOneClickInstall(project: HangarProject) {
    setInstallingId(project.namespace.slug);
    setError(null);

    try {
      const vers = await getHangarVersions(project.namespace.slug);
      if (vers.length === 0) {
        setSelectedProject(project);
        setNotification({
          type: 'error',
          text: 'No release builds found. Please inspect versions manually.',
        });
        setInstallingId(null);
        return;
      }

      const latest = vers[0];
      const download = latest.downloads?.PAPER || Object.values(latest.downloads || {})[0];
      if (!download?.downloadUrl || !download?.fileInfo?.name) {
        throw new Error('No direct download jar available for this release.');
      }

      const existingMod = findExistingOldJar(download.fileInfo.name, project);
      if (existingMod) {
        setInstallingId(null);
        setUpdatePrompt({
          projectTitle: project.name,
          existingMod,
          targetUrl: download.downloadUrl,
          targetFilename: download.fileInfo.name,
          projectSlug: project.namespace.slug,
          projectName: project.name,
        });
        return;
      }

      await executeInstall(
        download.downloadUrl,
        download.fileInfo.name,
        project.name,
        project.namespace.slug,
        project.name,
      );
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Download failed',
      });
      setInstallingId(null);
    }
  }

  async function handleInstallVersion(version: HangarVersion, projectTitle: string) {
    const download = version.downloads?.PAPER || Object.values(version.downloads || {})[0];
    if (!download?.downloadUrl || !download?.fileInfo?.name) {
      setNotification({ type: 'error', text: 'No download URL available for this version.' });
      return;
    }

    const existingMod = findExistingOldJar(download.fileInfo.name, selectedProject ?? undefined);
    if (existingMod) {
      setUpdatePrompt({
        projectTitle,
        existingMod,
        targetUrl: download.downloadUrl,
        targetFilename: download.fileInfo.name,
        projectSlug: selectedProject?.namespace.slug,
        projectName: selectedProject?.name,
      });
      return;
    }

    await executeInstall(
      download.downloadUrl,
      download.fileInfo.name,
      projectTitle,
      selectedProject?.namespace.slug,
      selectedProject?.name,
    );
  }

  function checkInstalled(project: HangarProject): boolean {
    if (installedSlugs.has(project.namespace.slug) || installedSlugs.has(project.name.toLowerCase())) {
      return true;
    }
    return isHangarProjectInstalled(project, installedMods);
  }

  return (
    <div className="space-y-4">
      {dialog}

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
              updatePrompt.projectSlug,
              updatePrompt.projectName,
              updatePrompt.existingMod,
            )
          }
          onConfirmKeepAndInstall={() =>
            executeInstall(
              updatePrompt.targetUrl,
              updatePrompt.targetFilename,
              updatePrompt.projectTitle,
              updatePrompt.projectSlug,
              updatePrompt.projectName,
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
            <X className="h-4 w-4 shrink-0" />
          )}
          <p className="text-sm font-medium">{notification.text}</p>
        </div>
      )}

      {/* Info Banner */}
      <div className="flex items-center justify-between gap-3 rounded-xl border border-sky-500/20 bg-sky-500/5 px-4 py-3 text-xs text-sky-950 dark:text-sky-200">
        <div className="flex items-center gap-2.5">
          <span className="flex h-2 w-2 rounded-full bg-sky-500" />
          <span>
            <strong>PaperMC Hangar:</strong> Official plugin repository for Paper, Purpur, Folia, and Velocity servers.
          </span>
        </div>
        <a
          href="https://hangar.papermc.io"
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 font-semibold text-sky-600 hover:underline dark:text-sky-400"
        >
          hangar.papermc.io
          <ExternalLink className="h-3 w-3" />
        </a>
      </div>

      {/* Search and Filters */}
      <Card>
        <CardContent className="p-4 space-y-3">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search PaperMC plugins on Hangar (e.g. Chunky, LuckPerms, EssentialsX)..."
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                className="pl-9 pr-8 text-sm"
              />
              {query && (
                <button
                  type="button"
                  onClick={() => setQuery('')}
                  className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  <X className="h-4 w-4" />
                </button>
              )}
            </div>

            <div className="flex items-center gap-2">
              <select
                aria-label="Sort by"
                value={sort}
                onChange={(e) => setSort(e.target.value as typeof sort)}
                className="h-9 rounded-md border border-input bg-background px-3 py-1 text-xs shadow-2xs focus:outline-hidden focus:ring-1 focus:ring-ring"
              >
                {HANGAR_SORT_OPTIONS.map((opt) => (
                  <option key={opt.id} value={opt.id}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>
          </div>

          {/* Category Badges */}
          <div className="flex flex-wrap items-center gap-1.5 pt-1">
            <span className="mr-1 text-xs font-medium text-muted-foreground">Category:</span>
            {HANGAR_CATEGORIES.map((cat) => (
              <button
                key={cat.id}
                type="button"
                onClick={() => setCategory(cat.id)}
                className={`rounded-full px-2.5 py-0.5 text-xs font-medium transition-all ${
                  category === cat.id
                    ? 'bg-sky-600 text-white shadow-2xs'
                    : 'bg-secondary/70 text-muted-foreground hover:bg-secondary hover:text-foreground'
                }`}
              >
                {cat.label}
              </button>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Error state */}
      {error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-center text-sm text-destructive">
          <p>{error}</p>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void performSearch(true)}
            className="mt-2 text-xs"
          >
            Retry
          </Button>
        </div>
      )}

      {/* Results Header */}
      {!loading && (
        <div className="flex items-center justify-between text-xs text-muted-foreground px-1">
          <span>
            Found <strong>{totalCount.toLocaleString()}</strong> plugins on Hangar
          </span>
          {results.length > 0 && <span>Showing {results.length} results</span>}
        </div>
      )}

      {/* Loading Skeleton */}
      {loading && <PluginGridSkeleton count={6} />}

      {/* Results Grid */}
      {!loading && results.length > 0 && (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {results.map((project) => {
            const installedProjectMod = findInstalledHangarProject(project, installedMods);
            const isInstalled = Boolean(installedProjectMod) || checkInstalled(project);
            const isInstalling = installingId === project.namespace.slug;

            return (
              <PluginCard
                key={project.id}
                id={project.id}
                title={project.name}
                author={project.namespace.owner}
                description={project.description}
                iconUrl={project.avatarUrl}
                provider="hangar"
                isInstalled={isInstalled}
                installedJar={installedProjectMod?.file}
                isInstalling={isInstalling}
                downloads={project.stats.downloads}
                stars={project.stats.stars}
                category={project.category.replace(/_/g, ' ')}
                externalUrl={`https://hangar.papermc.io/${project.namespace.owner}/${project.namespace.slug}`}
                externalLabel="Hangar"
                installLabel="Install Latest"
                onInstall={() => void handleOneClickInstall(project)}
                onDelete={installedProjectMod ? () => void handleDeleteMod(installedProjectMod) : undefined}
                onViewDetails={() => setSelectedProject(project)}
                detailsTitle="View all release versions"
                onClickTitle={() => setSelectedProject(project)}
              />
            );
          })}
        </div>
      )}

      {/* Empty State */}
      {!loading && results.length === 0 && !error && (
        <PluginEmptyState
          title="No plugins found on Hangar"
          description="Try adjusting your search query or selecting a different category."
          action={
            query ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setQuery('')}
                className="text-xs"
              >
                Clear search query
              </Button>
            ) : undefined
          }
        />
      )}

      {/* Infinite Scroll Sentinel / Load More Button */}
      {hasMore && (
        <div ref={sentinelRef} className="py-4 text-center">
          {loadingMore ? (
            <div className="inline-flex items-center gap-2 text-xs text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin text-sky-600" />
              Loading more plugins...
            </div>
          ) : (
            <Button
              variant="outline"
              size="sm"
              onClick={() => void performSearch(false)}
              className="text-xs gap-1.5"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              Load more results
            </Button>
          )}
        </div>
      )}

      {/* Version Selection Modal */}
      {selectedProject && (
        <div
          role="dialog"
          aria-modal="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-xs animate-in fade-in"
        >
          <div className="relative flex max-h-[85vh] w-full max-w-2xl flex-col rounded-xl border border-border bg-background shadow-2xl">
            <div className="flex items-center justify-between border-b px-6 py-4">
              <div className="min-w-0 flex-1">
                <h3 className="truncate font-semibold text-base text-foreground">
                  {selectedProject.name} — Releases
                </h3>
                <p className="text-xs text-muted-foreground">
                  Available versions on PaperMC Hangar
                </p>
              </div>
              <button
                type="button"
                onClick={() => setSelectedProject(null)}
                className="rounded-lg p-1 text-muted-foreground hover:bg-secondary hover:text-foreground"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            {/* Installed banner */}
            {(() => {
              const selectedInstalled = findInstalledHangarProject(selectedProject, installedMods);
              if (!selectedInstalled) return null;
              return (
                <div className="mx-6 mt-4 flex items-center justify-between gap-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-900 dark:text-emerald-200">
                  <div className="flex items-center gap-2 min-w-0">
                    <Check className="h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
                    <span className="font-semibold text-foreground">Installed Jar on Server:</span>
                    <span className="font-mono font-medium text-emerald-700 dark:text-emerald-300 truncate">
                      {selectedInstalled.file}
                    </span>
                  </div>
                  <Button
                    variant="destructive"
                    size="sm"
                    className="h-7 gap-1 px-2.5 text-xs shrink-0"
                    title={`Delete ${selectedInstalled.file}`}
                    onClick={() => void handleDeleteMod(selectedInstalled)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    Delete Jar
                  </Button>
                </div>
              );
            })()}

            <div className="flex-1 overflow-y-auto p-6 space-y-3">
              {loadingVersions && (
                <div className="flex flex-col items-center justify-center py-12 gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-6 w-6 animate-spin text-sky-600" />
                  Loading versions from Hangar...
                </div>
              )}

              {versionError && (
                <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
                  {versionError}
                </div>
              )}

              {!loadingVersions && versions.length === 0 && !versionError && (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  No public versions found for this plugin.
                </p>
              )}

              {!loadingVersions &&
                versions.map((ver) => {
                  const download = ver.downloads?.PAPER || Object.values(ver.downloads || {})[0];
                  const filename = download?.fileInfo?.name;
                  const isInstalled = isHangarVersionInstalled(ver, installedMods);
                  const isInstallingThis = installingId === ver.name;

                  return (
                    <div
                      key={ver.id}
                      className="flex items-center justify-between gap-3 rounded-lg border bg-card/60 p-3 text-sm transition-colors hover:bg-secondary/15"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-foreground">{ver.name}</span>
                          <span
                            className="rounded-full px-2 py-0.2 text-[10px] font-semibold text-white uppercase"
                            style={{ backgroundColor: ver.channel?.color || '#0284c7' }}
                          >
                            {ver.channel?.name || 'Release'}
                          </span>
                          {isInstalled && (
                            <Badge
                              variant="secondary"
                              className="bg-emerald-500/15 text-emerald-600 dark:text-emerald-300 text-[10px]"
                            >
                              Installed on server
                            </Badge>
                          )}
                        </div>

                        <p className="truncate text-xs text-muted-foreground mt-0.5">
                          {filename || 'Custom build'}
                          {download?.fileInfo?.sizeBytes && (
                            <> · {formatFileSize(download.fileInfo.sizeBytes)}</>
                          )}
                          {ver.platformDependencies?.PAPER && (
                            <> · Paper {ver.platformDependencies.PAPER.join(', ')}</>
                          )}
                        </p>
                      </div>

                      {isInstalled ? (
                        <div className="flex items-center gap-1 shrink-0">
                          <Button
                            size="sm"
                            disabled={isInstallingThis || !download?.downloadUrl}
                            onClick={() => void handleInstallVersion(ver, selectedProject.name)}
                            className="h-8 gap-1.5 border border-emerald-500/40 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/20 dark:text-emerald-300 text-xs"
                            title="Reinstall this version"
                          >
                            {isInstallingThis ? (
                              <Loader2 className="h-3.5 w-3.5 animate-spin" />
                            ) : (
                              <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" />
                            )}
                            Reinstall
                          </Button>
                          {(() => {
                            const matchingMod = installedMods.find(
                              (m) =>
                                m.file.toLowerCase() === filename?.toLowerCase() ||
                                m.file.toLowerCase() === `${filename?.toLowerCase()}.disabled`,
                            );
                            if (!matchingMod) return null;
                            return (
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-8 w-8 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
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
                          disabled={isInstallingThis || !download?.downloadUrl}
                          onClick={() => void handleInstallVersion(ver, selectedProject.name)}
                          className="h-8 gap-1.5 text-xs bg-sky-600 hover:bg-sky-700 text-white"
                        >
                          {isInstallingThis ? (
                            <>
                              <Loader2 className="h-3.5 w-3.5 animate-spin" />
                              Installing...
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

            <div className="border-t px-6 py-3 flex items-center justify-end">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setSelectedProject(null)}
                className="text-xs"
              >
                Close
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
