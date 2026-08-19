import { useState, type FormEvent } from 'react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';

export function ServerSettings({ server, onSaved }: { server: Server; onSaved: (s: Server) => void }) {
  const [name, setName] = useState(server.name);
  const [ramMb, setRamMb] = useState(server.ram_mb);
  const [backupEnabled, setBackupEnabled] = useState(server.backup_enabled ?? true);
  const [backupInterval, setBackupInterval] = useState(server.backup_interval_minutes ?? 720);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const updated = await api.updateServer(server.id, {
        name: name.trim(),
        ram_mb: ramMb,
        backup_enabled: backupEnabled,
        backup_interval_minutes: backupInterval,
      });
      onSaved(updated);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to save settings');
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Settings</CardTitle>
        <CardDescription>Update the server name and allocated memory.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="edit-name">Name</Label>
            <Input id="edit-name" required value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-ram">RAM (MB)</Label>
            <Input
              id="edit-ram"
              type="number"
              min={512}
              step={256}
              value={ramMb}
              onChange={(e) => setRamMb(Number(e.target.value))}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-backup-interval">Automatic backup interval (minutes)</Label>
            <Input
              id="edit-backup-interval"
              type="number"
              min={5}
              step={5}
              value={backupInterval}
              disabled={!backupEnabled}
              onChange={(e) => setBackupInterval(Number(e.target.value))}
            />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={backupEnabled}
              onChange={(e) => setBackupEnabled(e.target.checked)}
            />
            Enable automatic backups
          </label>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button type="submit" disabled={busy}>
            {busy ? 'Saving...' : 'Save'}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
