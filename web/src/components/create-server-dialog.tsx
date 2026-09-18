import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  AlertTriangle,
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
  JavaRelease,
  ModpackManifest,
  ModrinthSearchHit,
  ServerType,
  VersionInfo,
  VersionMeta,
} from '../api/types';
import { formatCount, getProjectVersions, searchModrinth } from '../api/modrinth';

function recommendJava(ver: string, serverType?: ServerType): number {
  if (
    serverType === 'geysermc' ||
    serverType === 'velocity' ||
    serverType === 'waterfall' ||
    serverType === 'bungeecord' ||
    serverType === 'limbo' ||
    serverType === 'nanolimbo'
  ) {
    return 21;
  }
  const clean = ver.trim().replace(/^v/i, '');
  const match = clean.match(/^(\d+)\.(\d+)(?:\.(\d+))?/);
  if (match) {
    const major = parseInt(match[1], 10);
    const minor = parseInt(match[2], 10);
    const patch = match[3] ? parseInt(match[3], 10) : 0;
    if (major >= 25) return 25;
    if (major === 1) {
      if (minor >= 22) return 25;
      if (minor >= 21 || (minor === 20 && patch >= 5)) return 21;
      if (minor >= 17) return 17;
      if (minor > 0 && minor <= 16) return 8;
    }
  }
  if (clean.startsWith('25w') || clean.startsWith('26w')) return 25;
  return 21;
}
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
import { ProgressBar } from './ui/progress';
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
  const [javaReleases, setJavaReleases] = useState<JavaRelease[]>([
    { version: 25, is_lts: true, name: 'Java 25 (LTS)' },
    { version: 24, is_lts: false, name: 'Java 24' },
    { version: 21, is_lts: true, name: 'Java 21 (LTS - Recommended)' },
    { version: 17, is_lts: true, name: 'Java 17 (LTS)' },
    { version: 11, is_lts: true, name: 'Java 11 (LTS)' },
    { version: 8, is_lts: true, name: 'Java 8 (LTS)' },
  ]);
  const [javaVersion, setJavaVersion] = useState<number>(21);
  const [versions, setVersions] = useState<VersionMeta[]>([]);
  const [builds, setBuilds] = useState<VersionInfo[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [loadingBuilds, setLoadingBuilds] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Port pool state
  const [port, setPort] = useState<string>('');
  const [behindProxy, setBehindProxy] = useState<boolean>(false);
  const [availablePorts, setAvailablePorts] = useState<number[]>([]);
  const [usedPorts, setUsedPorts] = useState<number[]>([]);
  const [usedPortDetails, setUsedPortDetails] = useState<Record<number, { serverName: string; description?: string }>>({});
  const [loadingPorts, setLoadingPorts] = useState(false);

  // Modpack file upload state
  const [modpackFile, setModpackFile] = useState<File | null>(null);
  const [modpackManifest, setModpackManifest] = useState<ModpackManifest | null>(null);
  const [inspectingModpack, setInspectingModpack] = useState(false);
  const [inspectProgress, setInspectProgress] = useState<number | null>(null);
  const [inspectStats, setInspectStats] = useState<{ loaded: number; total: number } | null>(null);
  const [createProgress, setCreateProgress] = useState<number | null>(null);
  const [createProgressStats, setCreateProgressStats] = useState<{ loaded: number; total: number } | null>(null);
  const [createStageInfo, setCreateStageInfo] = useState<{ index: number; total: number; title?: string } | null>(null);
  const [createStageDetail, setCreateStageDetail] = useState<string>('');

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
  const isPortUsed = !behindProxy && !isNaN(parsedPort) && usedPorts.includes(parsedPort);
  const isPortInvalid = !behindProxy && (isNaN(parsedPort) || parsedPort < 1 || parsedPort > 65535);
  const portConflict = !isNaN(parsedPort) ? usedPortDetails[parsedPort] : undefined;

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoadingPorts(true);
    api.javaVersions()
      .then((releases) => {
        if (!cancelled && releases && releases.length > 0) {
          setJavaReleases(releases);
        }
      })
      .catch(() => {});
    Promise.all([api.availablePorts(), api.listServers()])
      .then(([portsRes, serversRes]) => {
        if (cancelled) return;
        const free = portsRes.available ?? [];
        setAvailablePorts(free);
        const detailsMap: Record<number, { serverName: string; description?: string }> = {};
        const usedList: number[] = [];

        for (const s of serversRes ?? []) {
          if (s.host_port > 0) {
            if (!usedList.includes(s.host_port)) usedList.push(s.host_port);
            detailsMap[s.host_port] = { serverName: s.name, description: 'Primary game port' };
          }
          for (const ep of s.extra_ports ?? []) {
            if (ep.host_port > 0) {
              if (!usedList.includes(ep.host_port)) usedList.push(ep.host_port);
              detailsMap[ep.host_port] = {
                serverName: s.name,
                description: ep.description || `Additional port (${ep.protocol?.toUpperCase() || 'TCP'})`,
              };
            }
          }
        }

        for (const u of portsRes.used ?? []) {
          if (u.port > 0) {
            if (!usedList.includes(u.port)) usedList.push(u.port);
            if (!detailsMap[u.port]) {
              detailsMap[u.port] = {
                serverName: u.server_name,
                description: u.description || (u.type === 'extra_port' ? 'Additional port' : 'Primary game port'),
              };
            }
          }
        }

        setUsedPorts(usedList);
        setUsedPortDetails(detailsMap);
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
    if (serverType === 'custom') {
      setVersions([{ name: 'custom' }]);
      setVersion('custom');
      setBuilds([{ version: 'custom', build: 'custom', display: 'custom' }]);
      setBuild('custom');
      return;
    }
    let cancelled = false;
    setLoadingVersions(true);
    setError(null);
    api
      .jarVersions(serverType)
      .then((v) => {
        if (cancelled) return;
        const sorted = [...v].sort((a, b) => {
          const ma = (a.name || '').match(/(\d+)\.(\d+)(?:\.(\d+))?/);
          const mb = (b.name || '').match(/(\d+)\.(\d+)(?:\.(\d+))?/);
          if (ma && mb) {
            const a1 = parseInt(ma[1], 10), b1 = parseInt(mb[1], 10);
            if (a1 !== b1) return b1 - a1;
            const a2 = parseInt(ma[2], 10), b2 = parseInt(mb[2], 10);
            if (a2 !== b2) return b2 - a2;
            const a3 = ma[3] ? parseInt(ma[3], 10) : 0;
            const b3 = mb[3] ? parseInt(mb[3], 10) : 0;
            if (a3 !== b3) return b3 - a3;
          }
          return (b.name || '').localeCompare(a.name || '');
        });
        setVersions(sorted);
        const nextVer = (!version || !sorted.some((x) => x.name === version)) ? (sorted[0]?.name ?? '') : version;
        setVersion(nextVer);
        if (nextVer) {
          setJavaVersion(recommendJava(nextVer, serverType));
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
        const sorted = [...b].sort((x, y) => {
          const nx = parseInt(x.build, 10);
          const ny = parseInt(y.build, 10);
          if (!isNaN(nx) && !isNaN(ny) && String(nx) === x.build && String(ny) === y.build) {
            return ny - nx;
          }
          return (y.build || '').localeCompare(x.build || '', undefined, { numeric: true });
        });
        setBuilds(sorted);
        setBuild(sorted[0]?.build ?? '');
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
    setInspectProgress(0);
    setError(null);
    try {
      const manifest = await api.inspectModpackUpload(file, (loaded, total) => {
        if (total > 0) setInspectProgress(Math.round((loaded / total) * 100));
        setInspectStats({ loaded, total });
      });
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
        setJavaVersion(recommendJava(manifest.minecraft_version));
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
      setInspectProgress(null);
      setInspectStats(null);
    }
  }

  async function handleSelectModrinthHit(hit: ModrinthSearchHit) {
    setSelectingPack(true);
    setError(null);
    try {
      const versions = await getProjectVersions(hit.slug);
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
        loader_version: '',
        availableVersions: mappedVersions,
      });

      setName(hit.title);
      setServerType(detectedLoader);
      setVersion(mcVer);
      setBuild('');
      setRamMb(4096);
      setJavaVersion(recommendJava(mcVer));
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
        loader_version: '',
        availableVersions: mappedVersions,
      });

      setName(mod.name);
      setServerType(detectedLoader);
      setVersion(mcVer);
      setBuild('');
      setRamMb(4096);
      setJavaVersion(recommendJava(mcVer));
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
      loader_version: '',
    });
    setServerType(detectedLoader);
    setVersion(mcVer);
    setBuild('');
    setJavaVersion(recommendJava(mcVer));
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();

    if (!behindProxy) {
      if (isPortUsed) {
        setError(`Port ${port} is already in use by another server`);
        return;
      }
      if (port && isPortInvalid) {
        setError('Please enter a valid port between 1 and 65535');
        return;
      }
    }
    setBusy(true);
    setError(null);
    const input: CreateServerInput = {
      name: name.trim(),
      server_type: serverType,
      version: serverType === 'custom' ? (version || 'custom') : version,
      build: createMode === 'search-modpack' ? '' : (serverType === 'custom' ? (build || 'custom') : build),
      ram_mb: ramMb,
      java_version: javaVersion,
      host_port: behindProxy ? 0 : (parsedPort > 0 ? parsedPort : undefined),
      no_host_port: behindProxy,
    };
    let createdServerId: string | null = null;
    let unsubTask: (() => void) | null = null;
    try {
      const srv = await api.createServer(input);
      createdServerId = srv.id;

      if (createMode === 'search-modpack' || createMode === 'modpack') {
        unsubTask = api.subscribeTaskEvents(srv.id, (p) => {
          if (p.operation === 'modpack_install') {
            if (p.stage_index && p.stage_total) {
              setCreateStageInfo({ index: p.stage_index, total: p.stage_total, title: p.stage_title });
            }
            setCreateProgress(p.percent);
            setCreateStageDetail(p.message || '');
          }
        });
      }

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
        setCreateProgress(0);
        setCreateStageInfo({ index: 1, total: 4, title: 'Uploading Modpack' });
        await api.installModpackFile(srv.id, modpackFile, true, true, (loaded, total) => {
          if (total > 0) {
            setCreateProgress(Math.round((loaded / total) * 25));
            const mbDone = (loaded / (1024 * 1024)).toFixed(1);
            const mbTotal = (total / (1024 * 1024)).toFixed(1);
            setCreateStageDetail(`${mbDone} MB / ${mbTotal} MB`);
          }
          setCreateProgressStats({ loaded, total });
        });
      }
      setOpen(false);
      setName('');
      setModpackFile(null);
      setModpackManifest(null);
      setSelectedPack(null);
      onCreated();
    } catch (err) {
      if (createdServerId && (createMode === 'search-modpack' || createMode === 'modpack')) {
        try {
          await api.deleteServer(createdServerId);
        } catch {
          // ignore cleanup error
        }
      }
      setError(err instanceof ApiError ? err.detail : (err instanceof Error ? err.message : 'Failed to create server'));
    } finally {
      if (unsubTask) unsubTask();
      setBusy(false);
      setCreateProgress(null);
      setCreateProgressStats(null);
      setCreateStageInfo(null);
      setCreateStageDetail('');
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
                    <div className="rounded-xl border border-primary/20 bg-primary/5 p-3.5 animate-fadeIn">
                      <ProgressBar
                        value={inspectProgress}
                        label={
                          <span className="flex items-center gap-2 font-medium">
                            <Loader2 className="h-3.5 w-3.5 animate-spin text-primary shrink-0" />
                            <span>Uploading modpack archive for inspection...</span>
                          </span>
                        }
                        subtext={
                          inspectStats && inspectStats.total > 0
                            ? `${(inspectStats.loaded / (1024 * 1024)).toFixed(1)} MB / ${(inspectStats.total / (1024 * 1024)).toFixed(1)} MB`
                            : undefined
                        }
                        size="sm"
                      />
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
                        {modpackManifest.user_required_files && modpackManifest.user_required_files.length > 0 && (
                          <span className="text-amber-600 dark:text-amber-400 font-medium">
                            {' '}· ({modpackManifest.user_required_files.length} manual required)
                          </span>
                        )}
                      </p>
                      {modpackManifest.user_required_files && modpackManifest.user_required_files.length > 0 && (
                        <div className="mt-2 rounded-lg border border-amber-500/30 bg-amber-500/10 p-2 text-[11px] text-amber-900 dark:text-amber-200">
                          <div className="flex items-center gap-1.5 font-semibold text-amber-800 dark:text-amber-300">
                            <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
                            <span>Manual action needed after creation:</span>
                          </div>
                          <p className="mt-0.5 text-muted-foreground">
                            {modpackManifest.user_required_files.length} mod(s) cannot be auto-downloaded and must be placed into <code className="bg-muted px-1 py-0.5 rounded font-mono text-[10px]">mods/</code> manually ({modpackManifest.user_required_files.slice(0, 3).join(', ')}{modpackManifest.user_required_files.length > 3 ? '...' : ''}).
                          </p>
                        </div>
                      )}
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
                    {!behindProxy && (
                      isPortUsed ? (
                        <span className="text-[11px] font-semibold text-destructive">Already in use</span>
                      ) : isPortInvalid ? (
                        <span className="text-[11px] font-semibold text-destructive">Invalid port</span>
                      ) : (
                        <span className="text-[11px] font-semibold text-emerald-600 dark:text-emerald-400">Available</span>
                      )
                    )}
                  </div>
                  {behindProxy ? (
                    <div className="flex h-9 items-center justify-between px-3 rounded-lg border border-border bg-muted/40 text-xs text-muted-foreground font-mono">
                      <span>Internal Network Only (mcm-:id)</span>
                      <span className="text-[10px] text-primary font-sans font-medium">No host port required</span>
                    </div>
                  ) : (
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
                  )}
                  <div className="flex items-center justify-between pt-1">
                    <label className="flex items-center gap-1.5 cursor-pointer text-xs text-muted-foreground hover:text-foreground">
                      <input
                        type="checkbox"
                        checked={behindProxy}
                        onChange={(e) => setBehindProxy(e.target.checked)}
                        className="rounded border-border text-primary focus:ring-primary h-3.5 w-3.5"
                      />
                      <span>Behind Proxy (Internal only, no open host port)</span>
                    </label>
                  </div>
                  {!behindProxy && isPortUsed && (
                    <div
                      data-testid="port-conflict-alert"
                      className="rounded-md border border-destructive/50 bg-destructive/10 p-2.5 text-xs text-destructive space-y-1"
                    >
                      <div className="flex items-center gap-1.5 font-medium">
                        <AlertTriangle className="h-4 w-4 shrink-0 text-destructive" />
                        <span>Port {port} is already in use</span>
                      </div>
                      <p className="text-[11px] text-muted-foreground">
                        Occupied by <strong>{portConflict?.serverName || 'another server or container'}</strong>
                        {portConflict?.description ? ` (${portConflict.description})` : ''}.
                        Please choose an available port to avoid startup failure.
                      </p>
                    </div>
                  )}
                  {!behindProxy && !isPortUsed && !isPortInvalid && availablePorts.length > 0 && (
                    <p className="text-[11px] text-muted-foreground">
                      Prefilled from available port pool.
                    </p>
                  )}
                </div>
              </div>

              {createMode === 'standard' ? (
                <>
                  <div className="space-y-1.5">
                    <Label htmlFor="server-type">Server Software</Label>
                    <Select
                      id="server-type"
                      value={serverType}
                      onChange={(e) => {
                        const nextType = e.target.value as ServerType;
                        setServerType(nextType);
                        setVersion('');
                        setBuild('');
                        setBuilds([]);
                        if (nextType === 'velocity' || nextType === 'waterfall' || nextType === 'bungeecord') {
                          if (!port || port === '25565') setPort('25577');
                        } else if (nextType === 'geysermc') {
                          if (!port || port === '25565') setPort('19132');
                        }
                      }}
                    >
                      <optgroup label="Popular Server Platforms">
                        <option value="paper">Paper (Recommended)</option>
                        <option value="purpur">Purpur (High Performance &amp; Features)</option>
                        <option value="fabric">Fabric (Modded)</option>
                        <option value="neoforge">NeoForge (Modern Modded)</option>
                        <option value="forge">Forge (Modded)</option>
                        <option value="spigot">Spigot</option>
                        <option value="vanilla">Vanilla (Official Mojang)</option>
                      </optgroup>
                      <optgroup label="High Performance &amp; Async">
                        <option value="folia">Folia (Multithreaded Paper)</option>
                        <option value="pufferfish">Pufferfish (High Performance Paper)</option>
                        <option value="leaf">Leaf (Optimized Paper Fork)</option>
                      </optgroup>
                      <optgroup label="Modded &amp; Hybrid Platforms">
                        <option value="quilt">Quilt (Fabric-Compatible Modloader)</option>
                        <option value="mohist">Mohist (Forge + Paper Hybrid)</option>
                        <option value="ketting">Ketting (NeoForge/Forge Hybrid)</option>
                        <option value="sponge">Sponge (SpongeForge/Vanilla)</option>
                        <option value="crucible">Crucible (1.7.10 Hybrid)</option>
                      </optgroup>
                      <optgroup label="Proxies &amp; Lightweight">
                        <option value="velocity">Velocity (PaperMC Modern Proxy)</option>
                        <option value="waterfall">Waterfall (PaperMC Proxy)</option>
                        <option value="bungeecord">BungeeCord (Proxy)</option>
                        <option value="limbo">Limbo (Ultra-lightweight Fake Server)</option>
                        <option value="nanolimbo">NanoLimbo (Tiny Limbo Server)</option>
                      </optgroup>
                      <optgroup label="Bridges &amp; Custom">
                        <option value="geysermc">GeyserMC (Standalone Bedrock Bridge)</option>
                        <option value="custom">Custom JAR</option>
                      </optgroup>
                    </Select>
                  </div>

                  <div className="grid grid-cols-2 gap-4">
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
                    <div className="space-y-1.5">
                      <div className="flex items-center justify-between">
                        <Label htmlFor="server-java">Java Runtime</Label>
                        <span className="text-[10px] text-muted-foreground">Auto-recommended</span>
                      </div>
                      <Select
                        id="server-java"
                        value={String(javaVersion)}
                        onChange={(e) => setJavaVersion(parseInt(e.target.value, 10))}
                      >
                        {javaReleases.map((jr) => (
                          <option key={jr.version} value={jr.version}>
                            {jr.name}
                          </option>
                        ))}
                      </Select>
                    </div>
                  </div>

                  {serverType === 'custom' ? (
                    <div className="rounded-lg border border-primary/20 bg-primary/5 p-3 text-xs space-y-1">
                      <span className="font-semibold text-foreground">Custom Server JAR</span>
                      <p className="text-muted-foreground text-[11px]">
                        MCM will run <code>server.jar</code> (or <code>run.sh</code>) directly in the lightweight Java Alpine container. Place your JAR in the server directory after creation.
                      </p>
                    </div>
                  ) : (serverType === 'forge' || serverType === 'neoforge') ? (
                    <div className="rounded-lg border border-primary/20 bg-primary/5 p-3 text-xs space-y-1">
                      <span className="font-semibold text-foreground">{serverType === 'neoforge' ? 'NeoForge' : 'Forge'} Server Setup</span>
                      <p className="text-muted-foreground text-[11px]">
                        The installer jar will run automatically on first start to generate server libraries and launch scripts.
                      </p>
                    </div>
                  ) : null}

                  {serverType !== 'custom' && (
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
                            onChange={(e) => {
                              const nextVer = e.target.value;
                              setVersion(nextVer);
                              setBuild('');
                              setBuilds([]);
                              setJavaVersion(recommendJava(nextVer, serverType));
                            }}
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
                  )}
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
                      {build && createMode !== 'search-modpack' && <span className="text-muted-foreground text-[10px]">({build})</span>}
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

              {/* Server creation / modpack upload progress bar */}
              {busy && (
                <div className="space-y-2 rounded-xl border border-primary/20 bg-primary/5 p-3.5 animate-fadeIn">
                  {createStageInfo && createStageInfo.total > 0 && (
                    <div className="flex items-center justify-between text-xs text-muted-foreground pb-1">
                      <span className="font-semibold text-primary">
                        Stage {createStageInfo.index} of {createStageInfo.total}
                      </span>
                      <span className="font-medium text-foreground truncate max-w-[240px]">
                        {createStageInfo.title}
                      </span>
                    </div>
                  )}
                  <ProgressBar
                    value={createProgress}
                    showPercent={createProgress !== null}
                    label={
                      <span className="flex items-center gap-2 font-medium">
                        <Loader2 className="h-4 w-4 animate-spin text-primary shrink-0" />
                        <span>
                          {createStageInfo?.title
                            ? createStageInfo.title
                            : createMode === 'modpack' && createProgress !== null
                              ? 'Uploading & installing modpack archive...'
                              : createMode === 'search-modpack'
                                ? 'Creating container & downloading modpack...'
                                : 'Creating server container...'}
                        </span>
                      </span>
                    }
                    subtext={
                      createStageDetail ||
                      (createProgressStats && createProgressStats.total > 0
                        ? `${(createProgressStats.loaded / (1024 * 1024)).toFixed(1)} MB / ${(createProgressStats.total / (1024 * 1024)).toFixed(1)} MB`
                        : undefined)
                    }
                    size="md"
                  />
                </div>
              )}

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
