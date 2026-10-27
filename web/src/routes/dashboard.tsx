import { createFileRoute, Link } from '@tanstack/react-router';
import { Box, Clock, RefreshCw, Search, Server as ServerIcon, X } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import { formatUptimeFromStartedAt } from '../lib/uptime';
import { AppShell } from '../components/app-shell';
import { CopyServerDialog } from '../components/copy-server-dialog';
import { CreateServerDialog } from '../components/create-server-dialog';
import { HostCapacity } from '../components/host-capacity';
import { RequireAuth } from '../components/require-auth';
import { ServerInstallProgress } from '../components/server-install-progress';
import { StatusBadge } from '../components/status-badge';
import { ServerQuickActions } from '../components/server-quick-actions';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Skeleton } from '../components/ui/skeleton';

export const Route = createFileRoute('/dashboard')({ component: DashboardRoute });
type FleetFilter = 'all' | 'running' | 'stopped' | 'attention';

export function DashboardRoute() {
  const [servers, setServers] = useState<Server[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copyTargetServer, setCopyTargetServer] = useState<Server | null>(null);
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<FleetFilter>('all');
  const [refreshing, setRefreshing] = useState(false);
  const load = useCallback(async () => {
    setRefreshing(true);
    try { setServers(await api.listServers()); setError(null); }
    catch (err) { setError(err instanceof ApiError ? err.detail : 'Could not load servers. Try refreshing.'); }
    finally { setRefreshing(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);
  const hasActiveServers = servers?.some((s) => ['installing', 'building', 'starting', 'stopping'].includes(s.state));
  useEffect(() => {
    if (!hasActiveServers) return;
    const timer = setInterval(() => void load(), 2500);
    return () => clearInterval(timer);
  }, [hasActiveServers, load]);

  const running = servers?.filter((s) => s.state === 'running').length ?? 0;
  const attention = servers?.filter((s) => s.state === 'error' || s.needs_rebuild).length ?? 0;
  const allocatedRam = servers?.reduce((sum, s) => sum + s.ram_mb, 0) ?? 0;
  const visible = useMemo(() => (servers ?? []).filter((s) => {
    const matchesSearch = `${s.name} ${s.server_type} ${s.version} ${s.host_port}`.toLowerCase().includes(query.toLowerCase().trim());
    return matchesSearch && (filter === 'all' || (filter === 'attention' ? s.state === 'error' || s.needs_rebuild : s.state === filter));
  }), [servers, query, filter]);

  return (
    <RequireAuth><AppShell>
      <div className="page-heading">
        <div><h1 className="page-title">Servers</h1><p className="page-description">Manage your Minecraft server instances.</p></div>
        <CreateServerDialog onCreated={() => void load()} onError={setError} />
      </div>
      {error && <div role="alert" className="mb-6 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive"><p>{error}</p><Button variant="outline" size="sm" onClick={() => void load()} disabled={refreshing}>Try again</Button></div>}
      {servers !== null && <dl className="fleet-summary" aria-label="Fleet summary">
        <div className="flex items-center gap-2"><ServerIcon className="h-4 w-4" /><dt className="sr-only">Total servers</dt><dd><strong>{servers.length}</strong> {servers.length === 1 ? 'server' : 'servers'}</dd></div>
        <div className="flex items-center gap-2"><span className="h-1.5 w-1.5 rounded-full bg-primary" /><dt className="sr-only">Running</dt><dd><strong>{running}</strong> running</dd></div>
        <div><dt className="sr-only">Allocated memory</dt><dd><strong>{(allocatedRam / 1024).toLocaleString(undefined, { maximumFractionDigits: 1 })} GB</strong> allocated RAM</dd></div>
        {attention > 0 && <div><dt className="sr-only">Needs attention</dt><dd className="text-amber-800 dark:text-amber-300"><strong>{attention}</strong> need attention</dd></div>}
      </dl>}
      <HostCapacity servers={servers} />
      <div className="overflow-hidden rounded-lg border bg-card">
        <div className="flex flex-wrap items-center justify-between gap-4 border-b p-4 sm:px-6 sm:py-5">
          <div className="relative w-full sm:max-w-xs"><Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" /><Input aria-label="Search servers" placeholder="Search servers…" value={query} onChange={(e) => setQuery(e.target.value)} className="pl-9 pr-9" />{query && <button type="button" aria-label="Clear server search" className="absolute right-1 top-1 flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted" onClick={() => setQuery('')}><X className="h-4 w-4" /></button>}</div>
          <div className="flex w-full items-center justify-between gap-2 sm:w-auto">
            <div className="flex flex-wrap gap-1" aria-label="Filter servers">{([{ id: 'all', label: 'All' }, { id: 'running', label: 'Running' }, { id: 'stopped', label: 'Stopped' }, { id: 'attention', label: 'Attention' }] as const).map((item) => <button type="button" key={item.id} aria-pressed={filter === item.id} onClick={() => setFilter(item.id)} className={`rounded-md px-3 py-2 text-xs font-medium transition-colors ${filter === item.id ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-muted hover:text-foreground'}`}>{item.label}</button>)}</div>
            <Button variant="ghost" size="icon" aria-label="Refresh servers" onClick={() => void load()} disabled={refreshing}><RefreshCw className={refreshing ? 'animate-spin' : ''} /></Button>
          </div>
        </div>
        {servers === null ? <div className="space-y-4 p-6" aria-label="Loading servers"><Skeleton className="h-16 w-full" /><Skeleton className="h-16 w-full" /><Skeleton className="h-16 w-full" /></div> : servers.length === 0 ?
          <div className="empty-state"><Box /><h2 className="section-heading">No servers yet</h2><p>Create a server to get started.</p><div className="mt-6"><CreateServerDialog onCreated={() => void load()} onError={setError} /></div></div> : visible.length === 0 ?
          <div className="empty-state"><Search /><h2 className="section-heading">No matching servers</h2><p>Try another name, version, or server state.</p><Button variant="outline" className="mt-5" onClick={() => { setQuery(''); setFilter('all'); }}>Reset filters</Button></div> : <>
          <div className="fleet-row fleet-heading" aria-hidden="true"><span>Server</span><span>Status</span><span>Memory</span><span>Host port</span><span className="fleet-actions-heading text-right">Actions</span></div>
          <ul aria-label="Servers">{visible.map((server) => <li key={server.id} className="fleet-row group">
            <div className="flex min-w-0 items-center gap-4"><div className="hidden h-11 w-11 shrink-0 items-center justify-center rounded-lg border bg-secondary/40 sm:flex"><Box className="h-5 w-5 text-primary" strokeWidth={1.6} /></div><div className="min-w-0"><Link to="/servers/$id" params={{ id: server.id }} className="block truncate text-sm font-semibold hover:text-primary hover:underline">{server.name}</Link><p className="mt-1.5 truncate text-xs text-muted-foreground"><span className="capitalize">{server.server_type}</span> <span className="mx-1.5 opacity-50">/</span> {server.version}{server.build ? ` · Build ${server.build}` : ''}</p>{server.state === 'running' && (server.started_at || server.uptime_seconds != null) && <p data-testid="dashboard-server-uptime" className="mt-2 flex items-center gap-1.5 text-xs text-muted-foreground"><Clock className="h-3 w-3" />{formatUptimeFromStartedAt(server.started_at, server.uptime_seconds)} uptime</p>}{server.needs_rebuild && <p data-testid="dashboard-rebuild-badge" className="mt-2 text-xs text-amber-800 dark:text-amber-300">Rebuild required</p>}{['installing', 'building'].includes(server.state) && <div className="mt-3"><ServerInstallProgress serverId={server.id} state={server.state} compact /></div>}</div></div>
            <div><StatusBadge state={server.state} /></div>
            <div className="fleet-secondary"><p className="text-sm font-medium tabular-nums">{(server.ram_mb / 1024).toLocaleString(undefined, { maximumFractionDigits: 1 })} GB</p><p className="mt-1 text-xs text-muted-foreground">Allocated</p></div>
            <div className="fleet-secondary text-sm tabular-nums">{server.host_port > 0 ? server.host_port : <span className="text-xs text-muted-foreground">Behind proxy</span>}</div>
            <ServerQuickActions server={server} onCopy={() => setCopyTargetServer(server)} onUpdated={(updated) => setServers((current) => current?.map((item) => item.id === updated.id ? updated : item) ?? null)} />
          </li>)}</ul>
        </>}
        {servers !== null && servers.length > 0 && <div aria-live="polite" className="border-t px-6 py-3.5 text-xs text-muted-foreground">{visible.length} of {servers.length} {servers.length === 1 ? 'server' : 'servers'}</div>}
      </div>
      {copyTargetServer && <CopyServerDialog server={copyTargetServer} open onClose={() => setCopyTargetServer(null)} onCopied={() => void load()} />}
    </AppShell></RequireAuth>
  );
}
