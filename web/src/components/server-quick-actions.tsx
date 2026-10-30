import { usePermissions } from '../lib/auth';
import { useRef, useState, type ReactElement, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Link } from '@tanstack/react-router';
import { Archive, ArrowRight, Copy, Download, Loader2, MoreHorizontal, Play, RotateCw, Square, Zap } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import { Button } from './ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from './ui/dropdown-menu';
import { useModal } from './ui/modal';
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuLabel, ContextMenuSeparator, ContextMenuTrigger } from './ui/context-menu';

type PowerAction = 'start' | 'stop' | 'restart' | 'kill';

export function ServerQuickActions({ server, onUpdated, onCopy, children }: {
  server: Server;
  onUpdated: (server: Server) => void;
  onCopy: () => void;
  children?: (actions: ReactNode) => ReactElement;
}) {
  const can = usePermissions();
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

  function menuItems(context = false) {
    const Item = context ? ContextMenuItem : DropdownMenuItem;
    const Separator = context ? ContextMenuSeparator : DropdownMenuSeparator;
    return <>
      {context && <>
        <Item asChild><Link to="/servers/$id" params={{ id: server.id }}><ArrowRight className="h-4 w-4" />Manage server</Link></Item>
        <Item disabled={!can('servers.power', server.id) || pending !== null || (!canStart && !canStop)} onSelect={() => void run(canStart ? 'start' : 'stop')}>
          {canStart ? <Play className="h-4 w-4" /> : <Square className="h-4 w-4" />}{canStart ? 'Start' : 'Stop'}
        </Item>
      </>}
      <Item disabled={!can('servers.power', server.id) || pending !== null || server.state !== 'running'} onSelect={() => void run('restart')}><RotateCw className="h-4 w-4" />Restart</Item>
      <Item disabled={!can('backups.read', server.id) || pending !== null} asChild><Link to="/backups" search={{ server: server.id }}><Archive className="h-4 w-4" />Backups</Link></Item>
      <Item disabled={!can('servers.export', server.id) || pending !== null} onSelect={() => api.exportServer(server.id, server.name)}><Download className="h-4 w-4" />Export server (.zip)</Item>
      <Item disabled={!can('servers.copy', server.id) || pending !== null} onSelect={onCopy}><Copy className="h-4 w-4" />Copy server</Item>
      <Separator />
      <Item className="text-destructive focus:text-destructive" disabled={!can('servers.power', server.id) || pending !== null || !canKill} onSelect={() => void run('kill')}><Zap className="h-4 w-4" />Force kill</Item>
    </>;
  }

  const actions = <div className="fleet-actions min-w-0">
    <div className="flex flex-wrap items-center justify-end gap-2" aria-label={`Actions for ${server.name}`} aria-busy={pending !== null}>
      <Button variant={canStart ? 'default' : 'outline'} size="sm" disabled={!can('servers.power', server.id) || pending !== null || (!canStart && !canStop)}
        aria-label={`${canStart ? 'Start' : 'Stop'} ${server.name}`} onClick={() => void run(canStart ? 'start' : 'stop')}>
        {pending ? <Loader2 className="animate-spin" /> : canStart ? <Play /> : <Square />}
        {pending ? ({ start: 'Starting…', stop: 'Stopping…', restart: 'Restarting…', kill: 'Killing…' }[pending]) : canStart ? 'Start' : 'Stop'}
      </Button>
      <Button variant="outline" size="sm" asChild><Link to="/servers/$id" params={{ id: server.id }}>Manage<ArrowRight /></Link></Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="h-9 w-9" aria-label={`More actions for ${server.name}`} disabled={pending !== null}><MoreHorizontal /></Button></DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {menuItems()}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    {error && <p role="alert" className="mt-2 max-w-xs text-sm text-destructive">{error}</p>}
  </div>;

  return <>
    {dialog && createPortal(dialog, document.body)}
    <ContextMenu>
      <ContextMenuTrigger asChild tabIndex={0} className="focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring" aria-label={`Server ${server.name}`}>
        {children ? children(actions) : actions}
      </ContextMenuTrigger>
      <ContextMenuContent aria-label={`Actions for ${server.name}`}>
        <ContextMenuLabel>{server.name}</ContextMenuLabel>
        {menuItems(true)}
      </ContextMenuContent>
    </ContextMenu>
  </>;
}
