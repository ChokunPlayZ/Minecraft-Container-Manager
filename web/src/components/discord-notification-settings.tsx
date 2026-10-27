import { useEffect, useState, type FormEvent } from 'react';
import { Bell, Send } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { DiscordEvent, DiscordSettings } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Switch } from './ui/switch';

const eventOptions: { value: DiscordEvent; label: string; description: string }[] = [
  { value: 'server_started', label: 'Server starts', description: 'A server container starts, including scheduled starts.' },
  { value: 'server_stopped', label: 'Server stops', description: 'A server stops or is force stopped from the panel.' },
  { value: 'server_crashed', label: 'Server crashes', description: 'A container exits unexpectedly. Includes its exit code.' },
  { value: 'server_error', label: 'Server errors', description: 'A start, stop, installation, or other server operation fails.' },
  { value: 'backup_completed', label: 'Backup completed', description: 'A manual or automatic backup finishes successfully.' },
  { value: 'backup_failed', label: 'Backup failed', description: 'A manual or automatic backup fails.' },
];

function errorMessage(error: unknown) {
  return error instanceof ApiError ? error.detail : error instanceof Error ? error.message : 'Something went wrong. Try again.';
}

export function DiscordNotificationSettings() {
  const [settings, setSettings] = useState<DiscordSettings | null>(null);
  const [webhook, setWebhook] = useState('');
  const [removeWebhook, setRemoveWebhook] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<'save' | 'test' | null>(null);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');

  async function load() {
    setLoading(true);
    setError('');
    try { setSettings(await api.getDiscordSettings()); }
    catch (err) { setError(errorMessage(err)); }
    finally { setLoading(false); }
  }

  useEffect(() => { void load(); }, []);

  function edit(next: DiscordSettings) {
    setSettings(next);
    setMessage('');
    setError('');
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!settings) return;
    setBusy('save'); setError(''); setMessage('');
    try {
      const saved = await api.saveDiscordSettings({
        enabled: settings.enabled,
        events: settings.events,
        ...(removeWebhook ? { webhook_url: '' } : webhook.trim() ? { webhook_url: webhook.trim() } : {}),
      });
      setSettings(saved); setWebhook(''); setRemoveWebhook(false);
      setMessage('Discord notification settings saved.');
    } catch (err) { setError(errorMessage(err)); }
    finally { setBusy(null); }
  }

  async function test() {
    setBusy('test'); setError(''); setMessage('');
    try {
      const result = await api.testDiscordWebhook(webhook.trim() || undefined);
      setMessage(result.message);
    } catch (err) { setError(errorMessage(err)); }
    finally { setBusy(null); }
  }

  const hasWebhook = !!webhook.trim() || (!!settings?.configured && !removeWebhook);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base"><Bell className="h-4 w-4" aria-hidden="true" />Discord notifications</CardTitle>
        <CardDescription>Send server and backup updates to a Discord channel. Applies to all servers.</CardDescription>
      </CardHeader>
      <CardContent>
        {loading ? <p className="text-sm text-muted-foreground" role="status">Loading notification settings…</p> : !settings ? (
          <div className="space-y-3">
            <p className="text-sm text-destructive" role="alert">{error}</p>
            <Button variant="outline" onClick={() => void load()}>Retry</Button>
          </div>
        ) : (
          <form onSubmit={(event) => void save(event)} className="space-y-6">
            <fieldset disabled={busy !== null} className="min-w-0 space-y-6">
              <div className="flex items-start justify-between gap-4">
                <div className="space-y-1">
                  <Label htmlFor="discord-enabled">Enable Discord notifications</Label>
                  <p className="text-sm text-muted-foreground">Choose which events are sent below.</p>
                </div>
                <Switch id="discord-enabled" checked={settings.enabled} onCheckedChange={(enabled) => edit({ ...settings, enabled })} />
              </div>

              <div className="space-y-2">
                <Label htmlFor="discord-webhook">Discord webhook URL</Label>
                <Input id="discord-webhook" type="password" autoComplete="off" spellCheck={false}
                  value={webhook} placeholder={settings.configured && !removeWebhook ? 'Saved webhook · enter a URL to replace it' : 'https://discord.com/api/webhooks/…'}
                  aria-describedby="discord-webhook-help"
                  onChange={(event) => { setWebhook(event.target.value); setRemoveWebhook(false); setMessage(''); setError(''); }} />
                <p id="discord-webhook-help" className="text-sm text-muted-foreground">
                  In Discord, open Channel Settings → Integrations → Webhooks, then copy the webhook URL. The saved URL stays private; leave this field blank to keep it.
                </p>
                {settings.configured && (
                  <Button type="button" variant="ghost" size="sm" onClick={() => {
                    setRemoveWebhook(!removeWebhook); setWebhook('');
                    edit({ ...settings, enabled: false });
                  }}>{removeWebhook ? 'Keep saved webhook' : 'Remove saved webhook'}</Button>
                )}
                {removeWebhook && <p className="text-sm text-muted-foreground">The saved webhook will be removed when you save.</p>}
              </div>

              <fieldset className="min-w-0">
                <legend className="mb-2 text-sm font-medium">Events to send</legend>
                <div className="divide-y divide-border">
                  {eventOptions.map((option) => (
                    <div key={option.value} className="flex items-start justify-between gap-4 py-3">
                      <div className="space-y-1">
                        <Label htmlFor={`discord-${option.value}`}>{option.label}</Label>
                        <p id={`discord-${option.value}-help`} className="text-sm text-muted-foreground">{option.description}</p>
                      </div>
                      <Switch id={`discord-${option.value}`} checked={settings.events.includes(option.value)}
                        aria-describedby={`discord-${option.value}-help`}
                        onCheckedChange={(checked) => edit({ ...settings, events: checked ? [...settings.events, option.value] : settings.events.filter((value) => value !== option.value) })} />
                    </div>
                  ))}
                </div>
                <p className="mt-2 text-sm text-muted-foreground">Restarts send a stop and a start event. Crashes are monitored while the panel is running, even with your browser closed.</p>
              </fieldset>
            </fieldset>

            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            {message && <p role="status" className="text-sm text-foreground">{message}</p>}
            <div className="flex flex-wrap gap-3">
              <Button type="submit" disabled={busy !== null || (settings.enabled && !hasWebhook)}>{busy === 'save' ? 'Saving…' : 'Save notification settings'}</Button>
              <Button type="button" variant="outline" disabled={busy !== null || !hasWebhook} onClick={() => void test()}>
                <Send className="h-4 w-4" aria-hidden="true" />{busy === 'test' ? 'Sending…' : 'Send test message'}
              </Button>
            </div>
          </form>
        )}
      </CardContent>
    </Card>
  );
}
