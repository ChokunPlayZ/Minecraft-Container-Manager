import { useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { api } from '../api/client';
import { useUpdateChecker } from '../lib/use-update-checker';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Label } from './ui/label';
import { Switch } from './ui/switch';

export function UpdateCheckerSettings() {
  const { status, setStatus, error: loadError, reload } = useUpdateChecker();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const failures = Object.values(status?.servers ?? {}).filter((result) => result.error).length;

  async function toggle(enabled: boolean) {
    setSaving(true); setError(null); setMessage(null);
    try { setStatus(await api.saveUpdateChecker(enabled)); setMessage(enabled ? 'Nightly update checks enabled.' : 'Nightly update checks disabled.'); }
    catch (err) { setError(err instanceof Error ? err.message : 'Could not save update checks.'); }
    finally { setSaving(false); }
  }

  async function checkNow() {
    setSaving(true); setError(null); setMessage(null);
    try { await api.runUpdateCheck(); await reload(); }
    catch (err) { setError(err instanceof Error ? err.message : 'Could not start update checks.'); }
    finally { setSaving(false); }
  }

  return <Card>
    <CardHeader><CardTitle>Update checks</CardTitle><CardDescription>Check server software, plugins, and mods every midnight. Available updates appear on your servers.</CardDescription></CardHeader>
    <CardContent className="space-y-5">
      {(error || loadError) && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 text-sm text-destructive"><p>{error || loadError}</p>{loadError && <Button variant="outline" size="sm" onClick={() => void reload()}>Retry</Button>}</div>}
      {!status ? (!loadError && <p className="text-sm text-muted-foreground" role="status">Loading update settings…</p>) : <>
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1.5"><Label htmlFor="nightly-update-checks">Enable nightly update checks</Label><p id="update-check-description" className="text-sm text-muted-foreground">Off by default. Runs at 00:00 ({status.timezone}). Updates are installed only when you choose to apply them.</p></div>
          <Switch id="nightly-update-checks" aria-describedby="update-check-description" checked={status.enabled} disabled={saving} onCheckedChange={(enabled) => void toggle(enabled)} />
        </div>
        <p className="text-sm text-muted-foreground">Checks contact the software catalogs and Modrinth, Hangar, Spiget, or CurseForge for installed content. Shared plugins and mods reuse results during each check. CurseForge requires an API key configured on the panel.</p>
        {status.enabled && <dl className="space-y-2 border-t pt-4 text-sm">
          <div className="flex flex-wrap justify-between gap-2"><dt className="text-muted-foreground">Next check</dt><dd>{status.next_check_at ? new Date(status.next_check_at).toLocaleString(undefined, { timeZone: status.timezone }) : 'Midnight'} ({status.timezone})</dd></div>
          <div className="flex flex-wrap justify-between gap-2"><dt className="text-muted-foreground">Last check</dt><dd>{status.running ? 'Checking for updates…' : status.last_checked ? new Date(status.last_checked).toLocaleString() : 'No checks yet'}</dd></div>
        </dl>}
        {failures > 0 && status.enabled && <p role="status" className="text-sm text-amber-800 dark:text-amber-300">Some checks failed on {failures} {failures === 1 ? 'server' : 'servers'}. Last known updates remain visible. Check again to retry.</p>}
        <div className="flex flex-wrap items-center gap-3"><Button variant="outline" disabled={!status.enabled || saving || status.running} onClick={() => void checkNow()}><RefreshCw className={status.running ? 'animate-spin' : ''} />{status.running ? 'Checking…' : 'Check now'}</Button>{message && <p role="status" className="text-sm text-primary">{message}</p>}</div>
      </>}
    </CardContent>
  </Card>;
}
