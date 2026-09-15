import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  ArrowRight,
  Compass,
  Download,
  Flame,
  Loader2,
  Package,
  Search,
  Sparkles,
  UploadCloud,
  X,
} from 'lucide-react';
import { api, ApiError, type CreateServerInput } from '../api/client';
import type {
  CurseForgeMod,
  ModpackManifest,
  ModrinthSearchHit,
  ServerType,
  VersionInfo,
  VersionMeta,
} from '../api/types';
import { formatCount, searchModrinth } from '../api/modrinth';
import {
  CF_CLASS_MODPACKS,
  CF_SORT_OPTIONS,
  getCurseForgeApiKey,
  getCurseForgeFiles,
  hasCurseForgeApiKey,
  searchCurseForge,
} from '../api/curseforge';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';

export interface SelectedModpackState {
  source: 'modrinth' | 'curseforge';
  project_id: string;
  project_slug?: string;
  title: string;
  icon_url?: string;
  author?: string;
  summary?: string;
  selectedVersion: string;
  downloadUrl: string;
  minecraft_version: string;
  loader: ServerType;
  loader_version: string;
  availableVersions: Array<{
    id: string;
    name: string;
    version_number: string;
    game_versions: string[];
    loaders: string[];
    downloadUrl: string;
  }>;
}

