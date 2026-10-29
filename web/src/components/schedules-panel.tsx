import { useEffect, useRef, useState, type FormEvent } from 'react';
import { CalendarClock, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { BackupTarget, ScheduleInput, ScheduledAction, ScheduledTask } from '../api/types';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';
import { Switch } from './ui/switch';
import { useModal } from './ui/modal';

const presets = [
  { value: 'midnight', label: 'Every day at midnight' },
  { value: '0 * * * *', label: 'Every hour' },
  { value: '*/30 * * * *', label: 'Every 30 minutes' },
  { value: '0 0 * * 0', label: 'Every Sunday at midnight' },
  { value: '0 0 1 * *', label: 'First day of every month' },
];
const actions: Record<ScheduledAction, string> = { command: 'Console command', backup: 'Create backup', restart: 'Restart server', start: 'Start server', stop: 'Stop server' };
const backupTargets: Record<BackupTarget, string> = { '': 'Panel default', local: 'Local disk', s3: 'Remote (S3)' };

function defaultInput(timezone = 'UTC'): ScheduleInput {
  return { name: '', action: 'command', command: '', backup_target: '', cron: 'midnight', timezone, enabled: true };
}

function presetFor(cron: string): string {
  if (intervalMinutes(cron) !== null) return 'interval';
  if (['0 0 * * *', '@midnight', '@daily', 'daily', 'midnight'].includes(cron)) return 'midnight';
  return presets.find((preset) => preset.value === cron)?.value ?? 'custom';
}

function intervalMinutes(cron: string): number | null {
  const match = cron.match(/^@every (?:(\d+)h)?(?:(\d+)m)?$/);
  return match && (match[1] || match[2]) ? Number(match[1] ?? 0) * 60 + Number(match[2] ?? 0) : null;
}

function frequencyLabel(cron: string): string {
  const minutes = intervalMinutes(cron);
  if (minutes !== null) return minutes % 60 === 0 ? `Every ${minutes / 60} ${minutes === 60 ? 'hour' : 'hours'}` : `Every ${minutes} ${minutes === 1 ? 'minute' : 'minutes'}`;
  return presets.find((preset) => preset.value === presetFor(cron))?.label ?? 'Custom cron';
}

function localTime(value: string, timezone: string): string {
  if (!value) return '—';
  const date = new Date(value);
  try {
    return new Intl.DateTimeFormat(undefined, { timeZone: timezone, month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }).format(date);
  } catch {
    // Older browsers may not recognize every timezone available to the panel.
    return `${date.toLocaleString(undefined, { timeZone: 'UTC' })} UTC`;
  }
}

const errorMessage = (err: unknown, fallback: string) => err instanceof ApiError ? err.detail : fallback;

export function SchedulesPanel({ serverId }: { serverId: string }) {
  const [tasks, setTasks] = useState<ScheduledTask[]>([]);
  const [defaultTimezone, setDefaultTimezone] = useState('UTC');
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<ScheduledTask | 'new' | null>(null);
  const [input, setInput] = useState<ScheduleInput>(defaultInput);
  const [frequency, setFrequency] = useState('midnight');
  const [busy, setBusy] = useState(false);
  const [reload, setReload] = useState(0);
  const revision = useRef(0);
  const { confirm, dialog } = useModal();

  useEffect(() => {
    let active = true;
    async function load() {
      const current = ++revision.current;
      try {
        const result = await api.listSchedules(serverId);
        if (active && current === revision.current) { setTasks(result.schedules); setDefaultTimezone(result.default_timezone); setLoadError(null); }
      } catch (err) {
        if (active && current === revision.current) setLoadError(errorMessage(err, 'Could not load schedules. Try again.'));
      } finally {
        if (active) setLoading(false);
      }
    }
    void load();
    const timer = setInterval(() => void load(), 15000);
    return () => { active = false; clearInterval(timer); };
  }, [serverId, reload]);

  function open(task: ScheduledTask | 'new') {
    setInput(task === 'new' ? defaultInput(defaultTimezone) : { name: task.name, action: task.action, command: task.command, backup_target: task.backup_target, cron: task.cron, timezone: task.timezone, enabled: task.enabled });
    setFrequency(task === 'new' ? 'midnight' : presetFor(task.cron));
    setEditing(task);
    setError(null);
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!editing) return;
    setBusy(true); setError(null);
    try {
      const task = editing === 'new'
        ? await api.createSchedule(serverId, input)
        : await api.updateSchedule(serverId, editing.id, input);
      revision.current++;
      setTasks((previous) => editing === 'new' ? [...previous, task] : previous.map((item) => item.id === task.id ? task : item));
      setEditing(null);
    } catch (err) { setError(errorMessage(err, 'Could not save this schedule. Try again.')); }
    finally { setBusy(false); }
  }

  async function toggle(task: ScheduledTask) {
    setBusy(true); setError(null);
    try {
      const updated = await api.setScheduleEnabled(serverId, task.id, !task.enabled);
      revision.current++;
      setTasks((previous) => previous.map((item) => item.id === updated.id ? updated : item));
    } catch (err) { setError(errorMessage(err, 'Could not change this schedule. Try again.')); }
    finally { setBusy(false); }
  }

  async function remove(task: ScheduledTask) {
    if (!(await confirm(`Delete “${task.name}”? This stops future runs. A task already running will finish.`, { title: 'Delete schedule', confirmLabel: 'Delete schedule', destructive: true }))) return;
    setBusy(true); setError(null);
    try {
      await api.deleteSchedule(serverId, task.id);
      revision.current++;
      setTasks((previous) => previous.filter((item) => item.id !== task.id));
    } catch (err) { setError(errorMessage(err, 'Could not delete this schedule. Try again.')); }
    finally { setBusy(false); }
  }

  return (
    <section className="space-y-5" aria-labelledby="schedules-title">
      {dialog}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 id="schedules-title" className="text-lg font-semibold tracking-tight">Schedules</h2>
          <p className="mt-1 text-sm text-muted-foreground">Schedule backups, console commands, and server actions.</p>
        </div>
        {!editing && <Button onClick={() => open('new')} disabled={loading || !!loadError || busy}><Plus className="h-4 w-4" />Add schedule</Button>}
      </div>

      {loadError && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-4 text-sm"><p className="text-destructive">{loadError}</p><Button variant="outline" size="sm" onClick={() => setReload((value) => value + 1)}><RefreshCw className="h-4 w-4" />Retry</Button></div>}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}

      {editing && (
        <form onSubmit={(event) => void save(event)} className="space-y-5 rounded-lg border bg-card p-4 sm:p-6" aria-label={editing === 'new' ? 'New schedule' : 'Edit schedule'}>
          <h3 className="font-semibold">{editing === 'new' ? 'New schedule' : 'Edit schedule'}</h3>
          <fieldset disabled={busy} className="grid min-w-0 gap-4 sm:grid-cols-2">
            <div className="min-w-0 space-y-1.5"><Label htmlFor="schedule-name">Task name</Label><Input id="schedule-name" autoFocus required maxLength={100} placeholder="Nightly restart" value={input.name} onChange={(event) => setInput({ ...input, name: event.target.value })} /></div>
            <div className="min-w-0 space-y-1.5"><Label htmlFor="schedule-action">Action</Label><Select id="schedule-action" value={input.action} onValueChange={(value) => setInput({ ...input, action: value as ScheduledAction, command: value === 'command' ? input.command : '', backup_target: value === 'backup' ? input.backup_target : '' })}>{Object.entries(actions).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</Select></div>
            {input.action === 'backup' && <div className="min-w-0 space-y-1.5 sm:col-span-2"><Label htmlFor="schedule-backup-target">Backup target</Label><Select id="schedule-backup-target" value={input.backup_target || 'default'} onValueChange={(value) => setInput({ ...input, backup_target: value === 'default' ? '' : value as BackupTarget })}>{Object.entries(backupTargets).map(([value, label]) => <option key={value} value={value || 'default'}>{label}</option>)}</Select><p className="text-xs text-muted-foreground">Remote backups use the panel’s configured S3-compatible storage.</p></div>}
            {input.action === 'command' && <div className="min-w-0 space-y-1.5 sm:col-span-2"><Label htmlFor="schedule-command">Console command</Label><Input id="schedule-command" required maxLength={4096} placeholder="say Server restart in 5 minutes" className="font-mono" value={input.command} onChange={(event) => setInput({ ...input, command: event.target.value })} /><p className="text-xs text-muted-foreground">One Minecraft command per task. The server must be running.</p></div>}
            <div className="min-w-0 space-y-1.5"><Label htmlFor="schedule-frequency">Frequency</Label><Select id="schedule-frequency" value={frequency} onValueChange={(value) => { setFrequency(value); setInput({ ...input, cron: value === 'custom' ? '' : value === 'interval' ? '@every 720m' : value }); }}>{presets.map((preset) => <option key={preset.value} value={preset.value}>{preset.label}</option>)}<option value="interval">Fixed interval</option><option value="custom">Custom cron expression</option></Select></div>
            <div className="min-w-0 space-y-1.5"><Label htmlFor="schedule-timezone">Timezone</Label><Input id="schedule-timezone" required maxLength={100} list="schedule-timezones" placeholder="Asia/Bangkok" value={input.timezone} onChange={(event) => setInput({ ...input, timezone: event.target.value })} /><datalist id="schedule-timezones">{Array.from(new Set([defaultTimezone, 'UTC', 'Asia/Bangkok', 'Asia/Tokyo', 'Europe/London', 'Europe/Berlin', 'America/New_York', 'America/Los_Angeles', 'Australia/Sydney'])).map((zone) => <option key={zone} value={zone} />)}</datalist><p className="text-xs text-muted-foreground">Panel timezone: {defaultTimezone}. You can override it for this task.</p></div>
            {frequency === 'custom' && <div className="min-w-0 space-y-1.5 sm:col-span-2"><Label htmlFor="schedule-cron">Cron expression</Label><Input id="schedule-cron" required maxLength={120} className="font-mono" placeholder="0 0 * * *" aria-describedby="schedule-cron-help" value={input.cron} onChange={(event) => setInput({ ...input, cron: event.target.value })} /><p id="schedule-cron-help" className="text-xs text-muted-foreground">minute · hour · day of month · month · weekday (0 = Sunday). For example, <code>0 0 * * *</code> runs daily at midnight; <code>*/15 * * * *</code> runs every 15 minutes.</p></div>}
            {frequency === 'interval' && <div className="min-w-0 space-y-1.5 sm:col-span-2"><Label htmlFor="schedule-interval">Interval (minutes)</Label><Input id="schedule-interval" type="number" required min={1} max={153722867} step={1} value={intervalMinutes(input.cron) ?? ''} aria-describedby="schedule-interval-help" onChange={(event) => setInput({ ...input, cron: `@every ${event.target.value}m` })} /><p id="schedule-interval-help" className="text-xs text-muted-foreground">Repeat at this interval from when the schedule is saved or resumed. Use 720 minutes for every 12 hours.</p></div>}
          </fieldset>
          {input.action === 'restart' && <p className="text-sm text-muted-foreground">Restart gracefully stops the server, then starts it. A stopped server will also start.</p>}
          {input.action === 'backup' && <p className="text-sm text-muted-foreground">Uses the selected backup target and the panel’s retention settings. A running server is stopped for the backup, then started again.</p>}
          <div className="flex flex-wrap items-center justify-between gap-4 border-t pt-4">
            <div className="flex items-center gap-2"><Switch id="schedule-enabled" checked={input.enabled} disabled={busy} onCheckedChange={(enabled) => setInput({ ...input, enabled })} /><Label htmlFor="schedule-enabled">Enable schedule</Label></div>
            <div className="flex gap-2"><Button type="button" variant="outline" disabled={busy} onClick={() => { setEditing(null); setError(null); }}>Cancel</Button><Button type="submit" disabled={busy}>{busy ? 'Saving…' : editing === 'new' ? 'Create schedule' : 'Save changes'}</Button></div>
          </div>
        </form>
      )}

      {loading ? <p role="status" className="py-8 text-sm text-muted-foreground">Loading schedules…</p> : !loadError && tasks.length === 0 && !editing ? (
        <div className="rounded-lg border border-dashed px-5 py-10 text-center"><CalendarClock className="mx-auto mb-3 h-6 w-6 text-muted-foreground" /><h3 className="font-medium">No scheduled tasks</h3><p className="mx-auto mt-1 max-w-md text-sm text-muted-foreground">Schedule an automatic backup, a nightly restart, or a recurring announcement.</p></div>
      ) : tasks.length > 0 && (
        <ul className="divide-y rounded-lg border bg-card" aria-label="Scheduled tasks">
          {tasks.map((task) => (
            <li key={task.id} className="space-y-3 p-4 sm:p-5">
              <div className="flex flex-col items-start justify-between gap-3 sm:flex-row">
                <div className="min-w-0 flex-1 space-y-1"><h3 className="break-words font-semibold">{task.name}</h3><p className="text-sm text-muted-foreground">{actions[task.action]} · {frequencyLabel(task.cron)}{task.action === 'backup' && ` · ${backupTargets[task.backup_target]}`}</p>{task.action === 'command' && <code className="block break-all text-sm">{task.command}</code>}</div>
                <div className="flex shrink-0 items-center gap-2"><span className="text-xs text-muted-foreground">{task.enabled ? 'Enabled' : 'Paused'}</span><Switch checked={task.enabled} disabled={busy || !!editing} aria-label={`Enable ${task.name}`} onCheckedChange={() => void toggle(task)} /><Button variant="ghost" size="icon" disabled={busy || !!editing} aria-label={`Edit ${task.name}`} onClick={() => open(task)}><Pencil className="h-4 w-4" /></Button><Button variant="ghost" size="icon" disabled={busy || !!editing} aria-label={`Delete ${task.name}`} onClick={() => void remove(task)}><Trash2 className="h-4 w-4" /></Button></div>
              </div>
              <div className="flex flex-wrap gap-x-6 gap-y-2 text-xs text-muted-foreground"><span className="break-all"><code>{task.cron}</code> · {task.timezone}</span><span>Next run: <span className="font-medium text-foreground">{task.enabled ? localTime(task.next_run_at, task.timezone) : 'Paused'}</span></span><span>Last result: <span className={task.last_status === 'failed' ? 'font-medium text-destructive' : 'font-medium text-foreground'}>{({ success: 'Succeeded', failed: 'Failed', running: 'Running', skipped: 'Skipped', '': 'Not run yet' })[task.last_status]}</span>{task.last_run_at && ` · ${localTime(task.last_run_at, task.timezone)}`}</span></div>
              {task.last_error && <p className={`break-words text-sm ${task.last_status === 'failed' ? 'text-destructive' : 'text-muted-foreground'}`}>{task.last_error}</p>}
            </li>
          ))}
        </ul>
      )}
      <p className="max-w-prose text-xs leading-relaxed text-muted-foreground">The panel must stay online to run tasks. Missed runs are skipped after downtime. Pausing prevents future runs; a task already running will finish.</p>
    </section>
  );
}
