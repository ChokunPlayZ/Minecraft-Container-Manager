import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import {
  Ban,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Copy,
  Crown,
  Gavel,
  LayoutGrid,
  List,
  Map,
  Package,
  RefreshCw,
  Search,
  Send,
  Shield,
  ShieldCheck,
  Skull,
  User,
  UserCog,
  Users,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Op, Player, PlayerCommandAction, PlayerCommandArgs, Server } from '../api/types';
import { OpsPanel } from './ops-panel';
import { WhitelistPanel } from './whitelist-panel';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './ui/dropdown-menu';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';

interface ActionDef {
  action: PlayerCommandAction;
  label: string;
  icon: typeof Users;
  fields: {
    reason?: boolean;
    target?: boolean;
    item?: boolean;
    amount?: boolean;
    mode?: boolean;
    command?: boolean;
  };
  destructive?: boolean;
}

const ACTIONS: ActionDef[] = [
  { action: 'kick', label: 'Kick', icon: X, fields: { reason: true } },
  { action: 'ban', label: 'Ban', icon: Gavel, fields: { reason: true }, destructive: true },
  { action: 'pardon', label: 'Pardon', icon: Shield, fields: { target: true } },
  { action: 'op', label: 'Make Operator (OP)', icon: Crown, fields: {} },
  { action: 'deop', label: 'Revoke Operator (Deop)', icon: Crown, fields: {} },
  { action: 'give', label: 'Give Items', icon: Package, fields: { item: true, amount: true } },
  { action: 'gamemode', label: 'Change Gamemode', icon: Map, fields: { mode: true } },
  { action: 'tp', label: 'Teleport', icon: Send, fields: { target: true } },
  { action: 'kill', label: 'Kill', icon: Skull, fields: {}, destructive: true },
  { action: 'custom', label: 'Custom Command', icon: Ban, fields: { command: true } },
];

const MODE_LABELS: Record<string, string> = {
  survival: 'Survival',
  creative: 'Creative',
  adventure: 'Adventure',
  spectator: 'Spectator',
};

const COMMON_ITEMS = [
  'minecraft:diamond',
  'minecraft:netherite_ingot',
  'minecraft:golden_apple',
  'minecraft:elytra',
  'minecraft:bread',
  'minecraft:cooked_beef',
];

interface ActiveCommand {
  player: string;
  action: PlayerCommandAction;
}

