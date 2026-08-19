import { useCallback, useEffect, useState } from 'react';
import { Download, Plus, Trash2 } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { BackupRecord, Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

export function BackupsPanel({ server }: { server: Server }) {
  const [backups, setBackups] = useState<BackupRecord[]>([]);
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
      await api.createBackup(server.id, name.trim());
      setName('');
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to create backup');
    } finally {
      setBusy(false);
    }
  }

  async function restore(backupId: string) {
    if (!window.confirm('Restore this backup over the current world?')) return;
    setError(null);
    try {
      await api.restoreBackup(server.id, backupId);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Restore failed');
    }
  }

  async function remove(backupId: string) {
    if (!window.confirm('Delete this backup? This cannot be undone.')) return;
    try {
      await api.deleteBackup(backupId);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Delete failed');
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Backups</CardTitle>
        <CardDescription>Offsite world snapshots in S3-compatible storage.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-end gap-2">
          <div className="flex-1 space-y-1.5">
            <Label htmlFor="backup-name">Backup name (optional)</Label>
            <Input
              id="backup-name"
              placeholder="pre-update"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <Button onClick={() => void createBackup()} disabled={busy}>
            <Plus className="h-4 w-4" /> {busy ? 'Backing up...' : 'Create'}
          </Button>
        </div>

        {error && <p className="text-sm text-destructive">{error}</p>}

        <div className="space-y-2">
          {backups.length === 0 && (
            <p className="text-sm text-muted-foreground">No backups yet.</p>
          )}
          {backups.map((b) => (
            <div
              key={b.id}
              className="flex items-center justify-between gap-2 rounded-md border p-2.5 text-sm"
            >
              <div className="min-w-0">
                <p className="truncate font-medium">{b.name}</p>
                <p className="text-xs text-muted-foreground">
                  {new Date(b.created_at).toLocaleString()} · {formatBytes(b.size_bytes)} ·{' '}
                  <span className="capitalize">{b.status}</span>
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button
                  variant="outline"
                  size="icon"
                  aria-label="Restore"
                  title="Restore this backup"
                  onClick={() => void restore(b.id)}
                >
                  <Download className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Delete"
                  title="Delete this backup"
                  onClick={() => void remove(b.id)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
