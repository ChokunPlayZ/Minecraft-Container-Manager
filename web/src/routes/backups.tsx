import { usePermissions } from '../lib/auth';
import { useCallback, useEffect, useState } from 'react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Archive, ArrowRight, CalendarClock, RefreshCw } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import { AppShell } from '../components/app-shell';
import { BackupsPanel } from '../components/backups-panel';
import { RequireAuth } from '../components/require-auth';
import { StatusBadge } from '../components/status-badge';
import { Button } from '../components/ui/button';
import { Label } from '../components/ui/label';
import { Select } from '../components/ui/select';
import { Skeleton } from '../components/ui/skeleton';

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
  const can = usePermissions();
  const [servers, setServers] = useState<Server[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const load = useCallback(async () => {
    setLoading(true);
    try { setServers((await api.listServers()).filter((server) => can('backups.read', server.id))); setError(null); }
    catch (err) { setError(err instanceof ApiError ? err.detail : 'Could not load servers. Try again.'); }
    finally { setLoading(false); }
  }, [can]);
  useEffect(() => { void load(); }, [load]);
  const selected = serverId ? servers?.find((server) => server.id === serverId) : servers?.[0];

  return <RequireAuth><AppShell>
    <div className="page-heading">
      <div><h1 className="page-title">Backups</h1><p className="page-description">Create, restore, and manage backups for your servers.</p></div>
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
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-card p-5 sm:p-6">
            <div><h2 className="font-semibold">Automatic backups</h2><p className="mt-1 text-sm text-muted-foreground">Manage recurring backups in this server’s Schedules tab.</p></div>
            <Button variant="outline" disabled={!can('schedules.read', selected.id)} asChild><Link to="/servers/$id" params={{ id: selected.id }} search={{ tab: 'schedules' }}><CalendarClock />Manage schedules</Link></Button>
          </div>
        </>}
      </div>}
  </AppShell></RequireAuth>;
}
