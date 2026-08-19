import { useCallback, useEffect, useState } from 'react';
import { Plus, Trash2, UserCog } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Op, Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';

export function OpsPanel({ server, refreshKey }: { server: Server; refreshKey: string }) {
  const [ops, setOps] = useState<Op[]>([]);
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await api.ops(server.id);
      setOps(res.ops ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load OPs');
    }
  }, [server.id]);

  useEffect(() => {
    void load();
  }, [load, refreshKey]);

  async function add() {
    const trimmed = name.trim();
    if (!trimmed) return;
    setBusy(true);
    setError(null);
    try {
      const res = await api.addOp(server.id, trimmed);
      setOps(res.ops ?? []);
      setName('');
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to add OP');
    } finally {
      setBusy(false);
    }
  }

  async function remove(opName: string) {
    if (!window.confirm(`Remove ${opName} as operator?`)) return;
    setError(null);
    try {
      await api.removeOp(server.id, opName);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to remove OP');
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Operators</CardTitle>
        <CardDescription>Grant or revoke server operator status.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-end gap-2">
          <div className="flex-1 space-y-1.5">
            <Label htmlFor="op-name">Player name</Label>
            <Input
              id="op-name"
              placeholder="Steve"
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
          {ops.length === 0 && <p className="text-sm text-muted-foreground">No operators yet.</p>}
          {ops.map((o) => (
            <div
              key={`${o.name}-${o.uuid}`}
              className="flex items-center justify-between gap-2 rounded-md border p-2.5 text-sm"
            >
              <div className="flex min-w-0 items-center gap-2">
                <UserCog className="h-4 w-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0">
                  <p className="truncate font-medium">{o.name}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    level {o.level}
                    {o.bypassesPlayerLimit ? ' · bypasses limit' : ''}
                  </p>
                </div>
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label={`Remove ${o.name}`}
                title="Remove operator"
                onClick={() => void remove(o.name)}
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
