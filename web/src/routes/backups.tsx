import { useCallback, useEffect, useState } from 'react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Archive, ArrowRight, Loader2, RefreshCw } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import { AppShell } from '../components/app-shell';
import { BackupsPanel } from '../components/backups-panel';
import { RequireAuth } from '../components/require-auth';
import { StatusBadge } from '../components/status-badge';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';
import { Select } from '../components/ui/select';
import { Skeleton } from '../components/ui/skeleton';
import { Switch } from '../components/ui/switch';

export const Route = createFileRoute('/backups')({
  validateSearch: (search: Record<string, unknown>): { server?: string } => ({
    server: typeof search.server === 'string' ? search.server : undefined,
  }),
  component: BackupsRoute,
});

function BackupsRoute() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  return <BackupManager serverId={search.server} onSelect={(server) => void navigate({ search: { server }, replace: true })} />;
}

export function BackupManager({ serverId, onSelect }: { serverId?: string; onSelect: (id: string) => void }) {
  const [servers, setServers] = useState<Server[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const load = useCallback(async () => {
    setLoading(true);
    try { setServers(await api.listServers()); setError(null); }
    catch (err) { setError(err instanceof ApiError ? err.detail : 'Could not load servers. Try again.'); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);
  const selected = serverId ? servers?.find((server) => server.id === serverId) : servers?.[0];

  return <RequireAuth><AppShell>
    <div className="page-heading">
      <div><h1 className="page-title">Backups</h1><p className="page-description">Create, restore, and schedule backups for your servers.</p></div>
      <Button variant="outline" onClick={() => void load()} disabled={loading}><RefreshCw className={loading ? 'animate-spin' : ''} />Refresh servers</Button>
    </div>
    {error && <div role="alert" className="mb-6 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive"><p>{error}</p><Button variant="outline" size="sm" disabled={loading} onClick={() => void load()}>Try again</Button></div>}
    {servers === null ? loading && <div className="space-y-5" aria-label="Loading backup manager"><Skeleton className="h-24 w-full" /><Skeleton className="h-64 w-full" /></div> : servers.length === 0 ?
      <div className="empty-state rounded-lg border bg-card"><Archive /><h2 className="section-heading">No servers to back up</h2><p>Create a server to start managing its backups.</p><Button className="mt-5" asChild><Link to="/dashboard">Go to servers<ArrowRight /></Link></Button></div> : <div className="space-y-6">
        <div className="flex flex-wrap items-end gap-4 rounded-lg border bg-card p-5 sm:p-6">
          <div className="w-full space-y-2 sm:max-w-sm"><Label htmlFor="backup-server">Server</Label><Select id="backup-server" value={selected?.id ?? ''} onValueChange={(selectedValue) => onSelect(selectedValue)}>
            {!selected && <option value="" disabled>Select a server</option>}
            {servers.map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}
          </Select></div>
          {selected && <div className="flex w-full flex-wrap items-center justify-between gap-3 sm:w-auto sm:flex-1 sm:pb-1"><StatusBadge state={selected.state} /><Button variant="ghost" size="sm" asChild><Link to="/servers/$id" params={{ id: selected.id }}>Manage server<ArrowRight /></Link></Button></div>}
          {serverId && !selected && <p role="alert" className="text-sm text-destructive">This server is unavailable. Select another server to manage its backups.</p>}
        </div>
        {selected && <>
          <BackupsPanel key={selected.id} server={selected} />
          <BackupSchedule key={`schedule-${selected.id}`} server={selected} onSaved={(updated) => setServers((current) => current?.map((server) => server.id === updated.id ? updated : server) ?? null)} />
        </>}
      </div>}
  </AppShell></RequireAuth>;
}

export function BackupSchedule({ server, onSaved }: { server: Server; onSaved: (server: Server) => void }) {
  const [enabled, setEnabled] = useState(server.backup_enabled);
  const [interval, setInterval] = useState(String(server.backup_interval_minutes || 720));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const dirty = enabled !== server.backup_enabled || Number(interval) !== server.backup_interval_minutes;

  async function save(event: React.FormEvent) {
    event.preventDefault();
    const minutes = Number(interval);
    if (enabled && (!Number.isInteger(minutes) || minutes < 5)) {
      setError('Enter a whole number of at least 5 minutes.');
      return;
    }
    setBusy(true); setError(null); setSaved(false);
    try {
      onSaved(await api.updateServer(server.id, {
        backup_enabled: enabled,
        backup_interval_minutes: enabled ? minutes : server.backup_interval_minutes,
      }));
      if (!enabled) setInterval(String(server.backup_interval_minutes));
      setSaved(true);
    } catch (err) { setError(err instanceof ApiError ? err.detail : 'Could not save the backup schedule. Try again.'); }
    finally { setBusy(false); }
  }

  return <Card>
    <CardHeader><CardTitle className="text-base">Automatic backups</CardTitle><CardDescription>Scheduled backups use the panel’s default backup storage.</CardDescription></CardHeader>
    <CardContent><form onSubmit={(event) => void save(event)} className="space-y-5">
      <div className="flex items-center justify-between gap-4"><Label htmlFor="backup-schedule-enabled">Enable automatic backups</Label><Switch id="backup-schedule-enabled" checked={enabled} disabled={busy} onCheckedChange={(value) => { setEnabled(value); setSaved(false); }} /></div>
      <div className="flex flex-wrap items-end gap-3"><div className="w-full space-y-2 sm:max-w-xs"><Label htmlFor="backup-schedule-interval">Interval (minutes)</Label><Input id="backup-schedule-interval" type="number" min={5} step={1} required value={interval} disabled={!enabled || busy} onChange={(event) => { setInterval(event.target.value); setSaved(false); }} /></div><Button type="submit" variant="outline" disabled={busy || !dirty}>{busy && <Loader2 className="animate-spin" />}{busy ? 'Saving…' : 'Save schedule'}</Button></div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {saved && <p role="status" className="text-sm text-primary">Backup schedule saved.</p>}
    </form></CardContent>
  </Card>;
}
