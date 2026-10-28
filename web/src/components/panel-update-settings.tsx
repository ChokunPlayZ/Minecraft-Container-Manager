import { useEffect, useState } from 'react';
import { Download, RefreshCw } from 'lucide-react';
import { api } from '../api/client';
import type { PanelUpdateStatus } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { useModal } from './ui/modal';

function message(error: unknown) {
  return error instanceof Error ? error.message : 'Could not contact the panel. Try again.';
}

export function PanelUpdateSettings() {
  const [status, setStatus] = useState<PanelUpdateStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const { confirm, dialog } = useModal();
  const running = status?.job?.state === 'queued' || status?.job?.state === 'running';

  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      let keepPolling = running;
      try {
        const next = await api.getPanelUpdate();
        keepPolling = next.job?.state === 'queued' || next.job?.state === 'running';
        if (!disposed) { setStatus(next); setReconnecting(false); setError(null); }
      } catch (err) {
        if (!disposed) setError(message(err));
      }
      if (!disposed && keepPolling) timer = setTimeout(() => void poll(), 3000);
    }
    void poll();
    return () => { disposed = true; clearTimeout(timer); };
  }, [running]);

  // Preserve the last known progress while the panel restarts, and keep polling.
  useEffect(() => { setReconnecting(!!running && !!error); }, [running, error]);

  async function check() {
    setBusy(true); setError(null);
    try { setStatus(await api.checkPanelUpdate()); }
    catch (err) { setError(message(err)); }
    finally { setBusy(false); }
  }

  async function install() {
    if (!status?.latest_commit) return;
    const commit = status.latest_commit;
    const accepted = await confirm(
      `Update MCM to commit ${commit.slice(0, 12)}? The installer builds the app, backs up the panel database and settings, then restarts the panel. If readiness fails, it attempts to restore the previous version. Minecraft server containers keep running.`,
      { title: 'Update MCM panel', confirmLabel: 'Install update', destructive: false },
    );
    if (!accepted) return;
    setBusy(true); setError(null);
    try { setStatus(await api.startPanelUpdate(commit)); }
    catch (err) { setError(message(err)); }
    finally { setBusy(false); }
  }

  return <Card>
    <CardHeader>
      <CardTitle>Panel software</CardTitle>
      <CardDescription>View the build running MCM and update the management panel.</CardDescription>
    </CardHeader>
    <CardContent className="space-y-5">
      {!status && !error && <p role="status" className="text-sm text-muted-foreground">Loading panel build…</p>}
      {error && !reconnecting && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 text-sm text-destructive"><p className="min-w-0 break-words">{error}</p><Button variant="outline" size="sm" disabled={busy} onClick={() => void api.getPanelUpdate().then((next) => { setStatus(next); setError(null); }).catch((err: unknown) => setError(message(err)))}>Retry</Button></div>}
      {status && <>
        <dl className="space-y-3 text-sm">
          <div className="space-y-1"><dt className="text-muted-foreground">Build commit</dt><dd className="break-all font-mono select-all">{status.build.commit === 'unknown' ? 'Unavailable for this build' : status.build.commit}</dd></div>
          {status.build.built_at && <div className="flex flex-wrap justify-between gap-2"><dt className="text-muted-foreground">Built</dt><dd>{new Date(status.build.built_at).toLocaleString()}</dd></div>}
          {status.build.dirty && <div className="text-muted-foreground"><dt className="sr-only">Source changes</dt><dd>This build includes local source changes.</dd></div>}
          {status.latest_commit && <div className="space-y-1 border-t pt-3"><dt className="text-muted-foreground">Latest tracked commit</dt><dd className="break-all font-mono select-all">{status.latest_commit}</dd></div>}
        </dl>
        {!status.supported ? <p className="text-sm text-muted-foreground">{status.reason}</p> : <>
          <p className="text-sm text-muted-foreground">Updates follow the source checkout’s tracked branch and use the managed installer’s backup and rollback process. The panel briefly disconnects during restart.</p>
          {status.checked_at && <p role="status" className="text-sm text-muted-foreground">{status.available ? 'A panel update is available.' : 'This panel matches the tracked branch.'} Checked {new Date(status.checked_at).toLocaleString()}.</p>}
          <div className="flex flex-wrap gap-3">
            <Button variant="outline" disabled={busy || running} onClick={() => void check()}><RefreshCw className={busy ? 'animate-spin' : ''} />{busy ? 'Working…' : 'Check for panel updates'}</Button>
            {status.available && <Button disabled={busy || running} onClick={() => void install()}><Download />Install panel update</Button>}
          </div>
        </>}
        {status.job && <div className="space-y-3 border-t pt-4">
          <p role={status.job.state === 'failed' ? 'alert' : 'status'} className={`text-sm ${status.job.state === 'failed' ? 'text-destructive' : ''}`}>{reconnecting ? 'The panel is reconnecting. Update progress will resume automatically.' : status.job.message}</p>
          {running && <p className="flex items-center gap-2 text-sm text-muted-foreground"><RefreshCw className="h-4 w-4 animate-spin" />Update in progress</p>}
          {status.job.state === 'succeeded' && <Button variant="outline" onClick={() => window.location.reload()}>Reload updated panel</Button>}
          {status.log && <details><summary className="cursor-pointer text-sm font-medium">Update log</summary><pre role="region" aria-label="Panel update log" tabIndex={0} className="mt-3 max-h-80 overflow-auto whitespace-pre-wrap break-all rounded-md bg-secondary p-4 font-mono text-xs leading-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background">{status.log}</pre></details>}
        </div>}
      </>}
      {dialog}
    </CardContent>
  </Card>;
}
