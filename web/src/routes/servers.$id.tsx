import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useCallback, useEffect, useState } from 'react';
import { ArrowLeft, Play, RefreshCw, Square, Trash2 } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server, ServerState } from '../api/types';
import { AppShell } from '../components/app-shell';
import { BackupsPanel } from '../components/backups-panel';
import { ConsoleViewer } from '../components/console-viewer';
import { FilesPanel } from '../components/files-panel';
import { InstallPanel } from '../components/install-panel';
import { ModsPanel } from '../components/mods-panel';
import { OpsPanel } from '../components/ops-panel';
import { PlayersPanel } from '../components/players-panel';
import { PropertiesEditor } from '../components/properties-editor';
import { RequireAuth } from '../components/require-auth';
import { ServerSettings } from '../components/server-settings';
import { StatusBadge } from '../components/status-badge';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Separator } from '../components/ui/separator';

export const Route = createFileRoute('/servers/$id')({
  component: ServerDetailRoute,
});

function ServerDetailRoute() {
  const { id } = Route.useParams();
  const navigate = useNavigate();
  const [server, setServer] = useState<Server | null>(null);
  const [statusState, setStatusState] = useState<ServerState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<boolean>(false);

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
    const confirmed = window.confirm(`Delete server "${server.name}"?`);
    if (!confirmed) return;
    try {
      await api.deleteServer(server.id);
      void navigate({ to: '/dashboard', replace: true });
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to delete server');
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
              disabled={busy || state === 'stopped' || state === 'starting' || state === 'stopping'}
              onClick={() => void run(() => api.restartServer(server.id))}
            >
              <RefreshCw className="h-4 w-4" /> Restart
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

        <div className="grid gap-4 lg:grid-cols-3">
          <div className="lg:col-span-2">
            <ConsoleViewer serverId={server.id} running={state === 'running'} />
            <div className="mt-4">
              <PlayersPanel server={server} />
            </div>
          </div>
          <div className="space-y-4">
            <ServerSettings
              key={`${server.name}-${server.ram_mb}`}
              server={server}
              onSaved={(s) => setServer(s)}
            />
            <InstallPanel
              serverId={server.id}
              serverType={server.server_type}
              onInstalled={loadServer}
            />
            <OpsPanel server={server} refreshKey={state} />
            <ModsPanel server={server} />
            <PropertiesEditor server={server} />
            <FilesPanel server={server} />
            <BackupsPanel server={server} />
          </div>
        </div>

        <Separator className="my-6" />
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">About</CardTitle>
            <CardDescription>Instance metadata.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground">Container ID</dt>
              <dd>{server.container_id ?? 'not created'}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Created</dt>
              <dd>{new Date(server.created_at).toLocaleString()}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Updated</dt>
              <dd>{new Date(server.updated_at).toLocaleString()}</dd>
            </div>
          </CardContent>
        </Card>
      </AppShell>
    </RequireAuth>
  );
}
