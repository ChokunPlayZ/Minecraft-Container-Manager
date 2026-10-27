import { ArrowDown, ArrowUp, Cpu, HardDrive, Layers, Network, RefreshCw } from 'lucide-react';
import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { HostCapacity as Capacity } from '../api/types';
import { formatBytes } from '../lib/formatters';
import { Button } from './ui/button';
import { Skeleton } from './ui/skeleton';

function UsageMeter({ value, label }: { value: number | null; label: string }) {
  if (value === null) return <div className="mt-3 h-1 rounded-full bg-muted" />;
  const percent = Math.min(100, Math.max(0, value));
  return <div role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}
    className="mt-3 h-1 overflow-hidden rounded-full bg-muted">
    <div className={`h-full rounded-full transition-[width] duration-300 ${percent > 90 ? 'bg-destructive' : 'bg-primary'}`} style={{ width: `${percent}%` }} />
  </div>;
}

export function HostCapacity() {
  const [capacity, setCapacity] = useState<Capacity | null>(null);
  const [error, setError] = useState(false);
  const [refreshing, setRefreshing] = useState(true);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    let active = true;
    let pending = false;
    async function load() {
      if (pending) return;
      pending = true;
      setRefreshing(true);
      try {
        const result = await api.hostCapacity();
        if (active) { setCapacity(result); setError(false); }
      } catch {
        if (active) setError(true);
      } finally {
        pending = false;
        if (active) setRefreshing(false);
      }
    }
    void load();
    const timer = setInterval(() => void load(), 5000);
    return () => { active = false; clearInterval(timer); };
  }, [revision]);

  // Failed refreshes must not leave old readings labeled Live.
  const usage = error ? null : capacity?.usage;
  const disk = error ? null : capacity?.disk;
  const cpu = usage?.cpu_percent ?? null;
  const memoryPercent = usage && capacity && capacity.memory_total_bytes > 0 ? usage.memory_bytes / capacity.memory_total_bytes * 100 : null;
  const diskPercent = disk && disk.total_bytes > 0 ? disk.used_bytes / disk.total_bytes * 100 : null;
  const usageState = usage ? 'Live' : 'Unavailable';
  const cpuTotal = capacity?.cpu_cores ?? 0;
  const memoryTotal = capacity?.memory_total_bytes ?? 0;

  return <section aria-labelledby="host-capacity-title" className="mb-6">
    <div className="mb-3 flex items-center justify-between gap-3">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2 id="host-capacity-title" className="text-sm font-semibold">Host resources</h2>
        {capacity?.runtime && <span className="text-xs text-muted-foreground">{capacity.runtime === 'podman' ? 'Podman' : 'Docker'} host</span>}
      </div>
      <Button variant="ghost" size="icon" aria-label="Refresh host resources" disabled={refreshing} onClick={() => setRevision((value) => value + 1)}>
        <RefreshCw className={refreshing ? 'animate-spin' : ''} />
      </Button>
    </div>
    {!capacity && !error ? <div aria-label="Loading host resources" className="server-metrics !mb-0">
      {Array.from({ length: 4 }, (_, i) => <div key={i} className="server-metric"><Skeleton className="h-24" /></div>)}
    </div> : <div className="server-metrics !mb-0" data-testid="host-resources-grid">
      <div className="server-metric" data-testid="host-cpu">
        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground"><span>Server CPU usage</span><Cpu className="h-4 w-4 shrink-0" strokeWidth={1.5} /></div>
        <div className="mt-3"><span className="metric-value">{cpu === null ? '—' : `${cpu.toFixed(1)}%`}</span></div>
        <UsageMeter value={cpu} label="Server CPU usage" />
        <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground"><span>{cpuTotal > 0 ? `${cpuTotal} cores total` : 'Capacity not reported'}</span><span>{cpu === null ? 'Unavailable' : 'Live'}</span></div>
      </div>
      <div className="server-metric" data-testid="host-memory">
        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground"><span>Server memory</span><Layers className="h-4 w-4 shrink-0" strokeWidth={1.5} /></div>
        <div className="mt-3 flex flex-wrap items-baseline gap-x-2"><span className="metric-value">{usage ? formatBytes(usage.memory_bytes) : '—'}</span>{memoryTotal > 0 && <span className="text-xs text-muted-foreground">/ {formatBytes(memoryTotal)}</span>}</div>
        <UsageMeter value={memoryPercent} label="Server memory usage" />
        <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground"><span>{memoryPercent === null ? 'Host memory' : `${memoryPercent.toFixed(1)}% of host`}</span><span>{usageState}</span></div>
      </div>
      <div className="server-metric" data-testid="host-disk">
        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground"><span>Data disk</span><HardDrive className="h-4 w-4 shrink-0" strokeWidth={1.5} /></div>
        <div className="mt-3 flex flex-wrap items-baseline gap-x-2"><span className="metric-value">{disk ? formatBytes(disk.available_bytes) : '—'}</span><span className="text-xs text-muted-foreground">free</span></div>
        <UsageMeter value={diskPercent} label="Data disk usage" />
        <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground"><span>{disk ? `${formatBytes(disk.used_bytes)} / ${formatBytes(disk.total_bytes)} used` : 'Disk space unavailable'}</span><span>{disk ? 'Live' : 'Unavailable'}</span></div>
      </div>
      <div className="server-metric" data-testid="host-network">
        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground"><span>Network transfer</span><Network className="h-4 w-4 shrink-0" strokeWidth={1.5} /></div>
        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-base font-semibold tabular-nums">
          <span className="inline-flex items-center gap-1.5" aria-label="Received"><ArrowDown className="h-3.5 w-3.5 text-primary" /><span>{usage ? formatBytes(usage.net_rx_bytes) : '—'}</span></span>
          <span className="inline-flex items-center gap-1.5" aria-label="Sent"><ArrowUp className="h-3.5 w-3.5 text-muted-foreground" /><span>{usage ? formatBytes(usage.net_tx_bytes) : '—'}</span></span>
        </div>
        <div className="mt-4 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground"><span>Total received / sent</span><span>{usageState}</span></div>
      </div>
    </div>}
    {(error || capacity?.runtime_error || capacity?.usage_error || capacity?.disk_error) && <p role="status" className="mt-3 text-xs leading-5 text-destructive">
      {error ? 'Host resources could not be refreshed. Refresh to try again.' : [capacity?.runtime_error, capacity?.usage_error, capacity?.disk_error].filter(Boolean).join(' ')}
    </p>}
    <p className="mt-3 text-xs leading-5 text-muted-foreground">CPU, memory, and network show managed server usage. Disk space is for the filesystem containing {disk?.path ? <code className="break-all">{disk.path}</code> : 'the data directory'}.</p>
  </section>;
}
