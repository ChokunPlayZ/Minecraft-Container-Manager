import { Cpu, Layers, RefreshCw } from 'lucide-react';
import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { HostCapacity as Capacity, Server } from '../api/types';
import { formatBytes } from '../lib/formatters';
import { Button } from './ui/button';
import { Skeleton } from './ui/skeleton';

const formatCores = (value: number) => value.toLocaleString(undefined, { maximumFractionDigits: 2 });

function CapacityMeter({ label, configured, total, format }: {
  label: string;
  configured: number;
  total: number;
  format: (value: number) => string;
}) {
  const percent = Math.min(100, configured / total * 100);
  return <div role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={100}
    aria-valuenow={percent} aria-valuetext={`${format(configured)} configured out of ${format(total)} capacity`}
    className="mt-3 h-1 overflow-hidden rounded-full bg-muted">
    <div className={`h-full rounded-full ${configured > total ? 'bg-destructive' : 'bg-primary'}`} style={{ width: `${percent}%` }} />
  </div>;
}

export function HostCapacity({ servers }: { servers: Server[] | null }) {
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
    const timer = setInterval(() => void load(), 30_000);
    return () => { active = false; clearInterval(timer); };
  }, [revision]);

  const cpuLimit = servers?.reduce((sum, server) => sum + (server.cpu_limit || 0), 0) ?? 0;
  const uncapped = servers?.filter((server) => !server.cpu_limit).length ?? 0;
  // Match the runtime's default: Java heap plus 2 GiB unless an explicit cap is set.
  const memoryLimit = servers?.reduce((sum, server) => sum + (server.memory_limit_mb > 0 ? server.memory_limit_mb : server.ram_mb + 2048) * 1024 ** 2, 0) ?? 0;
  const cpuTotal = capacity?.cpu_cores ?? 0;
  const memoryTotal = capacity?.memory_total_bytes ?? 0;

  return <section aria-labelledby="host-capacity-title" className="mb-6 overflow-hidden rounded-lg border bg-card">
    <div className="flex items-center justify-between gap-3 border-b px-5 py-3 sm:px-6">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2 id="host-capacity-title" className="text-sm font-semibold">Host capacity</h2>
        {capacity && <span className="text-xs text-muted-foreground">{capacity.runtime === 'podman' ? 'Podman' : 'Docker'} host</span>}
      </div>
      <Button variant="ghost" size="icon" aria-label="Refresh host capacity" disabled={refreshing} onClick={() => setRevision((value) => value + 1)}>
        <RefreshCw className={refreshing ? 'animate-spin' : ''} />
      </Button>
    </div>
    {error ? <div role="status" className="px-5 py-4 text-sm text-muted-foreground sm:px-6">Host capacity is unavailable. Check the container runtime and refresh.</div> : !capacity ?
      <div aria-label="Loading host capacity" className="grid gap-5 p-5 sm:grid-cols-2 sm:px-6"><Skeleton className="h-20" /><Skeleton className="h-20" /></div> : <>
        <dl className="grid divide-y sm:grid-cols-2 sm:divide-x sm:divide-y-0">
          <div className="min-w-0 px-5 py-4 sm:px-6">
            <dt className="flex items-center gap-2 text-xs text-muted-foreground"><Cpu className="h-4 w-4" aria-hidden="true" />CPU capacity</dt>
            <dd className="mt-2 text-lg font-semibold tabular-nums">{cpuTotal > 0 ? `${formatCores(cpuTotal)} ${cpuTotal === 1 ? 'core' : 'cores'}` : 'Not reported'}</dd>
            {servers !== null && <>
              <p className="mt-1 text-xs text-muted-foreground">{formatCores(cpuLimit)} cores in configured limits{uncapped > 0 ? ` · ${uncapped} ${uncapped === 1 ? 'server' : 'servers'} uncapped` : ''}</p>
              {cpuTotal > 0 && <CapacityMeter label="Configured CPU limits" configured={cpuLimit} total={cpuTotal} format={(value) => `${formatCores(value)} cores`} />}
              {cpuTotal > 0 && cpuLimit > cpuTotal && <p className="mt-2 text-xs text-destructive">{formatCores(cpuLimit - cpuTotal)} cores over host capacity</p>}
            </>}
          </div>
          <div className="min-w-0 px-5 py-4 sm:px-6">
            <dt className="flex items-center gap-2 text-xs text-muted-foreground"><Layers className="h-4 w-4" aria-hidden="true" />Memory capacity</dt>
            <dd className="mt-2 text-lg font-semibold tabular-nums">{memoryTotal > 0 ? formatBytes(memoryTotal) : 'Not reported'}</dd>
            {servers !== null && <>
              <p className="mt-1 text-xs text-muted-foreground">{formatBytes(memoryLimit)} in configured limits</p>
              {memoryTotal > 0 && <CapacityMeter label="Configured memory limits" configured={memoryLimit} total={memoryTotal} format={formatBytes} />}
              {memoryTotal > 0 && memoryLimit > memoryTotal && <p className="mt-2 text-xs text-destructive">{formatBytes(memoryLimit - memoryTotal)} over host capacity</p>}
            </>}
          </div>
        </dl>
        <p className="border-t px-5 py-3 text-xs leading-5 text-muted-foreground sm:px-6">Configured limits include stopped servers and default memory overhead. Actual usage varies.</p>
      </>}
  </section>;
}
