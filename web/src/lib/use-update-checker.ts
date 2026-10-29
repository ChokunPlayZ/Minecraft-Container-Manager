import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { ServerUpdateCheck, UpdateCheckerStatus } from '../api/types';
import { UPDATE_DISCOVERED_EVENT } from './update-discovery';

export function hasAvailableUpdate(result?: ServerUpdateCheck): boolean {
  return !!(result?.software?.available || Object.keys(result?.mods?.updates ?? {}).length);
}

export function useUpdateChecker() {
  const [status, setStatus] = useState<UpdateCheckerStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const busy = useRef(false);
  const reloadQueued = useRef(false);
  const mounted = useRef(false);
  const reload = useCallback(async function reloadStatus() {
    if (busy.current) { reloadQueued.current = true; return; }
    reloadQueued.current = false;
    busy.current = true;
    try {
      const next = await api.getUpdateChecker();
      if (mounted.current) { setStatus(next); setError(null); }
    } catch (err) {
      if (mounted.current) setError(err instanceof Error ? err.message : 'Could not load update checks.');
    } finally {
      busy.current = false;
      if (reloadQueued.current && mounted.current) void reloadStatus();
    }
  }, []);
  useEffect(() => {
    mounted.current = true;
    void reload();
    return () => { mounted.current = false; };
  }, [reload]);
  useEffect(() => {
    const timer = setInterval(() => { if (!document.hidden) void reload(); }, status?.running ? 3000 : 60000);
    const refresh = () => { void reload(); };
    window.addEventListener('focus', refresh);
    window.addEventListener(UPDATE_DISCOVERED_EVENT, refresh);
    return () => { clearInterval(timer); window.removeEventListener('focus', refresh); window.removeEventListener(UPDATE_DISCOVERED_EVENT, refresh); };
  }, [reload, status?.running]);
  return { status, setStatus, error, reload };
}
