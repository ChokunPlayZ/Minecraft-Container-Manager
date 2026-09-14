import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Check,
  Download,
  ExternalLink,
  Flame,
  Loader2,
  RefreshCw,
  Search,
  Settings,
  Trash2,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import {
  CF_SORT_OPTIONS,
  findInstalledCurseForgeMod,
  getCurseForgeApiKey,
  getCurseForgeClassId,
  getCurseForgeDownloadUrl,
  getCurseForgeFiles,
  getCurseForgeLoaderType,
  isCurseForgeModInstalled,
  searchCurseForge,
  setCurseForgeApiKey,
} from '../api/curseforge';
import { extractModId, formatCount, formatFileSize } from '../api/modrinth';
import type {
  CurseForgeFile,
  CurseForgeMod,
  Mod,
  ModUpdateInfo,
  Server,
} from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent } from './ui/card';
import { Input } from './ui/input';
import { ModUpdateDialog } from './mod-update-dialog';
import { PluginCard, PluginEmptyState, PluginGridSkeleton } from './plugin-card';
import { useModal } from './ui/modal';

interface CurseForgeBrowserProps {
  server: Server;
  installedMods: Mod[];
  updates?: Record<string, ModUpdateInfo>;
  onOpenPicker?: (mod: Mod, updateInfo?: ModUpdateInfo) => void;
  onModInstalled: (mod: Mod) => void;
  onModDeleted?: (modName: string) => void;
}

