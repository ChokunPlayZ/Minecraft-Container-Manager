import { useCallback, useEffect, useState } from 'react';
import {
  Compass,
  FolderOpen,
  Plus,
  Power,
  RefreshCw,
  Search,
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

export function ModsPanel({ server }: { server: Server }) {
  const [activeTab, setActiveTab] = useState<'installed' | 'browse'>('installed');
  const [items, setItems] = useState<Mod[]>([]);
  const [type, setType] = useState<'mods' | 'plugins'>('mods');
  const [error, setError] = useState<string | null>(null);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [installedSearch, setInstalledSearch] = useState('');
  const { confirm, dialog } = useModal();

  const unsupported = server.server_type === 'vanilla';

  const load = useCallback(async () => {
    if (unsupported) {
      setItems([]);
      setError(null);
      return;
    }
    try {
      const res = await api.mods(server.id);
      setItems(res.items ?? []);
      setType(res.type);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load mods');
    }
  }, [server.id, unsupported]);

  useEffect(() => {
    void load();
  }, [load]);

  async function upload(file: File) {
    setError(null);
    setUploadProgress(0);
    try {
      await api.uploadMod(server.id, file, (loaded, total) => {
        setUploadProgress(total > 0 ? Math.round((loaded / total) * 100) : 0);
      });
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
        <div className="flex items-center justify-between border-b border-border/80 pb-2.5">
          <div className="flex items-center gap-2">
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

          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => void load()}
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
                  filteredItems.map((m) => (
                    <div
                      key={m.name}
                      className="flex items-center justify-between gap-2 rounded-lg border bg-card/60 p-3 text-sm transition-colors hover:bg-secondary/20"
                    >
                      <div className="min-w-0">
                        <p className="truncate font-medium text-foreground">{m.name}</p>
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
                        </p>
                      </div>
                      <div className="flex shrink-0 items-center gap-1">
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
                  ))
                )}
              </div>
            </CardContent>
          </Card>
        )}

        {/* Tab 2: Mod & Plugin Catalogs */}
        {activeTab === 'browse' && (
          <CatalogBrowser
            server={server}
            installedMods={items}
            onModInstalled={() => {
              void load();
            }}
          />
        )}
      </div>
    </>
  );
}
