import { useCallback, useEffect, useState } from 'react';
import { RefreshCw, Users } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Player, Server } from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';

export function PlayersPanel({ server }: { server: Server }) {
  const [players, setPlayers] = useState<Player[]>([]);
  const [source, setSource] = useState<'rcon' | 'console'>('console');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (server.state !== 'running') {
      setPlayers([]);
      setSource('console');
      setError(null);
      return;
    }
    setBusy(true);
    try {
      const res = await api.players(server.id);
      setPlayers(res.players ?? []);
      setSource(res.source);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load players');
    } finally {
      setBusy(false);
    }
  }, [server.id, server.state]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-base">Players</CardTitle>
            <CardDescription>
              {server.state === 'running'
                ? `${players.length} connected`
                : 'Server is not running.'}
            </CardDescription>
          </div>
          <Button
            variant="outline"
            size="icon"
            aria-label="Refresh players"
            title="Refresh players"
            disabled={busy || server.state !== 'running'}
            onClick={() => void load()}
          >
            <RefreshCw className={`h-4 w-4 ${busy ? 'animate-spin' : ''}`} />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {error && <p className="text-sm text-destructive">{error}</p>}
        {server.state === 'running' && players.length === 0 && !error && (
          <p className="text-sm text-muted-foreground">No players online.</p>
        )}
        {source === 'rcon' && players.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {players.map((p) => (
              <Badge key={p.name} variant="secondary" className="gap-1 px-2 py-1">
                <Users className="h-3 w-3" /> {p.name}
              </Badge>
            ))}
          </div>
        )}
        {source === 'console' && players.length === 0 && server.state === 'running' && !error && (
          <p className="text-xs text-muted-foreground">
            RCON is not enabled; showing players inferred from recent console logs.
          </p>
        )}
        {source === 'console' && players.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {players.map((p) => (
              <Badge key={p.name} variant="outline" className="gap-1 px-2 py-1">
                <Users className="h-3 w-3" /> {p.name}
              </Badge>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
