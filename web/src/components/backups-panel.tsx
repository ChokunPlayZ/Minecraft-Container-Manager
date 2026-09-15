import { useCallback, useEffect, useRef, useState } from 'react';
import { CheckCircle2, Download, Loader2, Plus, RotateCcw, Trash2, UploadCloud } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { BackupProgress, BackupRecord, Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { ProgressBar } from './ui/progress';
import { Select } from './ui/select';
import { useModal } from './ui/modal';

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

export function BackupsPanel({ server }: { server: Server }) {
  const [backups, setBackups] = useState<BackupRecord[]>([]);
  const [name, setName] = useState('');
  const [storage, setStorage] = useState<'local' | 's3'>('local');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeProgress, setActiveProgress] = useState<BackupProgress | null>(null);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [uploadFileName, setUploadFileName] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { confirm, dialog } = useModal();

  const load = useCallback(async () => {
    try {
      const res = await api.listBackups(server.id);
      setBackups(res.backups ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load backups');
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
      () => {
        // SSE error fallback: poll once if needed
      },
    );

    return () => {
      unmounted = true;
      unsub();
    };
  }, [server.id, load]);

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
      await api.createBackup(server.id, name.trim(), storage);
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

    try {
      await api.deleteBackup(backupId);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Delete failed');
    }
  }

  return (
    <>
      {dialog}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Backups</CardTitle>
          <CardDescription>World snapshots stored locally or in S3-compatible cloud storage.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Create / Upload form */}
          <div className="grid grid-cols-1 sm:grid-cols-5 gap-2">
            <div className="sm:col-span-2 space-y-1.5">
              <Label htmlFor="backup-name">Backup name (optional)</Label>
              <Input
                id="backup-name"
                placeholder="pre-update"
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={busy}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="backup-storage">Storage target</Label>
              <Select
                id="backup-storage"
                value={storage}
                onChange={(e) => setStorage(e.target.value as 'local' | 's3')}
                disabled={busy}
              >
                <option value="local">Local Disk</option>
                <option value="s3">S3 Cloud</option>
              </Select>
            </div>
            <div className="flex items-end gap-1.5 sm:col-span-2">
              <Button onClick={() => void createBackup()} disabled={busy} className="flex-1 font-semibold">
                <Plus className="h-4 w-4 mr-1" /> {busy ? 'Processing...' : 'Create'}
              </Button>
              <input
                ref={fileInputRef}
                type="file"
                accept=".tar.gz,.tar,.gz,.zip"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) void handleBackupUpload(f);
                  e.target.value = '';
                }}
              />
              <Button
                variant="outline"
                disabled={busy || uploadProgress !== null}
                onClick={() => fileInputRef.current?.click()}
                title="Upload external backup archive"
                className="font-medium"
              >
                <UploadCloud className="h-4 w-4 mr-1.5 text-primary" /> Upload
              </Button>
            </div>
          </div>

          {/* Active Backup / Restore Operation Progress Bar */}
          {activeProgress && activeProgress.stage !== 'completed' && activeProgress.stage !== 'failed' && (
            <div className="space-y-2 rounded-xl border border-primary/20 bg-primary/5 p-3.5 transition-all">
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
                  <UploadCloud className="h-4 w-4 text-sky-500 animate-bounce" />
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

          {error && <p className="text-sm text-destructive font-medium">{error}</p>}

          <div className="space-y-2">
            {backups.length === 0 && (
              <p className="text-sm text-muted-foreground py-2">No backups yet.</p>
            )}
            {backups.map((b) => {
              const isLocal = b.location?.startsWith('local:');
              return (
                <div
                  key={b.id}
                  className="flex items-center justify-between gap-2 rounded-lg border p-2.5 text-sm transition-colors hover:bg-muted/30"
                >
                  <div className="min-w-0 space-y-0.5">
                    <div className="flex items-center gap-2">
                      <p className="truncate font-medium text-foreground">{b.name}</p>
                      <span
                        className={`inline-flex items-center rounded-md px-1.5 py-0.2 text-[10px] font-semibold uppercase tracking-wider ${
                          isLocal
                            ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                            : 'bg-sky-500/10 text-sky-600 dark:text-sky-400 border border-sky-500/20'
                        }`}
                      >
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
                      disabled={busy}
                    >
                      <Download className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="outline"
                      size="icon"
                      aria-label="Restore"
                      title="Restore this backup"
                      onClick={() => void restore(b.id)}
                      disabled={busy}
                    >
                      <RotateCcw className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Delete"
                      title="Delete this backup"
                      onClick={() => void remove(b.id)}
                      disabled={busy}
                    >
                      <Trash2 className="h-4 w-4 text-destructive/80 hover:text-destructive" />
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        </CardContent>
      </Card>
    </>
  );
}
