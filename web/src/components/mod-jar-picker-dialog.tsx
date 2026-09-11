import { useEffect, useState } from 'react';
import {
  AlertCircle,
  ArrowRight,
  Check,
  Download,
  FolderArchive,
  Loader2,
  RefreshCw,
  Search,
  Sparkles,
  Trash2,
  UploadCloud,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import { formatFileSize } from '../api/modrinth';
import {
  fetchModAvailableJars,
  type AvailableModJar,
  type ModUpdateInfo,
} from '../api/mod-updates';
import type { Mod, Server } from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Input } from './ui/input';

export interface ModJarPickerDialogProps {
  isOpen: boolean;
  server: Server;
  mod: Mod;
  updateInfo?: ModUpdateInfo;
  onClose: () => void;
  onUpdated: (newMod: Mod) => void;
}

export function ModJarPickerDialog({
  isOpen,
  server,
  mod,
  updateInfo,
  onClose,
  onUpdated,
}: ModJarPickerDialogProps) {
  const [activeTab, setActiveTab] = useState<'catalog' | 'upload'>('catalog');
  const [jars, setJars] = useState<AvailableModJar[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searchFilter, setSearchFilter] = useState('');
  const [selectedJarUrl, setSelectedJarUrl] = useState<string | null>(null);
  const [isUpdating, setIsUpdating] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [deleteOldJar, setDeleteOldJar] = useState(true);

  // Load available jar versions whenever dialog opens or mod changes
  useEffect(() => {
    if (!isOpen) {
      setJars([]);
      setError(null);
      setSelectedJarUrl(null);
      setIsUpdating(false);
      setUploadProgress(null);
      setSearchFilter('');
      return;
    }

    let isMounted = true;
    const controller = new AbortController();

    setLoading(true);
    setError(null);

    fetchModAvailableJars(mod, updateInfo, server, controller.signal)
      .then((res) => {
        if (!isMounted) return;
        setJars(res);
        if (res.length === 0) {
          // If no online jars were found, default tab to upload
          setActiveTab('upload');
        } else {
          setActiveTab('catalog');
        }
      })
      .catch((err) => {
        if (!isMounted) return;
        setError(err instanceof Error ? err.message : 'Failed to fetch jar versions');
      })
      .finally(() => {
        if (isMounted) setLoading(false);
      });

    return () => {
      isMounted = false;
      controller.abort();
    };
  }, [isOpen, mod, updateInfo, server]);

  // Handle escape key
  useEffect(() => {
    if (!isOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !isUpdating) onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isOpen, isUpdating, onClose]);

  if (!isOpen) return null;

  async function handlePickJar(jar: AvailableModJar) {
    setIsUpdating(true);
    setSelectedJarUrl(jar.downloadUrl);
    setError(null);

    const oldName = deleteOldJar ? mod.name : undefined;

    try {
      const updated = await api.downloadMod(
        server.id,
        jar.downloadUrl,
        jar.filename,
        oldName,
        {
          projectId: updateInfo?.projectId || mod.project_id,
          projectSlug: updateInfo?.projectSlug || mod.project_slug,
          provider: updateInfo?.provider || mod.provider,
        },
      );
      onUpdated(updated);
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : err instanceof Error ? err.message : 'Download failed');
    } finally {
      setIsUpdating(false);
      setSelectedJarUrl(null);
    }
  }

  async function handleUploadJar(file: File) {
    setIsUpdating(true);
    setError(null);
    setUploadProgress(0);

    const oldName = deleteOldJar ? mod.name : undefined;

    try {
      const updated = await api.uploadMod(
        server.id,
        file,
        (loaded, total) => {
          setUploadProgress(total > 0 ? Math.round((loaded / total) * 100) : 0);
        },
        oldName,
      );
      onUpdated(updated);
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Upload failed');
    } finally {
      setIsUpdating(false);
      setUploadProgress(null);
    }
  }

  const filteredJars = jars.filter((j) => {
    if (!searchFilter.trim()) return true;
    const q = searchFilter.toLowerCase();
    return (
      j.filename.toLowerCase().includes(q) ||
      j.versionName.toLowerCase().includes(q) ||
      j.versionNumber.toLowerCase().includes(q) ||
      (j.gameVersions && j.gameVersions.some((gv) => gv.toLowerCase().includes(q)))
    );
  });

  const modTitle = mod.title || updateInfo?.title || mod.name;

  return (
    <div
      className="fixed inset-0 z-[110] flex items-center justify-center bg-black/60 backdrop-blur-xs p-4"
      role="presentation"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !isUpdating) onClose();
      }}
    >
      <div
        className="relative flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-border bg-card text-card-foreground shadow-2xl animate-in fade-in zoom-in-95"
        role="dialog"
        aria-modal="true"
        aria-labelledby="mod-jar-picker-title"
      >
        {/* Header */}
        <div className="flex items-start justify-between border-b p-5">
          <div className="flex items-start gap-3 min-w-0">
            <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary border border-primary/20">
              <Sparkles className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <h3
                  id="mod-jar-picker-title"
                  className="text-base font-bold leading-tight text-foreground truncate"
                >
                  Update {modTitle}
                </h3>
                {updateInfo?.provider && (
                  <Badge variant="outline" className="text-[10px] uppercase font-mono px-1.5 py-0">
                    {updateInfo.provider}
                  </Badge>
                )}
              </div>
              <p className="mt-1 text-xs text-muted-foreground">
                Pick a newer jar build from online releases or upload a replacement jar file.
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            disabled={isUpdating}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-secondary hover:text-foreground transition-colors disabled:opacity-50"
            aria-label="Close dialog"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Current Jar banner */}
        <div className="border-b bg-secondary/30 px-5 py-3 text-xs">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-center gap-2 min-w-0">
              <span className="font-semibold text-muted-foreground shrink-0">Current Jar:</span>
              <span className="font-mono font-medium text-destructive dark:text-red-400 truncate" title={mod.file}>
                {mod.file}
              </span>
              {mod.version && (
                <Badge variant="secondary" className="text-[10px] px-1.5 py-0 font-mono shrink-0">
                  v{mod.version}
                </Badge>
              )}
            </div>

            {updateInfo?.latestJar && updateInfo.latestJar !== mod.file && (
              <div className="flex items-center gap-1.5 text-emerald-700 dark:text-emerald-300 font-semibold shrink-0">
                <ArrowRight className="h-3.5 w-3.5" />
                <span>Latest: {updateInfo.latestVersion}</span>
              </div>
            )}
          </div>
        </div>

        {/* Subtabs: Online Releases vs Upload Jar */}
        <div className="flex items-center border-b px-5 pt-3 gap-2">
          <button
            type="button"
            onClick={() => setActiveTab('catalog')}
            className={`inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs font-semibold transition-all ${
              activeTab === 'catalog'
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            <FolderArchive className="h-4 w-4" />
            Available Releases
            {jars.length > 0 && (
              <Badge variant="secondary" className="ml-1 text-[10px] px-1.5 py-0">
                {jars.length}
              </Badge>
            )}
          </button>

          <button
            type="button"
            onClick={() => setActiveTab('upload')}
            className={`inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs font-semibold transition-all ${
              activeTab === 'upload'
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            <UploadCloud className="h-4 w-4" />
            Upload Local Jar
          </button>
        </div>

        {/* Content Body */}
        <div className="flex-1 overflow-y-auto p-5 space-y-4">
          {error && (
            <div className="flex items-start gap-2.5 rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0 mt-0.5" />
              <div className="flex-1 min-w-0">
                <p className="font-semibold">Operation failed</p>
                <p className="opacity-90">{error}</p>
              </div>
            </div>
          )}

          {activeTab === 'catalog' && (
            <>
              {loading ? (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground text-xs gap-3">
                  <Loader2 className="h-6 w-6 animate-spin text-primary" />
                  <span>Loading available releases for {server.server_type} {server.version ? `(MC ${server.version})` : ''}...</span>
                </div>
              ) : jars.length === 0 ? (
                <div className="rounded-xl border border-dashed py-10 px-4 text-center">
                  <FolderArchive className="mx-auto h-8 w-8 text-muted-foreground/60" />
                  <p className="mt-2 text-sm font-medium text-foreground">No online releases found</p>
                  <p className="mt-1 text-xs text-muted-foreground max-w-sm mx-auto">
                    Could not find compatible online builds for this server software. You can upload a new jar file directly using the Upload tab.
                  </p>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setActiveTab('upload')}
                    className="mt-4 gap-1.5 text-xs"
                  >
                    <UploadCloud className="h-3.5 w-3.5" />
                    Upload Local Jar
                  </Button>
                </div>
              ) : (
                <div className="space-y-3">
                  {jars.length > 4 && (
                    <div className="relative">
                      <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                      <Input
                        placeholder="Search version or jar name..."
                        value={searchFilter}
                        onChange={(e) => setSearchFilter(e.target.value)}
                        className="h-8 pl-8 text-xs"
                      />
                    </div>
                  )}

                  <div className="space-y-2 max-h-[380px] overflow-y-auto pr-1">
                    {filteredJars.map((j) => {
                      const isPickingThis = isUpdating && selectedJarUrl === j.downloadUrl;
                      const isAlreadyInstalled = j.isCurrent;

                      return (
                        <div
                          key={j.versionId + j.filename}
                          className={`flex items-center justify-between gap-3 rounded-lg border p-3 text-xs transition-all ${
                            isAlreadyInstalled
                              ? 'border-emerald-500/40 bg-emerald-500/10 dark:bg-emerald-950/20'
                              : 'border-border/80 bg-card hover:border-primary/40 hover:bg-secondary/20'
                          }`}
                        >
                          <div className="min-w-0 space-y-1 flex-1">
                            <div className="flex items-center gap-2 flex-wrap">
                              <span className="font-semibold text-foreground truncate">
                                {j.versionName || j.versionNumber}
                              </span>
                              {j.releaseType && (
                                <Badge
                                  variant={j.releaseType === 'release' ? 'default' : 'secondary'}
                                  className="text-[10px] px-1.5 py-0 uppercase"
                                >
                                  {j.releaseType}
                                </Badge>
                              )}
                              {isAlreadyInstalled && (
                                <Badge
                                  variant="outline"
                                  className="border-emerald-500/40 bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 gap-1 text-[10px] px-1.5 py-0 font-medium"
                                >
                                  <Check className="h-3 w-3 text-emerald-600 dark:text-emerald-400" />
                                  Currently Installed
                                </Badge>
                              )}
                            </div>

                            <p className="truncate font-mono text-[11px] text-muted-foreground">
                              {j.filename} {j.sizeBytes ? `· ${formatFileSize(j.sizeBytes)}` : ''}
                            </p>

                            {j.gameVersions && j.gameVersions.length > 0 && (
                              <div className="flex flex-wrap gap-1 pt-0.5">
                                {j.gameVersions.slice(0, 4).map((gv) => (
                                  <span
                                    key={gv}
                                    className="rounded bg-secondary px-1.5 py-0.2 text-[10px] text-secondary-foreground font-mono"
                                  >
                                    {gv}
                                  </span>
                                ))}
                                {j.gameVersions.length > 4 && (
                                  <span className="text-[10px] text-muted-foreground">
                                    +{j.gameVersions.length - 4} more
                                  </span>
                                )}
                              </div>
                            )}
                          </div>

                          <div className="flex items-center gap-2 shrink-0">
                            <Button
                              size="sm"
                              disabled={isUpdating}
                              onClick={() => void handlePickJar(j)}
                              className={`h-8 gap-1.5 text-xs ${
                                isAlreadyInstalled
                                  ? 'border border-emerald-500/40 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/20 dark:text-emerald-300'
                                  : 'bg-emerald-600 hover:bg-emerald-700 text-white'
                              }`}
                              title={isAlreadyInstalled ? 'Reinstall this jar version' : 'Select and install this jar'}
                            >
                              {isPickingThis ? (
                                <>
                                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                                  Updating...
                                </>
                              ) : isAlreadyInstalled ? (
                                <>
                                  <RefreshCw className="h-3.5 w-3.5" />
                                  Reinstall
                                </>
                              ) : (
                                <>
                                  <Download className="h-3.5 w-3.5" />
                                  Pick &amp; Update
                                </>
                              )}
                            </Button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}
            </>
          )}

          {activeTab === 'upload' && (
            <div className="space-y-4">
              <div className="flex flex-col sm:flex-row sm:items-center gap-3 rounded-xl border border-dashed border-border/80 bg-secondary/15 p-5">
                <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                  <UploadCloud className="h-6 w-6" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-semibold text-foreground">Upload replacement .jar</p>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    Select a .jar file from your computer to replace <span className="font-mono text-foreground font-medium">{mod.file}</span>.
                  </p>
                </div>
                <label className="cursor-pointer shrink-0">
                  <span className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3.5 py-2 text-xs font-semibold text-primary-foreground shadow-xs hover:bg-primary/90 transition-colors">
                    Choose .jar File
                  </span>
                  <input
                    type="file"
                    accept=".jar"
                    disabled={isUpdating}
                    className="hidden"
                    onChange={(e) => {
                      const f = e.target.files?.[0];
                      if (f) void handleUploadJar(f);
                    }}
                  />
                </label>
              </div>

              {uploadProgress !== null && (
                <div className="flex items-center gap-3 rounded-lg border p-3 text-xs">
                  <Loader2 className="h-4 w-4 animate-spin text-primary shrink-0" />
                  <span className="shrink-0 text-muted-foreground">Uploading replacement jar...</span>
                  <div className="h-2 flex-1 overflow-hidden rounded-full bg-secondary">
                    <div
                      className="h-full bg-primary transition-all"
                      style={{ width: `${uploadProgress}%` }}
                    />
                  </div>
                  <span className="shrink-0 tabular-nums font-mono text-muted-foreground">
                    {uploadProgress}%
                  </span>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between border-t bg-secondary/20 px-5 py-3 gap-3">
          <label className="flex items-center gap-2 cursor-pointer select-none text-xs text-muted-foreground">
            <input
              type="checkbox"
              checked={deleteOldJar}
              onChange={(e) => setDeleteOldJar(e.target.checked)}
              disabled={isUpdating}
              className="h-4 w-4 rounded border-input text-primary focus:ring-primary"
            />
            <span className="flex items-center gap-1">
              <Trash2 className="h-3 w-3 text-destructive" />
              Delete old jar (<span className="font-mono">{mod.file}</span>) to prevent conflicts
            </span>
          </label>

          <div className="flex items-center justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={isUpdating}
              onClick={onClose}
              className="text-xs"
            >
              Cancel
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