export function CurseForgeBrowser({
  server,
  installedMods,
  updates,
  onOpenPicker,
  onModInstalled,
  onModDeleted,
}: CurseForgeBrowserProps) {
  const [apiKey, setApiKey] = useState(() => getCurseForgeApiKey());
  const [tempKey, setTempKey] = useState('');
  const [showKeyConfig, setShowKeyConfig] = useState(false);

  const [query, setQuery] = useState('');
  const [sortField, setSortField] = useState(6); // Most Downloaded
  const [classId, setClassId] = useState(() => getCurseForgeClassId(server.server_type));
  const [loaderType] = useState(() => getCurseForgeLoaderType(server.server_type));

  const [results, setResults] = useState<CurseForgeMod[]>([]);
  const [totalHits, setTotalHits] = useState(0);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [installingId, setInstallingId] = useState<number | string | null>(null);
  const [installedSlugs, setInstalledSlugs] = useState<Set<string>>(new Set());
  const [notification, setNotification] = useState<{ text: string; type: 'success' | 'error' } | null>(null);

  // Version picker modal
  const [selectedMod, setSelectedMod] = useState<CurseForgeMod | null>(null);
  const [files, setFiles] = useState<CurseForgeFile[]>([]);
  const [loadingFiles, setLoadingFiles] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);

  const abortControllerRef = useRef<AbortController | null>(null);
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const offsetRef = useRef(0);

  const performSearch = useCallback(
    async (resetOffset = true) => {
      if (!apiKey) return;

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
        const data = await searchCurseForge(
          {
            query,
            classId,
            modLoaderType: loaderType,
            gameVersion: server.version,
            sortField,
            pageSize: 20,
            index: currentOffset,
          },
          apiKey,
          controller.signal,
        );

        if (resetOffset) {
          setResults(data.data);
          offsetRef.current = data.data.length;
        } else {
          setResults((prev) => [...prev, ...data.data]);
          offsetRef.current = currentOffset + data.data.length;
        }
        setTotalHits(data.pagination.totalCount);
      } catch (err: unknown) {
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : 'Failed to search CurseForge');
      } finally {
        setLoading(false);
        setLoadingMore(false);
      }
    },
    [apiKey, query, classId, loaderType, server.version, sortField],
  );

  useEffect(() => {
    if (!apiKey) return;
    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }
    searchTimeoutRef.current = setTimeout(() => {
      void performSearch(true);
    }, 250);

    return () => {
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    };
  }, [apiKey, performSearch]);

  useEffect(() => {
    if (!notification) return;
    const t = setTimeout(() => setNotification(null), 4000);
    return () => clearTimeout(t);
  }, [notification]);

  // Load files when a mod is selected
  useEffect(() => {
    if (!selectedMod || !apiKey) {
      setFiles([]);
      setFileError(null);
      return;
    }

    let isMounted = true;
    setLoadingFiles(true);
    setFileError(null);

    getCurseForgeFiles(selectedMod.id, apiKey, {
      gameVersion: server.version,
      modLoaderType: loaderType,
    })
      .then((fls) => {
        if (!isMounted) return;
        setFiles(fls);
      })
      .catch((err) => {
        if (!isMounted) return;
        setFileError(err instanceof Error ? err.message : 'Failed to load mod files');
      })
      .finally(() => {
        if (isMounted) setLoadingFiles(false);
      });

    return () => {
      isMounted = false;
    };
  }, [selectedMod, apiKey, server.version, loaderType]);

  const { confirm, dialog } = useModal();
  const [updatePrompt, setUpdatePrompt] = useState<{
    projectTitle: string;
    existingMod: Mod;
    targetUrl: string;
    targetFilename: string;
    modId?: number | string;
    modSlug?: string;
    modName?: string;
  } | null>(null);

  function findExistingOldJar(
    targetFilename: string,
    mod?: Pick<CurseForgeMod, 'name' | 'slug'>,
  ): Mod | undefined {
    let existingMod = mod ? findInstalledCurseForgeMod(mod, installedMods) : undefined;
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
    downloadUrl: string,
    filename: string,
    title: string,
    modId?: number | string,
    modSlug?: string,
    modName?: string,
    deleteOldMod?: Mod,
  ) {
    setInstallingId(modSlug || filename);
    setError(null);

    try {
      const installed = await api.downloadMod(
        server.id,
        downloadUrl,
        filename,
        deleteOldMod?.name,
        {
          projectId: modId !== undefined ? String(modId) : undefined,
          projectSlug: modSlug,
          provider: 'curseforge',
        },
      );
      onModInstalled(installed);
      if (deleteOldMod) {
        onModDeleted?.(deleteOldMod.name);
      }

      if (modSlug || modName) {
        setInstalledSlugs(
          (prev) =>
            new Set([
              ...prev,
              ...(modSlug ? [modSlug] : []),
              ...(modName ? [modName.toLowerCase()] : []),
            ]),
        );
      }
      setNotification({
        type: 'success',
        text: deleteOldMod
          ? `Installed ${title} (${filename}) and removed ${deleteOldMod.file}`
          : `Installed ${title} (${filename}) from CurseForge`,
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
      if (selectedMod) {
        setInstalledSlugs((prev) => {
          const next = new Set(prev);
          next.delete(selectedMod.slug);
          next.delete(selectedMod.name.toLowerCase());
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

  async function handleOneClickInstall(mod: CurseForgeMod) {
    if (!apiKey) return;
    setInstallingId(mod.id);
    setError(null);

    try {
      // Find compatible file
      let targetFile: CurseForgeFile | undefined;
      if (mod.latestFiles && mod.latestFiles.length > 0) {
        targetFile = mod.latestFiles.find((f) => f.fileName.toLowerCase().endsWith('.jar')) || mod.latestFiles[0];
      }

      if (!targetFile) {
        const fls = await getCurseForgeFiles(mod.id, apiKey, {
          gameVersion: server.version,
          modLoaderType: loaderType,
        });
        targetFile = fls.find((f) => f.fileName.toLowerCase().endsWith('.jar')) || fls[0];
      }

      if (!targetFile) {
        setSelectedMod(mod);
        setNotification({
          type: 'error',
          text: 'No compatible jar file found for current version. Please choose manually.',
        });
        setInstallingId(null);
        return;
      }

      let downloadUrl = targetFile.downloadUrl;
      if (!downloadUrl) {
        downloadUrl = await getCurseForgeDownloadUrl(mod.id, targetFile.id, apiKey);
      }

      const existingMod = findExistingOldJar(targetFile.fileName, mod);
      if (existingMod) {
        setInstallingId(null);
        setUpdatePrompt({
          projectTitle: mod.name,
          existingMod,
          targetUrl: downloadUrl,
          targetFilename: targetFile.fileName,
          modId: mod.id,
          modSlug: mod.slug,
          modName: mod.name,
        });
        return;
      }

      await executeInstall(downloadUrl, targetFile.fileName, mod.name, mod.id, mod.slug, mod.name);
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Download failed',
      });
      setInstallingId(null);
    }
  }

  async function handleInstallFile(file: CurseForgeFile, modName: string) {
    if (!apiKey || !selectedMod) return;
    setInstallingId(file.id);

    try {
      let downloadUrl = file.downloadUrl;
      if (!downloadUrl) {
        downloadUrl = await getCurseForgeDownloadUrl(selectedMod.id, file.id, apiKey);
      }

      const existingMod = findExistingOldJar(file.fileName, selectedMod);
      if (existingMod) {
        setInstallingId(null);
        setUpdatePrompt({
          projectTitle: modName,
          existingMod,
          targetUrl: downloadUrl,
          targetFilename: file.fileName,
          modId: selectedMod.id,
          modSlug: selectedMod.slug,
          modName: selectedMod.name,
        });
        return;
      }

      await executeInstall(downloadUrl, file.fileName, modName, selectedMod.id, selectedMod.slug, selectedMod.name);
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Download failed',
      });
      setInstallingId(null);
    }
  }

  function handleSaveKey() {
    const trimmed = tempKey.trim();
    if (!trimmed) return;
    setCurseForgeApiKey(trimmed);
    setApiKey(trimmed);
    setShowKeyConfig(false);
  }

  function checkInstalled(mod: CurseForgeMod): boolean {
    if (installedSlugs.has(mod.slug) || installedSlugs.has(mod.name.toLowerCase())) {
      return true;
    }
    return isCurseForgeModInstalled(mod, installedMods);
  }

  // State: No API key configured
  if (!apiKey || showKeyConfig) {
    return (
      <Card className="max-w-xl mx-auto border-orange-500/30 shadow-md">
        <CardContent className="p-6 space-y-4">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-orange-500/10 text-orange-600">
              <Flame className="h-5 w-5" />
            </div>
            <div>
              <h3 className="font-semibold text-base text-foreground">CurseForge API Key</h3>
              <p className="text-xs text-muted-foreground">
                Enter your free CurseForge API key to browse &amp; install mods directly.
              </p>
            </div>
          </div>

          <p className="text-xs text-muted-foreground leading-relaxed">
            CurseForge requires authentication for all API requests. You can generate a free Eternal API key in less than a minute on the CurseForge Console.
          </p>

          <div className="space-y-2">
            <label className="text-xs font-medium text-foreground">API Key</label>
            <Input
              type="password"
              placeholder="Paste your CurseForge API key (e.g. $2a$10$...)"
              value={tempKey}
              onChange={(e) => setTempKey(e.target.value)}
              className="text-xs font-mono"
            />
          </div>

          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pt-2">
            <a
              href="https://console.curseforge.com/#/api-keys"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1.5 text-xs text-orange-600 hover:underline dark:text-orange-400"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              Get key on CurseForge Console
            </a>

            <div className="flex items-center gap-2">
              {apiKey && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setShowKeyConfig(false)}
                  className="text-xs"
                >
                  Cancel
                </Button>
              )}
              <Button
                size="sm"
                onClick={handleSaveKey}
                disabled={!tempKey.trim()}
                className="text-xs bg-orange-600 hover:bg-orange-700 text-white"
              >
                Save &amp; Connect
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      {dialog}

      {/* Update / Replace Dialog */}
      {updatePrompt && (
        <ModUpdateDialog
          isOpen={true}
          modTitle={updatePrompt.projectTitle}
          installedJar={updatePrompt.existingMod.file}
          newJar={updatePrompt.targetFilename}
          onConfirmDeleteAndInstall={() =>
            executeInstall(
              updatePrompt.targetUrl,
              updatePrompt.targetFilename,
              updatePrompt.projectTitle,
              updatePrompt.modId,
              updatePrompt.modSlug,
              updatePrompt.modName,
              updatePrompt.existingMod,
            )
          }
          onConfirmKeepAndInstall={() =>
            executeInstall(
              updatePrompt.targetUrl,
              updatePrompt.targetFilename,
              updatePrompt.projectTitle,
              updatePrompt.modId,
              updatePrompt.modSlug,
              updatePrompt.modName,
            )
          }
          onDeleteOldOnly={() => {
            if (updatePrompt.existingMod) {
              void handleDeleteMod(updatePrompt.existingMod);
            }
            setUpdatePrompt(null);
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
      <div className="flex items-center justify-between gap-3 rounded-xl border border-orange-500/20 bg-orange-500/5 px-4 py-3 text-xs text-orange-950 dark:text-orange-200">
        <div className="flex items-center gap-2.5">
          <span className="flex h-2 w-2 rounded-full bg-orange-500" />
          <span>
            <strong>CurseForge:</strong> Connected with custom API Key.
          </span>
        </div>
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => {
              setTempKey(apiKey);
              setShowKeyConfig(true);
            }}
            className="inline-flex items-center gap-1 font-semibold text-orange-600 hover:underline dark:text-orange-400"
          >
            <Settings className="h-3 w-3" />
            Config API Key
          </button>
          <a
            href="https://curseforge.com/minecraft"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 font-semibold text-orange-600 hover:underline dark:text-orange-400"
          >
            curseforge.com
            <ExternalLink className="h-3 w-3" />
          </a>
        </div>
      </div>

      {/* Search and Filters */}
      <Card>
        <CardContent className="p-4 space-y-3">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search CurseForge mods & plugins..."
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
                aria-label="Filter category"
                value={classId}
                onChange={(e) => setClassId(Number(e.target.value))}
                className="h-9 rounded-md border border-input bg-background px-3 py-1 text-xs shadow-2xs focus:outline-hidden focus:ring-1 focus:ring-ring"
              >
                <option value={6}>Mods</option>
                <option value={5}>Bukkit Plugins</option>
              </select>

              <select
                aria-label="Sort by"
                value={sortField}
                onChange={(e) => setSortField(Number(e.target.value))}
                className="h-9 rounded-md border border-input bg-background px-3 py-1 text-xs shadow-2xs focus:outline-hidden focus:ring-1 focus:ring-ring"
              >
                {CF_SORT_OPTIONS.map((opt) => (
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
          <span>Found {totalHits.toLocaleString()} results on CurseForge</span>
          {results.length > 0 && <span>Showing {results.length} items</span>}
        </div>
      )}

      {/* Loading Skeleton */}
      {loading && <PluginGridSkeleton count={6} />}

      {/* Results Grid */}
      {!loading && results.length > 0 && (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {results.map((mod) => {
            const isInstalled = checkInstalled(mod);
            const installedMod = findInstalledCurseForgeMod(mod, installedMods);
            const isInstalling = installingId === mod.id || installingId === mod.slug;
            const logoUrl = mod.logo?.thumbnailUrl || mod.logo?.url;
            const updateInfo = installedMod ? updates?.[installedMod.name] : undefined;
            const hasUpdate = Boolean(updateInfo);

            return (
              <PluginCard
                key={mod.id}
                id={mod.id}
                title={mod.name}
                author={mod.authors?.[0]?.name || 'Unknown'}
                description={mod.summary || 'No summary available.'}
                iconUrl={logoUrl}
                provider="curseforge"
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
                downloads={mod.downloadCount || 0}
                externalUrl={mod.links?.websiteUrl || `https://curseforge.com/minecraft/mc-mods/${mod.slug}`}
                externalLabel="CurseForge"
                installLabel="Install Latest"
                installDisabled={isInstalled}
                onInstall={() => void handleOneClickInstall(mod)}
                onDelete={installedMod ? () => void handleDeleteMod(installedMod) : undefined}
                onViewDetails={() => setSelectedMod(mod)}
                detailsTitle="View all mod files"
                onClickTitle={() => setSelectedMod(mod)}
              />
            );
          })}
        </div>
      )}

      {/* Empty State */}
      {!loading && results.length === 0 && !error && (
        <PluginEmptyState
          title="No items found on CurseForge"
          description="Try adjusting your search query or class filter."
        />
      )}

      {/* Load More Button */}
      {results.length > 0 && results.length < totalHits && !loading && (
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
                <Loader2 className="h-3.5 w-3.5 animate-spin text-orange-600" />
                Loading next page...
              </>
            ) : (
              <>
                <RefreshCw className="h-3.5 w-3.5" />
                Load more results
              </>
            )}
          </Button>
        </div>
      )}

      {/* Version Files Modal */}
      {selectedMod && (
        <div
          role="dialog"
          aria-modal="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-xs animate-in fade-in"
        >
          <div className="relative flex max-h-[85vh] w-full max-w-2xl flex-col rounded-xl border border-border bg-background shadow-2xl">
            <div className="flex items-center justify-between border-b px-6 py-4">
              <div className="min-w-0 flex-1">
                <h3 className="truncate font-semibold text-base text-foreground">
                  {selectedMod.name} — Files
                </h3>
                <p className="text-xs text-muted-foreground">
                  Releases for Minecraft {server.version}
                </p>
              </div>
              <button
                type="button"
                onClick={() => setSelectedMod(null)}
                className="rounded-lg p-1 text-muted-foreground hover:bg-secondary hover:text-foreground"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            <div className="flex-1 overflow-y-auto p-6 space-y-3">
              {/* Installed Banner */}
              {(() => {
                const selectedInstalledMod = findInstalledCurseForgeMod(selectedMod, installedMods);
                if (!selectedInstalledMod) return null;
                return (
                  <div className="flex items-center justify-between rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs">
                    <div className="flex items-center gap-2 min-w-0">
                      <Check className="h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
                      <div className="min-w-0">
                        <span className="font-semibold text-emerald-950 dark:text-emerald-200">
                          Installed on Server
                        </span>
                        <span className="block truncate font-mono text-[11px] text-muted-foreground">
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

              {loadingFiles && (
                <div className="flex flex-col items-center justify-center py-12 gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-6 w-6 animate-spin text-orange-600" />
                  Loading files from CurseForge...
                </div>
              )}

              {fileError && (
                <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
                  {fileError}
                </div>
              )}

              {!loadingFiles && files.length === 0 && !fileError && (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  No compatible files found for this Minecraft version.
                </p>
              )}

              {!loadingFiles &&
                files.map((fl) => {
                  const isFileInstalled = installedMods.some(
                    (m) =>
                      m.file.toLowerCase() === fl.fileName.toLowerCase() ||
                      m.file.toLowerCase() === `${fl.fileName.toLowerCase()}.disabled`,
                  );
                  const matchingMod = installedMods.find(
                    (m) =>
                      m.file.toLowerCase() === fl.fileName.toLowerCase() ||
                      m.file.toLowerCase() === `${fl.fileName.toLowerCase()}.disabled`,
                  );

                  return (
                    <div
                      key={fl.id}
                      className="flex items-center justify-between gap-3 rounded-lg border bg-card/60 p-3 text-sm transition-colors hover:bg-secondary/15"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-foreground truncate">
                            {fl.displayName || fl.fileName}
                          </span>
                          <Badge variant="outline" className="text-[10px] uppercase">
                            {fl.releaseType === 1 ? 'Release' : fl.releaseType === 2 ? 'Beta' : 'Alpha'}
                          </Badge>
                        </div>
                        <p className="text-xs text-muted-foreground mt-0.5">
                          {fl.fileName} · {formatFileSize(fl.fileLength)} · {formatCount(fl.downloadCount)} downloads
                        </p>
                      </div>

                      {isFileInstalled ? (
                        <div className="flex items-center gap-1.5 shrink-0">
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => void handleInstallFile(fl, selectedMod.name)}
                            disabled={installingId === fl.id}
                            className="h-8 gap-1.5 text-xs text-muted-foreground"
                            title="Reinstall this version"
                          >
                            <Check className="h-3.5 w-3.5 text-emerald-500" />
                            Installed
                          </Button>
                          {matchingMod && (
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
                          )}
                        </div>
                      ) : (
                        <Button
                          size="sm"
                          onClick={() => void handleInstallFile(fl, selectedMod.name)}
                          disabled={installingId === fl.id}
                          className="h-8 gap-1.5 text-xs bg-orange-600 hover:bg-orange-700 text-white shrink-0"
                        >
                          <Download className="h-3.5 w-3.5" />
                          Install
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
                onClick={() => setSelectedMod(null)}
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
