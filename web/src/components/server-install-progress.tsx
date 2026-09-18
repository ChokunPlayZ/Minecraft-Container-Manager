import React, { useEffect, useState } from 'react';
import { Download, Loader2, Package, Sparkles } from 'lucide-react';
import { api } from '../api/client';
import type { ServerState, TaskProgress } from '../api/types';
import { ProgressBar } from './ui/progress';

interface ServerInstallProgressProps {
  serverId: string;
  state: ServerState;
  compact?: boolean;
  className?: string;
}

export function ServerInstallProgress({
  serverId,
  state,
  compact = false,
  className = '',
}: ServerInstallProgressProps) {
  const [progress, setProgress] = useState<TaskProgress | null>(null);

  useEffect(() => {
    let cancelled = false;

    // Fetch initial progress if already underway
    api
      .getTaskProgress(serverId)
      .then((res) => {
        if (!cancelled && res.active && res.progress) {
          setProgress(res.progress);
        }
      })
      .catch(() => {});

    // Stream real-time progress via SSE
    const unsub = api.subscribeTaskEvents(serverId, (p) => {
      if (!cancelled) {
        setProgress(p);
      }
    });

    return () => {
      cancelled = true;
      unsub();
    };
  }, [serverId]);

  const isBuilding = state === 'building';
  const defaultTitle = isBuilding ? 'Building Server from Source' : 'Installing Server';
  const title = progress?.stage_title || defaultTitle;
  const percent = progress?.percent != null && progress.percent >= 0 ? progress.percent : null;

  // Format bytes details if available
  let byteStats = '';
  if (progress?.bytes_done && progress?.bytes_total && progress.bytes_total > 0) {
    const doneMB = (progress.bytes_done / (1024 * 1024)).toFixed(1);
    const totalMB = (progress.bytes_total / (1024 * 1024)).toFixed(1);
    byteStats = `${doneMB} MB / ${totalMB} MB`;
  }

  const detailMessage = progress?.message || (isBuilding ? 'Compiling code via BuildTools...' : 'Setting up server container...');

  if (compact) {
    return (
      <div
        data-testid="server-install-progress-compact"
        className={`mt-3 rounded-lg border border-blue-500/20 bg-blue-500/5 p-2.5 text-xs ${className}`}
      >
        <div className="mb-1.5 flex items-center justify-between gap-1.5">
          <div className="flex items-center gap-1.5 min-w-0 font-medium text-blue-700 dark:text-blue-300 truncate">
            <Loader2 className="h-3 w-3 animate-spin shrink-0 text-blue-500" />
            <span className="truncate">{title}</span>
          </div>
          {percent !== null && (
            <span className="shrink-0 font-mono font-semibold text-blue-700 dark:text-blue-300">
              {percent}%
            </span>
          )}
        </div>

        <ProgressBar
          value={percent}
          size="xs"
          variant="sky"
          showPercent={false}
          className="mb-1"
        />

        <div className="flex items-center justify-between gap-2 text-[11px] text-muted-foreground truncate">
          <span className="truncate">{detailMessage}</span>
          {byteStats && <span className="shrink-0 font-mono">{byteStats}</span>}
        </div>
      </div>
    );
  }

  return (
    <div
      data-testid="server-install-progress-hero"
      className={`mb-6 rounded-xl border border-blue-500/30 bg-blue-500/10 p-4 sm:p-5 shadow-xs transition-all ${className}`}
    >
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-3">
        <div className="flex items-center gap-2.5">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-blue-500/20 text-blue-600 dark:text-blue-400">
            {isBuilding ? (
              <Sparkles className="h-5 w-5 animate-pulse" />
            ) : progress?.operation === 'modpack_install' ? (
              <Package className="h-5 w-5 animate-bounce" />
            ) : (
              <Download className="h-5 w-5 animate-bounce" />
            )}
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-foreground sm:text-base">
                {title}
              </h3>
              {progress?.stage_index && progress?.stage_total ? (
                <span className="rounded-full bg-blue-500/20 px-2 py-0.5 text-[10px] font-semibold text-blue-700 dark:text-blue-300">
                  Stage {progress.stage_index} of {progress.stage_total}
                </span>
              ) : null}
            </div>
            <p className="text-xs text-muted-foreground mt-0.5">
              {isBuilding
                ? 'SpigotMC BuildTools is compiling Minecraft from source code.'
                : 'Server software and required assets are currently being downloaded and provisioned.'}
            </p>
          </div>
        </div>

        {percent !== null && (
          <div className="text-right shrink-0">
            <span className="font-mono text-xl font-bold text-blue-600 dark:text-blue-400">
              {percent}%
            </span>
          </div>
        )}
      </div>

      <ProgressBar
        value={percent}
        size="sm"
        variant="sky"
        showPercent={false}
        className="mb-2"
      />

      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground font-mono">
        <span className="truncate">{detailMessage}</span>
        {byteStats && <span className="font-semibold text-foreground">{byteStats}</span>}
      </div>
    </div>
  );
}
