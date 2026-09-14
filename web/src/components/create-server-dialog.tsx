import { useEffect, useState, type FormEvent } from 'react';
import { Box, CheckCircle2, Loader2, Package, Sparkles, UploadCloud, X } from 'lucide-react';
import { api, ApiError, type CreateServerInput } from '../api/client';
import type { ModpackManifest, ServerType, VersionInfo, VersionMeta } from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';

export function CreateServerDialog({ onCreated }: { onCreated: () => void }) {
  const [open, setOpen] = useState(false);
  const [createMode, setCreateMode] = useState<'standard' | 'modpack'>('standard');
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

  // Modpack mode state
  const [modpackFile, setModpackFile] = useState<File | null>(null);
  const [modpackManifest, setModpackManifest] = useState<ModpackManifest | null>(null);
  const [inspectingModpack, setInspectingModpack] = useState(false);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoadingVersions(true);
    setError(null);
    api
      .jarVersions(serverType)
      .then((v) => {
        if (cancelled) return;
        setVersions(v);
        // Only override version if not already set by modpack inspection
        if (!version || !v.some((x) => x.name === version)) {
          setVersion(v[0]?.name ?? '');
        }
      })
      .catch((err) => !cancelled && setError(err instanceof ApiError ? err.detail : 'Failed to load versions'))
      .finally(() => !cancelled && setLoadingVersions(false));
    return () => {
      cancelled = true;
    };
  }, [open, serverType]);

  useEffect(() => {
    if (!open || !version) return;
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
  }, [open, serverType, version]);

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
      setRamMb(4096); // Modpacks require more RAM
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to inspect modpack archive');
      setModpackManifest(null);
    } finally {
      setInspectingModpack(false);
    }
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const input: CreateServerInput = {
      name: name.trim(),
      server_type: serverType,
      version,
      build,
      ram_mb: ramMb,
    };
    try {
      const srv = await api.createServer(input);
      if (modpackFile) {
        await api.installModpackFile(srv.id, modpackFile, true);
      }
      setOpen(false);
      setName('');
      setModpackFile(null);
      setModpackManifest(null);
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={() => !busy && setOpen(false)}>
          <div
            className="w-full max-w-lg rounded-2xl border bg-card p-6 shadow-2xl transition-all"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between pb-3 border-b border-border/80">
              <div>
                <h2 className="text-lg font-bold">Create server</h2>
                <p className="text-xs text-muted-foreground">
                  Deploy a new Minecraft server container with custom settings or a modpack.
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
            <div className="mt-4 grid grid-cols-2 gap-2 bg-secondary/30 p-1 rounded-xl border border-border/60 text-xs font-semibold">
              <button
                type="button"
                onClick={() => {
                  setCreateMode('standard');
                  setModpackFile(null);
                  setModpackManifest(null);
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
                onClick={() => setCreateMode('modpack')}
                className={`py-1.5 rounded-lg transition-all flex items-center justify-center gap-1.5 ${
                  createMode === 'modpack'
                    ? 'bg-primary text-primary-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                <Box className="h-3.5 w-3.5" />
                From Modpack (.mrpack, .zip)
              </button>
            </div>

            <form onSubmit={onSubmit} className="mt-4 space-y-4">
              {/* Modpack Dropzone in Modpack Mode */}
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
                        <strong className="text-foreground">{modpackManifest.server_files > 0 ? modpackManifest.server_files : modpackManifest.total_files}</strong>
                      </p>
                    </div>
                  )}
                </div>
              )}

              <div className="space-y-1.5">
                <Label htmlFor="server-name">Name</Label>
                <Input
                  id="server-name"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="My survival world"
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <Label htmlFor="server-type">Type</Label>
                  <Select
                    id="server-type"
                    value={serverType}
                    onChange={(e) => setServerType(e.target.value as ServerType)}
                  >
                    <option value="paper">Paper</option>
                    <option value="fabric">Fabric</option>
                    <option value="vanilla">Vanilla</option>
                    <option value="forge">Forge</option>
                    <option value="neoforge">NeoForge</option>
                    <option value="spigot">Spigot</option>
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
                      {versions.map((v) => (
                        <option key={v.name} value={v.name}>
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
                      {builds.map((b) => (
                        <option key={b.build} value={b.build}>
                          {b.build}
                        </option>
                      ))}
                    </Select>
                  )}
                </div>
              </div>

              {error && <p className="text-sm text-destructive">{error}</p>}

              <div className="flex justify-end gap-2 pt-2 border-t border-border/80">
                <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={busy}>
                  Cancel
                </Button>
                <Button type="submit" disabled={busy || loadingVersions || loadingBuilds}>
                  {busy ? (
                    <>
                      <Loader2 className="h-4 w-4 animate-spin" />
                      {modpackFile ? 'Creating & Installing...' : 'Creating...'}
                    </>
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
