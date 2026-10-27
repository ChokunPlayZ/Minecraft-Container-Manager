import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { ServerUpdateCheck, UpdateCheckerStatus } from '../api/types';

export function hasAvailableUpdate(result?: ServerUpdateCheck): boolean {
  return !!(result?.software?.available || Object.keys(result?.mods?.updates ?? {}).length);
}

export function useUpdateChecker() {
  const [status, setStatus] = useState<UpdateCheckerStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const busy = useRef(false);
  const mounted = useRef(false);
  const reload = useCallback(async () => {
    if (busy.current) return;
    busy.current = true;
    try {
      const next = await api.getUpdateChecker();
      if (mounted.current) { setStatus(next); setError(null); }
    } catch (err) {
      if (mounted.current) setError(err instanceof Error ? err.message : 'Could not load update checks.');
    } finally { busy.current = false; }
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
    return () => { clearInterval(timer); window.removeEventListener('focus', refresh); };
  }, [reload, status?.running]);
  return { status, setStatus, error, reload };
}
