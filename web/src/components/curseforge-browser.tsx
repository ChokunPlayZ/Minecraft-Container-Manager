import { canSwitchModChannel, modChannelLabel, normalizeModChannel, RELEASE_CHANNEL, matchesInstalledArtifact, curseForgeChannel } from '../api/mod-channels';
import { ModChannelBadge, ModChannelFilter } from './mod-channel-filter';
import { Select } from './ui/select';
import { DialogBackdrop } from './ui/dialog-backdrop';
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Check,
  Download,
  ExternalLink,
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
  getCurseForgeClassId,
  getCurseForgeDownloadUrl,
  getCurseForgeFiles,
  getCurseForgeLoaderType,
  isCurseForgeModInstalled,
  searchCurseForge,
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
import { ModDependenciesDialog } from './mod-dependencies-dialog';
import {
  resolveCurseForgeDependencies,
  type ResolvedDependency,
} from '../api/mod-dependencies';
import { PluginCard, PluginEmptyState, PluginGridSkeleton } from './plugin-card';
import { useModal } from './ui/modal';
import { usePermissions } from '../lib/auth';
import { useCurseForgeSettings } from '../lib/curseforge';
import { CurseForgeSettings } from './curseforge-settings';

interface CurseForgeBrowserProps {
  server: Server;
  installedMods: Mod[];
  updates?: Record<string, ModUpdateInfo>;
  onOpenPicker?: (mod: Mod, updateInfo?: ModUpdateInfo) => void;
  onModInstalled: (mod: Mod) => void;
  onModDeleted?: (modName: string) => void;
}

function buildChannel(ver: CurseForgeFile) {
  return curseForgeChannel(ver.releaseType);
}

function installedChannel(mod: Mod | undefined, versions: CurseForgeFile[]): string {
  if (mod?.release_type) return normalizeModChannel(mod.release_type);
  const current = mod && versions.find((ver) => matchesInstalledArtifact(mod, ver.fileName, ver.hashes?.find((hash) => hash.algo === 1)?.value));
  return current ? buildChannel(current) : RELEASE_CHANNEL;
}

