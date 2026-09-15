import { useCallback, useEffect, useState } from 'react';
import { Download, Plus, RotateCcw, Trash2 } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { BackupRecord, Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';
import { useModal } from './ui/modal';

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B';
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

  async function createBackup() {
    setBusy(true);
    setError(null);
    try {
      await api.createBackup(server.id, name.trim(), storage);
      setName('');
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to create backup');
    } finally {
      setBusy(false);
    }
  }

  async function restore(backupId: string) {
    if (!(await confirm('Restore this backup over the current world?', { title: 'Restore backup', confirmLabel: 'Restore', destructive: true }))) return;
    setError(null);
    try {
      await api.restoreBackup(server.id, backupId);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Restore failed');
    }
  }

  async function remove(backupId: string) {
    if (!(await confirm('Delete this backup? This cannot be undone.', { title: 'Delete backup', confirmLabel: 'Delete', destructive: true }))) return;
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
          <div className="grid grid-cols-1 sm:grid-cols-4 gap-2">
            <div className="sm:col-span-2 space-y-1.5">
              <Label htmlFor="backup-name">Backup name (optional)</Label>
              <Input
                id="backup-name"
                placeholder="pre-update"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="backup-storage">Storage target</Label>
              <Select
                id="backup-storage"
                value={storage}
                onChange={(e) => setStorage(e.target.value as 'local' | 's3')}
              >
                <option value="local">Local Disk</option>
                <option value="s3">S3 Cloud</option>
              </Select>
            </div>
            <div className="flex items-end">
              <Button onClick={() => void createBackup()} disabled={busy} className="w-full">
                <Plus className="h-4 w-4 mr-1" /> {busy ? 'Backing up...' : 'Create'}
              </Button>
            </div>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <div className="space-y-2">
            {backups.length === 0 && (
              <p className="text-sm text-muted-foreground">No backups yet.</p>
            )}
            {backups.map((b) => {
              const isLocal = b.location?.startsWith('local:');
              return (
                <div
                  key={b.id}
                  className="flex items-center justify-between gap-2 rounded-md border p-2.5 text-sm"
                >
                  <div className="min-w-0 space-y-0.5">
                    <div className="flex items-center gap-2">
                      <p className="truncate font-medium">{b.name}</p>
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
                      <span className="capitalize">{b.status}</span>
                    </p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Download backup"
                      title="Download backup archive"
                      onClick={() => api.downloadBackupFile(b.id, `${b.name || 'backup'}.tar.gz`)}
                    >
                      <Download className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="outline"
                      size="icon"
                      aria-label="Restore"
                      title="Restore this backup"
                      onClick={() => void restore(b.id)}
                    >
                      <RotateCcw className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Delete"
                      title="Delete this backup"
                      onClick={() => void remove(b.id)}
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

