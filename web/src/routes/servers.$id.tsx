import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useCallback, useEffect, useState } from 'react';
import { ArrowLeft, FolderOpen, LayoutGrid, Play, RefreshCw, RotateCcw, Square, Trash2, Zap } from 'lucide-react';
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
  const [tab, setTab] = useState<'overview' | 'files'>('overview');
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
        <Link to="/dashboard" className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="h-4 w-4" /> Back to servers
        </Link>

        <div className="mb-6 flex flex-wrap items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-semibold">{server.name}</h1>
              <StatusBadge state={state} />
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              {server.server_type} · {server.version} (build {server.build}) · Port {server.host_port} ·{' '}
              {server.ram_mb} MB
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              disabled={busy || state === 'running' || state === 'starting'}
              onClick={() => void run(() => api.startServer(server.id))}
            >
              <Play className="h-4 w-4" /> Start
            </Button>
            <Button
              variant="outline"
              disabled={busy || state === 'stopped' || state === 'stopping'}
              onClick={() => void run(() => api.stopServer(server.id))}
            >
              <Square className="h-4 w-4" /> Stop
            </Button>
            <Button
              variant="outline"
              className="text-destructive hover:text-destructive"
              disabled={busy || state === 'stopped' || state === 'stopping'}
              onClick={() => void handleKill()}
            >
              <Zap className="h-4 w-4" /> Kill
            </Button>
            <Button
              variant="outline"
              disabled={busy || state === 'stopped' || state === 'starting' || state === 'stopping'}
              onClick={() => void run(() => api.restartServer(server.id))}
            >
              <RefreshCw className="h-4 w-4" /> Restart
            </Button>
            <Button variant="outline" disabled={busy} onClick={() => void handleRecreate()}>
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

        {/* Tabs */}
        <div className="mb-4 flex items-center gap-1 border-b">
          <button
            type="button"
            onClick={() => setTab('overview')}
            className={
              tab === 'overview'
                ? 'inline-flex items-center gap-2 border-b-2 border-primary px-3 py-2 text-sm font-medium text-foreground'
                : 'inline-flex items-center gap-2 border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground hover:text-foreground'
            }
          >
            <LayoutGrid className="h-4 w-4" /> Overview
          </button>
          <button
            type="button"
            onClick={() => setTab('files')}
            className={
              tab === 'files'
                ? 'inline-flex items-center gap-2 border-b-2 border-primary px-3 py-2 text-sm font-medium text-foreground'
                : 'inline-flex items-center gap-2 border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground hover:text-foreground'
            }
          >
            <FolderOpen className="h-4 w-4" /> Files
          </button>
        </div>

        {tab === 'files' ? (
          <FileManager server={server} />
        ) : (
          <div className="space-y-4">
            <div className="grid gap-4 lg:grid-cols-3">
              <div className="space-y-4 lg:col-span-2">
                <ConsoleViewer serverId={server.id} running={state === 'running'} />
                <PlayersPanel server={server} />
              </div>
              <div className="space-y-4">
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

            <div className="grid gap-4 lg:grid-cols-2">
              <OpsPanel server={server} refreshKey={state} />
              <WhitelistPanel server={server} />
            </div>

            <div className="grid gap-4 lg:grid-cols-3">
              <ModsPanel server={server} />
              <PropertiesEditor server={server} />
              <BackupsPanel server={server} />
            </div>
          </div>
        )}
      </AppShell>
      </RequireAuth>
    </>
  );
}
