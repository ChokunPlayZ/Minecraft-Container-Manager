import { useCallback, useEffect, useMemo, useState } from 'react';
import { Database, Loader2, RefreshCw, Search } from 'lucide-react';
import { ApiError } from '../api/client';
import { nbtApi, type PlayerDataFile } from '../api/nbt';
import { NBTEditor } from './nbt-editor';
import { Button } from './ui/button';
import { Input } from './ui/input';

export function PlayerDataPanel({
  serverId,
  state,
  onDirtyChange,
}: {
  serverId: string;
  state: string;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const [players, setPlayers] = useState<PlayerDataFile[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(true);
  const [query, setQuery] = useState('');
  const [limit, setLimit] = useState(50);
  const load = useCallback(async () => {
    setBusy(true);
    setError(null);
    try {
      setPlayers((await nbtApi.players(serverId)).players);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Could not load saved player data. Try again.');
    } finally {
      setBusy(false);
    }
  }, [serverId]);
  useEffect(() => {
    void load();
  }, [load]);
  const filtered = useMemo(
    () =>
      players.filter((p) =>
        `${p.name} ${p.uuid} ${p.world}`.toLowerCase().includes(query.trim().toLowerCase()),
      ),
    [players, query],
  );
  if (selected)
    return (
      <NBTEditor
        key={selected}
        serverId={serverId}
        path={selected}
        state={state}
        onDirtyChange={onDirtyChange}
        onClose={() => setSelected(null)}
      />
    );
  return (
    <section aria-label="Saved player data" className="min-w-0 space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="section-heading">Player data</h3>
          <p className="mt-1 max-w-prose text-sm text-muted-foreground">
            Edit saved health, food, XP, position and inventory. Players are listed from world playerdata
            folders, including offline players. Open other NBT files from Files.
          </p>
        </div>
        <Button size="sm" variant="outline" disabled={busy} onClick={() => void load()}>
          <RefreshCw className="mr-2 h-4 w-4" />
          Refresh player data
        </Button>
      </div>
      <div className="relative max-w-md">
        <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
        <Input
          aria-label="Search saved players"
          placeholder="Search player name, UUID or world"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setLimit(50);
          }}
          className="pl-9"
        />
      </div>
      {error && (
        <p role="alert" className="rounded-lg border border-destructive/40 p-3 text-sm">
          {error}
        </p>
      )}
      {busy ? (
        <p role="status" className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Finding player files…
        </p>
      ) : (
        !error && (
          <>
            <p role="status" className="text-xs text-muted-foreground">
              {filtered.length} player {filtered.length === 1 ? 'file' : 'files'}
            </p>
            <div className="divide-y rounded-lg border bg-card">
              {filtered.length === 0 ? (
                <div className="empty-state">
                  <Database />
                  <h4 className="section-heading">
                    {query ? 'No matching players' : 'No saved player data'}
                  </h4>
                  <p>
                    {query
                      ? 'Try a different name, UUID or world.'
                      : 'Player files appear after a player joins a Java Edition world and the server saves. You can browse other NBT files in Files.'}
                  </p>
                </div>
              ) : (
                filtered.slice(0, limit).map((p) => (
                  <button
                    key={p.path}
                    type="button"
                    onClick={() => setSelected(p.path)}
                    className="flex w-full flex-col gap-2 px-4 py-3 text-left hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring sm:flex-row sm:items-center sm:justify-between"
                  >
                    <span className="min-w-0">
                      <span className="block break-all text-sm font-medium">{p.name || p.uuid}</span>
                      <span className="block break-all font-mono text-xs text-muted-foreground">
                        {p.name ? `${p.uuid} · ` : ''}
                        {p.world}
                      </span>
                    </span>
                    <span className="shrink-0 text-xs text-primary">Open player data</span>
                  </button>
                ))
              )}
            </div>
            {filtered.length > limit && (
              <Button size="sm" variant="outline" onClick={() => setLimit((n) => n + 50)}>
                Show more players
              </Button>
            )}
          </>
        )
      )}
    </section>
  );
}