export function CurseForgeBrowser({
  server,
  installedMods,
  updates,
  onOpenPicker,
  onModInstalled,
  onModDeleted,
}: CurseForgeBrowserProps) {
  const can = usePermissions();
  const { settings: curseForgeSettings, loading: loadingSettings, error: settingsError, refresh: refreshSettings } = useCurseForgeSettings();
  const configured = curseForgeSettings?.configured ?? false;
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
  const [channelFilter, setChannelFilter] = useState<string>();
  const [loadingFiles, setLoadingFiles] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);

  const abortControllerRef = useRef<AbortController | null>(null);
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const offsetRef = useRef(0);

  const performSearch = useCallback(
    async (resetOffset = true) => {
      if (!configured) return;

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
    [configured, query, classId, loaderType, server.version, sortField],
  );

  useEffect(() => {
    if (!configured) return;
    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }
    searchTimeoutRef.current = setTimeout(() => {
      void performSearch(true);
    }, 250);

    return () => {
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    };
  }, [configured, performSearch]);

  useEffect(() => {
    if (!notification) return;
    const t = setTimeout(() => setNotification(null), 4000);
    return () => clearTimeout(t);
  }, [notification]);

  // Load files when a mod is selected
  useEffect(() => {
    if (!selectedMod || !configured) {
      setFiles([]);
      setFileError(null);
      return;
    }

    const controller = new AbortController();
    let isMounted = true;
    setLoadingFiles(true);
    setFileError(null);

    getCurseForgeFiles(selectedMod.id, {
      gameVersion: server.version,
      modLoaderType: loaderType,
      allPages: true,
    }, controller.signal)
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
      controller.abort();
    };
  }, [selectedMod, configured, server.version, loaderType]);

  useEffect(() => {
    setChannelFilter(undefined);
  }, [selectedMod]);

  const selectedInstalled = selectedMod ? findInstalledCurseForgeMod(selectedMod, installedMods) : undefined;
  const currentChannel = installedChannel(selectedInstalled, files);
  const visibleChannel = channelFilter ?? currentChannel;
  const visibleBuilds = files.filter((version) => visibleChannel === 'all' || buildChannel(version) === visibleChannel);

  const { confirm, dialog } = useModal();
  const [updatePrompt, setUpdatePrompt] = useState<{
    projectTitle: string;
    existingMod: Mod;
    targetUrl: string;
    targetFilename: string;
    build: { releaseType: string; versionId: string; allowChannelSwitch?: boolean };
    modId?: number | string;
    modSlug?: string;
    modName?: string;
  } | null>(null);

  const [dependencyPrompt, setDependencyPrompt] = useState<{
    primaryMod: {
      title: string;
      versionNumber?: string;
      filename: string;
      iconUrl?: string | null;
      url: string;
      build: { releaseType: string; versionId: string; allowChannelSwitch?: boolean };
      existingMod?: Mod;
      modId?: number | string;
      modSlug?: string;
      modName?: string;
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
    build?: { releaseType: string; versionId: string; allowChannelSwitch?: boolean },
    deleteOldMod?: Mod,
  ) {
    setInstallingId(modSlug || filename);
    setError(null);

    try {
      const installed = await api.downloadMod(
        server.id,
        downloadUrl,
        filename,
        deleteOldMod?.file,
        {
          projectId: modId !== undefined ? String(modId) : undefined,
          projectSlug: modSlug,
          provider: 'curseforge',
          ...build,
        },
      );
      onModInstalled(installed);
      if (deleteOldMod && deleteOldMod.file !== installed.file) {
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

      const primaryExistingMod = primaryMod.existingMod ?? findExistingOldJar(primaryMod.filename, {
        name: primaryMod.modName || primaryMod.title,
        slug: primaryMod.modSlug || '',
      });

      const downloadedPrimary = await api.downloadMod(
        server.id,
        primaryMod.url,
        primaryMod.filename,
        primaryExistingMod?.file,
        {
          projectId: primaryMod.modId !== undefined ? String(primaryMod.modId) : undefined,
          projectSlug: primaryMod.modSlug,
          provider: 'curseforge',
          ...primaryMod.build,
        },
      );

      onModInstalled(downloadedPrimary);
      if (primaryExistingMod && primaryExistingMod.file !== downloadedPrimary.file) {
        onModDeleted?.(primaryExistingMod.name);
      }
      if (primaryMod.modSlug) {
        setInstalledSlugs((prev) => new Set([...prev, primaryMod.modSlug!, primaryMod.title.toLowerCase()]));
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
          name: dep.title,
          slug: dep.slug,
        });

        const downloadedDep = await api.downloadMod(
          server.id,
          dep.downloadUrl,
          dep.filename,
          depExistingMod?.file,
          {
            projectId: dep.id,
            projectSlug: dep.slug,
            provider: 'curseforge',
            releaseType: dep.releaseType,
            versionId: dep.versionId,
          },
        );

        onModInstalled(downloadedDep);
        if (depExistingMod && depExistingMod.file !== downloadedDep.file) {
          onModDeleted?.(depExistingMod.name);
        }
        setInstalledSlugs((prev) => new Set([...prev, dep.slug, dep.title.toLowerCase()]));
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

  async function handleOneClickInstall(mod: CurseForgeMod) {
    if (!configured) return;
    setInstallingId(mod.id);
    setError(null);

    try {
      const installed = findInstalledCurseForgeMod(mod, installedMods);
      const preferred = installedChannel(installed, []);
      const matches = (file: CurseForgeFile, channel: string) =>
        buildChannel(file) === channel && file.fileName.toLowerCase().endsWith('.jar');
      const builds = await getCurseForgeFiles(mod.id, {
        gameVersion: server.version, modLoaderType: loaderType, allPages: true,
        stopWhen: !installed || installed.release_type ? (file) => matches(file, preferred) : undefined,
      });
      const channel = installedChannel(installed, builds);
      const targetFile = builds.find((file) => matches(file, channel));

      if (!targetFile) {
        setSelectedMod(mod);
        setNotification({
          type: 'error',
          text: `No compatible ${modChannelLabel(channel)} build found. Choose a channel to see other builds.`,
        });
        setInstallingId(null);
        return;
      }

      const build = { releaseType: buildChannel(targetFile), versionId: String(targetFile.id) };
      let downloadUrl = targetFile.downloadUrl;
      if (!downloadUrl) {
        downloadUrl = await getCurseForgeDownloadUrl(mod.id, targetFile.id);
      }

      // Check for dependencies before proceeding
      if (targetFile.dependencies && targetFile.dependencies.length > 0) {
        try {
          const resolution = await resolveCurseForgeDependencies({
            file: targetFile,
            primaryMod: mod,
            gameVersion: server.version,
            modLoaderType: loaderType,
            installedMods,
            installedSlugs,
          });

          if (resolution.missingCount > 0) {
            setInstallingId(null);
            setDependencyPrompt({
              primaryMod: {
                title: mod.name,
                versionNumber: targetFile.displayName || targetFile.fileName,
                filename: targetFile.fileName,
                iconUrl: mod.logo?.thumbnailUrl || mod.logo?.url,
                url: downloadUrl,
                build,
                modId: mod.id,
                modSlug: mod.slug,
                modName: mod.name,
              },
              dependencies: resolution.dependencies,
            });
            return;
          }
        } catch {
          // If resolution encounters an error, proceed with standard install
        }
      }

      const existingMod = findExistingOldJar(targetFile.fileName, mod);
      if (existingMod) {
        setInstallingId(null);
        setUpdatePrompt({
          projectTitle: mod.name,
          existingMod,
          targetUrl: downloadUrl,
          build,
          targetFilename: targetFile.fileName,
          modId: mod.id,
          modSlug: mod.slug,
          modName: mod.name,
        });
        return;
      }

      await executeInstall(downloadUrl, targetFile.fileName, mod.name, mod.id, mod.slug, mod.name, build);
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Download failed',
      });
      setInstallingId(null);
    }
  }

  async function handleInstallFile(file: CurseForgeFile, modName: string) {
    if (!configured || !selectedMod) return;
    const releaseType = buildChannel(file);
    const switching = Boolean(selectedInstalled && currentChannel !== releaseType);
    if (switching && (!canSwitchModChannel(currentChannel, releaseType) || !(await confirm(
      `Switch ${modName} from ${modChannelLabel(currentChannel)} to ${modChannelLabel(releaseType)}? Future updates will stay on ${modChannelLabel(releaseType)}.`,
      { title: 'Switch Release Channel', confirmLabel: `Switch to ${modChannelLabel(releaseType)}` },
    )))) return;
    const build = { releaseType, versionId: String(file.id), allowChannelSwitch: switching };
    setInstallingId(file.id);

    try {
      let downloadUrl = file.downloadUrl;
      if (!downloadUrl) {
        downloadUrl = await getCurseForgeDownloadUrl(selectedMod.id, file.id);
      }

      if (file.dependencies && file.dependencies.length > 0) {
        try {
          const resolution = await resolveCurseForgeDependencies({
            file,
            primaryMod: selectedMod ?? undefined,
            gameVersion: server.version,
            modLoaderType: loaderType,
            installedMods,
            installedSlugs,
          });

          if (resolution.missingCount > 0) {
            setInstallingId(null);
            setDependencyPrompt({
              primaryMod: {
                title: modName,
                versionNumber: file.displayName || file.fileName,
                filename: file.fileName,
                iconUrl: selectedMod?.logo?.thumbnailUrl || selectedMod?.logo?.url,
                url: downloadUrl,
                build,
                existingMod: switching ? selectedInstalled : undefined,
                modId: selectedMod?.id,
                modSlug: selectedMod?.slug,
                modName: selectedMod?.name,
              },
              dependencies: resolution.dependencies,
            });
            return;
          }
        } catch {
          // Ignore
        }
      }

      const existingMod = findExistingOldJar(file.fileName, selectedMod);
      if (existingMod && !switching) {
        setInstallingId(null);
        setUpdatePrompt({
          projectTitle: modName,
          existingMod,
          targetUrl: downloadUrl,
          build,
          targetFilename: file.fileName,
          modId: selectedMod.id,
          modSlug: selectedMod.slug,
          modName: selectedMod.name,
        });
        return;
      }

      await executeInstall(downloadUrl, file.fileName, modName, selectedMod.id, selectedMod.slug, selectedMod.name, build, switching ? selectedInstalled : undefined);
    } catch (err: unknown) {
      setNotification({
        type: 'error',
        text: err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Download failed',
      });
      setInstallingId(null);
    }
  }

  function checkInstalled(mod: CurseForgeMod): boolean {
    if (installedSlugs.has(mod.slug) || installedSlugs.has(mod.name.toLowerCase())) {
      return true;
    }
    return isCurseForgeModInstalled(mod, installedMods);
  }

  if (loadingSettings) {
    return <p role="status" className="text-sm text-muted-foreground">Loading CurseForge settings…</p>;
  }
  if (settingsError) {
    return <div role="alert" className="space-y-2"><p className="text-sm text-destructive">{settingsError}</p><Button variant="outline" onClick={() => void refreshSettings()}>Retry</Button></div>;
  }
  if (!configured || showKeyConfig) {
    return (
      <div className="max-w-xl mx-auto space-y-3">
        <CurseForgeSettings onSaved={() => setShowKeyConfig(false)} />
        {configured && <Button variant="outline" onClick={() => setShowKeyConfig(false)}>Back to catalog</Button>}
      </div>
    );
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
            const existingMod = primaryMod.existingMod ?? findExistingOldJar(primaryMod.filename, {
              name: primaryMod.modName || primaryMod.title,
              slug: primaryMod.modSlug || '',
            });
            if (existingMod && !primaryMod.build.allowChannelSwitch) {
              setUpdatePrompt({
                projectTitle: primaryMod.title,
                existingMod,
                targetUrl: primaryMod.url,
                build: primaryMod.build,
                targetFilename: primaryMod.filename,
                modId: primaryMod.modId,
                modSlug: primaryMod.modSlug,
                modName: primaryMod.modName,
              });
              return;
            }
            void executeInstall(
              primaryMod.url,
              primaryMod.filename,
              primaryMod.title,
              primaryMod.modId,
              primaryMod.modSlug,
              primaryMod.modName,
              primaryMod.build,
              existingMod,
            );
          }}
          onCancel={() => {
            setDependencyPrompt(null);
            setInstallingId(null);
          }}
        />
      )}

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
              updatePrompt.build,
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
              updatePrompt.build,
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
          className={`fixed bottom-6 right-6 z-50 flex items-center gap-3 rounded-lg border px-4 py-3 shadow-lg transition-colors animate-in fade-in slide-in-from-bottom-2 ${
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
            <strong>CurseForge:</strong> Connected with the shared server-side API key.
          </span>
        </div>
        <div className="flex items-center gap-3">
          {can('settings.manage') && <button
            type="button"
            onClick={() => {
              setShowKeyConfig(true);
            }}
            className="inline-flex items-center gap-1 font-semibold text-orange-600 hover:underline dark:text-orange-400"
          >
            <Settings className="h-3 w-3" />
            Config API Key
          </button>}
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
                <button aria-label="Close dialog"
                  type="button"
                  onClick={() => setQuery('')}
                  className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  <X className="h-4 w-4" />
                </button>
              )}
            </div>

            <div className="flex items-center gap-2">
              <Select
                aria-label="Filter category"
                value={classId}
                onValueChange={(selectedValue) => setClassId(Number(selectedValue))}
                className="h-9 rounded-md border border-input bg-background px-3 py-1 text-xs  focus:outline-hidden focus:ring-1 focus:ring-ring"
              >
                <option value={6}>Mods</option>
                <option value={5}>Bukkit Plugins</option>
              </Select>

              <Select
                aria-label="Sort by"
                value={sortField}
                onValueChange={(selectedValue) => setSortField(Number(selectedValue))}
                className="h-9 rounded-md border border-input bg-background px-3 py-1 text-xs  focus:outline-hidden focus:ring-1 focus:ring-ring"
              >
                {CF_SORT_OPTIONS.map((opt) => (
                  <option key={opt.id} value={opt.id}>
                    {opt.label}
                  </option>
                ))}
              </Select>
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
        <DialogBackdrop onDismiss={() => setSelectedMod(null)}


          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4  animate-in fade-in"
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
              <button aria-label="Close dialog"
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
                        className="shrink-0 border-emerald-500/40 bg-background/60 font-mono text-[11px] text-emerald-600 dark:text-emerald-400"
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

              <ModChannelFilter value={visibleChannel} channels={files.map(buildChannel)} onChange={setChannelFilter} />
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

              {!loadingFiles && visibleBuilds.length === 0 && !fileError && (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  No builds match this channel.
                </p>
              )}

              {!loadingFiles &&
                visibleBuilds.map((fl) => {
                  const channel = buildChannel(fl);
                  const switching = Boolean(selectedInstalled && currentChannel !== channel);
                  const locked = switching && !canSwitchModChannel(currentChannel, channel);

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
                          <ModChannelBadge channel={channel} />
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
                            disabled={installingId === fl.id || locked}
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
                          disabled={installingId === fl.id || locked}
                          className="h-8 gap-1.5 text-xs bg-primary hover:bg-primary/90 text-primary-foreground shrink-0"
                        >
                          <Download className="h-3.5 w-3.5" />
                          {locked ? 'Channel locked' : switching ? `Switch to ${modChannelLabel(channel)}` : 'Install'}
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
        </DialogBackdrop>
      )}
    </div>
  );
}