export function PlayersPanel({ server }: { server: Server }) {
  const [subTab, setSubTab] = useState<'online' | 'ops' | 'whitelist'>('online');
  const [players, setPlayers] = useState<Player[]>([]);
  const [ops, setOps] = useState<Op[]>([]);
  const [source, setSource] = useState<'rcon' | 'console'>('console');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [active, setActive] = useState<ActiveCommand | null>(null);

  // Filters, sorting, and pagination
  const [search, setSearch] = useState('');
  const [roleFilter, setRoleFilter] = useState<'all' | 'ops' | 'regular'>('all');
  const [sortBy, setSortBy] = useState<'asc' | 'desc'>('asc');
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [pageSize, setPageSize] = useState<number>(24);
  const [page, setPage] = useState<number>(1);
  const [copiedPlayer, setCopiedPlayer] = useState<string | null>(null);

  // Modal form fields
  const [reason, setReason] = useState('');
  const [target, setTarget] = useState('');
  const [item, setItem] = useState('');
  const [amount, setAmount] = useState(1);
  const [mode, setMode] = useState('creative');
  const [command, setCommand] = useState('');
  const [formBusy, setFormBusy] = useState(false);

  const running = server.state === 'running';

  const opNamesSet = useMemo(() => {
    return new Set(ops.map((o) => o.name.toLowerCase()));
  }, [ops]);

  const load = useCallback(async () => {
    if (!running) {
      setPlayers([]);
      setSource('console');
      setError(null);
      return;
    }
    setBusy(true);
    try {
      const [res, opsRes] = await Promise.all([
        api.players(server.id).catch((err) => {
          throw err;
        }),
        api.ops(server.id).catch(() => ({ ops: [] })),
      ]);
      setPlayers(res.players ?? []);
      setOps(opsRes.ops ?? []);
      setSource(res.source);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load players');
    } finally {
      setBusy(false);
    }
  }, [server.id, running]);

  useEffect(() => {
    void load();
  }, [load]);

  // Filtered & sorted player list
  const filteredPlayers = useMemo(() => {
    return players
      .filter((p) => {
        if (search.trim()) {
          const q = search.trim().toLowerCase();
          if (!p.name.toLowerCase().includes(q)) return false;
        }
        const isOp = opNamesSet.has(p.name.toLowerCase());
        if (roleFilter === 'ops' && !isOp) return false;
        if (roleFilter === 'regular' && isOp) return false;
        return true;
      })
      .sort((a, b) => {
        return sortBy === 'asc'
          ? a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
          : b.name.localeCompare(a.name, undefined, { sensitivity: 'base' });
      });
  }, [players, search, roleFilter, sortBy, opNamesSet]);

  // Reset page when search or filters change
  useEffect(() => {
    setPage(1);
  }, [search, roleFilter, sortBy, pageSize]);

  // Pagination calculations
  const totalPages = Math.max(1, Math.ceil(filteredPlayers.length / pageSize));
  const currentPlayers = useMemo(() => {
    if (pageSize >= 9999) return filteredPlayers;
    const start = (page - 1) * pageSize;
    return filteredPlayers.slice(start, start + pageSize);
  }, [filteredPlayers, page, pageSize]);

  function copyPlayerName(name: string) {
    void navigator.clipboard.writeText(name);
    setCopiedPlayer(name);
    setTimeout(() => setCopiedPlayer((prev) => (prev === name ? null : prev)), 2000);
  }

  function openAction(player: string, action: PlayerCommandAction) {
    setReason('');
    setTarget('');
    setItem('');
    setAmount(1);
    setMode('creative');
    setCommand('');
    setFormBusy(false);
    setError(null);
    setNotice(null);
    setActive({ player, action });
  }

  async function submit() {
    if (!active) return;
    const args: PlayerCommandArgs = {
      reason: reason || undefined,
      target: target || undefined,
      item: item || undefined,
      amount: amount > 0 ? amount : 1,
      mode: mode || undefined,
      command: command || undefined,
    };
    setFormBusy(true);
    setError(null);
    setNotice(null);
    try {
      await api.runPlayerCommand(server.id, active.player, active.action, args);
      const label = activeDef?.label ?? active.action;
      setNotice(`Successfully ran ${label} on ${active.player}.`);
      setActive(null);
      void load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to run command');
      setFormBusy(false);
    }
  }

  const activeDef = ACTIONS.find((a) => a.action === active?.action);

  return (
    <div className="space-y-4">
      {/* Sub-tabs bar */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-3">
        <div className="inline-flex rounded-lg border bg-muted/40 p-1">
          <button
            type="button"
            onClick={() => setSubTab('online')}
            className={`inline-flex items-center gap-2 rounded-md px-3.5 py-1.5 text-sm font-medium transition-colors ${
              subTab === 'online'
                ? 'bg-background text-foreground shadow-xs'
                : 'text-muted-foreground hover:bg-background/60 hover:text-foreground'
            }`}
          >
            <Users className="h-4 w-4" />
            <span>Online Players</span>
            <Badge variant="secondary" className="ml-0.5 px-1.5 py-0 text-xs">
              {running ? players.length : 0}
            </Badge>
          </button>

          <button
            type="button"
            onClick={() => setSubTab('ops')}
            className={`inline-flex items-center gap-2 rounded-md px-3.5 py-1.5 text-sm font-medium transition-colors ${
              subTab === 'ops'
                ? 'bg-background text-foreground shadow-xs'
                : 'text-muted-foreground hover:bg-background/60 hover:text-foreground'
            }`}
          >
            <UserCog className="h-4 w-4" />
            <span>Operators</span>
            {ops.length > 0 && (
              <Badge variant="outline" className="ml-0.5 px-1.5 py-0 text-xs">
                {ops.length}
              </Badge>
            )}
          </button>

          <button
            type="button"
            onClick={() => setSubTab('whitelist')}
            className={`inline-flex items-center gap-2 rounded-md px-3.5 py-1.5 text-sm font-medium transition-colors ${
              subTab === 'whitelist'
                ? 'bg-background text-foreground shadow-xs'
                : 'text-muted-foreground hover:bg-background/60 hover:text-foreground'
            }`}
          >
            <ShieldCheck className="h-4 w-4" />
            <span>Whitelist</span>
          </button>
        </div>

        {subTab === 'online' && (
          <div className="flex items-center gap-2">
            {source === 'console' && running && players.length > 0 && (
              <Badge variant="outline" className="text-xs text-muted-foreground">
                Inferred from logs
              </Badge>
            )}
            <Button
              variant="outline"
              size="sm"
              disabled={busy || !running}
              onClick={() => void load()}
              className="gap-1.5 text-xs"
              title="Refresh online players list"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${busy ? 'animate-spin' : ''}`} />
              <span className="hidden sm:inline">Refresh</span>
            </Button>
          </div>
        )}
      </div>

      {notice && (
        <div className="flex items-center justify-between rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-700 dark:text-emerald-300">
          <span>{notice}</span>
          <button
            type="button"
            onClick={() => setNotice(null)}
            className="text-emerald-700 hover:opacity-75 dark:text-emerald-300"
            aria-label="Dismiss notice"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      {error && (
        <div className="flex items-center justify-between rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
          <span>{error}</span>
          <button
            type="button"
            onClick={() => setError(null)}
            className="hover:opacity-75"
            aria-label="Dismiss error"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* Sub-tab content: Ops */}
      {subTab === 'ops' && <OpsPanel server={server} refreshKey={server.state} />}

      {/* Sub-tab content: Whitelist */}
      {subTab === 'whitelist' && <WhitelistPanel server={server} />}

      {/* Sub-tab content: Online Players */}
      {subTab === 'online' && (
        <Card>
          <CardHeader className="p-4 sm:p-5 pb-3">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <CardTitle className="text-base font-semibold">Connected Players</CardTitle>
                <CardDescription className="mt-0.5">
                  {running
                    ? `${players.length} player${players.length === 1 ? '' : 's'} online (${ops.length} operator${ops.length === 1 ? '' : 's'})`
                    : 'Server is stopped. Start the server to see connected players.'}
                </CardDescription>
              </div>

              {/* Toolbar: Search, Filters, View Modes */}
              {running && players.length > 0 && (
                <div className="flex flex-wrap items-center gap-2">
                  {/* Search input */}
                  <div className="relative min-w-[180px] flex-1 sm:w-56 sm:flex-initial">
                    <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
                    <Input
                      value={search}
                      onChange={(e) => setSearch(e.target.value)}
                      placeholder="Search players..."
                      className="h-8 pl-8 pr-7 text-xs"
                    />
                    {search && (
                      <button
                        type="button"
                        onClick={() => setSearch('')}
                        className="absolute right-2 top-2 text-muted-foreground hover:text-foreground"
                        aria-label="Clear search"
                      >
                        <X className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>

                  {/* Role filter */}
                  <Select
                    value={roleFilter}
                    onChange={(e) => setRoleFilter(e.target.value as 'all' | 'ops' | 'regular')}
                    className="h-8 text-xs w-28"
                  >
                    <option value="all">All Roles</option>
                    <option value="ops">Operators</option>
                    <option value="regular">Non-Ops</option>
                  </Select>

                  {/* Sort order */}
                  <Select
                    value={sortBy}
                    onChange={(e) => setSortBy(e.target.value as 'asc' | 'desc')}
                    className="h-8 text-xs w-28"
                  >
                    <option value="asc">Name (A-Z)</option>
                    <option value="desc">Name (Z-A)</option>
                  </Select>

                  {/* View Mode Toggle */}
                  <div className="inline-flex rounded-md border bg-muted/40 p-0.5">
                    <button
                      type="button"
                      onClick={() => setViewMode('grid')}
                      className={`rounded p-1 transition-colors ${
                        viewMode === 'grid' ? 'bg-background shadow-xs text-foreground' : 'text-muted-foreground hover:text-foreground'
                      }`}
                      title="Grid view"
                      aria-label="Grid view"
                    >
                      <LayoutGrid className="h-3.5 w-3.5" />
                    </button>
                    <button
                      type="button"
                      onClick={() => setViewMode('list')}
                      className={`rounded p-1 transition-colors ${
                        viewMode === 'list' ? 'bg-background shadow-xs text-foreground' : 'text-muted-foreground hover:text-foreground'
                      }`}
                      title="List view"
                      aria-label="List view"
                    >
                      <List className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              )}
            </div>

            {/* Filter result count banner if filtering */}
            {running && players.length > 0 && (search || roleFilter !== 'all') && (
              <div className="mt-2 flex items-center justify-between text-xs text-muted-foreground">
                <span>
                  Showing {filteredPlayers.length} of {players.length} players
                </span>
                {(search || roleFilter !== 'all') && (
                  <button
                    type="button"
                    onClick={() => {
                      setSearch('');
                      setRoleFilter('all');
                    }}
                    className="text-primary hover:underline"
                  >
                    Reset filters
                  </button>
                )}
              </div>
            )}
          </CardHeader>

          <CardContent className="p-4 sm:p-5 pt-0">
            {!running && (
              <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
                The server is not running. Start it to view and interact with active players.
              </div>
            )}

            {running && players.length === 0 && (
              <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
                No players are currently connected to this server.
              </div>
            )}

            {running && players.length > 0 && filteredPlayers.length === 0 && (
              <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
                No players match your search or filter criteria.
              </div>
            )}

            {/* Grid View */}
            {running && viewMode === 'grid' && currentPlayers.length > 0 && (
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                {currentPlayers.map((p) => {
                  const isOp = opNamesSet.has(p.name.toLowerCase());
                  const copied = copiedPlayer === p.name;
                  return (
                    <div
                      key={p.name}
                      className="group relative flex flex-col justify-between rounded-lg border bg-card p-3 shadow-xs transition-all hover:border-primary/40 hover:shadow-sm"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="flex items-center gap-2.5 min-w-0">
                          {/* Minecraft Player Head */}
                          <PlayerAvatar name={p.name} size={36} />
                          <div className="min-w-0">
                            <div className="flex items-center gap-1">
                              <span className="truncate text-sm font-semibold text-foreground">
                                {p.name}
                              </span>
                              <button
                                type="button"
                                onClick={() => copyPlayerName(p.name)}
                                title="Copy player name"
                                className="opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-foreground"
                              >
                                {copied ? <Check className="h-3 w-3 text-emerald-500" /> : <Copy className="h-3 w-3" />}
                              </button>
                            </div>
                            <div className="mt-0.5 flex flex-wrap items-center gap-1">
                              {isOp && (
                                <Badge variant="secondary" className="gap-0.5 px-1 py-0 text-[10px] font-semibold text-amber-600 dark:text-amber-400">
                                  <Crown className="h-2.5 w-2.5" /> OP
                                </Badge>
                              )}
                              {source === 'console' && (
                                <Badge variant="outline" className="px-1 py-0 text-[10px] text-muted-foreground">
                                  console
                                </Badge>
                              )}
                            </div>
                          </div>
                        </div>

                        {/* Commands dropdown */}
                        <PlayerActionsMenu
                          player={p.name}
                          isOp={isOp}
                          onSelectAction={(action) => openAction(p.name, action)}
                        />
                      </div>

                      {/* Quick action buttons row on card */}
                      <div className="mt-3 flex items-center justify-end gap-1 border-t pt-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
                          onClick={() => openAction(p.name, 'gamemode')}
                        >
                          Mode
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
                          onClick={() => openAction(p.name, 'tp')}
                        >
                          TP
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 px-2 text-xs text-destructive hover:bg-destructive/10"
                          onClick={() => openAction(p.name, 'kick')}
                        >
                          Kick
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}

            {/* List View */}
            {running && viewMode === 'list' && currentPlayers.length > 0 && (
              <div className="divide-y rounded-lg border">
                {currentPlayers.map((p) => {
                  const isOp = opNamesSet.has(p.name.toLowerCase());
                  const copied = copiedPlayer === p.name;
                  return (
                    <div
                      key={p.name}
                      className="flex items-center justify-between gap-3 p-2.5 hover:bg-muted/30 transition-colors"
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <PlayerAvatar name={p.name} size={32} />
                        <div className="min-w-0">
                          <div className="flex items-center gap-1.5">
                            <span className="truncate font-medium text-sm text-foreground">
                              {p.name}
                            </span>
                            <button
                              type="button"
                              onClick={() => copyPlayerName(p.name)}
                              title="Copy player name"
                              className="text-muted-foreground hover:text-foreground"
                            >
                              {copied ? <Check className="h-3 w-3 text-emerald-500" /> : <Copy className="h-3 w-3" />}
                            </button>
                            {isOp && (
                              <Badge variant="secondary" className="gap-0.5 px-1.5 py-0 text-xs font-semibold text-amber-600 dark:text-amber-400">
                                <Crown className="h-3 w-3" /> OP
                              </Badge>
                            )}
                            {source === 'console' && (
                              <Badge variant="outline" className="hidden sm:inline-flex text-[10px]">
                                console
                              </Badge>
                            )}
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center gap-1.5">
                        <Button
                          variant="outline"
                          size="sm"
                          className="hidden sm:inline-flex h-8 px-2.5 text-xs"
                          onClick={() => openAction(p.name, 'gamemode')}
                        >
                          Gamemode
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          className="hidden sm:inline-flex h-8 px-2.5 text-xs text-destructive hover:bg-destructive/10"
                          onClick={() => openAction(p.name, 'kick')}
                        >
                          Kick
                        </Button>
                        <PlayerActionsMenu
                          player={p.name}
                          isOp={isOp}
                          onSelectAction={(action) => openAction(p.name, action)}
                        />
                      </div>
                    </div>
                  );
                })}
              </div>
            )}

            {/* Pagination Controls */}
            {running && filteredPlayers.length > 0 && (
              <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between border-t pt-3">
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <span>Show</span>
                  <Select
                    value={String(pageSize)}
                    onChange={(e) => setPageSize(Number(e.target.value))}
                    className="h-7 w-20 text-xs"
                  >
                    <option value="12">12</option>
                    <option value="24">24</option>
                    <option value="48">48</option>
                    <option value="96">96</option>
                    <option value="9999">All</option>
                  </Select>
                  <span>
                    per page · Showing {Math.min((page - 1) * pageSize + 1, filteredPlayers.length)}–
                    {Math.min(page * pageSize, filteredPlayers.length)} of {filteredPlayers.length}
                  </span>
                </div>

                {totalPages > 1 && (
                  <div className="flex items-center gap-1">
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-7 w-7 p-0"
                      disabled={page <= 1}
                      onClick={() => setPage((p) => Math.max(1, p - 1))}
                      aria-label="Previous page"
                    >
                      <ChevronLeft className="h-3.5 w-3.5" />
                    </Button>
                    <span className="px-2 text-xs font-medium">
                      Page {page} of {totalPages}
                    </span>
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-7 w-7 p-0"
                      disabled={page >= totalPages}
                      onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                      aria-label="Next page"
                    >
                      <ChevronRight className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                )}
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Action Dialog Modal */}
      {active && activeDef && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in"
          role="presentation"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget && !formBusy) setActive(null);
          }}
        >
          <div
            className="w-full max-w-lg rounded-xl border bg-card p-5 text-card-foreground shadow-xl sm:p-6"
            role="dialog"
            aria-modal="true"
            aria-labelledby="player-action-dialog-title"
          >
            <div className="flex items-center justify-between border-b pb-3">
              <div className="flex items-center gap-2.5">
                <PlayerAvatar name={active.player} size={32} />
                <div>
                  <h3 id="player-action-dialog-title" className="text-base font-semibold">
                    {activeDef.label}
                  </h3>
                  <p className="text-xs text-muted-foreground">Target: {active.player}</p>
                </div>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7"
                onClick={() => setActive(null)}
                disabled={formBusy}
                aria-label="Close dialog"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>

            <div className="space-y-4 py-4">
              {activeDef.fields.reason && (
                <Field label="Reason (optional)">
                  <Input
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                    placeholder="e.g. Inappropriate behavior or griefing"
                    autoFocus
                  />
                </Field>
              )}

              {activeDef.fields.target && (
                <Field label="Target player name">
                  <Input
                    value={target}
                    onChange={(e) => setTarget(e.target.value)}
                    placeholder="e.g. Alex"
                    autoFocus
                  />
                </Field>
              )}

              {activeDef.fields.item && (
                <div className="space-y-2">
                  <Field label="Item identifier">
                    <Input
                      value={item}
                      onChange={(e) => setItem(e.target.value)}
                      placeholder="minecraft:diamond"
                      autoFocus
                    />
                  </Field>
                  <div className="flex flex-wrap items-center gap-1">
                    <span className="text-[11px] text-muted-foreground mr-1">Suggestions:</span>
                    {COMMON_ITEMS.map((ci) => (
                      <button
                        key={ci}
                        type="button"
                        onClick={() => setItem(ci)}
                        className="rounded bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-secondary hover:text-foreground transition-colors"
                      >
                        {ci.replace('minecraft:', '')}
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {activeDef.fields.amount && (
                <Field label="Amount">
                  <Input
                    type="number"
                    min={1}
                    max={64}
                    value={amount}
                    onChange={(e) => setAmount(Number(e.target.value))}
                  />
                </Field>
              )}

              {activeDef.fields.mode && (
                <Field label="Gamemode">
                  <Select value={mode} onChange={(e) => setMode(e.target.value)}>
                    {Object.entries(MODE_LABELS).map(([value, label]) => (
                      <option key={value} value={value}>
                        {label}
                      </option>
                    ))}
                  </Select>
                </Field>
              )}

              {activeDef.fields.command && (
                <Field label="Custom command (omit leading slash)">
                  <Input
                    value={command}
                    onChange={(e) => setCommand(e.target.value)}
                    placeholder="time set day"
                    autoFocus
                  />
                </Field>
              )}

              {activeDef.destructive && (
                <p className="rounded-md bg-destructive/10 p-2.5 text-xs text-destructive">
                  Warning: This is a destructive administrative action that takes effect immediately.
                </p>
              )}
            </div>

            <div className="flex items-center justify-end gap-2 border-t pt-3">
              <Button
                variant="outline"
                onClick={() => setActive(null)}
                disabled={formBusy}
              >
                Cancel
              </Button>
              <Button
                variant={activeDef.destructive ? 'destructive' : 'default'}
                onClick={() => void submit()}
                disabled={formBusy}
              >
                {formBusy ? 'Sending...' : 'Confirm Action'}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function PlayerAvatar({ name, size = 32 }: { name: string; size?: number }) {
  const [loadError, setLoadError] = useState(false);
  const avatarUrl = `https://mc-heads.net/avatar/${encodeURIComponent(name)}/${size}`;

  if (loadError) {
    return (
      <div
        style={{ width: size, height: size }}
        className="flex shrink-0 items-center justify-center rounded-md bg-secondary text-muted-foreground font-semibold text-xs select-none"
      >
        <User className="h-4 w-4" />
      </div>
    );
  }

  return (
    <img
      src={avatarUrl}
      alt={name}
      width={size}
      height={size}
      loading="lazy"
      onError={() => setLoadError(true)}
      className="shrink-0 rounded-md bg-muted object-contain shadow-2xs"
    />
  );
}

function PlayerActionsMenu({
  player,
  isOp,
  onSelectAction,
}: {
  player: string;
  isOp: boolean;
  onSelectAction: (action: PlayerCommandAction) => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="h-8 gap-1 px-2.5 text-xs"
          aria-label={`Commands for ${player}`}
        >
          <span>Actions</span>
          <ChevronDown className="h-3 w-3" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-52">
        <DropdownMenuLabel className="truncate">{player}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {ACTIONS.map((a) => {
          // Adjust label for op vs deop based on current status
          if (a.action === 'op' && isOp) return null;
          if (a.action === 'deop' && !isOp) return null;
          const Icon = a.icon;
          return (
            <DropdownMenuItem
              key={a.action}
              onSelect={() => onSelectAction(a.action)}
              className={a.destructive ? 'text-destructive focus:text-destructive' : ''}
            >
              <Icon className="h-4 w-4 mr-2" />
              <span>{a.label}</span>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs font-medium">{label}</Label>
      {children}
    </div>
  );
}

