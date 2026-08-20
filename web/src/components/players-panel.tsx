import { useCallback, useEffect, useState, type ReactNode } from 'react';
import {
  Ban,
  ChevronDown,
  Crown,
  Gavel,
  Map,
  Package,
  RefreshCw,
  Send,
  Shield,
  Skull,
  Users,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Player, PlayerCommandAction, PlayerCommandArgs, Server } from '../api/types';
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
  { action: 'op', label: 'OP', icon: Crown, fields: {} },
  { action: 'deop', label: 'Deop', icon: Crown, fields: {} },
  { action: 'give', label: 'Give', icon: Package, fields: { item: true, amount: true } },
  { action: 'gamemode', label: 'Gamemode', icon: Map, fields: { mode: true } },
  { action: 'tp', label: 'Teleport', icon: Send, fields: { target: true } },
  { action: 'kill', label: 'Kill', icon: Skull, fields: {}, destructive: true },
  { action: 'custom', label: 'Custom command', icon: Ban, fields: { command: true } },
];

const MODE_LABELS: Record<string, string> = {
  survival: 'Survival',
  creative: 'Creative',
  adventure: 'Adventure',
  spectator: 'Spectator',
};

interface ActiveCommand {
  player: string;
  action: PlayerCommandAction;
}

export function PlayersPanel({ server }: { server: Server }) {
  const [players, setPlayers] = useState<Player[]>([]);
  const [source, setSource] = useState<'rcon' | 'console'>('console');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [active, setActive] = useState<ActiveCommand | null>(null);

  // Detail form state.
  const [reason, setReason] = useState('');
  const [target, setTarget] = useState('');
  const [item, setItem] = useState('');
  const [amount, setAmount] = useState(1);
  const [mode, setMode] = useState('creative');
  const [command, setCommand] = useState('');
  const [formBusy, setFormBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  const running = server.state === 'running';

  const load = useCallback(async () => {
    if (!running) {
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
  }, [server.id, running]);

  useEffect(() => {
    void load();
  }, [load]);

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
      setNotice(
        `Sent ${activeDef?.label ?? active.action} to ${active.player}.${
          source === 'console' ? ' (RCON is not enabled; command may not have run)' : ''
        }`,
      );
      setActive(null);
      void load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to run command');
      setFormBusy(false);
    }
  }

  const activeDef = ACTIONS.find((a) => a.action === active?.action);

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-base">Players</CardTitle>
            <CardDescription>
              {running ? `${players.length} connected` : 'Server is not running.'}
            </CardDescription>
          </div>
          <Button
            variant="outline"
            size="icon"
            aria-label="Refresh players"
            title="Refresh players"
            disabled={busy || !running}
            onClick={() => void load()}
          >
            <RefreshCw className={`h-4 w-4 ${busy ? 'animate-spin' : ''}`} />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {error && <p className="text-sm text-destructive">{error}</p>}
        {notice && <p className="text-sm text-emerald-600">{notice}</p>}
        {running && players.length === 0 && !error && (
          <p className="text-sm text-muted-foreground">No players online.</p>
        )}
        {source === 'console' && players.length === 0 && running && !error && (
          <p className="text-xs text-muted-foreground">
            RCON is not enabled; showing players inferred from recent console logs.
          </p>
        )}

        <div className="space-y-2">
          {players.map((p) => (
            <div
              key={p.name}
              className="flex items-center justify-between gap-2 rounded-md border px-2.5 py-2 text-sm"
            >
              <div className="flex min-w-0 items-center gap-2">
                <Users className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="truncate font-medium">{p.name}</span>
                {source === 'console' && (
                  <Badge variant="outline" className="hidden sm:inline-flex">
                    console
                  </Badge>
                )}
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={!running}
                    className="gap-1"
                    aria-label={`Commands for ${p.name}`}
                  >
                    Commands <ChevronDown className="h-3.5 w-3.5" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-48">
                  <DropdownMenuLabel>{p.name}</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  {ACTIONS.map((a) => {
                    const Icon = a.icon;
                    return (
                      <DropdownMenuItem
                        key={a.action}
                        onSelect={() => openAction(p.name, a.action)}
                        className={a.destructive ? 'text-destructive focus:text-destructive' : ''}
                      >
                        <Icon className="h-4 w-4" /> {a.label}
                      </DropdownMenuItem>
                    );
                  })}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ))}
        </div>

        {active && activeDef && (
          <div className="rounded-md border bg-muted/40 p-3">
            <div className="mb-3 flex items-center justify-between gap-2">
              <p className="text-sm font-medium">
                {activeDef.label} · {active.player}
              </p>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Cancel"
                onClick={() => setActive(null)}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>

            <div className="grid gap-3">
              {activeDef.fields.reason && (
                <Field label="Reason (optional)">
                  <Input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="reason" />
                </Field>
              )}
              {activeDef.fields.target && (
                <Field label="Player name">
                  <Input value={target} onChange={(e) => setTarget(e.target.value)} placeholder="Alex" />
                </Field>
              )}
              {activeDef.fields.item && (
                <Field label="Item">
                  <Input
                    value={item}
                    onChange={(e) => setItem(e.target.value)}
                    placeholder="minecraft:diamond"
                  />
                </Field>
              )}
              {activeDef.fields.amount && (
                <Field label="Amount">
                  <Input
                    type="number"
                    min={1}
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
                <Field label="Command">
                  <Input
                    value={command}
                    onChange={(e) => setCommand(e.target.value)}
                    placeholder="time set day"
                  />
                </Field>
              )}

              <div className="flex justify-end gap-2 pt-1">
                <Button variant="outline" onClick={() => setActive(null)} disabled={formBusy}>
                  Cancel
                </Button>
                <Button
                  variant={activeDef.destructive ? 'destructive' : 'default'}
                  onClick={() => void submit()}
                  disabled={formBusy}
                >
                  {formBusy ? 'Sending...' : 'Confirm'}
                </Button>
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs">{label}</Label>
      {children}
    </div>
  );
}
