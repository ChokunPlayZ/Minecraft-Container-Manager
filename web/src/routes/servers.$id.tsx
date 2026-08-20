import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useCallback, useEffect, useState } from 'react';
import {
  Activity,
  ArrowLeft,
  FolderOpen,
  LayoutGrid,
  Play,
  RefreshCw,
  RotateCcw,
  Settings2,
  Square,
  Trash2,
  Zap,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server, ServerState } from '../api/types';
import { AppShell } from '../components/app-shell';
import { BackupsPanel } from '../components/backups-panel';
import { ConsoleViewer } from '../components/console-viewer';
import { FileManager } from '../components/file-manager';
import { InstallPanel } from '../components/install-panel';
import { ModsPanel } from '../components/mods-panel';
import { OpsPanel } from '../components/ops-panel';
import { PlayersPanel } from '../components/players-panel';
import { PropertiesEditor } from '../components/properties-editor';
import { RequireAuth } from '../components/require-auth';
import { ServerSettings } from '../components/server-settings';
import { StatusBadge } from '../components/status-badge';
import { WhitelistPanel } from '../components/whitelist-panel';
import { Button } from '../components/ui/button';
import { Card, CardContent } from '../components/ui/card';
import { useModal } from '../components/ui/modal';

export const Route = createFileRoute('/servers/$id')({
  component: ServerDetailRoute,
});

