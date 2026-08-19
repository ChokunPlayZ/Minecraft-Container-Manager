import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { api, ApiError } from '../api/client';
import type { GatewayInfo, ServerState } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';

export function GatewayPanel({ serverId, state }: { serverId: string; state: ServerState }) {
  const [info, setInfo] = useState<GatewayInfo | null>(null);
  const [wakeMessage, setWakeMessage] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const i = await api.serverGateway(serverId);
      setInfo(i);
      setWakeMessage(i.wake_message);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load gateway info');
    }
  }, [serverId]);

  useEffect(() => {
    void load();
  }, [load]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const i = await api.putServerGateway(serverId, wakeMessage.trim());
      setInfo(i);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to save wake message');
    } finally {
      setBusy(false);
    }
  }

  const sleeping = state === 'stopped' || state === 'error';

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Wake-on-rejoin</CardTitle>
        <CardDescription>
          The gateway owns this server's port and wakes it when a player joins.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-2 text-sm">
          <span
            className={`inline-block h-2 w-2 rounded-full ${info?.enabled ? 'bg-emerald-500' : 'bg-muted'}`}
          />
          <span className="text-muted-foreground">
            {info?.enabled ? 'Gateway active' : 'Gateway disabled'}
          </span>
        </div>

        {sleeping && info?.last_motd ? (
          <div className="rounded-md border border-border bg-muted/40 p-3 text-sm">
            <div className="mb-1 text-xs text-muted-foreground">Last known server MOTD</div>
            <div className="whitespace-pre-wrap">{info.last_motd}</div>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            {sleeping
              ? 'No MOTD captured yet. It will appear here once the server has been running.'
              : 'MOTD is captured while the server runs.'}
          </p>
        )}

        <form onSubmit={onSubmit} className="space-y-2">
          <div className="space-y-1.5">
            <Label htmlFor="wake-message">Wait message</Label>
            <Input
              id="wake-message"
              value={wakeMessage}
              placeholder="Server is starting up, please wait..."
              onChange={(e) => setWakeMessage(e.target.value)}
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button type="submit" size="sm" disabled={busy}>
            {busy ? 'Saving...' : 'Save wait message'}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