export function CreateServerDialog({ onCreated }: { onCreated: () => void }) {
  const [open, setOpen] = useState(false);
  const [createMode, setCreateMode] = useState<'standard' | 'search-modpack' | 'modpack'>('standard');
  const [name, setName] = useState('');
  const [serverType, setServerType] = useState<ServerType>('paper');
  const [version, setVersion] = useState('');
  const [build, setBuild] = useState('');
  const [ramMb, setRamMb] = useState(2048);
  const [versions, setVersions] = useState<VersionMeta[]>([]);
  const [builds, setBuilds] = useState<VersionInfo[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [loadingBuilds, setLoadingBuilds] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Port pool state
  const [port, setPort] = useState<string>('');
  const [availablePorts, setAvailablePorts] = useState<number[]>([]);
  const [usedPorts, setUsedPorts] = useState<number[]>([]);
  const [loadingPorts, setLoadingPorts] = useState(false);

  // Modpack file upload state
  const [modpackFile, setModpackFile] = useState<File | null>(null);
  const [modpackManifest, setModpackManifest] = useState<ModpackManifest | null>(null);
  const [inspectingModpack, setInspectingModpack] = useState(false);

  // Modpack search state
  const [searchSource, setSearchSource] = useState<'modrinth' | 'curseforge'>('modrinth');
  const [searchQuery, setSearchQuery] = useState('');
  const [loaderFilter, setLoaderFilter] = useState('all');
  const [mrSort, setMrSort] = useState<'downloads' | 'relevance' | 'updated' | 'newest'>('downloads');
  const [cfSort, setCfSort] = useState<number>(6);
  const [mrHits, setMrHits] = useState<ModrinthSearchHit[]>([]);
  const [cfHits, setCfHits] = useState<CurseForgeMod[]>([]);
  const [searching, setSearching] = useState(false);
  const [selectingPack, setSelectingPack] = useState(false);
  const [selectedPack, setSelectedPack] = useState<SelectedModpackState | null>(null);

  const parsedPort = parseInt(port, 10);
  const isPortUsed = !isNaN(parsedPort) && usedPorts.includes(parsedPort);
  const isPortInvalid = isNaN(parsedPort) || parsedPort < 1 || parsedPort > 65535;

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoadingPorts(true);
    Promise.all([api.availablePorts(), api.listServers()])
      .then(([portsRes, serversRes]) => {
        if (cancelled) return;
        const free = portsRes.available ?? [];
        setAvailablePorts(free);
        const used = (serversRes ?? []).map((s) => s.host_port);
        setUsedPorts(used);
        if (!port && free.length > 0) {
          setPort(String(free[0]));
        }
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setLoadingPorts(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open]);

  useEffect(() => {
    if (!open || createMode !== 'standard') return;
    let cancelled = false;
    setLoadingVersions(true);
    setError(null);
    api
      .jarVersions(serverType)
      .then((v) => {
        if (cancelled) return;
        setVersions(v);
        if (!version || !v.some((x) => x.name === version)) {
          setVersion(v[0]?.name ?? '');
        }
      })
      .catch((err) => !cancelled && setError(err instanceof ApiError ? err.detail : 'Failed to load versions'))
      .finally(() => !cancelled && setLoadingVersions(false));
    return () => {
      cancelled = true;
    };
  }, [open, serverType, createMode]);

  useEffect(() => {
    if (!open || !version || createMode !== 'standard') return;
    let cancelled = false;
    setLoadingBuilds(true);
    api
      .jarBuilds(serverType, version)
      .then((b) => {
        if (cancelled) return;
        setBuilds(b);
        if (!build || !b.some((x) => x.build === build)) {
          setBuild(b[0]?.build ?? '');
        }
      })
      .catch((err) => !cancelled && setError(err instanceof ApiError ? err.detail : 'Failed to load builds'))
      .finally(() => !cancelled && setLoadingBuilds(false));
    return () => {
      cancelled = true;
    };
  }, [open, serverType, version, createMode]);

  // Search Modrinth Modpacks
  const executeModrinthSearch = useCallback(async () => {
    setSearching(true);
    try {
      const res = await searchModrinth({
        query: searchQuery,
        projectType: 'modpack',
        loaders: loaderFilter !== 'all' ? [loaderFilter] : undefined,
        sort: mrSort,
        limit: 12,
      });
      setMrHits(res.hits ?? []);
    } catch {
      setMrHits([]);
    } finally {
      setSearching(false);
    }
  }, [searchQuery, loaderFilter, mrSort]);

  // Search CurseForge Modpacks
  const executeCurseForgeSearch = useCallback(async () => {
    const apiKey = getCurseForgeApiKey();
    if (!apiKey) {
      setCfHits([]);
      return;
    }
    setSearching(true);
    try {
      const cfLoader =
        loaderFilter === 'forge'
          ? 1
          : loaderFilter === 'fabric'
          ? 4
          : loaderFilter === 'neoforge'
          ? 6
          : loaderFilter === 'quilt'
          ? 5
          : undefined;

      const res = await searchCurseForge(
        {
          query: searchQuery,
          classId: CF_CLASS_MODPACKS,
          modLoaderType: cfLoader,
          sortField: cfSort,
          pageSize: 12,
        },
        apiKey,
      );
      setCfHits(res.data ?? []);
    } catch {
      setCfHits([]);
    } finally {
      setSearching(false);
    }
  }, [searchQuery, loaderFilter, cfSort]);

  useEffect(() => {
    if (!open || createMode !== 'search-modpack' || selectedPack) return;
    const timer = setTimeout(() => {
      if (searchSource === 'modrinth') {
        void executeModrinthSearch();
      } else {
        void executeCurseForgeSearch();
      }
    }, 250);
    return () => clearTimeout(timer);
  }, [open, createMode, searchSource, selectedPack, executeModrinthSearch, executeCurseForgeSearch]);

  async function onModpackSelected(file: File) {
    setModpackFile(file);
    setInspectingModpack(true);
    setError(null);
    try {
      const manifest = await api.inspectModpackUpload(file);
      setModpackManifest(manifest);
      if (manifest.name) {
        setName(manifest.name);
      }
      if (manifest.loader) {
        const l = manifest.loader.toLowerCase();
        if (l === 'fabric' || l === 'quilt') setServerType('fabric');
        else if (l === 'forge') setServerType('forge');
        else if (l === 'neoforge') setServerType('neoforge');
      }
      if (manifest.minecraft_version) {
        setVersion(manifest.minecraft_version);
      }
      if (manifest.loader_version) {
        setBuild(manifest.loader_version);
      }
      setRamMb(4096);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to inspect modpack archive');
      setModpackManifest(null);
    } finally {
      setInspectingModpack(false);
    }
  }

  async function handleSelectModrinthHit(hit: ModrinthSearchHit) {
    setSelectingPack(true);
    setError(null);
    try {
      const res = await fetch(`https://api.modrinth.com/v2/project/${hit.slug}/version`, {
        headers: { Accept: 'application/json' },
      });
      if (!res.ok) throw new Error('Failed to fetch modpack versions');
      const versions = (await res.json()) as Array<{
        id: string;
        name: string;
        version_number: string;
        game_versions: string[];
        loaders: string[];
        files: Array<{ url: string; filename: string; primary: boolean }>;
      }>;
      if (versions.length === 0) throw new Error('No versions found for this modpack');

      const mappedVersions = versions
        .map((v) => {
          const file = v.files.find((f) => f.filename.endsWith('.mrpack')) || v.files[0];
          return {
            id: v.id,
            name: v.name || v.version_number,
            version_number: v.version_number,
            game_versions: v.game_versions,
            loaders: v.loaders,
            downloadUrl: file ? file.url : '',
          };
        })
        .filter((v) => !!v.downloadUrl);

      if (mappedVersions.length === 0) throw new Error('No .mrpack download files found for this modpack');

      const first = mappedVersions[0];
      let detectedLoader: ServerType = 'fabric';
      const l = (first.loaders[0] || '').toLowerCase();
      if (l === 'forge') detectedLoader = 'forge';
      else if (l === 'neoforge') detectedLoader = 'neoforge';
      else if (l === 'fabric' || l === 'quilt') detectedLoader = 'fabric';

      const mcVer = first.game_versions[0] || '1.20.1';
      const buildVer = first.id;

      setSelectedPack({
        source: 'modrinth',
        project_id: hit.project_id,
        project_slug: hit.slug,
        title: hit.title,
        icon_url: hit.icon_url ?? undefined,
        author: hit.author,
        summary: hit.description,
        selectedVersion: first.id,
        downloadUrl: first.downloadUrl,
        minecraft_version: mcVer,
        loader: detectedLoader,
        loader_version: buildVer,
        availableVersions: mappedVersions,
      });

      setName(hit.title);
      setServerType(detectedLoader);
      setVersion(mcVer);
      setBuild(buildVer);
      setRamMb(4096);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load modpack details');
    } finally {
      setSelectingPack(false);
    }
  }

  async function handleSelectCurseForgeHit(mod: CurseForgeMod) {
    const apiKey = getCurseForgeApiKey();
    if (!apiKey) return;
    setSelectingPack(true);
    setError(null);
    try {
      const files = await getCurseForgeFiles(mod.id, apiKey, { pageSize: 20 });
      if (files.length === 0) throw new Error('No files found for this modpack');
      const validFiles = files.filter((f) => !!f.downloadUrl);
      if (validFiles.length === 0) throw new Error('No direct download files found');
      const first = validFiles[0];

      const l = (first.gameVersions?.find((v) =>
        ['forge', 'fabric', 'neoforge', 'quilt'].includes(v.toLowerCase()),
      ) || 'forge').toLowerCase();

      let detectedLoader: ServerType = 'forge';
      if (l === 'fabric' || l === 'quilt') detectedLoader = 'fabric';
      else if (l === 'neoforge') detectedLoader = 'neoforge';
      else if (l === 'forge') detectedLoader = 'forge';

      const mcVer = first.gameVersions?.find((v) => /^\d+\.\d+(\.\d+)?$/.test(v)) || '1.20.1';

      const mappedVersions = validFiles.map((f) => ({
        id: String(f.id),
        name: f.displayName || f.fileName,
        version_number: f.displayName || f.fileName,
        game_versions: f.gameVersions || [],
        loaders: [l],
        downloadUrl: f.downloadUrl || '',
      }));

      setSelectedPack({
        source: 'curseforge',
        project_id: String(mod.id),
        title: mod.name,
        icon_url: mod.logo?.thumbnailUrl,
        author: mod.authors?.[0]?.name,
        summary: mod.summary,
        selectedVersion: String(first.id),
        downloadUrl: first.downloadUrl || '',
        minecraft_version: mcVer,
        loader: detectedLoader,
        loader_version: String(first.id),
        availableVersions: mappedVersions,
      });

      setName(mod.name);
      setServerType(detectedLoader);
      setVersion(mcVer);
      setBuild(String(first.id));
      setRamMb(4096);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to prepare CurseForge modpack');
    } finally {
      setSelectingPack(false);
    }
  }

  function handleVersionChange(verId: string) {
    if (!selectedPack) return;
    const ver = selectedPack.availableVersions.find((v) => v.id === verId);
    if (!ver) return;

    let detectedLoader = selectedPack.loader;
    const l = (ver.loaders[0] || '').toLowerCase();
    if (l === 'forge') detectedLoader = 'forge';
    else if (l === 'neoforge') detectedLoader = 'neoforge';
    else if (l === 'fabric' || l === 'quilt') detectedLoader = 'fabric';

    const mcVer = ver.game_versions[0] || selectedPack.minecraft_version;

    setSelectedPack({
      ...selectedPack,
      selectedVersion: ver.id,
      downloadUrl: ver.downloadUrl,
      minecraft_version: mcVer,
      loader: detectedLoader,
      loader_version: ver.id,
    });
    setServerType(detectedLoader);
    setVersion(mcVer);
    setBuild(ver.id);
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (isPortUsed) {
      setError(`Port ${port} is already in use by another server`);
      return;
    }
    if (port && isPortInvalid) {
      setError('Please enter a valid port between 1 and 65535');
      return;
    }
    setBusy(true);
    setError(null);
    const input: CreateServerInput = {
      name: name.trim(),
      server_type: serverType,
      version,
      build,
      ram_mb: ramMb,
      host_port: parsedPort > 0 ? parsedPort : undefined,
    };
    try {
      const srv = await api.createServer(input);
      if (createMode === 'search-modpack' && selectedPack) {
        await api.installModpackRemote(srv.id, {
          source: selectedPack.source,
          url: selectedPack.downloadUrl,
          project_id: selectedPack.project_id,
          project_slug: selectedPack.project_slug,
          version_id: selectedPack.selectedVersion,
          auto_configure_server: true,
          created_with_modpack: true,
        });
      } else if (createMode === 'modpack' && modpackFile) {
        await api.installModpackFile(srv.id, modpackFile, true, true);
      }
      setOpen(false);
      setName('');
      setModpackFile(null);
      setModpackManifest(null);
      setSelectedPack(null);
      onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to create server');
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Button onClick={() => setOpen(true)} className="gap-1.5">
        <Package className="h-4 w-4" />
        Create server
      </Button>
      {open && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          onClick={() => !busy && setOpen(false)}
        >
          <div
            className="w-full max-w-xl max-h-[90vh] overflow-y-auto rounded-2xl border bg-card p-6 shadow-2xl transition-all"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between pb-3 border-b border-border/80">
              <div>
                <h2 className="text-lg font-bold">Create server</h2>
                <p className="text-xs text-muted-foreground">
                  Deploy a new Minecraft server container with custom settings or an official modpack.
                </p>
              </div>
              <button
                type="button"
                onClick={() => !busy && setOpen(false)}
                className="rounded-lg p-1 text-muted-foreground hover:bg-secondary hover:text-foreground"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            {/* Mode Switcher Tabs */}
            <div className="mt-4 grid grid-cols-3 gap-1.5 bg-secondary/30 p-1 rounded-xl border border-border/60 text-xs font-semibold">
              <button
                type="button"
                onClick={() => {
                  setCreateMode('standard');
                  setModpackFile(null);
                  setModpackManifest(null);
                  setSelectedPack(null);
                }}
                className={`py-1.5 rounded-lg transition-all flex items-center justify-center gap-1.5 ${
                  createMode === 'standard'
                    ? 'bg-card text-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                Standard Server
              </button>
              <button
                type="button"
                onClick={() => {
                  setCreateMode('search-modpack');
                  setModpackFile(null);
                  setModpackManifest(null);
                }}
                className={`py-1.5 rounded-lg transition-all flex items-center justify-center gap-1.5 ${
                  createMode === 'search-modpack'
                    ? 'bg-primary text-primary-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                <Compass className="h-3.5 w-3.5" />
                Search Modpacks
              </button>
              <button
                type="button"
                onClick={() => {
                  setCreateMode('modpack');
                  setSelectedPack(null);
                }}
                className={`py-1.5 px-2 rounded-lg transition-all flex items-center justify-center gap-1.5 text-center ${
                  createMode === 'modpack'
                    ? 'bg-primary text-primary-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                <UploadCloud className="h-3.5 w-3.5 shrink-0" />
                <span className="flex flex-col items-center leading-tight">
                  <span>From Modpack</span>
                  <span className="text-[10px] font-normal opacity-80">(.mrpack, .zip)</span>
                </span>
              </button>
            </div>

            <form onSubmit={onSubmit} className="mt-4 space-y-4">
              {/* MODE 1: SEARCH MODPACKS */}
              {createMode === 'search-modpack' && (
                <div className="space-y-3">
                  {!selectedPack ? (
                    <div className="space-y-3 rounded-xl border border-border/80 bg-secondary/10 p-3.5">
                      <div className="flex items-center justify-between gap-2">
                        <div className="flex items-center gap-1 bg-secondary/50 p-0.5 rounded-lg border border-border/60 text-[11px] font-semibold">
                          <button
                            type="button"
                            onClick={() => setSearchSource('modrinth')}
                            className={`px-2.5 py-1 rounded-md flex items-center gap-1 transition-all ${
                              searchSource === 'modrinth'
                                ? 'bg-emerald-600 text-white shadow-xs'
                                : 'text-muted-foreground hover:text-foreground'
                            }`}
                          >
                            <Compass className="h-3 w-3" />
                            Modrinth
                          </button>
                          <button
                            type="button"
                            onClick={() => setSearchSource('curseforge')}
                            className={`px-2.5 py-1 rounded-md flex items-center gap-1 transition-all ${
                              searchSource === 'curseforge'
                                ? 'bg-orange-600 text-white shadow-xs'
                                : 'text-muted-foreground hover:text-foreground'
                            }`}
                          >
                            <Flame className="h-3 w-3" />
                            CurseForge
                          </button>
                        </div>

                        <div className="flex items-center gap-1.5">
                          <select
                            value={loaderFilter}
                            onChange={(e) => setLoaderFilter(e.target.value)}
                            className="h-8 rounded-lg border border-border bg-card px-2 text-[11px] font-medium text-foreground focus:outline-hidden"
                          >
                            <option value="all">All Loaders</option>
                            <option value="fabric">Fabric</option>
                            <option value="forge">Forge</option>
                            <option value="neoforge">NeoForge</option>
                            <option value="quilt">Quilt</option>
                          </select>

                          {searchSource === 'modrinth' ? (
                            <select
                              value={mrSort}
                              onChange={(e) => setMrSort(e.target.value as typeof mrSort)}
                              className="h-8 rounded-lg border border-border bg-card px-2 text-[11px] font-medium text-foreground focus:outline-hidden"
                            >
                              <option value="downloads">Most Downloaded</option>
                              <option value="relevance">Relevance</option>
                              <option value="updated">Recently Updated</option>
                              <option value="newest">Newest</option>
                            </select>
                          ) : (
                            <select
                              value={cfSort}
                              onChange={(e) => setCfSort(Number(e.target.value))}
                              className="h-8 rounded-lg border border-border bg-card px-2 text-[11px] font-medium text-foreground focus:outline-hidden"
                            >
                              {CF_SORT_OPTIONS.map((opt) => (
                                <option key={opt.id} value={opt.id}>
                                  {opt.label}
                                </option>
                              ))}
                            </select>
                          )}
                        </div>
                      </div>

                      {/* Search Bar */}
                      <div className="relative">
                        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
                        <Input
                          value={searchQuery}
                          onChange={(e) => setSearchQuery(e.target.value)}
                          placeholder="Search modpacks (e.g. Cobblemon, All the Mods, Better MC)..."
                          className="pl-8 h-8 text-xs"
                        />
                        {searchQuery && (
                          <button
                            type="button"
                            onClick={() => setSearchQuery('')}
                            className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                          >
                            <X className="h-3 w-3" />
                          </button>
                        )}
                      </div>

                      {/* Search Results List */}
                      {selectingPack ? (
                        <div className="flex h-36 items-center justify-center gap-2 text-xs text-muted-foreground">
                          <Loader2 className="h-4 w-4 animate-spin text-primary" />
                          Loading modpack details and compatible versions...
                        </div>
                      ) : searching ? (
                        <div className="flex h-36 items-center justify-center gap-2 text-xs text-muted-foreground">
                          <Loader2 className="h-4 w-4 animate-spin text-primary" />
                          Searching modpacks...
                        </div>
                      ) : searchSource === 'curseforge' && !hasCurseForgeApiKey() ? (
                        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-4 text-center text-xs space-y-1">
                          <p className="font-semibold text-foreground">CurseForge API Key Required</p>
                          <p className="text-muted-foreground text-[11px]">
                            Configure your API key in Settings or use Modrinth.
                          </p>
                        </div>
                      ) : (searchSource === 'modrinth' ? mrHits.length === 0 : cfHits.length === 0) ? (
                        <div className="rounded-lg border border-border/60 bg-card p-6 text-center text-xs text-muted-foreground">
                          No modpacks found. Try a different search term or filter.
                        </div>
                      ) : (
                        <div className="max-h-56 overflow-y-auto space-y-1.5 pr-1">
                          {searchSource === 'modrinth'
                            ? mrHits.map((hit) => (
                                <div
                                  key={hit.project_id}
                                  className="flex items-center justify-between gap-2.5 p-2 rounded-lg border border-border/60 bg-card hover:border-primary/50 hover:bg-secondary/20 transition-all text-xs"
                                >
                                  <div className="flex items-center gap-2.5 min-w-0 flex-1">
                                    {hit.icon_url ? (
                                      <img
                                        src={hit.icon_url}
                                        alt={hit.title}
                                        className="h-8 w-8 rounded-lg object-cover border border-border/80 shrink-0"
                                      />
                                    ) : (
                                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-600 border border-emerald-500/20">
                                        <Compass className="h-4 w-4" />
                                      </div>
                                    )}
                                    <div className="min-w-0 flex-1">
                                      <div className="flex items-center gap-1.5">
                                        <h4 className="font-bold text-foreground truncate">{hit.title}</h4>
                                        <span className="text-[10px] text-muted-foreground shrink-0 flex items-center gap-0.5">
                                          <Download className="h-2.5 w-2.5" />
                                          {formatCount(hit.downloads)}
                                        </span>
                                      </div>
                                      <p className="text-[11px] text-muted-foreground truncate">{hit.description}</p>
                                    </div>
                                  </div>

                                  <Button
                                    type="button"
                                    size="sm"
                                    onClick={() => void handleSelectModrinthHit(hit)}
                                    className="h-7 px-2.5 text-xs font-semibold bg-emerald-600 hover:bg-emerald-700 text-white shrink-0 gap-1"
                                  >
                                    Select
                                    <ArrowRight className="h-3 w-3" />
                                  </Button>
                                </div>
                              ))
                            : cfHits.map((mod) => (
                                <div
                                  key={mod.id}
                                  className="flex items-center justify-between gap-2.5 p-2 rounded-lg border border-border/60 bg-card hover:border-primary/50 hover:bg-secondary/20 transition-all text-xs"
                                >
                                  <div className="flex items-center gap-2.5 min-w-0 flex-1">
                                    {mod.logo?.thumbnailUrl ? (
                                      <img
                                        src={mod.logo.thumbnailUrl}
                                        alt={mod.name}
                                        className="h-8 w-8 rounded-lg object-cover border border-border/80 shrink-0"
                                      />
                                    ) : (
                                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-orange-500/10 text-orange-600 border border-orange-500/20">
                                        <Flame className="h-4 w-4" />
                                      </div>
                                    )}
                                    <div className="min-w-0 flex-1">
                                      <div className="flex items-center gap-1.5">
                                        <h4 className="font-bold text-foreground truncate">{mod.name}</h4>
                                        <span className="text-[10px] text-muted-foreground shrink-0 flex items-center gap-0.5">
                                          <Download className="h-2.5 w-2.5" />
                                          {formatCount(mod.downloadCount)}
                                        </span>
                                      </div>
                                      <p className="text-[11px] text-muted-foreground truncate">{mod.summary}</p>
                                    </div>
                                  </div>

                                  <Button
                                    type="button"
                                    size="sm"
                                    onClick={() => void handleSelectCurseForgeHit(mod)}
                                    className="h-7 px-2.5 text-xs font-semibold bg-orange-600 hover:bg-orange-700 text-white shrink-0 gap-1"
                                  >
                                    Select
                                    <ArrowRight className="h-3 w-3" />
                                  </Button>
                                </div>
                              ))}
                        </div>
                      )}
                    </div>
                  ) : (
                    /* Selected Modpack Preview Card */
                    <div className="rounded-xl border border-primary/40 bg-gradient-to-br from-card via-card to-primary/5 p-3.5 text-xs space-y-2.5">
                      <div className="flex items-start justify-between gap-2">
                        <div className="flex items-center gap-2.5 min-w-0">
                          {selectedPack.icon_url ? (
                            <img
                              src={selectedPack.icon_url}
                              alt={selectedPack.title}
                              className="h-10 w-10 rounded-xl object-cover border border-border shadow-2xs shrink-0"
                            />
                          ) : (
                            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary border border-primary/20">
                              <Package className="h-5 w-5" />
                            </div>
                          )}
                          <div className="min-w-0">
                            <div className="flex items-center gap-1.5 flex-wrap">
                              <h4 className="font-bold text-sm text-foreground truncate">{selectedPack.title}</h4>
                              <Badge variant="outline" className="text-[9px] uppercase font-bold px-1.5 py-0">
                                {selectedPack.source}
                              </Badge>
                            </div>
                            <span className="text-[11px] text-muted-foreground block">
                              Loader: <strong className="capitalize text-foreground">{selectedPack.loader}</strong> · MC:{' '}
                              <strong className="text-foreground">{selectedPack.minecraft_version}</strong>
                            </span>
                          </div>
                        </div>

                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={() => setSelectedPack(null)}
                          className="h-7 px-2 text-[11px] text-muted-foreground hover:text-foreground"
                        >
                          Change
                        </Button>
                      </div>

                      {/* Version Selector for the selected pack */}
                      {selectedPack.availableVersions.length > 1 && (
                        <div className="flex items-center gap-2 pt-1 border-t border-border/60">
                          <span className="text-[11px] text-muted-foreground shrink-0 font-medium">Pack Version:</span>
                          <select
                            value={selectedPack.selectedVersion}
                            onChange={(e) => handleVersionChange(e.target.value)}
                            className="h-7 rounded-lg border border-border bg-card px-2 text-xs font-semibold text-foreground flex-1 focus:outline-hidden"
                          >
                            {selectedPack.availableVersions.map((v) => (
                              <option key={v.id} value={v.id}>
                                {v.name} ({v.game_versions[0] || selectedPack.minecraft_version})
                              </option>
                            ))}
                          </select>
                        </div>
                      )}

                      <div className="rounded-lg bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-1 text-[11px] text-emerald-800 dark:text-emerald-200 flex items-center gap-1.5 font-medium">
                        <Sparkles className="h-3 w-3 text-emerald-600 shrink-0" />
                        <span>Dedicated Modpack Server — locked to this modpack with version updates supported.</span>
                      </div>
                    </div>
                  )}
                </div>
              )}

              {/* MODE 2: UPLOAD MODPACK ARCHIVE */}
              {createMode === 'modpack' && (
                <div className="space-y-3">
                  <label className="flex flex-col items-center justify-center rounded-xl border-2 border-dashed border-border/80 bg-secondary/20 p-5 text-center hover:bg-secondary/40 hover:border-primary/50 cursor-pointer transition-all">
                    <input
                      type="file"
                      accept=".mrpack,.zip"
                      className="hidden"
                      onChange={(e) => {
                        const f = e.target.files?.[0];
                        if (f) void onModpackSelected(f);
                      }}
                    />
                    <UploadCloud className="h-6 w-6 text-primary mb-1.5" />
                    <span className="text-xs font-bold text-foreground">
                      {modpackFile ? modpackFile.name : 'Choose or drop modpack archive (.mrpack, .zip)'}
                    </span>
                    <span className="text-[11px] text-muted-foreground mt-0.5">
                      Auto-detects software (Fabric/Forge), Minecraft version, and RAM
                    </span>
                  </label>

                  {inspectingModpack && (
                    <div className="flex items-center justify-center gap-2 text-xs text-muted-foreground p-2">
                      <Loader2 className="h-3.5 w-3.5 animate-spin text-primary" />
                      Analyzing modpack manifest...
                    </div>
                  )}

                  {modpackManifest && (
                    <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs space-y-1">
                      <div className="flex items-center justify-between">
                        <span className="font-bold text-emerald-900 dark:text-emerald-200">
                          {modpackManifest.name} (v{modpackManifest.version})
                        </span>
                        <Badge variant="outline" className="text-[10px] uppercase font-semibold">
                          {modpackManifest.format}
                        </Badge>
                      </div>
                      <p className="text-[11px] text-muted-foreground">
                        Loader: <strong className="capitalize text-foreground">{modpackManifest.loader}</strong> · MC:{' '}
                        <strong className="text-foreground">{modpackManifest.minecraft_version}</strong> · Mods to install:{' '}
                        <strong className="text-foreground">
                          {modpackManifest.server_files > 0
                            ? modpackManifest.server_files
                            : modpackManifest.total_files}
                        </strong>
                      </p>
                    </div>
                  )}
                </div>
              )}

              {/* COMMON FORM FIELDS */}
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <Label htmlFor="server-name">Server Name</Label>
                  <Input
                    id="server-name"
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="My survival world"
                  />
                </div>
                <div className="space-y-1.5">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-1.5">
                      <Label htmlFor="server-port">Server Port</Label>
                      {loadingPorts && <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />}
                    </div>
                    {isPortUsed ? (
                      <span className="text-[11px] font-semibold text-destructive">Already in use</span>
                    ) : isPortInvalid ? (
                      <span className="text-[11px] font-semibold text-destructive">Invalid port</span>
                    ) : (
                      <span className="text-[11px] font-semibold text-emerald-600 dark:text-emerald-400">Available</span>
                    )}
                  </div>
                  <Input
                    id="server-port"
                    type="number"
                    min={1}
                    max={65535}
                    value={port}
                    onChange={(e) => setPort(e.target.value)}
                    placeholder={availablePorts[0] ? String(availablePorts[0]) : "25565"}
                    className={isPortUsed ? "border-destructive focus-visible:ring-destructive" : ""}
                  />
                  {isPortUsed && (
                    <p className="text-[11px] text-destructive font-medium">
                      Port {port} is already used by an existing server.
                    </p>
                  )}
                  {!isPortUsed && !isPortInvalid && availablePorts.length > 0 && (
                    <p className="text-[11px] text-muted-foreground">
                      Prefilled from available port pool.
                    </p>
                  )}
                </div>
              </div>

              {createMode === 'standard' ? (
                <>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-1.5">
                      <Label htmlFor="server-type">Type</Label>
                      <Select
                        id="server-type"
                        value={serverType}
                        onChange={(e) => setServerType(e.target.value as ServerType)}
                      >
                        <option key="paper" value="paper">Paper</option>
                        <option key="fabric" value="fabric">Fabric</option>
                        <option key="vanilla" value="vanilla">Vanilla</option>
                        <option key="forge" value="forge">Forge</option>
                        <option key="neoforge" value="neoforge">NeoForge</option>
                        <option key="spigot" value="spigot">Spigot</option>
                      </Select>
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="server-ram">RAM (MB)</Label>
                      <Input
                        id="server-ram"
                        type="number"
                        min={512}
                        step={256}
                        value={ramMb}
                        onChange={(e) => setRamMb(Number(e.target.value))}
                      />
                    </div>
                  </div>

                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-1.5">
                      <Label htmlFor="server-version">Version</Label>
                      {loadingVersions ? (
                        <div className="flex h-9 items-center text-muted-foreground">
                          <Loader2 className="h-4 w-4 animate-spin" />
                        </div>
                      ) : (
                        <Select
                          id="server-version"
                          value={version}
                          onChange={(e) => setVersion(e.target.value)}
                          disabled={versions.length === 0}
                        >
                          {versions.map((v, idx) => (
                            <option key={`${v.name || idx}-${idx}`} value={v.name}>
                              {v.latest ?? v.name}
                            </option>
                          ))}
                        </Select>
                      )}
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="server-build">Build</Label>
                      {loadingBuilds ? (
                        <div className="flex h-9 items-center text-muted-foreground">
                          <Loader2 className="h-4 w-4 animate-spin" />
                        </div>
                      ) : (
                        <Select
                          id="server-build"
                          value={build}
                          onChange={(e) => setBuild(e.target.value)}
                          disabled={builds.length === 0}
                        >
                          {builds.map((b, idx) => (
                            <option key={`${b.build || idx}-${idx}`} value={b.build}>
                              {b.build}
                            </option>
                          ))}
                        </Select>
                      )}
                    </div>
                  </div>
                </>
              ) : (
                /* Modpack runtime summary */
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-1.5">
                    <Label>Software &amp; Version</Label>
                    <div className="h-9 flex items-center gap-1.5 px-3 rounded-lg border border-border bg-secondary/30 text-xs font-semibold">
                      <span className="capitalize text-foreground">{serverType}</span>
                      <span className="text-muted-foreground">·</span>
                      <span className="text-foreground">{version || 'Auto-detected'}</span>
                      {build && <span className="text-muted-foreground text-[10px]">({build})</span>}
                    </div>
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="server-ram">RAM (MB)</Label>
                    <Input
                      id="server-ram"
                      type="number"
                      min={1024}
                      step={256}
                      value={ramMb}
                      onChange={(e) => setRamMb(Number(e.target.value))}
                    />
                  </div>
                </div>
              )}

              {error && <p className="text-sm text-destructive">{error}</p>}

              <div className="flex justify-end gap-2 pt-2 border-t border-border/80">
                <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={busy}>
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={
                    busy ||
                    isPortUsed ||
                    (port ? isPortInvalid : false) ||
                    (createMode === 'standard' && (loadingVersions || loadingBuilds)) ||
                    (createMode === 'search-modpack' && !selectedPack) ||
                    (createMode === 'modpack' && !modpackFile)
                  }
                >
                  {busy ? (
                    <>
                      <Loader2 className="h-4 w-4 animate-spin" />
                      {createMode !== 'standard' ? 'Creating & Installing Modpack...' : 'Creating...'}
                    </>
                  ) : createMode === 'search-modpack' ? (
                    selectedPack ? (
                      'Create Server from Modpack'
                    ) : (
                      'Select a Modpack First'
                    )
                  ) : (
                    'Create'
                  )}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  );
}
