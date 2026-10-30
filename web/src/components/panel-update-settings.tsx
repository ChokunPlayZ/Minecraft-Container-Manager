import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import { ArrowDownToLine, Check, ChevronDown, Download, Pause, Play, RefreshCw } from 'lucide-react';
import { api } from '../api/client';
import type { PanelUpdateStatus } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { useModal } from './ui/modal';
import { ProgressBar } from './ui/progress';

function message(error: unknown) {
  return error instanceof Error ? error.message : 'Could not contact the panel. Try again.';
}

function PanelUpdateProgress({ job, log, reconnecting }: {
  job: NonNullable<PanelUpdateStatus['job']>;
  log?: string;
  reconnecting: boolean;
}) {
  const running = job.state === 'queued' || job.state === 'running';
  const succeeded = job.state === 'succeeded';
  const failed = job.state === 'failed';
  const [logsOpen, setLogsOpen] = useState(running || failed);
  const [following, setFollowing] = useState(true);
  const logRef = useRef<HTMLPreElement>(null);
  const logId = useId();
  const progress = job.progress;
  const hasSteps = !!progress && progress.step > 0 && progress.total > 0;
  const title = succeeded ? 'Update complete' : failed ? 'Update failed' : progress?.title ?? (job.state === 'queued' ? 'Preparing update' : 'Updating panel');
  const stepText = hasSteps ? `Step ${progress.step} of ${progress.total}` : undefined;
  const value = succeeded ? 100 : hasSteps ? ((progress.step - 1) / progress.total) * 100 : null;

  useLayoutEffect(() => {
    if (logsOpen && following && logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [log, logsOpen, following]);

  function jumpToLatest() {
    setFollowing(true);
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }

  return <div className="min-w-0 space-y-4 border-t pt-4">
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        <p className={`flex min-w-0 items-center gap-2 font-medium ${failed ? 'text-destructive' : ''}`}>
          {running && <RefreshCw aria-hidden="true" className="h-4 w-4 shrink-0 motion-safe:animate-spin" />}
          {succeeded && <Check aria-hidden="true" className="h-4 w-4 shrink-0 text-primary" />}
          <span>{title}</span>
        </p>
        {running && stepText && <span className="text-xs tabular-nums text-muted-foreground">{stepText}</span>}
      </div>
      <ProgressBar
        aria-label="Panel update progress"
        aria-valuetext={`${title}${running && stepText ? `, ${stepText}` : ''}${reconnecting ? ', reconnecting to panel' : ''}`}
        value={failed && !hasSteps ? 0 : value}
        showPercent={false}
        variant={failed ? 'destructive' : 'default'}
        animated={running}
        size="sm"
      />
      <p role={failed ? 'alert' : 'status'} className={`break-words text-sm ${failed ? 'text-destructive' : 'text-muted-foreground'}`}>
        {reconnecting ? 'The panel is reconnecting. Update progress and logs will resume automatically.' : job.message}
      </p>
    </div>
    {succeeded && <Button variant="outline" onClick={() => window.location.reload()}>Reload updated panel</Button>}
    <div className="min-w-0 space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button variant="ghost" size="sm" aria-expanded={logsOpen} aria-controls={logId} onClick={() => setLogsOpen(!logsOpen)}>
          <ChevronDown aria-hidden="true" className={logsOpen ? '' : '-rotate-90'} />Update log
        </Button>
        {logsOpen && <div className="flex flex-wrap items-center gap-2">
          {!following && <Button variant="outline" size="sm" onClick={jumpToLatest}><ArrowDownToLine aria-hidden="true" />Jump to latest</Button>}
          <Button variant="ghost" size="sm" aria-label={following ? 'Pause auto-scroll' : 'Resume auto-scroll'} aria-pressed={following} onClick={() => following ? setFollowing(false) : jumpToLatest()}>
            {following ? <Pause aria-hidden="true" /> : <Play aria-hidden="true" />}{following ? 'Auto-scroll on' : 'Auto-scroll paused'}
          </Button>
        </div>}
      </div>
      {logsOpen && <>
        <pre
          ref={logRef}
          id={logId}
          role="region"
          aria-label="Panel update log"
          tabIndex={0}
          onScroll={(event) => {
            const viewer = event.currentTarget;
            setFollowing(viewer.scrollHeight - viewer.clientHeight - viewer.scrollTop <= 24);
          }}
          className="console-output h-72 max-h-[50vh] overflow-auto overscroll-contain whitespace-pre-wrap break-all rounded-md p-4 font-mono text-xs leading-6 [scrollbar-gutter:stable] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
        >{log || (running ? 'Waiting for installer output…' : 'No installer output was recorded.')}</pre>
        <p className="text-xs text-muted-foreground">Latest installer output. Scroll up to pause auto-scroll.</p>
      </>}
    </div>
  </div>;
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
        {status.job && <PanelUpdateProgress key={status.job.started_at} job={status.job} log={status.log} reconnecting={reconnecting} />}
      </>}
      {dialog}
    </CardContent>
  </Card>;
}
