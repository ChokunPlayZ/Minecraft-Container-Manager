import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Check,
  Download,
  ExternalLink,
  Filter,
  Flame,
  Info,
  Layers,
  Loader2,
  Package,
  RefreshCw,
  Search,
  SlidersHorizontal,
  Sparkles,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import {
  findInstalledMod,
  formatCount,
  formatFileSize,
  getLoaderLabel,
  getServerLoaders,
  getProjectVersions,
  isModInstalled,
  isVersionFileInstalled,
  searchModrinth,
} from '../api/modrinth';
import type {
  Mod,
  ModrinthSearchHit,
  ModrinthVersion,
  ModrinthVersionFile,
  Server,
} from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent } from './ui/card';
import { Input } from './ui/input';

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
  onModInstalled: (mod: Mod) => void;
}

export function ModrinthBrowser({
  server,
  installedMods,
  onModInstalled,
}: ModrinthBrowserProps) {
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('all');
  const [sort, setSort] = useState<'downloads' | 'relevance' | 'follows' | 'updated' | 'newest'>('downloads');

  // Matching toggles
  const [matchLoader, setMatchLoader] = useState(true);
  const [matchVersion, setMatchVersion] = useState(true);
  const [serverSideOnly, setServerSideOnly] = useState(true);

  // Results & pagination
  const [results, setResults] = useState<ModrinthSearchHit[]>([]);
  const [totalHits, setTotalHits] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Installation state
  const [installingId, setInstallingId] = useState<string | null>(null);
  const [installedIds, setInstalledIds] = useState<Set<string>>(new Set());
  const [installedFiles, setInstalledFiles] = useState<Set<string>>(new Set());
  const [notification, setNotification] = useState<{ text: string; type: 'success' | 'error' } | null>(null);

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

  const performSearch = useCallback(
    async (resetOffset = true) => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
      const controller = new AbortController();
      abortControllerRef.current = controller;

      const currentOffset = resetOffset ? 0 : offset;
      if (resetOffset) {
        setLoading(true);
        setError(null);
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
            sort,
            offset: currentOffset,
            limit: 20,
          },
          controller.signal,
        );

        if (resetOffset) {
          setResults(data.hits);
          setOffset(20);
        } else {
          setResults((prev) => [...prev, ...data.hits]);
          setOffset((prev) => prev + 20);
        }
        setTotalHits(data.total_hits);
      } catch (err: unknown) {
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : 'Failed to search Modrinth');
      } finally {
        setLoading(false);
        setLoadingMore(false);
      }
    },
    [query, activeLoaders, activeVersion, category, serverSideOnly, sort, offset],
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

      const mod = await api.downloadMod(server.id, primaryFile.url, primaryFile.filename);
      onModInstalled(mod);
      setInstalledIds((prev) => new Set([...prev, project.project_id, project.slug]));
      setInstalledFiles((prev) => new Set([...prev, primaryFile.filename.toLowerCase()]));
      setNotification({
        type: 'success',
        text: `Installed ${project.title} (${primaryFile.filename})`,
      });
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Download failed',
      });
    } finally {
      setInstallingId(null);
    }
  }

  // Install a specific version file from the dialog
  async function handleInstallVersion(file: ModrinthVersionFile, projectTitle: string) {
    setInstallingId(file.url);
    try {
      const mod = await api.downloadMod(server.id, file.url, file.filename);
      onModInstalled(mod);
      if (selectedProject) {
        setInstalledIds((prev) => new Set([...prev, selectedProject.project_id, selectedProject.slug]));
      }
      setInstalledFiles((prev) => new Set([...prev, file.filename.toLowerCase()]));
      setNotification({
        type: 'success',
        text: `Installed ${projectTitle} (${file.filename})`,
      });
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Download failed',
      });
    } finally {
      setInstallingId(null);
    }
  }

  function checkInstalled(project: ModrinthSearchHit): boolean {
    if (installedIds.has(project.project_id) || installedIds.has(project.slug)) {
      return true;
    }
    return isModInstalled(project, installedMods);
  }

  return (
    <div className="space-y-4">
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
                Showing {server.server_type === 'paper' || server.server_type === 'spigot' ? 'plugins' : 'mods'} verified for your current server environment.
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
              Server-Only
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
              placeholder={`Search Modrinth for ${server.server_type === 'paper' || server.server_type === 'spigot' ? 'plugins' : 'mods'} (e.g. WorldEdit, Essentials, Chunky)...`}
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
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <div
              key={i}
              className="flex animate-pulse items-start gap-3.5 rounded-xl border bg-card p-4"
            >
              <div className="h-12 w-12 shrink-0 rounded-lg bg-secondary" />
              <div className="flex-1 space-y-2">
                <div className="h-4 w-1/2 rounded bg-secondary" />
                <div className="h-3 w-3/4 rounded bg-secondary/70" />
                <div className="h-3 w-1/3 rounded bg-secondary/50" />
              </div>
            </div>
          ))}
        </div>
      ) : results.length === 0 && !error ? (
        <div className="rounded-xl border border-dashed p-10 text-center">
          <Package className="mx-auto h-10 w-10 text-muted-foreground/60" />
          <h3 className="mt-3 text-base font-semibold text-foreground">No matching projects found</h3>
          <p className="mx-auto mt-1 max-w-md text-xs text-muted-foreground">
            No projects matched your criteria for {loaderLabel} {server.version ? `MC ${server.version}` : ''}. Try adjusting search terms or toggling the compatibility filters.
          </p>
          <div className="mt-4 flex justify-center gap-2">
            {(matchVersion || matchLoader || serverSideOnly || category !== 'all') && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setMatchVersion(false);
                  setMatchLoader(false);
                  setServerSideOnly(false);
                  setCategory('all');
                  setQuery('');
                }}
              >
                Reset All Filters
              </Button>
            )}
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3.5 md:grid-cols-2">
          {results.map((project) => {
            const installedMod = findInstalledMod(project, installedMods);
            const isInstalled = Boolean(installedMod) || checkInstalled(project);
            const isInstalling = installingId === project.project_id;
            const isClientOnly = project.server_side === 'unsupported';

            return (
              <Card
                key={project.project_id}
                className="group relative flex flex-col justify-between overflow-hidden border transition-all hover:border-primary/40 hover:shadow-sm"
              >
                <CardContent className="p-4">
                  <div className="flex items-start gap-3.5">
                    {/* Project Icon */}
                    <div className="relative flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-lg border bg-secondary/40 shadow-2xs">
                      {project.icon_url ? (
                        <img
                          src={project.icon_url}
                          alt={project.title}
                          className="h-full w-full object-cover"
                          loading="lazy"
                          onError={(e) => {
                            (e.target as HTMLElement).style.display = 'none';
                          }}
                        />
                      ) : (
                        <Package className="h-6 w-6 text-muted-foreground/60" />
                      )}
                    </div>

                    {/* Meta info */}
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <h4
                          className="truncate font-semibold text-foreground transition-colors group-hover:text-primary cursor-pointer text-sm"
                          onClick={() => setSelectedProject(project)}
                          title={project.title}
                        >
                          {project.title}
                        </h4>
                      </div>

                      <p className="truncate text-xs text-muted-foreground">
                        by <span className="font-medium text-foreground/80">{project.author}</span>
                      </p>

                      {/* Description with 2-line clamp */}
                      <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                        {project.description}
                      </p>

                      {/* Installed jar indicator if on server */}
                      {installedMod && (
                        <div className="mt-1.5 flex items-center gap-1.5 rounded bg-emerald-500/10 px-2 py-0.5 text-[11px] font-mono text-emerald-700 dark:text-emerald-300">
                          <Check className="h-3 w-3 shrink-0 text-emerald-600 dark:text-emerald-400" />
                          <span className="truncate">Installed: {installedMod.file}</span>
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Badges & Stats */}
                  <div className="mt-3 flex flex-wrap items-center gap-1.5 text-xs">
                    {isClientOnly && (
                      <Badge variant="destructive" className="text-[10px] px-1.5 py-0">
                        Client Only
                      </Badge>
                    )}
                    {project.server_side === 'required' && (
                      <Badge variant="outline" className="border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300 text-[10px] px-1.5 py-0">
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
                  </div>

                  {/* Card Footer: stats & download button */}
                  <div className="mt-3.5 flex items-center justify-between border-t border-border/60 pt-3 text-xs">
                    <div className="flex items-center gap-3 text-muted-foreground">
                      <span className="inline-flex items-center gap-1" title="Downloads">
                        <Download className="h-3.5 w-3.5" />
                        {formatCount(project.downloads)}
                      </span>
                      <span className="inline-flex items-center gap-1" title="Followers">
                        <Flame className="h-3.5 w-3.5 text-amber-500" />
                        {formatCount(project.follows)}
                      </span>
                    </div>

                    <div className="flex items-center gap-1.5">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 px-2 text-xs"
                        onClick={() => setSelectedProject(project)}
                        title="View versions and details"
                      >
                        Details
                      </Button>

                      {isInstalled ? (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={isInstalling}
                          onClick={() => void handleOneClickInstall(project)}
                          className="h-8 gap-1.5 border-emerald-500/40 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/20 dark:text-emerald-300"
                          title="Click to reinstall or update"
                        >
                          {isInstalling ? (
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" />
                          )}
                          Installed
                        </Button>
                      ) : (
                        <Button
                          variant="default"
                          size="sm"
                          disabled={isInstalling}
                          onClick={() => void handleOneClickInstall(project)}
                          className="h-8 gap-1.5 shadow-2xs"
                        >
                          {isInstalling ? (
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
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* Pagination: Load More */}
      {results.length > 0 && results.length < totalHits && !loading && (
        <div className="flex justify-center pt-2">
          <Button
            variant="outline"
            onClick={() => void performSearch(false)}
            disabled={loadingMore}
            className="gap-2"
          >
            {loadingMore ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Loading more...
              </>
            ) : (
              `Load More (${results.length} of ${totalHits.toLocaleString()})`
            )}
          </Button>
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
                const selectedInstalledMod = findInstalledMod(selectedProject, installedMods);
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
                    <Badge
                      variant="outline"
                      className="shrink-0 border-emerald-500/40 bg-background/60 font-mono text-[10px] text-emerald-600 dark:text-emerald-400"
                    >
                      {selectedInstalledMod.enabled ? 'Enabled' : 'Disabled'}
                    </Badge>
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
                            <Button
                              size="sm"
                              variant="outline"
                              disabled={isDownloadingThis}
                              onClick={() => void handleInstallVersion(primaryFile, selectedProject.title)}
                              className="shrink-0 gap-1.5 border-emerald-500/40 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/20 dark:text-emerald-300"
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
                          ) : (
                            <Button
                              size="sm"
                              disabled={isDownloadingThis}
                              onClick={() => void handleInstallVersion(primaryFile, selectedProject.title)}
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
