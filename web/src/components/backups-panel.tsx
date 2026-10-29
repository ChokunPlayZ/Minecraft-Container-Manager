import { usePermissions } from '../lib/auth';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Archive, CheckCircle2, Download, Loader2, Plus, RefreshCw, RotateCcw, Search, Trash2, UploadCloud } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { BackupProgress, BackupRecord, Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { ProgressBar } from './ui/progress';
import { Select } from './ui/select';
import { Skeleton } from './ui/skeleton';
import { useModal } from './ui/modal';

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

export function BackupsPanel({ server }: { server: Server }) {
  const can = usePermissions();
  const canManage = can('backups.create', server.id);
  const [backups, setBackups] = useState<BackupRecord[]>([]);
  const [name, setName] = useState('');
  const [storage, setStorage] = useState<'local' | 's3'>('local');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [locationFilter, setLocationFilter] = useState('all');
  const [activeProgress, setActiveProgress] = useState<BackupProgress | null>(null);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [uploadFileName, setUploadFileName] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { confirm, dialog } = useModal();

  const load = useCallback(async () => {
    setRefreshing(true);
    try {
      const res = await api.listBackups(server.id);
      setBackups(res.backups ?? []);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof ApiError ? err.detail : 'Could not load backups. Try again.');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [server.id]);

  useEffect(() => {
    void load();
  }, [load]);

  // Initial check for in-flight progress & subscribe to real-time events
  useEffect(() => {
    let unmounted = false;

    // 1. Initial status query
    api.getBackupProgress(server.id).then((res) => {
      if (!unmounted && res.active && res.progress) {
        setActiveProgress(res.progress);
        if (res.progress.stage !== 'completed' && res.progress.stage !== 'failed') {
          setBusy(true);
        }
      }
    }).catch(() => {});

    // 2. Subscribe to SSE events
    const unsub = api.subscribeBackupEvents(
      server.id,
      (prog) => {
        if (unmounted) return;
        setActiveProgress(prog);
        if (prog.stage === 'completed') {
          setBusy(false);
          void load();
          setTimeout(() => {
            if (!unmounted) setActiveProgress((curr) => (curr?.id === prog.id ? null : curr));
          }, 3500);
        } else if (prog.stage === 'failed') {
          setBusy(false);
          if (prog.error) setError(prog.error);
        } else {
          setBusy(true);
        }
      },
    );

    return () => {
      unmounted = true;
      unsub();
    };
  }, [server.id, load]);

  // Recover progress if the event stream disconnects during a long operation.
  useEffect(() => {
    if (!busy) return;
    let cancelled = false;
    const timer = setInterval(() => {
      void api.getBackupProgress(server.id).then((res) => {
        if (cancelled || !res.progress) return;
        setActiveProgress(res.progress);
        if (res.progress.stage === 'completed' || res.progress.stage === 'failed') {
          setBusy(false);
          if (res.progress.stage === 'failed') setError(res.progress.error || res.progress.message);
          void load();
        }
      }).catch(() => {});
    }, 5000);
    return () => { cancelled = true; clearInterval(timer); };
  }, [busy, server.id, load]);

  const disabled = busy || uploadProgress !== null || deleting !== null;
  const visibleBackups = backups.filter((backup) => backup.name.toLowerCase().includes(query.trim().toLowerCase()) &&
    (locationFilter === 'all' || (locationFilter === 'local' ? backup.location.startsWith('local:') : !backup.location.startsWith('local:'))));

  async function createBackup() {
    setBusy(true);
    setError(null);
    setActiveProgress({
      id: 'pending',
      server_id: server.id,
      operation: 'backup',
      stage: 'scanning',
      percent: 5,
      message: 'Starting backup process...',
    });

    try {
      const backup = await api.createBackup(server.id, name.trim(), storage);
      setActiveProgress({ id: backup.id, server_id: server.id, operation: 'backup', stage: 'completed', percent: 100, message: 'Backup created.' });
      setName('');
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to create backup');
      setActiveProgress(null);
    } finally {
      setBusy(false);
    }
  }

  async function restore(backupId: string) {
    if (
      !(await confirm('Restore this backup over the current world? This will overwrite your current save files.', {
        title: 'Restore backup',
        confirmLabel: 'Restore',
        destructive: true,
      }))
    )
      return;

    setBusy(true);
    setError(null);
    setActiveProgress({
      id: backupId,
      server_id: server.id,
      operation: 'restore',
      stage: 'downloading',
      percent: 5,
      message: 'Initiating world restoration...',
    });

    try {
      await api.restoreBackup(server.id, backupId);
      setActiveProgress({ id: backupId, server_id: server.id, operation: 'restore', stage: 'completed', percent: 100, message: 'Backup restored.' });
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Restore failed');
      setActiveProgress(null);
    } finally {
      setBusy(false);
    }
  }

  async function handleBackupUpload(file: File) {
    if (!file) return;
    setUploadFileName(file.name);
    setUploadProgress(0);
    setError(null);

    try {
      await api.uploadBackup(server.id, file, undefined, storage, (loaded, total) => {
        if (total > 0) {
          setUploadProgress(Math.round((loaded / total) * 100));
        }
      });
      setUploadProgress(null);
      setUploadFileName(null);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Upload failed');
      setUploadProgress(null);
      setUploadFileName(null);
    }
  }

  async function remove(backupId: string) {
    if (
      !(await confirm('Delete this backup? This cannot be undone.', {
        title: 'Delete backup',
        confirmLabel: 'Delete',
        destructive: true,
      }))
    )
      return;

    setError(null);
    setDeleting(backupId);
    try {
      await api.deleteBackup(backupId);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Delete failed');
    } finally {
      setDeleting(null);
    }
  }

  return (
    <>
      {dialog}
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3 pb-3">
          <div className="space-y-1.5"><CardTitle className="text-base">Backups</CardTitle>
          <CardDescription>{loading ? 'Loading backup archives…' : `${backups.length} ${backups.length === 1 ? 'backup' : 'backups'} · ${formatBytes(backups.reduce((sum, backup) => sum + backup.size_bytes, 0))} stored`}</CardDescription></div>
          <Button variant="ghost" size="icon" aria-label="Refresh backups" disabled={refreshing || disabled} onClick={() => void load()}><RefreshCw className={refreshing ? 'animate-spin' : ''} /></Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Create / Upload form */}
          <form className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_160px_auto]" onSubmit={(event) => { event.preventDefault(); if (!disabled) void createBackup(); }}>
            <div className="space-y-1.5">
              <Label htmlFor="backup-name">Backup name (optional)</Label>
              <Input
                id="backup-name"
                placeholder="pre-update"
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={disabled || !canManage}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="backup-storage">Storage target</Label>
              <Select
                id="backup-storage"
                value={storage}
                onValueChange={(selectedValue) => setStorage(selectedValue as 'local' | 's3')}
                disabled={disabled || !canManage}
              >
                <option value="local">Local Disk</option>
                <option value="s3">S3 Cloud</option>
              </Select>
            </div>
            <div className="flex flex-wrap items-end gap-2 sm:col-span-2 xl:col-span-1">
              <Button type="submit" disabled={disabled || !canManage} className="flex-1 font-semibold">
                <Plus className="h-4 w-4 mr-1" /> {busy ? 'Processing…' : 'Create backup'}
              </Button>
              <input
                ref={fileInputRef}
                type="file"
                accept=".tar.gz"
                aria-label="Upload backup archive"
                disabled={disabled || !canManage}
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) void handleBackupUpload(f);
                  e.target.value = '';
                }}
              />
              <Button
                variant="outline"
                disabled={disabled || !canManage}
                onClick={() => fileInputRef.current?.click()}
                title="Upload external backup archive"
                className="font-medium"
              >
                <UploadCloud className="h-4 w-4 mr-1.5 text-primary" /> Upload
              </Button>
            </div>
          </form>

          {/* Active Backup / Restore Operation Progress Bar */}
          {activeProgress && activeProgress.stage !== 'completed' && activeProgress.stage !== 'failed' && (
            <div className="space-y-2 rounded-xl border border-primary/20 bg-primary/5 p-3.5 transition-colors">
              <div className="flex items-center justify-between text-xs">
                <span className="font-semibold text-foreground flex items-center gap-2">
                  <Loader2 className="h-4 w-4 animate-spin text-primary" />
                  <span className="capitalize">{activeProgress.operation}:</span> {activeProgress.message}
                </span>
                <span className="font-bold text-primary tabular-nums">{activeProgress.percent}%</span>
              </div>
              <ProgressBar
                value={activeProgress.percent}
                showPercent={false}
                variant="default"
                size="md"
              />
              {activeProgress.bytes_total && activeProgress.bytes_total > 0 && (
                <div className="flex justify-between text-[11px] text-muted-foreground tabular-nums">
                  <span>Processed: {formatBytes(activeProgress.bytes_done ?? 0)}</span>
                  <span>Total: {formatBytes(activeProgress.bytes_total)}</span>
                </div>
              )}
            </div>
          )}

          {/* Completed banner */}
          {activeProgress && activeProgress.stage === 'completed' && (
            <div className="flex items-center gap-2 rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-800 dark:text-emerald-300 font-medium animate-fadeIn">
              <CheckCircle2 className="h-4 w-4 text-emerald-600 shrink-0" />
              <span>{activeProgress.message || 'Operation completed successfully!'}</span>
            </div>
          )}

          {/* Upload Progress Bar */}
          {uploadProgress !== null && (
            <div className="space-y-2 rounded-xl border border-sky-500/20 bg-sky-500/5 p-3.5">
              <div className="flex items-center justify-between text-xs">
                <span className="font-medium text-foreground flex items-center gap-2 truncate">
                  <UploadCloud className="h-4 w-4 text-sky-500 " />
                  Uploading {uploadFileName || 'backup archive'}...
                </span>
                <span className="font-bold text-sky-600 dark:text-sky-400 tabular-nums">
                  {uploadProgress}%
                </span>
              </div>
              <ProgressBar
                value={uploadProgress}
                showPercent={false}
                variant="sky"
                size="md"
              />
            </div>
          )}

          {error && <p role="alert" className="text-sm text-destructive font-medium">{error}</p>}
          {loadError && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 text-sm text-destructive"><p>{loadError}</p><Button variant="outline" size="sm" disabled={refreshing} onClick={() => void load()}>Try again</Button></div>}

          {backups.length > 0 && <div className="flex flex-wrap gap-3 border-t pt-5">
            <div className="relative min-w-0 flex-1 basis-48"><Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" /><Input aria-label="Search backups" placeholder="Search backups…" value={query} onChange={(event) => setQuery(event.target.value)} className="pl-9" /></div>
            <Select aria-label="Filter backup storage" className="w-full sm:w-40" value={locationFilter} onValueChange={(selectedValue) => setLocationFilter(selectedValue)}><option value="all">All storage</option><option value="local">Local disk</option><option value="s3">S3 cloud</option></Select>
          </div>}

          <div className="space-y-2">
            {loading ? <div className="space-y-3 py-4" aria-label="Loading backups"><Skeleton className="h-16 w-full" /><Skeleton className="h-16 w-full" /></div> : !loadError && backups.length === 0 && (
              <div className="empty-state"><Archive /><h3 className="section-heading">No backups yet</h3><p>Create a backup or upload a .tar.gz archive for this server.</p></div>
            )}
            {backups.length > 0 && visibleBackups.length === 0 && <div className="empty-state"><Search /><h3 className="section-heading">No matching backups</h3><p>Try another name or storage target.</p><Button variant="outline" className="mt-5" onClick={() => { setQuery(''); setLocationFilter('all'); }}>Reset filters</Button></div>}
            {visibleBackups.map((b) => {
              const isLocal = b.location?.startsWith('local:');
              return (
                <div
                  key={b.id}
                  className="flex flex-wrap items-center justify-between gap-3 border-b py-4 text-sm last:border-0"
                >
                  <div className="min-w-0 flex-1 basis-48 space-y-1">
                    <div className="flex min-w-0 flex-wrap items-center gap-2">
                      <p className="min-w-0 break-all font-medium text-foreground">{b.name}</p>
                      <span className="inline-flex items-center rounded-md border bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
                        {isLocal ? 'Local' : 'S3'}
                      </span>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {new Date(b.created_at).toLocaleString()} · {formatBytes(b.size_bytes)} ·{' '}
                      <span className="capitalize font-medium">{b.status}</span>
                    </p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Download backup"
                      title="Download backup archive"
                      onClick={() => api.downloadBackupFile(b.id, `${b.name || 'backup'}.tar.gz`)}
                      disabled={disabled || !can('backups.download', server.id) || b.status !== 'completed'}
                    >
                      <Download className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="outline"
                      size="icon"
                      aria-label="Restore"
                      title="Restore this backup"
                      onClick={() => void restore(b.id)}
                      disabled={disabled || !can('backups.restore', server.id) || b.status !== 'completed'}
                    >
                      <RotateCcw className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Delete"
                      title="Delete this backup"
                      onClick={() => void remove(b.id)}
                      disabled={disabled || !can('backups.delete', server.id)}
                    >
                      {deleting === b.id ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4 text-destructive" />}
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
          {backups.length > 0 && <p role="status" className="border-t pt-3 text-xs text-muted-foreground">{visibleBackups.length} of {backups.length} {backups.length === 1 ? 'backup' : 'backups'}</p>}
        </CardContent>
      </Card>
    </>
  );
}
