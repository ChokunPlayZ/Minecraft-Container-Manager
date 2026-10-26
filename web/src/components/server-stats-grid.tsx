import { useCallback, useEffect, useState } from 'react';
import { ArrowDown, ArrowUp, Cpu, HardDrive, Layers, Network } from 'lucide-react';
import { api } from '../api/client';
import type { ServerStats } from '../api/types';
import { formatBytes } from '../lib/formatters';

export function ServerStatsGrid({ serverId, isRunning }: { serverId: string; isRunning: boolean }) {
  const [stats, setStats] = useState<ServerStats | null>(null);
  const fetchStats = useCallback(async () => {
    try { setStats(await api.serverStats(serverId)); } catch { /* Retain the last reading during reconnection. */ }
  }, [serverId]);
  useEffect(() => {
    void fetchStats();
    if (!isRunning) return;
    const interval = setInterval(() => void fetchStats(), 2500);
    return () => clearInterval(interval);
  }, [isRunning, fetchStats]);
  const online = isRunning && (stats?.online ?? false);
  const cpu = online ? stats?.cpu_percent ?? 0 : 0;
  const memory = online ? stats?.memory_bytes ?? 0 : 0;
  const memoryLimit = stats?.memory_limit_bytes ?? 0;
  const memoryPercent = online && memoryLimit > 0 ? stats?.memory_percent ?? 0 : 0;
  const disk = stats?.disk_bytes ?? 0;
  const stateLabel = online ? 'Live' : 'Offline';
  function meter(value: number, label: string) {
    return <div role="meter" aria-label={label} aria-valuenow={Math.round(value)} aria-valuemin={0} aria-valuemax={100} className="mt-3 h-1 overflow-hidden rounded-full bg-muted"><div className={`h-full rounded-full transition-[width] duration-300 ${value > 90 ? 'bg-destructive' : 'bg-primary'}`} style={{ width: `${Math.min(Math.max(value, 0), 100)}%` }} /></div>;
  }
  return (
    <div data-testid="server-stats-grid" className="server-metrics">
      <div data-testid="stat-cpu" className="server-metric">
        <div className="flex items-center justify-between text-xs text-muted-foreground"><span>CPU usage</span><Cpu className="h-4 w-4" strokeWidth={1.5} /></div>
        <div className="mt-3 flex flex-wrap items-baseline gap-x-2"><span data-testid="stat-cpu-value" className="metric-value">{cpu.toFixed(1)}%</span>{online && (stats?.cpu_limit ?? 0) > 0 && <span className="text-xs text-muted-foreground">/ {stats?.cpu_limit} cores</span>}</div>
        {meter(cpu, 'CPU usage')}
        <div className="mt-2 flex items-center justify-between gap-2 text-xs text-muted-foreground"><span>{stats?.cpu_cores ? `${stats.cpu_cores} cores detected` : 'Container CPU'}</span><span>{stateLabel}</span></div>
      </div>
      <div data-testid="stat-mem" className="server-metric">
        <div className="flex items-center justify-between text-xs text-muted-foreground"><span>Memory</span><Layers className="h-4 w-4" strokeWidth={1.5} /></div>
        <div className="mt-3 flex flex-wrap items-baseline gap-x-2"><span data-testid="stat-mem-value" className="metric-value">{formatBytes(memory)}</span>{memoryLimit > 0 && <span className="text-xs text-muted-foreground">/ {formatBytes(memoryLimit)}</span>}</div>
        {meter(memoryPercent, 'Memory usage')}
        <div className="mt-2 flex items-center justify-between gap-2 text-xs text-muted-foreground"><span>{online ? `${memoryPercent.toFixed(1)}% used` : 'Allocated memory'}</span><span>{stateLabel}</span></div>
      </div>
      <div data-testid="stat-disk" className="server-metric">
        <div className="flex items-center justify-between text-xs text-muted-foreground"><span>Storage</span><HardDrive className="h-4 w-4" strokeWidth={1.5} /></div>
        <div className="mt-3"><span data-testid="stat-disk-value" className="metric-value">{formatBytes(disk)}</span><span className="ml-2 text-xs text-muted-foreground">used</span></div>
        <p className="mt-4 text-xs leading-5 text-muted-foreground">{online && (stats?.disk_read_bytes || stats?.disk_write_bytes) ? `Read ${formatBytes(stats?.disk_read_bytes ?? 0)} · Written ${formatBytes(stats?.disk_write_bytes ?? 0)}` : 'Server data directory'}</p>
      </div>
      <div data-testid="stat-network" className="server-metric">
        <div className="flex items-center justify-between text-xs text-muted-foreground"><span>Network transfer</span><Network className="h-4 w-4" strokeWidth={1.5} /></div>
        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-base font-semibold tabular-nums"><span className="inline-flex items-center gap-1.5" title="Received"><ArrowDown className="h-3.5 w-3.5 text-primary" /><span data-testid="stat-network-rx">{formatBytes(online ? stats?.net_rx_bytes ?? 0 : 0)}</span></span><span className="inline-flex items-center gap-1.5" title="Sent"><ArrowUp className="h-3.5 w-3.5 text-muted-foreground" /><span data-testid="stat-network-tx">{formatBytes(online ? stats?.net_tx_bytes ?? 0 : 0)}</span></span></div>
        <div className="mt-4 flex items-center justify-between gap-2 text-xs text-muted-foreground"><span>Total received / sent</span><span>{stateLabel}</span></div>
      </div>
    </div>
  );
}
