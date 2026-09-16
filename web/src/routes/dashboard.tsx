import { createFileRoute, Link } from '@tanstack/react-router';
import { Clock, Copy } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import { formatUptimeFromStartedAt } from '../lib/uptime';
import { AppShell } from '../components/app-shell';
import { CopyServerDialog } from '../components/copy-server-dialog';
import { CreateServerDialog } from '../components/create-server-dialog';
import { RequireAuth } from '../components/require-auth';
import { StatusBadge } from '../components/status-badge';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Skeleton } from '../components/ui/skeleton';

export const Route = createFileRoute('/dashboard')({
  component: DashboardRoute,
});

export function DashboardRoute() {
  const [servers, setServers] = useState<Server[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copyTargetServer, setCopyTargetServer] = useState<Server | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      setServers(await api.listServers());
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load servers');
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <RequireAuth>
      <AppShell>
        <div className="mb-6 flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold">Servers</h1>
            <p className="text-sm text-muted-foreground">Manage your Minecraft instances.</p>
          </div>
          <CreateServerDialog onCreated={() => void load()} />
        </div>

        {error && (
          <Card className="mb-4 border-destructive/50">
            <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
          </Card>
        )}

        {servers === null ? (
          <div className="space-y-3">
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
          </div>
        ) : servers.length === 0 ? (
          <Card>
            <CardContent className="py-12 text-center text-muted-foreground">
              No servers yet. Use the create button above to add one.
            </CardContent>
          </Card>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {servers.map((server) => (
              <Link
                key={server.id}
                to="/servers/$id"
                params={{ id: server.id }}
                className="block"
              >
                <Card className="h-full transition-colors hover:border-primary/50">
                  <CardHeader>
                    <div className="flex items-start justify-between gap-2">
                      <CardTitle className="truncate">{server.name}</CardTitle>
                      <div className="flex items-center gap-1.5 shrink-0">
                        {server.needs_rebuild && (
                          <span
                            data-testid="dashboard-rebuild-badge"
                            title="Container rebuild required to apply settings"
                            className="inline-flex items-center rounded-md border border-amber-500/40 bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:text-amber-400"
                          >
                            Rebuild required
                          </span>
                        )}
                        <StatusBadge state={server.state} />
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-6 w-6 text-muted-foreground hover:text-foreground"
                          title="Copy server"
                          onClick={(e) => {
                            e.preventDefault();
                            e.stopPropagation();
                            setCopyTargetServer(server);
                          }}
                        >
                          <Copy className="h-3 w-3" />
                        </Button>
                      </div>
                    </div>
                    <CardDescription>
                      {server.server_type} · {server.version} (build {server.build})
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <dl className="grid grid-cols-2 gap-2 text-sm">
                      <div>
                        <dt className="text-muted-foreground">RAM</dt>
                        <dd>{server.ram_mb} MB</dd>
                      </div>
                      <div>
                        <dt className="text-muted-foreground">Port</dt>
                        <dd>
                          {server.host_port > 0 ? (
                            server.host_port
                          ) : (
                            <span className="text-xs text-primary font-medium">Behind Proxy</span>
                          )}
                        </dd>
                      </div>
                      {server.state === 'running' && (server.started_at || server.uptime_seconds != null) && (
                        <div
                          data-testid="dashboard-server-uptime"
                          className="col-span-2 flex items-center justify-between border-t pt-2 mt-1 text-xs"
                        >
                          <span className="text-muted-foreground flex items-center gap-1">
                            <Clock className="h-3 w-3 text-emerald-500" /> Uptime
                          </span>
                          <span className="font-mono font-medium text-emerald-600 dark:text-emerald-400">
                            {formatUptimeFromStartedAt(server.started_at, server.uptime_seconds)}
                          </span>
                        </div>
                      )}
                    </dl>
                  </CardContent>
                </Card>
              </Link>
            ))}
          </div>
        )}

        {copyTargetServer && (
          <CopyServerDialog
            server={copyTargetServer}
            open={!!copyTargetServer}
            onClose={() => setCopyTargetServer(null)}
            onCopied={() => {
              void load();
            }}
          />
        )}
      </AppShell>
    </RequireAuth>
  );
}
