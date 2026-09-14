import { useCallback, useEffect, useState } from 'react';
import {
  ArrowRight,
  ArrowUpCircle,
  CheckCircle2,
  Compass,
  FolderOpen,
  Loader2,
  Plus,
  Power,
  RefreshCw,
  Search,
  Sparkles,
  Trash2,
  UploadCloud,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Mod, Server } from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { useModal } from './ui/modal';
import { CatalogBrowser } from './catalog-browser';

import { extractModId } from '../api/modrinth';
import type { ModUpdateInfo } from '../api/mod-updates';
import { ModJarPickerDialog } from './mod-jar-picker-dialog';

export function ModsPanel({ server }: { server: Server }) {
  const [activeTab, setActiveTab] = useState<'installed' | 'updates' | 'browse'>('installed');
  const [items, setItems] = useState<Mod[]>([]);
  const [type, setType] = useState<'mods' | 'plugins'>('mods');
  const [error, setError] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [installedSearch, setInstalledSearch] = useState('');
  const [updates, setUpdates] = useState<Record<string, ModUpdateInfo>>({});
  const [checkingUpdates, setCheckingUpdates] = useState(false);
  const [lastChecked, setLastChecked] = useState<Date | null>(null);
  const [pickerMod, setPickerMod] = useState<{ mod: Mod; updateInfo?: ModUpdateInfo } | null>(null);
  const { confirm, dialog } = useModal();

  const unsupported = server.server_type === 'vanilla';

  const checkForUpdates = useCallback(
    async (installedList?: Mod[], force = false) => {
      const list = installedList ?? items;
      if (unsupported || list.length === 0) {
        setUpdates({});
        return;
      }
      setCheckingUpdates(true);
      try {
        // Query server-side updates endpoint (cached, rate-limited, and paced on the server)
        const serverRes = await api.checkModUpdates(server.id, force);
        const mappedUpdates: Record<string, ModUpdateInfo> = {};
        if (
          serverRes?.updates &&
          typeof serverRes.updates === 'object' &&
          !Array.isArray(serverRes.updates)
        ) {
          for (const [key, u] of Object.entries(serverRes.updates)) {
            mappedUpdates[key] = {
              modName: u.mod_name,
              modFile: u.mod_file,
              provider: u.provider,
              projectId: u.project_id,
              projectSlug: u.project_slug,
              title: u.title,
              currentVersion: u.current_version,
              latestVersion: u.latest_version,
              latestJar: u.latest_jar,
              latestDownloadUrl: u.latest_download_url,
              latestReleaseType: u.latest_release_type,
              latestReleaseDate: u.latest_release_date,
              changelog: u.changelog,
            };
          }
        }
        setUpdates(mappedUpdates);
        if (serverRes?.last_checked) {
          setLastChecked(new Date(serverRes.last_checked));
        } else {
          setLastChecked(new Date());
        }
      } catch {
        // Server-side update check error (handled gracefully without client-side spamming)
      } finally {
        setCheckingUpdates(false);
      }
    },
    [items, server, unsupported],
  );

  const load = useCallback(
    async (andCheckUpdates = false) => {
      if (unsupported) {
        setItems([]);
        setError(null);
        return;
      }
      try {
        const res = await api.mods(server.id);
        const loadedItems = res.items ?? [];
        setItems(loadedItems);
        setType(res.type);
        setError(null);
        if (andCheckUpdates) {
          void checkForUpdates(loadedItems);
        }
      } catch (err) {
        setError(err instanceof ApiError ? err.detail : 'Failed to load mods');
      }
    },
    [server.id, unsupported, checkForUpdates],
  );

  useEffect(() => {
    void load(true);
  }, [load]);

  async function upload(file: File) {
    setError(null);

    const baseId = extractModId(file.name).replace(/_/g, '-');
    const existingOld = items.find((m) => {
      if (m.file.toLowerCase() === file.name.toLowerCase()) return false;
      const mId = extractModId(m.file).replace(/_/g, '-');
      return mId === baseId && mId.length > 0;
    });

    let deleteOldName: string | undefined;
    if (existingOld) {
      const shouldDelete = await confirm(
        `An older version (${existingOld.file}) is installed on your server. Would you like to delete the old jar file to prevent conflicts?`,
        {
          title: 'Older Jar Version Detected',
          confirmLabel: 'Delete Old Jar & Upload',
          cancelLabel: 'Keep Both',
        },
      );
      if (shouldDelete) {
        deleteOldName = existingOld.name;
      }
    }

    setUploadProgress(0);
    try {
      await api.uploadMod(
        server.id,
        file,
        (loaded, total) => {
          setUploadProgress(total > 0 ? Math.round((loaded / total) * 100) : 0);
        },
        deleteOldName,
      );
      setUploadProgress(null);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Upload failed');
      setUploadProgress(null);
    }
  }

  async function toggle(mod: Mod) {
    setError(null);
    try {
      await api.setModEnabled(server.id, mod.name, !mod.enabled);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Update failed');
    }
  }

  async function remove(mod: Mod) {
    if (
      !(await confirm(`Delete ${mod.file}? This cannot be undone.`, {
        title: 'Delete artifact',
        confirmLabel: 'Delete',
        destructive: true,
      }))
    ) {
      return;
    }
    setError(null);
    try {
      await api.deleteMod(server.id, mod.name);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Delete failed');
    }
  }

  const label = type === 'mods' ? 'Mods' : 'Plugins';
  const updateCount = Object.keys(updates).length;

  if (unsupported) {
    return (
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Mods &amp; plugins</CardTitle>
          <CardDescription>Manage installed artifacts.</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            This server type doesn&apos;t support mods or plugins. Switch server software to Paper,
            Fabric, Forge, or NeoForge in settings to enable modding.
          </p>
        </CardContent>
      </Card>
    );
  }

  const filteredItems = items.filter(
    (m) =>
      m.name.toLowerCase().includes(installedSearch.toLowerCase()) ||
      m.file.toLowerCase().includes(installedSearch.toLowerCase()),
  );

  return (
    <>
      {dialog}

      <div className="space-y-4">
        {/* Sub-navigation Tabs */}
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between border-b border-border/80 pb-2.5">
          <div className="flex items-center gap-1.5 flex-wrap">
            <button
              type="button"
              onClick={() => setActiveTab('installed')}
              className={`inline-flex items-center gap-2 rounded-lg px-3.5 py-2 text-sm font-semibold transition-all ${
                activeTab === 'installed'
                  ? 'bg-primary text-primary-foreground shadow-xs'
                  : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
              }`}
            >
              <FolderOpen className="h-4 w-4" />
              Installed {label}
              <Badge
                variant={activeTab === 'installed' ? 'secondary' : 'outline'}
                className="ml-1 px-1.5 py-0 text-xs"
              >
                {items.length}
              </Badge>
            </button>

            <button
              type="button"
              onClick={() => setActiveTab('updates')}
              className={`inline-flex items-center gap-2 rounded-lg px-3.5 py-2 text-sm font-semibold transition-all ${
                activeTab === 'updates'
                  ? 'bg-primary text-primary-foreground shadow-xs'
                  : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
              }`}
            >
              <ArrowUpCircle className="h-4 w-4" />
              Mod Update Tool
              {updateCount > 0 ? (
                <Badge
                  variant="secondary"
                  className="ml-1 bg-amber-500/20 text-amber-700 dark:text-amber-300 border border-amber-500/30 px-1.5 py-0 text-xs font-semibold animate-pulse"
                >
                  {updateCount}
                </Badge>
              ) : checkingUpdates ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground ml-1" />
              ) : null}
            </button>

            <button
              type="button"
              onClick={() => setActiveTab('browse')}
              className={`inline-flex items-center gap-2 rounded-lg px-3.5 py-2 text-sm font-semibold transition-all ${
                activeTab === 'browse'
                  ? 'bg-primary text-primary-foreground shadow-xs'
                  : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
              }`}
            >
              <Compass className="h-4 w-4" />
              Browse Modrinth &amp; Catalogs
              <span className="flex h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />
            </button>
          </div>

          <div className="flex items-center gap-2 self-end sm:self-auto">
            <Button
              variant="outline"
              size="sm"
              onClick={() => void checkForUpdates(undefined, true)}
              disabled={checkingUpdates || items.length === 0}
              className="h-8 gap-1.5 text-xs border-amber-500/30 hover:bg-amber-500/10 text-foreground"
              title="Check online catalogs for newer mod releases"
            >
              <ArrowUpCircle
                className={`h-3.5 w-3.5 ${
                  checkingUpdates ? 'animate-spin text-primary' : 'text-amber-600 dark:text-amber-400'
                }`}
              />
              {checkingUpdates ? 'Checking...' : 'Check Updates'}
            </Button>

            <Button
              variant="outline"
              size="sm"
              onClick={() => void load(true)}
              className="h-8 gap-1.5 text-xs"
              title="Refresh installed items"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              Refresh
            </Button>
          </div>
        </div>

        {/* Tab 1: Installed Mods / Plugins */}
        {activeTab === 'installed' && (
          <Card>
            <CardHeader className="pb-3">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <CardTitle className="text-base">Installed {label}</CardTitle>
                  <CardDescription>
                    Enable, disable, delete, or upload custom {label.toLowerCase()}.
                  </CardDescription>
                </div>

                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setActiveTab('browse')}
                    className="gap-1.5 text-xs border-primary/30 text-primary hover:bg-primary/5"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    Browse Catalogs
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* Upload Dropzone / File Picker */}
              <div className="flex flex-col sm:flex-row sm:items-center gap-3 rounded-lg border border-dashed border-border/80 bg-secondary/15 p-3.5">
                <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <UploadCloud className="h-5 w-5" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-xs font-medium text-foreground">Upload .jar artifact</p>
                  <p className="text-xs text-muted-foreground">
                    Directly drop or choose a jar file to place in the {type} folder.
                  </p>
                </div>
                <label className="cursor-pointer shrink-0">
                  <span className="inline-flex items-center gap-1.5 rounded-md bg-secondary px-3 py-1.5 text-xs font-medium text-secondary-foreground shadow-2xs hover:bg-secondary/80 transition-colors">
                    Choose file
                  </span>
                  <input
                    type="file"
                    accept=".jar"
                    className="hidden"
                    onChange={(e) => {
                      const f = e.target.files?.[0];
                      if (f) void upload(f);
                    }}
                  />
                </label>
              </div>

              {error && <p className="text-sm text-destructive">{error}</p>}

              {uploadProgress !== null && (
                <div className="flex items-center gap-3 rounded-md border p-3 text-sm">
                  <span className="shrink-0 text-muted-foreground">Uploading...</span>
                  <div className="h-2 flex-1 overflow-hidden rounded-full bg-secondary">
                    <div
                      className="h-full bg-primary transition-all"
                      style={{ width: `${uploadProgress}%` }}
                    />
                  </div>
                  <span className="shrink-0 tabular-nums text-muted-foreground">
                    {uploadProgress}%
                  </span>
                </div>
              )}

              {/* Search filter for installed items */}
              {items.length > 3 && (
                <div className="relative">
                  <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    placeholder={`Filter installed ${label.toLowerCase()}...`}
                    value={installedSearch}
                    onChange={(e) => setInstalledSearch(e.target.value)}
                    className="h-8 pl-8 pr-7 text-xs"
                  />
                  {installedSearch && (
                    <button
                      type="button"
                      onClick={() => setInstalledSearch('')}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    >
                      <X className="h-3.5 w-3.5" />
                    </button>
                  )}
                </div>
              )}

              {/* Items List */}
              <div className="space-y-2">
                {items.length === 0 ? (
                  <div className="rounded-lg border border-dashed py-8 text-center">
                    <p className="text-sm text-muted-foreground">No {label.toLowerCase()} installed yet.</p>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setActiveTab('browse')}
                      className="mt-3 gap-1.5 text-xs"
                    >
                      <Compass className="h-3.5 w-3.5" />
                      Browse Online Catalogs
                    </Button>
                  </div>
                ) : filteredItems.length === 0 ? (
                  <p className="py-4 text-center text-xs text-muted-foreground">
                    No {label.toLowerCase()} match &quot;{installedSearch}&quot;.
                  </p>
                ) : (
                  filteredItems.map((m) => {
                    const updateInfo = updates[m.name];
                    return (
                      <div
                        key={m.name}
                        className={`flex items-center justify-between gap-2 rounded-lg border p-3 text-sm transition-colors ${
                          updateInfo
                            ? 'border-amber-500/40 bg-amber-500/5 hover:bg-amber-500/10'
                            : 'bg-card/60 hover:bg-secondary/20'
                        }`}
                      >
                        <div className="min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <p className="truncate font-medium text-foreground">{m.name}</p>
                            {updateInfo && (
                              <Badge
                                variant="outline"
                                className="border-amber-500/40 bg-amber-500/15 text-amber-700 dark:text-amber-300 text-[10px] px-1.5 py-0 font-medium shrink-0 flex items-center gap-1"
                              >
                                <ArrowUpCircle className="h-2.5 w-2.5" />
                                Update available: {updateInfo.latestVersion}
                              </Badge>
                            )}
                          </div>
                          <p className="truncate text-xs text-muted-foreground">
                            {m.file} ·{' '}
                            <span
                              className={
                                m.enabled
                                  ? 'font-medium text-emerald-600 dark:text-emerald-400'
                                  : 'text-muted-foreground'
                              }
                            >
                              {m.enabled ? 'enabled' : 'disabled'}
                            </span>
                            {m.version && ` · v${m.version}`}
                          </p>
                        </div>
                        <div className="flex shrink-0 items-center gap-1.5">
                          {updateInfo && (
                            <Button
                              variant="outline"
                              size="sm"
                              className="h-8 gap-1 text-xs font-semibold border-amber-500/40 bg-amber-500/10 text-amber-700 hover:bg-amber-500/20 dark:text-amber-300"
                              title={`Update available: ${updateInfo.latestJar}. Click to pick a new jar`}
                              onClick={() => setPickerMod({ mod: m, updateInfo })}
                            >
                              <ArrowUpCircle className="h-3.5 w-3.5 text-amber-600 dark:text-amber-400" />
                              Update
                            </Button>
                          )}
                          <Button
                            variant="outline"
                            size="icon"
                            className="h-8 w-8"
                            aria-label={m.enabled ? 'Disable' : 'Enable'}
                            title={m.enabled ? 'Disable' : 'Enable'}
                            onClick={() => void toggle(m)}
                          >
                            <Power
                              className={`h-4 w-4 ${
                                m.enabled
                                  ? 'text-emerald-600 dark:text-emerald-400'
                                  : 'text-muted-foreground'
                              }`}
                            />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-muted-foreground hover:text-destructive"
                            aria-label="Delete"
                            title="Delete"
                            onClick={() => void remove(m)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            </CardContent>
          </Card>
        )}

        {/* Tab 2: Mod Update Tool */}
        {activeTab === 'updates' && (
          <Card>
            <CardHeader className="pb-3">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <div className="flex items-center gap-2">
                    <CardTitle className="text-base">
                      <h3 className="text-base font-semibold">Mod Update Tool</h3>
                    </CardTitle>
                    {updateCount > 0 && (
                      <Badge className="bg-amber-500/20 text-amber-700 dark:text-amber-300 border-amber-500/30 text-xs px-2 py-0.5">
                        {updateCount} {updateCount === 1 ? 'update' : 'updates'} available
                      </Badge>
                    )}
                  </div>
                  <CardDescription>
                    Compare installed jar versions against online catalogs and pick a new jar to update.
                  </CardDescription>
                </div>

                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void checkForUpdates(undefined, true)}
                    disabled={checkingUpdates || items.length === 0}
                    className="gap-1.5 text-xs border-amber-500/30 text-foreground hover:bg-amber-500/10"
                  >
                    <RefreshCw className={`h-3.5 w-3.5 ${checkingUpdates ? 'animate-spin' : ''}`} />
                    {checkingUpdates ? 'Scanning catalogs...' : 'Re-check Updates'}
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {checkingUpdates && (
                <div className="flex items-center gap-3 rounded-lg border border-primary/20 bg-primary/5 p-4 text-xs text-foreground">
                  <Loader2 className="h-5 w-5 animate-spin text-primary shrink-0" />
                  <div className="min-w-0 flex-1">
                    <p className="font-semibold">Checking for updates...</p>
                    <p className="text-muted-foreground">Scanning Modrinth, Hangar, SpigotMC, and CurseForge for newer builds.</p>
                  </div>
                </div>
              )}

              {/* Updates Available List */}
              {updateCount > 0 ? (
                <div className="space-y-3">
                  <div className="flex items-center justify-between text-xs text-muted-foreground">
                    <span className="font-semibold text-foreground">Available Updates ({updateCount})</span>
                    <span>Click Update on any mod to view releases and pick a jar</span>
                  </div>

                  <div className="space-y-2.5">
                    {items
                      .filter((m) => Boolean(updates[m.name]))
                      .map((m) => {
                        const u = updates[m.name];
                        return (
                          <div
                            key={m.name}
                            className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 rounded-xl border border-amber-500/40 bg-card p-4 shadow-2xs transition-all hover:border-amber-500/60 hover:shadow-xs"
                          >
                            <div className="min-w-0 space-y-1.5 flex-1">
                              <div className="flex items-center gap-2 flex-wrap">
                                <span className="font-bold text-foreground text-sm">{u.title || m.name}</span>
                                <Badge variant="outline" className="border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300 text-[10px] px-1.5 py-0 font-mono uppercase">
                                  {u.provider}
                                </Badge>
                                {u.latestReleaseType && (
                                  <Badge variant="secondary" className="text-[10px] px-1.5 py-0 uppercase">
                                    {u.latestReleaseType}
                                  </Badge>
                                )}
                              </div>

                              <div className="flex items-center gap-2 text-xs flex-wrap rounded-lg bg-secondary/30 px-3 py-1.5 border border-border/60">
                                <div className="flex items-center gap-1.5 min-w-0">
                                  <span className="text-muted-foreground font-medium shrink-0">Installed:</span>
                                  <span className="font-mono text-destructive dark:text-red-400 font-semibold truncate max-w-[200px]" title={m.file}>
                                    {m.file}
                                  </span>
                                </div>
                                <ArrowRight className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                                <div className="flex items-center gap-1.5 min-w-0">
                                  <span className="text-muted-foreground font-medium shrink-0">New jar:</span>
                                  <span className="font-mono text-emerald-600 dark:text-emerald-400 font-semibold truncate max-w-[240px]" title={u.latestJar}>
                                    {u.latestJar}
                                  </span>
                                  <Badge variant="outline" className="border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 text-[10px] px-1 py-0 shrink-0">
                                    {u.latestVersion}
                                  </Badge>
                                </div>
                              </div>
                            </div>

                            <div className="flex items-center gap-2 shrink-0 sm:self-center">
                              <Button
                                size="sm"
                                onClick={() => setPickerMod({ mod: m, updateInfo: u })}
                                className="h-8 gap-1.5 text-xs bg-amber-600 hover:bg-amber-700 text-white font-semibold shadow-xs"
                              >
                                <ArrowUpCircle className="h-3.5 w-3.5" />
                                Update &amp; Pick Jar
                              </Button>
                            </div>
                          </div>
                        );
                      })}
                  </div>
                </div>
              ) : !checkingUpdates ? (
                <div className="rounded-xl border border-dashed border-emerald-500/30 bg-emerald-500/5 py-12 px-4 text-center">
                  <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 mb-3">
                    <CheckCircle2 className="h-6 w-6" />
                  </div>
                  <p className="text-base font-semibold text-foreground">All mods are up to date</p>
                  <p className="mt-1 text-xs text-muted-foreground max-w-sm mx-auto">
                    No updates were detected for your {items.length} installed {label.toLowerCase()} on {server.server_type} {server.version ? `(MC ${server.version})` : ''}.
                  </p>
                  {lastChecked && (
                    <p className="mt-2 text-[11px] text-muted-foreground font-mono">
                      Last checked: {lastChecked.toLocaleTimeString()}
                    </p>
                  )}
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => void checkForUpdates(undefined, true)}
                    className="mt-4 gap-1.5 text-xs"
                  >
                    <RefreshCw className="h-3.5 w-3.5" />
                    Check Again
                  </Button>
                </div>
              ) : null}

              {/* Up to Date Section when some updates are available */}
              {updateCount > 0 && items.length > updateCount && (
                <div className="pt-4 border-t border-border/60">
                  <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
                    Up to Date ({items.length - updateCount})
                  </p>
                  <div className="space-y-1.5">
                    {items
                      .filter((m) => !updates[m.name])
                      .map((m) => (
                        <div
                          key={m.name}
                          className="flex items-center justify-between gap-2 rounded-lg border bg-secondary/20 px-3 py-2 text-xs"
                        >
                          <div className="flex items-center gap-2 min-w-0">
                            <CheckCircle2 className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400 shrink-0" />
                            <span className="font-medium text-foreground truncate">{m.name}</span>
                            <span className="font-mono text-muted-foreground truncate text-[11px]">{m.file}</span>
                          </div>
                          <Badge variant="outline" className="border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 text-[10px] px-1.5 py-0 shrink-0">
                            Current
                          </Badge>
                        </div>
                      ))}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        )}

        {/* Tab 3: Mod & Plugin Catalogs */}
        {activeTab === 'browse' && (
          <CatalogBrowser
            server={server}
            installedMods={items}
            updates={updates}
            onOpenPicker={(mod, updateInfo) => setPickerMod({ mod, updateInfo })}
            onModInstalled={() => {
              void load(true);
            }}
            onModDeleted={() => {
              void load(true);
            }}
          />
        )}
      </div>

      {pickerMod && (
        <ModJarPickerDialog
          isOpen={true}
          server={server}
          mod={pickerMod.mod}
          updateInfo={pickerMod.updateInfo}
          onClose={() => setPickerMod(null)}
          onUpdated={() => {
            setPickerMod(null);
            void load(true);
          }}
        />
      )}
    </>
  );
}
