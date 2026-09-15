import { useCallback, useEffect, useState } from 'react';
import {
  ArrowUpCircle,
  CheckCircle2,
  HardDrive,
  Loader2,
  Lock,
  Package,
  RefreshCw,
  Trash2,
  UploadCloud,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import {
  getCurseForgeApiKey,
  getCurseForgeFiles,
} from '../api/curseforge';
import type {
  InstalledModpack,
  ModpackManifest,
  Server,
} from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { useModal } from './ui/modal';
import { ModpackInstallDialog } from './modpack-install-dialog';

interface ModpacksPanelProps {
  server: Server;
  onViewInstalledMods?: () => void;
  onModpackChanged?: () => void;
}

export function ModpacksPanel({
  server,
  onViewInstalledMods,
  onModpackChanged,
}: ModpacksPanelProps) {
  const [installedModpack, setInstalledModpack] = useState<InstalledModpack | null>(null);
  const [loadingInstalled, setLoadingInstalled] = useState(true);

  // Install / Update Dialog State
  const [dialogManifest, setDialogManifest] = useState<ModpackManifest | null>(null);
  const [dialogOptions, setDialogOptions] = useState<{
    source: string;
    url?: string;
    project_id?: string;
    project_slug?: string;
    version_id?: string;
  } | null>(null);
  const [dialogFile, setDialogFile] = useState<File | undefined>(undefined);

  const [uninstalling, setUninstalling] = useState(false);
  const { confirm, dialog } = useModal();

  // Modpack update state
  const [packVersions, setPackVersions] = useState<Array<{
    id: string;
    name: string;
    version_number: string;
    game_versions: string[];
    loaders: string[];
    release_type?: string;
    date_published?: string;
    downloadUrl: string;
  }>>([]);
  const [loadingPackVersions, setLoadingPackVersions] = useState(false);
  const [packVersionsError, setPackVersionsError] = useState<string | null>(null);

  const loadPackVersions = useCallback(async (pack: InstalledModpack) => {
    if (!pack) return;
    setLoadingPackVersions(true);
    setPackVersionsError(null);
    try {
      if (pack.format === 'modrinth' && (pack.project_slug || pack.project_id)) {
        const id = pack.project_slug || pack.project_id;
        const res = await fetch(`https://api.modrinth.com/v2/project/${id}/version`, {
          headers: { Accept: 'application/json' },
        });
        if (!res.ok) throw new Error('Failed to fetch modpack versions');
        const list = (await res.json()) as Array<{
          id: string;
          name: string;
          version_number: string;
          game_versions: string[];
          loaders: string[];
          version_type?: string;
          date_published?: string;
          files: Array<{ url: string; filename: string; primary: boolean }>;
        }>;
        const mapped = list
          .map((v) => {
            const f = v.files.find((file) => file.filename.endsWith('.mrpack')) || v.files[0];
            return {
              id: v.id,
              name: v.name || v.version_number,
              version_number: v.version_number,
              game_versions: v.game_versions,
              loaders: v.loaders,
              release_type: v.version_type,
              date_published: v.date_published,
              downloadUrl: f ? f.url : '',
            };
          })
          .filter((v) => !!v.downloadUrl);
        setPackVersions(mapped);
      } else if (pack.format === 'curseforge' && pack.project_id) {
        const apiKey = getCurseForgeApiKey();
        if (apiKey) {
          const files = await getCurseForgeFiles(Number(pack.project_id), apiKey, { pageSize: 25 });
          const mapped = files
            .filter((f) => !!f.downloadUrl)
            .map((f) => ({
              id: String(f.id),
              name: f.displayName || f.fileName,
              version_number: f.displayName || f.fileName,
              game_versions: f.gameVersions || [],
              loaders: f.gameVersions?.filter((v) =>
                ['forge', 'fabric', 'neoforge', 'quilt'].includes(v.toLowerCase())
              ) || [],
              release_type: f.releaseType === 1 ? 'release' : f.releaseType === 2 ? 'beta' : 'alpha',
              date_published: f.fileDate,
              downloadUrl: f.downloadUrl as string,
            }));
          setPackVersions(mapped);
        } else {
          setPackVersions([]);
        }
      } else {
        setPackVersions([]);
      }
    } catch (err) {
      setPackVersionsError(err instanceof Error ? err.message : 'Failed to fetch versions');
      setPackVersions([]);
    } finally {
      setLoadingPackVersions(false);
    }
  }, []);

  function handleUpdateToVersion(ver: (typeof packVersions)[0]) {
    if (!installedModpack) return;
    const manifest: ModpackManifest = {
      format: installedModpack.format,
      name: installedModpack.name,
      version: ver.version_number,
      summary: installedModpack.summary,
      author: installedModpack.author,
      minecraft_version: ver.game_versions[0] || installedModpack.minecraft_version,
      loader: ver.loaders[0] || installedModpack.loader,
      loader_version: ver.id,
      total_files: 0,
      server_files: 0,
      client_only_files: 0,
      icon_url: installedModpack.icon_url,
    };
    setDialogManifest(manifest);
    setDialogOptions({
      source: installedModpack.source,
      url: ver.downloadUrl,
      project_id: installedModpack.project_id,
      project_slug: installedModpack.project_slug,
      version_id: ver.id,
    });
    setDialogFile(undefined);
  }

  async function handleFileSelect(file: File) {
    if (!installedModpack) return;
    try {
      const manifest = await api.inspectModpackFile(server.id, file);
      setDialogManifest(manifest);
      setDialogOptions({
        source: 'upload',
      });
      setDialogFile(file);
    } catch (err) {
      setPackVersionsError(err instanceof Error ? err.message : 'Failed to inspect modpack archive');
    }
  }

  // 1. Fetch Installed Modpack
  const loadInstalled = useCallback(async () => {
    setLoadingInstalled(true);
    try {
      const res = await api.getInstalledModpack(server.id);
      const pack = res.installed ? res.modpack : null;
      setInstalledModpack(pack);
      if (pack) {
        void loadPackVersions(pack);
      }
    } catch {
      setInstalledModpack(null);
    } finally {
      setLoadingInstalled(false);
    }
  }, [server.id, loadPackVersions]);

  useEffect(() => {
    void loadInstalled();
  }, [loadInstalled]);

  // Handle Uninstall
  async function handleUninstall() {
    if (!installedModpack) return;
    const confirmed = await confirm(
      `Are you sure you want to uninstall "${installedModpack.name}"? This will delete all installed modpack jars and configs.`,
      {
        title: 'Uninstall Modpack',
        confirmLabel: 'Uninstall',
        destructive: true,
      },
    );
    if (!confirmed) return;

    setUninstalling(true);
    try {
      await api.uninstallModpack(server.id);
      setInstalledModpack(null);
      setPackVersions([]);
      onModpackChanged?.();
    } catch (err) {
      alert(err instanceof ApiError ? err.detail : 'Failed to uninstall modpack');
    } finally {
      setUninstalling(false);
    }
  }

  if (loadingInstalled) {
    return (
      <div className="flex h-48 items-center justify-center gap-2 text-xs text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin text-primary" />
        Loading modpack status...
      </div>
    );
  }

  return (
    <>
      {dialog}

      <div className="space-y-6">
        {/* ACTIVE MODPACK HERO CARD */}
        {installedModpack ? (
          <Card className="border-primary/30 bg-gradient-to-br from-card via-card to-primary/5 shadow-md">
            <CardHeader className="pb-3">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-center gap-3.5">
                  <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10 border border-primary/20 text-primary shadow-xs">
                    {installedModpack.icon_url ? (
                      <img
                        src={installedModpack.icon_url}
                        alt={installedModpack.name}
                        className="h-full w-full rounded-2xl object-cover"
                      />
                    ) : (
                      <Package className="h-7 w-7" />
                    )}
                  </div>
                  <div>
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-xs font-semibold text-primary uppercase tracking-wider">
                        Active Modpack
                      </span>
                      {installedModpack.created_with_modpack && (
                        <Badge
                          variant="outline"
                          className="border-amber-500/40 text-amber-600 dark:text-amber-400 text-[10px] uppercase font-bold px-2 py-0.5 gap-1"
                        >
                          <Lock className="h-3 w-3" />
                          Modpack Server (Locked)
                        </Badge>
                      )}
                      <Badge variant="secondary" className="text-[10px] uppercase font-bold px-2 py-0.5">
                        {installedModpack.format}
                      </Badge>
                      <Badge variant="outline" className="text-[10px] font-medium capitalize">
                        {installedModpack.loader} {installedModpack.minecraft_version}
                      </Badge>
                    </div>
                    <CardTitle className="text-xl font-bold mt-1">
                      {installedModpack.name}
                    </CardTitle>
                    <CardDescription className="text-xs mt-0.5">
                      Version {installedModpack.version}
                      {installedModpack.author ? ` · by ${installedModpack.author}` : ''}
                      {installedModpack.installed_at
                        ? ` · Installed ${new Date(installedModpack.installed_at).toLocaleDateString()}`
                        : ''}
                    </CardDescription>
                  </div>
                </div>

                <div className="flex items-center gap-2 self-start sm:self-auto">
                  {onViewInstalledMods && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={onViewInstalledMods}
                      className="h-8 gap-1.5 text-xs"
                    >
                      <HardDrive className="h-3.5 w-3.5" />
                      View Mods ({installedModpack.installed_files?.length || 0})
                    </Button>
                  )}
                  {!installedModpack.created_with_modpack && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => void handleUninstall()}
                      disabled={uninstalling}
                      className="h-8 gap-1.5 text-xs text-destructive border-destructive/30 hover:bg-destructive/10"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      {uninstalling ? 'Uninstalling...' : 'Uninstall'}
                    </Button>
                  )}
                </div>
              </div>
            </CardHeader>
            {installedModpack.summary && (
              <CardContent className="pt-0">
                <p className="text-xs text-muted-foreground line-clamp-2">
                  {installedModpack.summary}
                </p>
              </CardContent>
            )}
          </Card>
        ) : (
          /* NO MODPACK INSTALLED */
          <div className="rounded-xl border border-dashed border-border/80 bg-card/40 p-8 text-center max-w-md mx-auto my-8">
            <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-secondary text-muted-foreground mb-3">
              <Package className="h-6 w-6" />
            </div>
            <h3 className="text-sm font-semibold text-foreground">No Modpack Installed</h3>
            <p className="text-xs text-muted-foreground mt-1.5 leading-relaxed">
              This server does not have a modpack installed. Modpack servers configure the entire server environment and are selected during server creation in the <strong>Create Server</strong> menu.
            </p>
          </div>
        )}

        {/* MODPACK UPDATES SECTION (ONLY SHOWN WHEN A MODPACK IS INSTALLED) */}
        {installedModpack && (
          <div className="space-y-4">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between border-b border-border pb-2.5">
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="text-sm font-bold text-foreground">Modpack Updates</h3>
                  <Badge
                    variant="outline"
                    className="text-[10px] font-semibold border-amber-500/40 text-amber-600 dark:text-amber-400 gap-1"
                  >
                    <Lock className="h-3 w-3" />
                    Locked to {installedModpack.name}
                  </Badge>
                </div>
                <p className="text-xs text-muted-foreground">
                  Check and update {installedModpack.name} to newer release versions.
                </p>
              </div>

              <Button
                variant="outline"
                size="sm"
                onClick={() => void loadPackVersions(installedModpack)}
                disabled={loadingPackVersions}
                className="h-8 gap-1.5 text-xs"
              >
                <RefreshCw className={`h-3.5 w-3.5 ${loadingPackVersions ? 'animate-spin' : ''}`} />
                Check Updates
              </Button>
            </div>

            <div className="rounded-xl border border-primary/20 bg-primary/5 p-3 text-xs flex items-center gap-2 text-muted-foreground">
              <Lock className="h-4 w-4 text-primary shrink-0" />
              <span>
                This server was created for <strong>{installedModpack.name}</strong> and cannot be switched to another modpack. You can update to newer releases below.
              </span>
            </div>

            {loadingPackVersions ? (
              <div className="flex h-36 items-center justify-center gap-2 text-xs text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin text-primary" />
                Checking for newer releases of {installedModpack.name}...
              </div>
            ) : packVersionsError ? (
              <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-xs text-destructive">
                {packVersionsError}
              </div>
            ) : packVersions.length > 0 ? (
              <div className="space-y-2">
                {packVersions.map((v) => {
                  const isCurrent =
                    v.version_number === installedModpack.version ||
                    v.name === installedModpack.version ||
                    v.id === installedModpack.version;
                  return (
                    <Card
                      key={v.id}
                      className={`border-border/80 transition-all ${
                        isCurrent ? 'bg-primary/5 border-primary/40' : 'hover:border-primary/50'
                      }`}
                    >
                      <CardContent className="p-3.5 flex items-center justify-between gap-3">
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2 flex-wrap">
                            <h4 className="font-bold text-xs text-foreground">{v.name}</h4>
                            <Badge variant="secondary" className="text-[10px] font-mono px-1.5 py-0">
                              {v.version_number}
                            </Badge>
                            {v.release_type && (
                              <Badge variant="outline" className="text-[9px] uppercase font-semibold">
                                {v.release_type}
                              </Badge>
                            )}
                            {isCurrent && (
                              <Badge
                                variant="secondary"
                                className="bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 font-bold text-[10px] gap-1 px-1.5 py-0"
                              >
                                <CheckCircle2 className="h-3 w-3" />
                                Current Active Version
                              </Badge>
                            )}
                          </div>
                          <p className="text-[11px] text-muted-foreground mt-0.5">
                            MC: {v.game_versions.join(', ') || installedModpack.minecraft_version}
                            {v.loaders.length > 0 ? ` · ${v.loaders.join(', ')}` : ''}
                            {v.date_published ? ` · Released ${new Date(v.date_published).toLocaleDateString()}` : ''}
                          </p>
                        </div>

                        {!isCurrent && (
                          <Button
                            size="sm"
                            onClick={() => handleUpdateToVersion(v)}
                            className="h-7 px-3 text-xs font-semibold gap-1.5 bg-primary text-primary-foreground shrink-0"
                          >
                            <ArrowUpCircle className="h-3.5 w-3.5" />
                            Update
                          </Button>
                        )}
                      </CardContent>
                    </Card>
                  );
                })}

                {/* Manual Archive Update Option */}
                <div className="pt-2">
                  <div className="rounded-xl border border-dashed border-border/80 bg-card/50 p-4 text-center space-y-2">
                    <p className="text-xs text-muted-foreground">
                      Have a newer <code>.mrpack</code> or <code>.zip</code> file for <strong>{installedModpack.name}</strong>?
                    </p>
                    <label className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-primary/40 bg-primary/10 text-primary font-semibold text-xs cursor-pointer hover:bg-primary/20 transition-all">
                      <UploadCloud className="h-3.5 w-3.5" />
                      Upload Update File
                      <input
                        type="file"
                        accept=".mrpack,.zip"
                        className="hidden"
                        onChange={(e) => {
                          const f = e.target.files?.[0];
                          if (f) void handleFileSelect(f);
                        }}
                      />
                    </label>
                  </div>
                </div>
              </div>
            ) : (
              <div className="rounded-xl border border-border/80 bg-card p-6 text-center space-y-3">
                <Package className="h-8 w-8 text-muted-foreground mx-auto" />
                <h4 className="text-sm font-semibold text-foreground">Upload Updated Modpack Archive</h4>
                <p className="text-xs text-muted-foreground max-w-md mx-auto">
                  To update <strong>{installedModpack.name}</strong>, upload an updated <code>.mrpack</code> or <code>.zip</code> archive.
                </p>
                <label className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-primary/40 bg-primary/10 text-primary font-semibold text-xs cursor-pointer hover:bg-primary/20 transition-all">
                  <UploadCloud className="h-3.5 w-3.5" />
                  Choose Update File
                  <input
                    type="file"
                    accept=".mrpack,.zip"
                    className="hidden"
                    onChange={(e) => {
                      const f = e.target.files?.[0];
                      if (f) void handleFileSelect(f);
                    }}
                  />
                </label>
              </div>
            )}
          </div>
        )}
      </div>

      {/* UPDATE DIALOG */}
      {dialogManifest && dialogOptions && (
        <ModpackInstallDialog
          server={server}
          manifest={dialogManifest}
          installOptions={dialogOptions}
          modpackFile={dialogFile}
          isUpdate={true}
          onClose={() => {
            setDialogManifest(null);
            setDialogOptions(null);
            setDialogFile(undefined);
          }}
          onInstalled={() => {
            void loadInstalled();
            onModpackChanged?.();
          }}
        />
      )}
    </>
  );
}
