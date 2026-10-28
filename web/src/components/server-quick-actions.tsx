import { useRef, useState } from 'react';
import { Link } from '@tanstack/react-router';
import { Archive, ArrowRight, Copy, Loader2, MoreHorizontal, Play, RotateCw, Square, Zap } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import { Button } from './ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from './ui/dropdown-menu';
import { useModal } from './ui/modal';

type PowerAction = 'start' | 'stop' | 'restart' | 'kill';

export function ServerQuickActions({ server, onUpdated, onCopy }: {
  server: Server;
  onUpdated: (server: Server) => void;
  /** Omit to hide Copy server, e.g. for members. */
  onCopy?: () => void;
}) {
  const [pending, setPending] = useState<PowerAction | null>(null);
  const [error, setError] = useState<string | null>(null);
  const inFlight = useRef(false);
  const { confirm, dialog } = useModal();
  const canStart = server.state === 'stopped' || server.state === 'error';
  const canStop = server.state === 'running' || server.state === 'starting';
  const canKill = ['running', 'starting', 'stopping', 'installing', 'building'].includes(server.state);

  async function run(action: PowerAction) {
    if (inFlight.current) return;
    inFlight.current = true;
    try {
      if (action === 'kill' && !(await confirm(
        `Force kill "${server.name}"? This stops the server immediately without saving world data. Unsaved changes may be lost.`,
        { title: 'Force kill server', confirmLabel: 'Force kill', destructive: true },
      ))) return;
      setPending(action);
      setError(null);
      const actions = { start: api.startServer, stop: api.stopServer, restart: api.restartServer, kill: api.killServer };
      onUpdated(await actions[action](server.id));
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : `Could not ${action} the server. Try again.`);
    } finally {
      inFlight.current = false;
      setPending(null);
    }
  }

  return <div className="fleet-actions min-w-0">
    {dialog}
    <div className="flex flex-wrap items-center justify-end gap-2" aria-label={`Actions for ${server.name}`} aria-busy={pending !== null}>
      <Button variant={canStart ? 'default' : 'outline'} size="sm" disabled={pending !== null || (!canStart && !canStop)}
        aria-label={`${canStart ? 'Start' : 'Stop'} ${server.name}`} onClick={() => void run(canStart ? 'start' : 'stop')}>
        {pending ? <Loader2 className="animate-spin" /> : canStart ? <Play /> : <Square />}
        {pending ? ({ start: 'Starting…', stop: 'Stopping…', restart: 'Restarting…', kill: 'Killing…' }[pending]) : canStart ? 'Start' : 'Stop'}
      </Button>
      <Button variant="outline" size="sm" asChild><Link to="/servers/$id" params={{ id: server.id }}>Manage<ArrowRight /></Link></Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="h-9 w-9" aria-label={`More actions for ${server.name}`} disabled={pending !== null}><MoreHorizontal /></Button></DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem disabled={server.state !== 'running'} onSelect={() => void run('restart')}><RotateCw className="h-4 w-4" />Restart</DropdownMenuItem>
          <DropdownMenuItem asChild><Link to="/backups" search={{ server: server.id }}><Archive className="h-4 w-4" />Backups</Link></DropdownMenuItem>
          {onCopy && <DropdownMenuItem onSelect={onCopy}><Copy className="h-4 w-4" />Copy server</DropdownMenuItem>}
          <DropdownMenuSeparator />
          <DropdownMenuItem className="text-destructive focus:text-destructive" disabled={!canKill} onSelect={() => void run('kill')}><Zap className="h-4 w-4" />Force kill</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    {error && <p role="alert" className="mt-2 max-w-xs text-sm text-destructive">{error}</p>}
  </div>;
}
