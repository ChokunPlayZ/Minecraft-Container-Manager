import { useCallback, useEffect, useState } from 'react';
import { ArrowDown, ArrowUp, Cpu, HardDrive, Layers, Network } from 'lucide-react';
import { api } from '../api/client';
import type { ServerStats } from '../api/types';
import { formatBytes } from '../lib/formatters';

interface ServerStatsGridProps {
  serverId: string;
  isRunning: boolean;
}

export function ServerStatsGrid({ serverId, isRunning }: ServerStatsGridProps) {
  const [stats, setStats] = useState<ServerStats | null>(null);

  const fetchStats = useCallback(async () => {
    try {
      const data = await api.serverStats(serverId);
      setStats(data);
    } catch {
      // Keep last known stats on transient error
    }
  }, [serverId]);

  useEffect(() => {
    void fetchStats();
    if (!isRunning) return;

    const interval = setInterval(() => {
      void fetchStats();
    }, 2500);

    return () => clearInterval(interval);
  }, [isRunning, fetchStats]);

  const isOnline = isRunning && (stats?.online ?? false);
  const cpuPercent = isOnline ? (stats?.cpu_percent ?? 0) : 0;
  const memoryBytes = isOnline ? (stats?.memory_bytes ?? 0) : 0;
  const memoryLimit = stats?.memory_limit_bytes ?? 0;
  const memoryPercent = isOnline && memoryLimit > 0 ? (stats?.memory_percent ?? 0) : 0;
  const diskBytes = stats?.disk_bytes ?? 0;
  const netRx = isOnline ? (stats?.net_rx_bytes ?? 0) : 0;
  const netTx = isOnline ? (stats?.net_tx_bytes ?? 0) : 0;

  // CPU progress bar width (clamped to 100 for bar display)
  const cpuBarWidth = Math.min(Math.max(cpuPercent, 0), 100);
  const memBarWidth = Math.min(Math.max(memoryPercent, 0), 100);

  return (
    <div
      data-testid="server-stats-grid"
      className="mb-6 grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4"
    >
      {/* 1. CPU Card */}
      <div
        data-testid="stat-cpu"
        className="group relative overflow-hidden rounded-xl border bg-card/90 p-4 shadow-2xs backdrop-blur-xs transition-all hover:border-cyan-500/40 hover:shadow-xs"
      >
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">CPU</span>
          <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-cyan-500/10 text-cyan-600 transition-colors group-hover:bg-cyan-500/20 dark:text-cyan-400">
            <Cpu className="h-4 w-4" />
          </div>
        </div>

        <div className="mt-3 flex items-baseline gap-2">
          <span
            data-testid="stat-cpu-value"
            className="font-mono text-2xl font-bold tracking-tight text-foreground"
          >
            {isOnline ? `${cpuPercent.toFixed(1)}%` : '0.0%'}
          </span>
          {isOnline && stats?.cpu_limit && stats.cpu_limit > 0 ? (
            <span className="text-xs text-muted-foreground">
              / {stats.cpu_limit} core{stats.cpu_limit > 1 ? 's' : ''}
            </span>
          ) : null}
        </div>

        {/* Progress Bar */}
        <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-muted/70">
          <div
            className={`h-full rounded-full transition-all duration-500 ease-out ${
              !isOnline
                ? 'w-0'
                : cpuPercent > 85
                  ? 'bg-rose-500'
                  : 'bg-gradient-to-r from-cyan-500 to-blue-500'
            }`}
            style={{ width: `${cpuBarWidth}%` }}
          />
        </div>

        <div className="mt-2 flex items-center justify-between text-[11px] text-muted-foreground">
          <span>{isOnline && stats?.cpu_cores ? `${stats.cpu_cores} cores detected` : isOnline ? 'Active' : 'Offline'}</span>
          {isOnline && (
            <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400 font-medium">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
              Live
            </span>
          )}
        </div>
      </div>

      {/* 2. Memory (MEM) Card */}
      <div
        data-testid="stat-mem"
        className="group relative overflow-hidden rounded-xl border bg-card/90 p-4 shadow-2xs backdrop-blur-xs transition-all hover:border-violet-500/40 hover:shadow-xs"
      >
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">MEM</span>
          <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-violet-500/10 text-violet-600 transition-colors group-hover:bg-violet-500/20 dark:text-violet-400">
            <Layers className="h-4 w-4" />
          </div>
        </div>

        <div className="mt-3 flex items-baseline gap-1.5">
          <span
            data-testid="stat-mem-value"
            className="font-mono text-2xl font-bold tracking-tight text-foreground"
          >
            {isOnline ? formatBytes(memoryBytes) : '0 B'}
          </span>
          {memoryLimit > 0 && (
            <span className="text-xs text-muted-foreground">
              / {formatBytes(memoryLimit)}
            </span>
          )}
        </div>

        {/* Progress Bar */}
        <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-muted/70">
          <div
            className={`h-full rounded-full transition-all duration-500 ease-out ${
              !isOnline
                ? 'w-0'
                : memoryPercent > 90
                  ? 'bg-rose-500'
                  : 'bg-gradient-to-r from-violet-500 to-purple-500'
            }`}
            style={{ width: `${memBarWidth}%` }}
          />
        </div>

        <div className="mt-2 flex items-center justify-between text-[11px] text-muted-foreground">
          <span>{isOnline && memoryLimit > 0 ? `${memoryPercent.toFixed(1)}% used` : isOnline ? 'Active' : 'Offline'}</span>
          {isOnline && (
            <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400 font-medium">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
              Live
            </span>
          )}
        </div>
      </div>

      {/* 3. Disk Card */}
      <div
        data-testid="stat-disk"
        className="group relative overflow-hidden rounded-xl border bg-card/90 p-4 shadow-2xs backdrop-blur-xs transition-all hover:border-amber-500/40 hover:shadow-xs"
      >
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">Disk</span>
          <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-amber-500/10 text-amber-600 transition-colors group-hover:bg-amber-500/20 dark:text-amber-400">
            <HardDrive className="h-4 w-4" />
          </div>
        </div>

        <div className="mt-3 flex items-baseline gap-1.5">
          <span
            data-testid="stat-disk-value"
            className="font-mono text-2xl font-bold tracking-tight text-foreground"
          >
            {formatBytes(diskBytes)}
          </span>
          <span className="text-xs text-muted-foreground">used</span>
        </div>

        {/* Disk I/O or Storage indicator */}
        <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-muted/70">
          <div
            className="h-full rounded-full bg-gradient-to-r from-amber-500 to-orange-500 transition-all duration-500 ease-out"
            style={{ width: diskBytes > 0 ? '100%' : '0%' }}
          />
        </div>

        <div className="mt-2 flex items-center justify-between text-[11px] text-muted-foreground">
          {isOnline && (stats?.disk_read_bytes || stats?.disk_write_bytes) ? (
            <span title={`Read: ${formatBytes(stats?.disk_read_bytes ?? 0)} · Written: ${formatBytes(stats?.disk_write_bytes ?? 0)}`}>
              I/O: R {formatBytes(stats?.disk_read_bytes ?? 0)} / W {formatBytes(stats?.disk_write_bytes ?? 0)}
            </span>
          ) : (
            <span>Data directory</span>
          )}
          <span className="text-[10px] uppercase font-semibold text-muted-foreground/80">/data</span>
        </div>
      </div>

      {/* 4. Network Card */}
      <div
        data-testid="stat-network"
        className="group relative overflow-hidden rounded-xl border bg-card/90 p-4 shadow-2xs backdrop-blur-xs transition-all hover:border-emerald-500/40 hover:shadow-xs"
      >
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">Network</span>
          <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-600 transition-colors group-hover:bg-emerald-500/20 dark:text-emerald-400">
            <Network className="h-4 w-4" />
          </div>
        </div>

        <div className="mt-3 flex items-center gap-3">
          <div className="flex items-center gap-1 font-mono text-sm font-semibold text-foreground">
            <ArrowDown className="h-3.5 w-3.5 text-emerald-500" />
            <span data-testid="stat-network-rx">{formatBytes(netRx)}</span>
          </div>
          <div className="flex items-center gap-1 font-mono text-sm font-semibold text-foreground">
            <ArrowUp className="h-3.5 w-3.5 text-blue-500" />
            <span data-testid="stat-network-tx">{formatBytes(netTx)}</span>
          </div>
        </div>

        {/* Network indicator bar */}
        <div className="mt-3.5 h-1.5 w-full overflow-hidden rounded-full bg-muted/70">
          <div
            className={`h-full rounded-full transition-all duration-500 ease-out ${
              isOnline
                ? 'w-full bg-gradient-to-r from-emerald-500 to-teal-500'
                : 'w-0'
            }`}
          />
        </div>

        <div className="mt-2 flex items-center justify-between text-[11px] text-muted-foreground">
          <span>{isOnline ? 'Total transfer' : 'Offline'}</span>
          {isOnline && (
            <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400 font-medium">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
              Live
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