function ServerDetailRoute() {
  const { id } = Route.useParams();
  const navigate = useNavigate();
  const [tab, setTab] = useState<'overview' | 'files' | 'settings'>('overview');
  const [server, setServer] = useState<Server | null>(null);
  const [statusState, setStatusState] = useState<ServerState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<boolean>(false);
  const { confirm, dialog } = useModal();

  const loadServer = useCallback(async () => {
    setError(null);
    try {
      const s = await api.getServer(id);
      setServer(s);
      setStatusState(s.state);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load server');
    }
  }, [id]);

  const pollStatus = useCallback(async () => {
    if (!server) return;
    try {
      const status = await api.serverStatus(id);
      setStatusState(status.state);
    } catch {
      /* keep last known */
    }
  }, [id, server]);

  useEffect(() => {
    void loadServer();
  }, [loadServer]);

  useEffect(() => {
    if (!server) return;
    void pollStatus();
    const timer = setInterval(() => void pollStatus(), 3000);
    return () => clearInterval(timer);
  }, [server, pollStatus]);

  const run = useCallback(
    async (fn: () => Promise<Server>) => {
      if (!server) return;
      setBusy(true);
      setError(null);
      try {
        const updated = await fn();
        setServer(updated);
        setStatusState(updated.state);
      } catch (err) {
        setError(err instanceof ApiError ? err.detail : 'Action failed');
      } finally {
        setBusy(false);
      }
    },
    [server],
  );

  async function handleDelete() {
    if (!server) return;
    const confirmed = await confirm(`Delete server "${server.name}"?`, { title: 'Delete server', confirmLabel: 'Delete', destructive: true });
    if (!confirmed) return;
    try {
      await api.deleteServer(server.id);
      void navigate({ to: '/dashboard', replace: true });
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to delete server');
    }
  }

  async function handleRecreate() {
    if (!server) return;
    const confirmed = await confirm(
      `Rebuild the container for "${server.name}"? This stops the server now and provisions a fresh container on the next start.`,
      { title: 'Rebuild container', confirmLabel: 'Rebuild', destructive: true },
    );
    if (!confirmed) return;
    try {
      setBusy(true);
      setError(null);
      const updated = await api.recreateServer(server.id);
      setServer(updated);
      setStatusState('stopped');
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to rebuild container');
    } finally {
      setBusy(false);
    }
  }

  async function handleKill() {
    if (!server) return;
    const confirmed = await confirm(
      `Force-kill "${server.name}"? This stops the container immediately without a graceful shutdown, which may skip saving worlds. Use this only if the server is unresponsive.`,
      { title: 'Force-kill server', confirmLabel: 'Force-kill', destructive: true },
    );
    if (!confirmed) return;
    try {
      setBusy(true);
      setError(null);
      const updated = await api.killServer(server.id);
      setServer(updated);
      setStatusState(updated.state);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to kill server');
    } finally {
      setBusy(false);
    }
  }

  if (!server) {
    return (
      <RequireAuth>
        <AppShell>
          <p className="text-muted-foreground">{error ?? 'Loading...'}</p>
        </AppShell>
      </RequireAuth>
    );
  }

  const state = statusState ?? server.state;

  return (
    <>
      {dialog}
      <RequireAuth>
      <AppShell>
        <Link to="/dashboard" className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground">
          <ArrowLeft className="h-4 w-4" /> Back to servers
        </Link>

        <div className="mb-5 flex flex-col gap-4 rounded-xl border bg-card p-4 shadow-sm sm:p-5 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-3">
              <h1 className="truncate text-2xl font-semibold tracking-tight">{server.name}</h1>
              <StatusBadge state={state} />
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
              <span className="inline-flex items-center gap-1.5"><Activity className="h-3.5 w-3.5" /> {server.server_type}</span>
              <span>{server.version} · build {server.build}</span>
              <span>Port {server.host_port}</span>
              <span>{server.ram_mb} MB RAM</span>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2 lg:justify-end">
            <Button
              variant="outline"
              size="sm"
              disabled={busy || state === 'running' || state === 'starting'}
              onClick={() => void run(() => api.startServer(server.id))}
            >
              <Play className="h-4 w-4" /> Start
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={busy || state === 'stopped' || state === 'stopping'}
              onClick={() => void run(() => api.stopServer(server.id))}
            >
              <Square className="h-4 w-4" /> Stop
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="text-destructive hover:text-destructive"
              disabled={busy || state === 'stopped' || state === 'stopping'}
              onClick={() => void handleKill()}
            >
              <Zap className="h-4 w-4" /> Kill
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={busy || state === 'stopped' || state === 'starting' || state === 'stopping'}
              onClick={() => void run(() => api.restartServer(server.id))}
            >
              <RefreshCw className="h-4 w-4" /> Restart
            </Button>
            <Button variant="outline" size="sm" disabled={busy} onClick={() => void handleRecreate()}>
              <RotateCcw className="h-4 w-4" /> Rebuild
            </Button>
            <Button variant="ghost" size="icon" onClick={() => void handleDelete()} aria-label="Delete server">
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>

        {error && (
          <Card className="mb-4 border-destructive/50">
            <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
          </Card>
        )}

        <div className="mb-5 flex items-center gap-1 overflow-x-auto rounded-lg border bg-muted/35 p-1" role="tablist" aria-label="Server sections">
          <button
            type="button"
            onClick={() => setTab('overview')}
            role="tab"
            aria-selected={tab === 'overview'}
            className={
              tab === 'overview'
                ? 'inline-flex shrink-0 items-center gap-2 rounded-md bg-background px-4 py-2 text-sm font-medium text-foreground shadow-sm'
                : 'inline-flex shrink-0 items-center gap-2 rounded-md px-4 py-2 text-sm text-muted-foreground transition-colors hover:bg-background/70 hover:text-foreground'
            }
          >
            <LayoutGrid className="h-4 w-4" /> Overview
          </button>
          <button
            type="button"
            onClick={() => setTab('files')}
            role="tab"
            aria-selected={tab === 'files'}
            className={
              tab === 'files'
                ? 'inline-flex shrink-0 items-center gap-2 rounded-md bg-background px-4 py-2 text-sm font-medium text-foreground shadow-sm'
                : 'inline-flex shrink-0 items-center gap-2 rounded-md px-4 py-2 text-sm text-muted-foreground transition-colors hover:bg-background/70 hover:text-foreground'
            }
          >
            <FolderOpen className="h-4 w-4" /> Files
          </button>
          <button
            type="button"
            onClick={() => setTab('settings')}
            role="tab"
            aria-selected={tab === 'settings'}
            className={
              tab === 'settings'
                ? 'inline-flex shrink-0 items-center gap-2 rounded-md bg-background px-4 py-2 text-sm font-medium text-foreground shadow-sm'
                : 'inline-flex shrink-0 items-center gap-2 rounded-md px-4 py-2 text-sm text-muted-foreground transition-colors hover:bg-background/70 hover:text-foreground'
            }
          >
            <Settings2 className="h-4 w-4" /> Settings
          </button>
        </div>

        {tab === 'files' ? (
          <FileManager server={server} />
        ) : tab === 'settings' ? (
          <div className="space-y-5">
            <div>
              <h2 className="text-lg font-semibold tracking-tight">Server settings</h2>
              <p className="mt-1 text-sm text-muted-foreground">Manage identity, resources, networking, backups, and the installed server jar.</p>
            </div>
            <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1.5fr)_minmax(320px,1fr)]">
              <ServerSettings
                key={`${server.name}-${server.ram_mb}-${server.host_port}`}
                server={server}
                onSaved={(s) => setServer(s)}
              />
              <InstallPanel
                serverId={server.id}
                serverType={server.server_type}
                onInstalled={loadServer}
              />
            </div>
          </div>
        ) : (
          <div className="space-y-5">
            <ConsoleViewer serverId={server.id} running={state === 'running'} />

            <div className="grid items-start gap-5 lg:grid-cols-2">
              <PlayersPanel server={server} />
              <div className="grid items-start gap-5 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">
                <OpsPanel server={server} refreshKey={state} />
                <WhitelistPanel server={server} />
              </div>
            </div>

            <div className="grid items-start gap-5 lg:grid-cols-2">
              <ModsPanel server={server} />
              <BackupsPanel server={server} />
            </div>

            <div className="grid items-start gap-5">
              <PropertiesEditor server={server} />
            </div>
          </div>
        )}
      </AppShell>
      </RequireAuth>
    </>
  );
}
