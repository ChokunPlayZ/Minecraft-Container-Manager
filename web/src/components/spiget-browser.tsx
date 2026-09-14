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
import { extractModId, formatCount } from '../api/modrinth';
import type { ModUpdateInfo } from '../api/mod-updates';
import {
  findInstalledSpigetResource,
  getSpigetDownloadUrl,
  getSpigetIconUrl,
  getSpigetSafeFilename,
  getSpigetVersions,
  isSpigetResourceInstalled,
  searchSpiget,
  SPIGET_SORT_OPTIONS,
} from '../api/spiget';
import type {
  Mod,
  Server,
  SpigetResource,
  SpigetVersion,
} from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent } from './ui/card';
import { Input } from './ui/input';
import { ModUpdateDialog } from './mod-update-dialog';
import { PluginCard, PluginEmptyState, PluginGridSkeleton } from './plugin-card';
import { useModal } from './ui/modal';

interface SpigetBrowserProps {
  server: Server;
  installedMods: Mod[];
  updates?: Record<string, ModUpdateInfo>;
  onOpenPicker?: (mod: Mod, updateInfo?: ModUpdateInfo) => void;
  onModInstalled: (mod: Mod) => void;
  onModDeleted?: (modName: string) => void;
}

export function SpigetBrowser({
  server,
  installedMods,
  updates,
  onOpenPicker,
  onModInstalled,
  onModDeleted,
}: SpigetBrowserProps) {
  const [query, setQuery] = useState('');
  const [sort, setSort] = useState<'-downloads' | '-rating' | '-releaseDate' | '-updateDate'>('-downloads');

  const [results, setResults] = useState<SpigetResource[]>([]);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [installingId, setInstallingId] = useState<number | null>(null);
  const [installedIds, setInstalledIds] = useState<Set<number>>(new Set());
  const [notification, setNotification] = useState<{ text: string; type: 'success' | 'error' } | null>(null);

  // Version picker dialog
  const [selectedResource, setSelectedResource] = useState<SpigetResource | null>(null);
  const [versions, setVersions] = useState<SpigetVersion[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [versionError, setVersionError] = useState<string | null>(null);

  const abortControllerRef = useRef<AbortController | null>(null);
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const performSearch = useCallback(
    async (resetPage = true) => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
      const controller = new AbortController();
      abortControllerRef.current = controller;

      const targetPage = resetPage ? 1 : page + 1;
      if (resetPage) {
        setLoading(true);
        setError(null);
        setPage(1);
      } else {
        setLoadingMore(true);
      }

      try {
        const data = await searchSpiget(
          {
            query,
            sort,
            limit: 20,
            page: targetPage,
          },
          controller.signal,
        );

        if (resetPage) {
          setResults(data);
          setPage(1);
        } else {
          setResults((prev) => [...prev, ...data]);
          setPage(targetPage);
        }
      } catch (err: unknown) {
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : 'Failed to search SpigotMC resources');
      } finally {
        setLoading(false);
        setLoadingMore(false);
      }
    },
    [query, sort, page],
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

  useEffect(() => {
    if (!notification) return;
    const t = setTimeout(() => setNotification(null), 4000);
    return () => clearTimeout(t);
  }, [notification]);

  // Load versions when a resource is selected
  useEffect(() => {
    if (!selectedResource) {
      setVersions([]);
      setVersionError(null);
      return;
    }

    let isMounted = true;
    setLoadingVersions(true);
    setVersionError(null);

    getSpigetVersions(selectedResource.id)
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
  }, [selectedResource]);

  const { confirm, dialog } = useModal();
  const [updatePrompt, setUpdatePrompt] = useState<{
    projectTitle: string;
    existingMod: Mod;
    targetUrl: string;
    targetFilename: string;
    resourceId?: number;
  } | null>(null);

  function findExistingOldJar(
    targetFilename: string,
    resource?: Pick<SpigetResource, 'id' | 'name'>,
  ): Mod | undefined {
    let existingMod = resource ? findInstalledSpigetResource(resource, installedMods) : undefined;
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
    resourceId?: number,
    deleteOldMod?: Mod,
  ) {
    setInstallingId(resourceId ?? 0);
    setError(null);

    try {
      const mod = await api.downloadMod(server.id, url, filename, deleteOldMod?.name, {
        projectId: resourceId !== undefined ? String(resourceId) : undefined,
        projectSlug: title,
        provider: 'spiget',
      });
      onModInstalled(mod);
      if (deleteOldMod) {
        onModDeleted?.(deleteOldMod.name);
      }

      if (resourceId !== undefined) {
        setInstalledIds((prev) => new Set([...prev, resourceId]));
      }

      setNotification({
        type: 'success',
        text: deleteOldMod
          ? `Installed ${title} (${filename}) and removed ${deleteOldMod.file}`
          : `Installed ${title} (${filename}) from SpigotMC`,
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
      if (selectedResource) {
        setInstalledIds((prev) => {
          const next = new Set(prev);
          next.delete(selectedResource.id);
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

  async function handleInstallResource(resource: SpigetResource, versionName?: string) {
    const filename = getSpigetSafeFilename(resource.name, versionName);
    const downloadUrl = getSpigetDownloadUrl(resource.id);

    const existingMod = findExistingOldJar(filename, resource);
    if (existingMod) {
      setUpdatePrompt({
        projectTitle: resource.name,
        existingMod,
        targetUrl: downloadUrl,
        targetFilename: filename,
        resourceId: resource.id,
      });
      return;
    }

    await executeInstall(downloadUrl, filename, resource.name, resource.id);
  }

  function checkInstalled(resource: SpigetResource): boolean {
    if (installedIds.has(resource.id)) {
      return true;
    }
    return isSpigetResourceInstalled(resource, installedMods);
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
              updatePrompt.resourceId,
              updatePrompt.existingMod,
            )
          }
          onConfirmKeepAndInstall={() =>
            executeInstall(
              updatePrompt.targetUrl,
              updatePrompt.targetFilename,
              updatePrompt.projectTitle,
              updatePrompt.resourceId,
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
      <div className="flex items-center justify-between gap-3 rounded-xl border border-amber-500/20 bg-amber-500/5 px-4 py-3 text-xs text-amber-950 dark:text-amber-200">
        <div className="flex items-center gap-2.5">
          <span className="flex h-2 w-2 rounded-full bg-amber-500" />
          <span>
            <strong>SpigotMC Catalog:</strong> Access over 100,000 Minecraft plugins from SpigotMC via the Spiget API.
          </span>
        </div>
        <a
          href="https://spigotmc.org/resources"
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 font-semibold text-amber-600 hover:underline dark:text-amber-400"
        >
          spigotmc.org
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
                placeholder="Search SpigotMC plugins (e.g. WorldEdit, Vault, Multiverse)..."
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
                {SPIGET_SORT_OPTIONS.map((opt) => (
                  <option key={opt.id} value={opt.id}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>
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
            {query.trim()
              ? `Search results for "${query.trim()}"`
              : 'Popular SpigotMC plugins'}
          </span>
          {results.length > 0 && <span>Showing {results.length} plugins</span>}
        </div>
      )}

      {/* Loading Skeleton */}
      {loading && <PluginGridSkeleton count={6} />}

      {/* Results Grid */}
      {!loading && results.length > 0 && (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {results.map((res) => {
            const installedMod = findInstalledSpigetResource(res, installedMods);
            const isInstalled = Boolean(installedMod) || checkInstalled(res);
            const isInstalling = installingId === res.id;
            const iconUrl = getSpigetIconUrl(res);
            const updateInfo = installedMod ? updates?.[installedMod.name] : undefined;
            const hasUpdate = Boolean(updateInfo);

            return (
              <PluginCard
                key={res.id}
                id={res.id}
                title={res.name}
                author={res.author?.name ? res.author.name : `Resource #${res.id}`}
                description={res.tag || 'No summary available.'}
                iconUrl={iconUrl}
                provider="spiget"
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
                downloads={res.downloads || 0}
                rating={res.rating?.average}
                externalUrl={`https://spigotmc.org/resources/${res.id}`}
                externalLabel="SpigotMC"
                installLabel="Install Jar"
                onInstall={() => void handleInstallResource(res)}
                onDelete={installedMod ? () => void handleDeleteMod(installedMod) : undefined}
                onViewDetails={() => setSelectedResource(res)}
                detailsTitle="View release history"
                onClickTitle={() => setSelectedResource(res)}
              />
            );
          })}
        </div>
      )}

      {/* Empty State */}
      {!loading && results.length === 0 && !error && (
        <PluginEmptyState
          title="No plugins found on SpigotMC"
          description="Try a different search keyword."
          action={
            query ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setQuery('')}
                className="mt-4 text-xs"
              >
                Clear search query
              </Button>
            ) : undefined
          }
        />
      )}

      {/* Load More Button */}
      {results.length > 0 && !loading && (
        <div className="py-4 text-center">
          <Button
            variant="outline"
            size="sm"
            onClick={() => void performSearch(false)}
            disabled={loadingMore}
            className="text-xs gap-1.5"
          >
            {loadingMore ? (
              <>
                <Loader2 className="h-3.5 w-3.5 animate-spin text-amber-600" />
                Loading next page...
              </>
            ) : (
              <>
                <RefreshCw className="h-3.5 w-3.5" />
                Load next 20 plugins
              </>
            )}
          </Button>
        </div>
      )}

      {/* Version Selection Modal */}
      {selectedResource && (
        <div
          role="dialog"
          aria-modal="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-xs animate-in fade-in"
        >
          <div className="relative flex max-h-[85vh] w-full max-w-2xl flex-col rounded-xl border border-border bg-background shadow-2xl">
            <div className="flex items-center justify-between border-b px-6 py-4">
              <div className="min-w-0 flex-1">
                <h3 className="truncate font-semibold text-base text-foreground">
                  {selectedResource.name} — Version History
                </h3>
                <p className="text-xs text-muted-foreground">
                  SpigotMC releases for resource #{selectedResource.id}
                </p>
              </div>
              <button
                type="button"
                onClick={() => setSelectedResource(null)}
                className="rounded-lg p-1 text-muted-foreground hover:bg-secondary hover:text-foreground"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            {/* Installed jar banner */}
            {(() => {
              const installedResourceMod = findInstalledSpigetResource(selectedResource, installedMods);
              if (!installedResourceMod) return null;
              return (
                <div className="mx-6 mt-4 flex items-center justify-between gap-3 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-900 dark:text-emerald-200">
                  <div className="flex items-center gap-2 min-w-0">
                    <Check className="h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
                    <span className="font-semibold text-foreground">Installed Jar on Server:</span>
                    <span className="font-mono font-medium text-emerald-700 dark:text-emerald-300 truncate">
                      {installedResourceMod.file}
                    </span>
                  </div>
                  <Button
                    variant="destructive"
                    size="sm"
                    className="h-7 gap-1 px-2.5 text-xs shrink-0"
                    title={`Delete ${installedResourceMod.file}`}
                    onClick={() => void handleDeleteMod(installedResourceMod)}
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
                  <Loader2 className="h-6 w-6 animate-spin text-amber-600" />
                  Loading versions from SpigotMC...
                </div>
              )}

              {versionError && (
                <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
                  {versionError}
                </div>
              )}

              {!loadingVersions && versions.length === 0 && !versionError && (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  No previous versions listed for this resource.
                </p>
              )}

              {!loadingVersions &&
                versions.map((ver) => {
                  const verFilename = getSpigetSafeFilename(selectedResource.name, ver.name);
                  const matchingMod = installedMods.find(
                    (m) =>
                      m.file.toLowerCase() === verFilename.toLowerCase() ||
                      m.file.toLowerCase() === `${verFilename.toLowerCase()}.disabled`,
                  );
                  const isThisInstalled = Boolean(matchingMod);

                  return (
                    <div
                      key={ver.id}
                      className="flex items-center justify-between gap-3 rounded-lg border bg-card/60 p-3 text-sm transition-colors hover:bg-secondary/15"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-foreground">{ver.name}</span>
                          {isThisInstalled && (
                            <Badge
                              variant="secondary"
                              className="bg-emerald-500/15 text-emerald-600 dark:text-emerald-300 text-[10px]"
                            >
                              Installed
                            </Badge>
                          )}
                          <span className="text-xs text-muted-foreground">
                            {formatCount(ver.downloads || 0)} downloads
                          </span>
                        </div>
                        <p className="font-mono text-xs text-muted-foreground mt-0.5 truncate">
                          {verFilename}
                        </p>
                      </div>

                      <div className="flex items-center gap-1 shrink-0">
                        <Button
                          size="sm"
                          onClick={() => void handleInstallResource(selectedResource, ver.name)}
                          disabled={installingId === selectedResource.id}
                          className={`h-8 gap-1.5 text-xs ${
                            isThisInstalled
                              ? 'border border-emerald-500/40 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/20 dark:text-emerald-300'
                              : 'bg-amber-600 hover:bg-amber-700 text-white'
                          }`}
                        >
                          <Download className="h-3.5 w-3.5" />
                          {isThisInstalled ? 'Reinstall' : `Install ${ver.name}`}
                        </Button>

                        {matchingMod && (
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
                        )}
                      </div>
                    </div>
                  );
                })}
            </div>

            <div className="border-t px-6 py-3 flex items-center justify-end">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setSelectedResource(null)}
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
