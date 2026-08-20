import { useCallback, useEffect, useState } from 'react';
import { Plus, ShieldCheck, Trash2 } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server, WhitelistEntry } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';

export function WhitelistPanel({ server }: { server: Server }) {
  const [entries, setEntries] = useState<WhitelistEntry[]>([]);
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await api.whitelist(server.id);
      setEntries(res.whitelist ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load whitelist');
    }
  }, [server.id]);

  useEffect(() => {
    void load();
  }, [load]);

  async function add() {
    const trimmed = name.trim();
    if (!trimmed) return;
    setBusy(true);
    setError(null);
    try {
      const res = await api.addWhitelist(server.id, trimmed);
      setEntries(res.whitelist ?? []);
      setName('');
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to add player');
    } finally {
      setBusy(false);
    }
  }

  async function remove(playerName: string) {
    if (!window.confirm(`Remove ${playerName} from the whitelist?`)) return;
    setError(null);
    try {
      await api.removeWhitelist(server.id, playerName);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to remove player');
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Whitelist</CardTitle>
        <CardDescription>Control which players are allowed to join.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-end gap-2">
          <div className="flex-1 space-y-1.5">
            <Label htmlFor="wl-name">Player name</Label>
            <Input
              id="wl-name"
              placeholder="Alex"
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void add();
              }}
            />
          </div>
          <Button onClick={() => void add()} disabled={busy || !name.trim()}>
            <Plus className="h-4 w-4" /> {busy ? 'Adding...' : 'Add'}
          </Button>
        </div>

        {error && <p className="text-sm text-destructive">{error}</p>}

        <div className="space-y-2">
          {entries.length === 0 && (
            <p className="text-sm text-muted-foreground">No whitelisted players yet.</p>
          )}
          {entries.map((e) => (
            <div
              key={e.uuid}
              className="flex items-center justify-between gap-2 rounded-md border p-2.5 text-sm"
            >
              <div className="flex min-w-0 items-center gap-2">
                <ShieldCheck className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="truncate font-medium">{e.name}</span>
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label={`Remove ${e.name}`}
                title="Remove from whitelist"
                onClick={() => void remove(e.name)}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
