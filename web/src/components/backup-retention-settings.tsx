import { useCallback, useEffect, useState } from 'react';
import { Loader2, RefreshCw, ShieldCheck } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { BackupPolicy, BackupRetentionPreview } from '../api/types';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Skeleton } from './ui/skeleton';
import { useModal } from './ui/modal';

function bytes(value: number) {
  if (!value) return '0 B';
  const unit = Math.min(Math.floor(Math.log(value) / Math.log(1024)), 4);
  return `${(value / 1024 ** unit).toFixed(1)} ${['B', 'KB', 'MB', 'GB', 'TB'][unit]}`;
}

export function BackupRetentionSettings({ serverId, canManage, disabled, onCleanup }: {
  serverId: string; canManage: boolean; disabled: boolean; onCleanup: () => Promise<void>;
}) {
  const [preview, setPreview] = useState<BackupRetentionPreview | null>(null);
  const [fields, setFields] = useState({ keep_count: '', max_age_days: '', trash_retention_days: '' });
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const { confirm, dialog } = useModal();
  const apply = useCallback((value: BackupRetentionPreview) => {
    setPreview(value);
    setFields({ keep_count: String(value.policy.keep_count), max_age_days: String(value.policy.max_age_days), trash_retention_days: String(value.policy.trash_retention_days) });
  }, []);
  const load = useCallback(async () => {
    setPending(true);
    setError(null);
    try { apply(await api.getBackupRetention(serverId)); }
    catch (err) { setError(err instanceof ApiError ? err.detail : 'Could not load retention settings. Try again.'); }
    finally { setPending(false); }
  }, [serverId, apply]);
  useEffect(() => { void load(); }, [load]);

  const policy = Object.fromEntries(Object.entries(fields).map(([key, value]) => [key, Number(value)])) as unknown as BackupPolicy;
  const valid = Object.entries(fields).every(([key, value]) => value.trim() !== '' && Number.isInteger(Number(value)) && Number(value) >= 0 && Number(value) <= (key === 'keep_count' ? 100000 : 36500));
  const dirty = preview !== null && Object.keys(fields).some((key) => policy[key as keyof BackupPolicy] !== preview.policy[key as keyof BackupPolicy]);

  async function save() {
    if (!valid || !dirty) return;
    setPending(true); setError(null); setMessage(null);
    try {
      apply(await api.saveBackupRetention(serverId, policy));
      setMessage('Policy saved. It will apply after successful backups and during hourly cleanup.');
      await onCleanup();
    } catch (err) { setError(err instanceof ApiError ? err.detail : 'Could not save retention settings. Try again.'); }
    finally { setPending(false); }
  }

  async function cleanup() {
    if (!preview || dirty) return;
    if (!await confirm(`Move ${preview.to_trash.length} backups to trash and permanently delete ${preview.to_purge.length} expired backups? Trash continues to use storage until permanently deleted. Cleanup uses the latest backup list.`, { title: 'Run backup cleanup', confirmLabel: 'Run cleanup', destructive: preview.to_purge.length > 0 })) return;
    setPending(true); setError(null); setMessage(null);
    try {
      const result = await api.cleanupBackups(serverId);
      setMessage(`Cleanup complete: ${result.trashed} moved to trash, ${result.purged} permanently deleted.`);
    } catch (err) { setError(err instanceof ApiError ? err.detail : 'Cleanup could not finish. Refresh the preview and try again.'); }
    finally {
      // A storage error can leave a partially completed cleanup. Always reload.
      await onCleanup();
      try { apply(await api.getBackupRetention(serverId)); } catch { /* Keep the action error visible. */ }
      setPending(false);
    }
  }

  return <section aria-label="Backup retention" className="space-y-6">
    {dialog}
    <div><h3 className="section-heading">Retention policy</h3><p className="mt-1 text-sm text-muted-foreground">Set limits for this server across local and S3 storage. Protected backups are kept in addition to the count limit.</p></div>
    {error && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 text-sm text-destructive"><p>{error}</p>{!preview && <Button variant="outline" size="sm" disabled={pending} onClick={() => void load()}>Try again</Button>}</div>}
    {!preview ? pending && <Skeleton className="h-32 w-full" /> : <>
      <form onSubmit={(event) => { event.preventDefault(); void save(); }} className="space-y-4">
        <div className="grid gap-5 md:grid-cols-3">
          <div className="space-y-2"><Label htmlFor="backup-keep-count">Backups to keep</Label><Input id="backup-keep-count" type="number" min={0} max={100000} step={1} value={fields.keep_count} onChange={(e) => setFields({ ...fields, keep_count: e.target.value })} disabled={!canManage || pending || disabled} aria-describedby="backup-keep-help" /><p id="backup-keep-help" className="text-xs text-muted-foreground">Newest successful, unprotected backups. Set 0 to keep any number.</p></div>
          <div className="space-y-2"><Label htmlFor="backup-max-age">Maximum age (days)</Label><Input id="backup-max-age" type="number" min={0} max={36500} step={1} value={fields.max_age_days} onChange={(e) => setFields({ ...fields, max_age_days: e.target.value })} disabled={!canManage || pending || disabled} aria-describedby="backup-age-help" /><p id="backup-age-help" className="text-xs text-muted-foreground">Older backups go to trash even below the count limit. Set 0 for no age limit.</p></div>
          <div className="space-y-2"><Label htmlFor="backup-trash-days">Keep trash (days)</Label><Input id="backup-trash-days" type="number" min={0} max={36500} step={1} value={fields.trash_retention_days} onChange={(e) => setFields({ ...fields, trash_retention_days: e.target.value })} disabled={!canManage || pending || disabled} aria-describedby="backup-trash-help" /><p id="backup-trash-help" className="text-xs text-muted-foreground">Then permanently delete archives. Set 0 to keep trash until you empty it. Applies to existing trash too.</p></div>
        </div>
        <div className="flex flex-wrap items-center gap-3"><Button type="submit" disabled={!canManage || pending || disabled || !dirty || !valid}>{pending && <Loader2 className="animate-spin" />}Save policy</Button>{dirty && <><Button type="button" variant="ghost" disabled={pending} onClick={() => { apply(preview); setMessage(null); }}>Discard changes</Button><p className="text-xs text-muted-foreground">Save changes to update the preview.</p></>}</div>
      </form>
      <p className="flex items-start gap-2 text-sm text-muted-foreground"><ShieldCheck className="mt-0.5 h-4 w-4 shrink-0" />The newest successful backup and all protected backups stay available. Failed and pending backups do not count toward the limit. Saving a policy does not run cleanup immediately.</p>
      <div className="space-y-4 border-t pt-5">
        <div className="flex flex-wrap items-center justify-between gap-3"><div><h3 className="section-heading">Cleanup preview</h3><p className="mt-1 text-sm text-muted-foreground">{preview.to_trash.length} to trash ({bytes(preview.trash_bytes)}) · {preview.to_purge.length} to delete permanently ({bytes(preview.purge_bytes)} freed)</p></div><Button variant="ghost" size="sm" onClick={() => void load()} disabled={pending || dirty || disabled}><RefreshCw className={pending ? 'animate-spin' : ''} />Refresh preview</Button></div>
        {preview.to_trash.length + preview.to_purge.length === 0 ? <p className="text-sm text-muted-foreground">No backups need cleanup under the saved policy.</p> : <ul aria-label="Backups affected by cleanup" className="max-h-64 overflow-y-auto divide-y">
          {preview.to_trash.map((b) => <li key={b.id} className="flex flex-wrap justify-between gap-2 py-2 text-sm"><span className="min-w-0 break-all font-medium">{b.name}</span><span className="text-muted-foreground">Move to trash · {b.reason === 'age' ? 'Age limit' : 'Count limit'}</span></li>)}
          {preview.to_purge.map((b) => <li key={b.id} className="flex flex-wrap justify-between gap-2 py-2 text-sm"><span className="min-w-0 break-all font-medium">{b.name}</span><span className="text-destructive">Delete permanently · Trash expired</span></li>)}
        </ul>}
        <Button variant="outline" onClick={() => void cleanup()} disabled={!canManage || disabled || pending || dirty || preview.to_trash.length + preview.to_purge.length === 0}>Run cleanup now</Button>
      </div>
    </>}
    {message && <p role="status" className="text-sm text-foreground">{message}</p>}
  </section>;
}
