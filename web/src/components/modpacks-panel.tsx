import { useCallback, useEffect, useState } from 'react';
import {
  AlertCircle,
  Archive,
  ArrowRight,
  Box,
  CheckCircle2,
  Compass,
  Download,
  Flame,
  Globe,
  HardDrive,
  Layers,
  Loader2,
  Package,
  RefreshCw,
  Search,
  Sparkles,
  Trash2,
  UploadCloud,
  Wrench,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import {
  formatCount,
  formatFileSize,
  getServerLoaders,
  searchModrinth,
} from '../api/modrinth';
import {
  CF_CLASS_MODPACKS,
  CF_SORT_OPTIONS,
  getCurseForgeApiKey,
  getCurseForgeFiles,
  getCurseForgeLoaderType,
  hasCurseForgeApiKey,
  searchCurseForge,
} from '../api/curseforge';
import type {
  CurseForgeMod,
  InstalledModpack,
  ModpackManifest,
  ModrinthSearchHit,
  Server,
} from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { useModal } from './ui/modal';
import { ModpackInstallDialog } from './modpack-install-dialog';

interface ModpacksPanelProps {
  server: Server;
  onViewInstalledMods?: () => void;
  onModpackChanged?: () => void;
}

type ModpackSourceTab = 'modrinth' | 'curseforge' | 'upload' | 'url';

export function ModpacksPanel({
  server,
  onViewInstalledMods,
  onModpackChanged,
}: ModpacksPanelProps) {
  const [installedModpack, setInstalledModpack] = useState<InstalledModpack | null>(null);
  const [loadingInstalled, setLoadingInstalled] = useState(true);
  const [sourceTab, setSourceTab] = useState<ModpackSourceTab>('modrinth');

  // Modrinth State
  const [mrQuery, setMrQuery] = useState('');
  const [mrHits, setMrHits] = useState<ModrinthSearchHit[]>([]);
  const [mrLoading, setMrLoading] = useState(false);
  const [mrTotal, setMrTotal] = useState(0);
  const [mrLoaderFilter, setMrLoaderFilter] = useState<string>('all');
  const [mrSort, setMrSort] = useState<'downloads' | 'relevance' | 'updated' | 'newest'>('downloads');

  // CurseForge State
  const [cfQuery, setCfQuery] = useState('');
  const [cfHits, setCfHits] = useState<CurseForgeMod[]>([]);
  const [cfLoading, setCfLoading] = useState(false);
  const [cfTotal, setCfTotal] = useState(0);
  const [cfSort, setCfSort] = useState<number>(6);

  // Upload State
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [inspectingUpload, setInspectingUpload] = useState(false);
  const [uploadManifest, setUploadManifest] = useState<ModpackManifest | null>(null);
  const [uploadError, setUploadError] = useState<string | null>(null);

  // URL State
  const [inputUrl, setInputUrl] = useState('');
  const [inspectingUrl, setInspectingUrl] = useState(false);
  const [urlManifest, setUrlManifest] = useState<ModpackManifest | null>(null);
  const [urlError, setUrlError] = useState<string | null>(null);

  // Install Dialog State
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

  // 1. Fetch Installed Modpack
  const loadInstalled = useCallback(async () => {
    setLoadingInstalled(true);
    try {
      const res = await api.getInstalledModpack(server.id);
      setInstalledModpack(res.installed ? res.modpack : null);
    } catch {
      setInstalledModpack(null);
    } finally {
      setLoadingInstalled(false);
    }
  }, [server.id]);

  useEffect(() => {
    void loadInstalled();
  }, [loadInstalled]);

  // 2. Fetch Modrinth Modpacks
  const searchModrinthModpacks = useCallback(async () => {
    setMrLoading(true);
    try {
      const loaders =
        mrLoaderFilter !== 'all'
          ? [mrLoaderFilter]
          : server.server_type !== 'vanilla'
          ? getServerLoaders(server.server_type)
          : [];

      const res = await searchModrinth({
        query: mrQuery,
        projectType: 'modpack',
        loaders: loaders.length > 0 ? loaders : undefined,
        gameVersion: server.version,
        sort: mrSort,
        limit: 20,
      });

      setMrHits(res.hits ?? []);
      setMrTotal(res.total_hits ?? 0);
    } catch {
      setMrHits([]);
    } finally {
      setMrLoading(false);
    }
  }, [mrQuery, mrLoaderFilter, mrSort, server.server_type, server.version]);

  // 3. Fetch CurseForge Modpacks
  const searchCurseForgeModpacks = useCallback(async () => {
    const apiKey = getCurseForgeApiKey();
    if (!apiKey) {
      setCfHits([]);
      return;
    }
    setCfLoading(true);
    try {
      const res = await searchCurseForge(
        {
          query: cfQuery,
          classId: CF_CLASS_MODPACKS,
          gameVersion: server.version,
          modLoaderType: getCurseForgeLoaderType(server.server_type),
          sortField: cfSort,
          pageSize: 20,
        },
        apiKey,
      );
      setCfHits(res.data ?? []);
      setCfTotal(res.pagination?.totalCount ?? 0);
    } catch {
      setCfHits([]);
    } finally {
      setCfLoading(false);
    }
  }, [cfQuery, cfSort, server.server_type, server.version]);

  useEffect(() => {
    if (sourceTab === 'modrinth') {
      const timer = setTimeout(() => {
        void searchModrinthModpacks();
      }, 250);
      return () => clearTimeout(timer);
    }
  }, [sourceTab, searchModrinthModpacks]);

  useEffect(() => {
    if (sourceTab === 'curseforge') {
      const timer = setTimeout(() => {
        void searchCurseForgeModpacks();
      }, 250);
      return () => clearTimeout(timer);
    }
  }, [sourceTab, searchCurseForgeModpacks]);

  // File Upload Inspection
  async function handleFileSelect(file: File) {
    setUploadFile(file);
    setUploadError(null);
    setInspectingUpload(true);
    try {
      const manifest = await api.inspectModpackFile(server.id, file);
      setUploadManifest(manifest);
    } catch (err) {
      setUploadError(err instanceof ApiError ? err.detail : 'Failed to inspect modpack archive');
      setUploadManifest(null);
    } finally {
      setInspectingUpload(false);
    }
  }

  // URL Inspection
  async function handleInspectUrl() {
    if (!inputUrl.trim()) return;
    setUrlError(null);
    setInspectingUrl(true);
    try {
      const manifest = await api.inspectModpackUrl(server.id, inputUrl.trim());
      setUrlManifest(manifest);
    } catch (err) {
      setUrlError(err instanceof ApiError ? err.detail : 'Failed to inspect modpack URL');
      setUrlManifest(null);
    } finally {
      setInspectingUrl(false);
    }
  }

  // Modrinth Install Click
  async function handleModrinthInstall(hit: ModrinthSearchHit) {
    setMrLoading(true);
    try {
      // Get project versions to find latest version and .mrpack file
      const versionsRes = await fetch(
        `https://api.modrinth.com/v2/project/${hit.slug}/version`,
        { headers: { Accept: 'application/json' } },
      );
      if (!versionsRes.ok) {
        throw new Error('Failed to fetch modpack versions');
      }
      const versions = (await versionsRes.json()) as Array<{
        id: string;
        name: string;
        version_number: string;
        game_versions: string[];
        loaders: string[];
        files: Array<{
          url: string;
          filename: string;
          primary: boolean;
        }>;
      }>;

      if (versions.length === 0) {
        throw new Error('No versions found for this modpack');
      }

      // Select first compatible or latest
      const ver = versions[0];
      const mrpackFile = ver.files.find((f) => f.filename.endsWith('.mrpack')) || ver.files[0];
      if (!mrpackFile) {
        throw new Error('No .mrpack file found in release');
      }

      const manifest: ModpackManifest = {
        format: 'modrinth',
        name: hit.title,
        version: ver.version_number,
        summary: hit.description,
        author: hit.author,
        minecraft_version: ver.game_versions[0] || server.version,
        loader: ver.loaders[0] || 'fabric',
        loaderVersion: '',
        total_files: 0,
        server_files: 0,
        client_only_files: 0,
        icon_url: hit.icon_url,
      };

      setDialogManifest(manifest);
      setDialogOptions({
        source: 'modrinth',
        url: mrpackFile.url,
        project_id: hit.project_id,
        project_slug: hit.slug,
        version_id: ver.id,
      });
      setDialogFile(undefined);
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to load modpack details');
    } finally {
      setMrLoading(false);
    }
  }

  // CurseForge Install Click
  async function handleCurseForgeInstall(mod: CurseForgeMod) {
    const apiKey = getCurseForgeApiKey();
    if (!apiKey) return;

    setCfLoading(true);
    try {
      const files = await getCurseForgeFiles(mod.id, apiKey, { pageSize: 10 });
      if (files.length === 0) {
        throw new Error('No files found for this modpack');
      }

      const file = files[0];
      const downloadUrl = file.downloadUrl;
      if (!downloadUrl) {
        throw new Error('CurseForge file does not have direct download permission');
      }

      const manifest: ModpackManifest = {
        format: 'curseforge',
        name: mod.name,
        version: file.displayName || file.fileName,
        summary: mod.summary,
        author: mod.authors?.[0]?.name,
        minecraft_version: file.gameVersions?.find((v) => /^\d+\.\d+(\.\d+)?$/.test(v)) || server.version,
        loader:
          file.gameVersions?.find((v) =>
            ['forge', 'fabric', 'neoforge', 'quilt'].includes(v.toLowerCase()),
          )?.toLowerCase() || 'forge',
        loaderVersion: '',
        total_files: 0,
        server_files: 0,
        client_only_files: 0,
        icon_url: mod.logo?.thumbnailUrl,
      };

      setDialogManifest(manifest);
      setDialogOptions({
        source: 'curseforge',
        url: downloadUrl,
        project_id: String(mod.id),
        version_id: String(file.id),
      });
      setDialogFile(undefined);
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to prepare CurseForge modpack');
    } finally {
      setCfLoading(false);
    }
  }

  // Uninstall
  async function handleUninstall() {
    if (!installedModpack) return;
    const confirmed = await confirm(
      `Are you sure you want to uninstall "${installedModpack.name}"? This will delete the mods and configuration files installed by this modpack.`,
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
      await loadInstalled();
      onModpackChanged?.();
    } catch (err) {
      alert(err instanceof ApiError ? err.detail : 'Failed to uninstall modpack');
    } finally {
      setUninstalling(false);
    }
  }

  return (
    <>
      {dialog}

      {/* Modal Dialog for Installation */}
      {dialogManifest && dialogOptions && (
        <ModpackInstallDialog
          server={server}
          manifest={dialogManifest}
          installOptions={dialogOptions}
          modpackFile={dialogFile}
          onClose={() => {
            setDialogManifest(null);
            setDialogOptions(null);
            setDialogFile(undefined);
          }}
          onInstalled={async () => {
            await loadInstalled();
            onModpackChanged?.();
          }}
        />
      )}

      <div className="space-y-6">
        {/* Currently Installed Modpack Hero Card */}
        {installedModpack ? (
          <Card className="border-primary/30 bg-gradient-to-br from-card via-card to-primary/5 shadow-md">
            <CardHeader className="pb-3">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-center gap-3.5">
                  {installedModpack.icon_url ? (
                    <img
                      src={installedModpack.icon_url}
                      alt={installedModpack.name}
                      className="h-14 w-14 rounded-2xl object-cover border border-border shadow-xs"
                    />
                  ) : (
                    <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10 border border-primary/20 text-primary shadow-xs">
                      <Package className="h-7 w-7" />
                    </div>
                  )}
                  <div>
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-xs font-semibold text-primary uppercase tracking-wider">
                        Active Modpack
                      </span>
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
          !loadingInstalled && (
            <div className="rounded-xl border border-dashed border-border/80 bg-card/40 p-6 text-center">
              <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-secondary text-muted-foreground mb-3">
                <Package className="h-6 w-6" />
              </div>
              <h3 className="text-sm font-semibold text-foreground">No Modpack Installed</h3>
              <p className="text-xs text-muted-foreground max-w-md mx-auto mt-1">
                This server is currently running individual mods or vanilla jars. You can install an official
                modpack from Modrinth, CurseForge, or upload your own pack archive below.
              </p>
            </div>
          )
        )}

        {/* Modpack Installation Hub */}
        <div className="space-y-4">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between border-b border-border pb-2.5">
            <div>
              <h3 className="text-sm font-bold text-foreground">Install a Modpack</h3>
              <p className="text-xs text-muted-foreground">
                Browse popular modpack catalogs or upload a custom modpack archive.
              </p>
            </div>

            {/* Hub Subtabs */}
            <div className="flex items-center gap-1.5 bg-secondary/40 p-1 rounded-xl border border-border/60">
              <button
                type="button"
                onClick={() => setSourceTab('modrinth')}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                  sourceTab === 'modrinth'
                    ? 'bg-emerald-600 text-white shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                <Compass className="h-3.5 w-3.5" />
                Modrinth
              </button>

              <button
                type="button"
                onClick={() => setSourceTab('curseforge')}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                  sourceTab === 'curseforge'
                    ? 'bg-orange-600 text-white shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                <Flame className="h-3.5 w-3.5" />
                CurseForge
              </button>

              <button
                type="button"
                onClick={() => setSourceTab('upload')}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                  sourceTab === 'upload'
                    ? 'bg-primary text-primary-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                <UploadCloud className="h-3.5 w-3.5" />
                Upload File
              </button>

              <button
                type="button"
                onClick={() => setSourceTab('url')}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                  sourceTab === 'url'
                    ? 'bg-primary text-primary-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                <Globe className="h-3.5 w-3.5" />
                From URL
              </button>
            </div>
          </div>

          {/* TAB 1: MODRINTH MODPACKS */}
          {sourceTab === 'modrinth' && (
            <div className="space-y-4">
              {/* Search and Filters Bar */}
              <div className="flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between">
                <div className="relative flex-1 max-w-md">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <Input
                    value={mrQuery}
                    onChange={(e) => setMrQuery(e.target.value)}
                    placeholder="Search Modrinth modpacks (e.g. Cobblemon, All the Mods, Better MC)..."
                    className="pl-9 h-9 text-xs"
                  />
                  {mrQuery && (
                    <button
                      type="button"
                      onClick={() => setMrQuery('')}
                      className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    >
                      <X className="h-3.5 w-3.5" />
                    </button>
                  )}
                </div>

                <div className="flex items-center gap-2 flex-wrap">
                  <select
                    value={mrLoaderFilter}
                    onChange={(e) => setMrLoaderFilter(e.target.value)}
                    className="h-9 rounded-lg border border-border bg-card px-2.5 text-xs font-medium text-foreground focus:outline-hidden"
                  >
                    <option value="all">All Loaders</option>
                    <option value="fabric">Fabric</option>
                    <option value="forge">Forge</option>
                    <option value="neoforge">NeoForge</option>
                    <option value="quilt">Quilt</option>
                  </select>

                  <select
                    value={mrSort}
                    onChange={(e) => setMrSort(e.target.value as typeof mrSort)}
                    className="h-9 rounded-lg border border-border bg-card px-2.5 text-xs font-medium text-foreground focus:outline-hidden"
                  >
                    <option value="downloads">Most Downloaded</option>
                    <option value="relevance">Relevance</option>
                    <option value="updated">Recently Updated</option>
                    <option value="newest">Newest</option>
                  </select>

                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => void searchModrinthModpacks()}
                    disabled={mrLoading}
                    className="h-9 gap-1 text-xs"
                  >
                    <RefreshCw className={`h-3.5 w-3.5 ${mrLoading ? 'animate-spin' : ''}`} />
                  </Button>
                </div>
              </div>

              {/* Modrinth Grid */}
              {mrLoading && mrHits.length === 0 ? (
                <div className="flex h-40 items-center justify-center text-muted-foreground gap-2 text-xs">
                  <Loader2 className="h-5 w-5 animate-spin text-emerald-600" />
                  Loading Modrinth modpacks...
                </div>
              ) : mrHits.length === 0 ? (
                <div className="rounded-xl border border-border/80 bg-card p-8 text-center text-xs text-muted-foreground">
                  No Modrinth modpacks found matching your search criteria.
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  {mrHits.map((hit) => (
                    <Card
                      key={hit.project_id}
                      className="group flex flex-col justify-between border-border/80 hover:border-emerald-500/50 hover:shadow-xs transition-all"
                    >
                      <CardContent className="p-4 space-y-3">
                        <div className="flex items-start gap-3">
                          {hit.icon_url ? (
                            <img
                              src={hit.icon_url}
                              alt={hit.title}
                              className="h-12 w-12 rounded-xl object-cover border border-border/80 shrink-0 shadow-2xs"
                            />
                          ) : (
                            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-600">
                              <Compass className="h-6 w-6" />
                            </div>
                          )}

                          <div className="min-w-0 flex-1">
                            <div className="flex items-center justify-between gap-1">
                              <h4 className="font-bold text-sm text-foreground truncate group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors">
                                {hit.title}
                              </h4>
                              <span className="text-[11px] font-semibold text-muted-foreground shrink-0 flex items-center gap-1">
                                <Download className="h-3 w-3" />
                                {formatCount(hit.downloads)}
                              </span>
                            </div>
                            <span className="text-[11px] text-muted-foreground block truncate">
                              by {hit.author}
                            </span>
                            <p className="text-xs text-muted-foreground line-clamp-2 mt-1">
                              {hit.description}
                            </p>
                          </div>
                        </div>

                        {/* Badges & Actions */}
                        <div className="flex items-center justify-between gap-2 pt-2 border-t border-border/60">
                          <div className="flex items-center gap-1 flex-wrap">
                            {hit.categories?.slice(0, 3).map((cat) => (
                              <Badge
                                key={cat}
                                variant="secondary"
                                className="text-[9px] uppercase px-1.5 py-0 font-medium"
                              >
                                {cat}
                              </Badge>
                            ))}
                          </div>

                          <Button
                            size="sm"
                            onClick={() => void handleModrinthInstall(hit)}
                            disabled={mrLoading}
                            className="h-7 px-2.5 text-xs font-semibold bg-emerald-600 hover:bg-emerald-700 text-white gap-1"
                          >
                            <Download className="h-3.5 w-3.5" />
                            Install Modpack
                          </Button>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* TAB 2: CURSEFORGE MODPACKS */}
          {sourceTab === 'curseforge' && (
            <div className="space-y-4">
              {!hasCurseForgeApiKey() ? (
                <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-6 text-center space-y-2">
                  <Flame className="h-8 w-8 text-orange-600 mx-auto" />
                  <h4 className="text-sm font-bold text-foreground">CurseForge API Key Required</h4>
                  <p className="text-xs text-muted-foreground max-w-md mx-auto">
                    To browse and install modpacks from CurseForge, configure a free CurseForge API key in the
                    Browse tab or settings.
                  </p>
                </div>
              ) : (
                <>
                  {/* Search and Filters Bar */}
                  <div className="flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between">
                    <div className="relative flex-1 max-w-md">
                      <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                      <Input
                        value={cfQuery}
                        onChange={(e) => setCfQuery(e.target.value)}
                        placeholder="Search CurseForge modpacks (e.g. RL Craft, Medieval MC)..."
                        className="pl-9 h-9 text-xs"
                      />
                      {cfQuery && (
                        <button
                          type="button"
                          onClick={() => setCfQuery('')}
                          className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                        >
                          <X className="h-3.5 w-3.5" />
                        </button>
                      )}
                    </div>

                    <div className="flex items-center gap-2 flex-wrap">
                      <select
                        value={cfSort}
                        onChange={(e) => setCfSort(Number(e.target.value))}
                        className="h-9 rounded-lg border border-border bg-card px-2.5 text-xs font-medium text-foreground focus:outline-hidden"
                      >
                        {CF_SORT_OPTIONS.map((opt) => (
                          <option key={opt.id} value={opt.id}>
                            {opt.label}
                          </option>
                        ))}
                      </select>

                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => void searchCurseForgeModpacks()}
                        disabled={cfLoading}
                        className="h-9 gap-1 text-xs"
                      >
                        <RefreshCw className={`h-3.5 w-3.5 ${cfLoading ? 'animate-spin' : ''}`} />
                      </Button>
                    </div>
                  </div>

                  {/* CurseForge Grid */}
                  {cfLoading && cfHits.length === 0 ? (
                    <div className="flex h-40 items-center justify-center text-muted-foreground gap-2 text-xs">
                      <Loader2 className="h-5 w-5 animate-spin text-orange-600" />
                      Loading CurseForge modpacks...
                    </div>
                  ) : cfHits.length === 0 ? (
                    <div className="rounded-xl border border-border/80 bg-card p-8 text-center text-xs text-muted-foreground">
                      No CurseForge modpacks found.
                    </div>
                  ) : (
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                      {cfHits.map((mod) => (
                        <Card
                          key={mod.id}
                          className="group flex flex-col justify-between border-border/80 hover:border-orange-500/50 hover:shadow-xs transition-all"
                        >
                          <CardContent className="p-4 space-y-3">
                            <div className="flex items-start gap-3">
                              {mod.logo?.thumbnailUrl ? (
                                <img
                                  src={mod.logo.thumbnailUrl}
                                  alt={mod.name}
                                  className="h-12 w-12 rounded-xl object-cover border border-border/80 shrink-0 shadow-2xs"
                                />
                              ) : (
                                <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-orange-500/10 border border-orange-500/20 text-orange-600">
                                  <Flame className="h-6 w-6" />
                                </div>
                              )}

                              <div className="min-w-0 flex-1">
                                <div className="flex items-center justify-between gap-1">
                                  <h4 className="font-bold text-sm text-foreground truncate group-hover:text-orange-600 dark:group-hover:text-orange-400 transition-colors">
                                    {mod.name}
                                  </h4>
                                  <span className="text-[11px] font-semibold text-muted-foreground shrink-0 flex items-center gap-1">
                                    <Download className="h-3 w-3" />
                                    {formatCount(mod.downloadCount)}
                                  </span>
                                </div>
                                <span className="text-[11px] text-muted-foreground block truncate">
                                  by {mod.authors?.[0]?.name}
                                </span>
                                <p className="text-xs text-muted-foreground line-clamp-2 mt-1">
                                  {mod.summary}
                                </p>
                              </div>
                            </div>

                            <div className="flex items-center justify-between gap-2 pt-2 border-t border-border/60">
                              <div className="flex items-center gap-1 flex-wrap">
                                {mod.categories?.slice(0, 3).map((cat) => (
                                  <Badge
                                    key={cat.id}
                                    variant="secondary"
                                    className="text-[9px] px-1.5 py-0 font-medium"
                                  >
                                    {cat.name}
                                  </Badge>
                                ))}
                              </div>

                              <Button
                                size="sm"
                                onClick={() => void handleCurseForgeInstall(mod)}
                                disabled={cfLoading}
                                className="h-7 px-2.5 text-xs font-semibold bg-orange-600 hover:bg-orange-700 text-white gap-1"
                              >
                                <Download className="h-3.5 w-3.5" />
                                Install Modpack
                              </Button>
                            </div>
                          </CardContent>
                        </Card>
                      ))}
                    </div>
                  )}
                </>
              )}
            </div>
          )}

          {/* TAB 3: UPLOAD MODPACK */}
          {sourceTab === 'upload' && (
            <Card className="border-border/80">
              <CardContent className="p-6 space-y-5">
                <label
                  className="flex flex-col items-center justify-center rounded-2xl border-2 border-dashed border-border/80 bg-secondary/20 p-8 text-center hover:bg-secondary/40 hover:border-primary/50 cursor-pointer transition-all"
                >
                  <input
                    type="file"
                    accept=".mrpack,.zip"
                    className="hidden"
                    onChange={(e) => {
                      const f = e.target.files?.[0];
                      if (f) void handleFileSelect(f);
                    }}
                  />
                  <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary/10 text-primary mb-3">
                    <UploadCloud className="h-6 w-6" />
                  </div>
                  <span className="text-sm font-bold text-foreground">
                    {uploadFile ? uploadFile.name : 'Choose or drop a modpack archive (.mrpack, .zip)'}
                  </span>
                  <span className="text-xs text-muted-foreground mt-1">
                    Supports Modrinth (.mrpack), CurseForge export (.zip), or server pack (.zip)
                  </span>
                  {uploadFile && (
                    <span className="text-[11px] font-semibold text-primary mt-2">
                      {formatFileSize(uploadFile.size)} · Click to change file
                    </span>
                  )}
                </label>

                {inspectingUpload && (
                  <div className="flex items-center justify-center gap-2 text-xs text-muted-foreground p-4">
                    <Loader2 className="h-4 w-4 animate-spin text-primary" />
                    Inspecting modpack archive contents...
                  </div>
                )}

                {uploadError && (
                  <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
                    {uploadError}
                  </div>
                )}

                {/* Upload Manifest Preview */}
                {uploadManifest && (
                  <div className="rounded-xl border border-border/80 bg-secondary/10 p-4 space-y-4">
                    <div className="flex items-center justify-between">
                      <div>
                        <h4 className="font-bold text-sm text-foreground">
                          {uploadManifest.name}
                        </h4>
                        <p className="text-xs text-muted-foreground">
                          Version {uploadManifest.version} · Format: {uploadManifest.format}
                        </p>
                      </div>
                      <Badge variant="outline" className="text-xs font-semibold capitalize">
                        {uploadManifest.loader || 'Fabric'} {uploadManifest.minecraft_version}
                      </Badge>
                    </div>

                    <div className="grid grid-cols-3 gap-2 text-center text-xs">
                      <div className="rounded-lg border border-border bg-card p-2">
                        <span className="text-[10px] text-muted-foreground block">Server Files</span>
                        <span className="font-bold text-foreground block mt-0.5">
                          {uploadManifest.server_files}
                        </span>
                      </div>
                      <div className="rounded-lg border border-border bg-card p-2">
                        <span className="text-[10px] text-muted-foreground block">Client-Only (Skipped)</span>
                        <span className="font-bold text-foreground block mt-0.5">
                          {uploadManifest.client_only_files}
                        </span>
                      </div>
                      <div className="rounded-lg border border-border bg-card p-2">
                        <span className="text-[10px] text-muted-foreground block">Total Files</span>
                        <span className="font-bold text-foreground block mt-0.5">
                          {uploadManifest.total_files}
                        </span>
                      </div>
                    </div>

                    <div className="flex justify-end pt-2">
                      <Button
                        size="sm"
                        onClick={() => {
                          if (uploadFile) {
                            setDialogManifest(uploadManifest);
                            setDialogOptions({ source: 'upload' });
                            setDialogFile(uploadFile);
                          }
                        }}
                        className="gap-1.5 bg-primary text-primary-foreground font-semibold"
                      >
                        <Download className="h-4 w-4" />
                        Proceed to Install
                      </Button>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {/* TAB 4: FROM URL */}
          {sourceTab === 'url' && (
            <Card className="border-border/80">
              <CardContent className="p-6 space-y-4">
                <div>
                  <label className="text-xs font-semibold text-foreground block mb-1">
                    Direct Modpack URL
                  </label>
                  <div className="flex gap-2">
                    <Input
                      value={inputUrl}
                      onChange={(e) => setInputUrl(e.target.value)}
                      placeholder="https://cdn.modrinth.com/data/.../modpack.mrpack"
                      className="h-9 text-xs"
                    />
                    <Button
                      size="sm"
                      onClick={() => void handleInspectUrl()}
                      disabled={inspectingUrl || !inputUrl.trim()}
                      className="h-9 px-4 text-xs font-semibold shrink-0 gap-1.5"
                    >
                      {inspectingUrl ? (
                        <>
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          Inspecting...
                        </>
                      ) : (
                        'Inspect URL'
                      )}
                    </Button>
                  </div>
                  <span className="text-[11px] text-muted-foreground mt-1 block">
                    Enter a direct link to any `.mrpack` or `.zip` file hosted online.
                  </span>
                </div>

                {urlError && (
                  <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
                    {urlError}
                  </div>
                )}

                {/* URL Manifest Preview */}
                {urlManifest && (
                  <div className="rounded-xl border border-border/80 bg-secondary/10 p-4 space-y-4">
                    <div className="flex items-center justify-between">
                      <div>
                        <h4 className="font-bold text-sm text-foreground">
                          {urlManifest.name}
                        </h4>
                        <p className="text-xs text-muted-foreground">
                          Version {urlManifest.version} · Format: {urlManifest.format}
                        </p>
                      </div>
                      <Badge variant="outline" className="text-xs font-semibold capitalize">
                        {urlManifest.loader || 'Fabric'} {urlManifest.minecraft_version}
                      </Badge>
                    </div>

                    <div className="flex justify-end pt-2">
                      <Button
                        size="sm"
                        onClick={() => {
                          setDialogManifest(urlManifest);
                          setDialogOptions({ source: 'url', url: inputUrl.trim() });
                          setDialogFile(undefined);
                        }}
                        className="gap-1.5 bg-primary text-primary-foreground font-semibold"
                      >
                        <Download className="h-4 w-4" />
                        Proceed to Install
                      </Button>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </>
  );
}
