import { createFileRoute, Link } from '@tanstack/react-router';
import { useCallback, useEffect, useState } from 'react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import { AppShell } from '../components/app-shell';
import { CreateServerDialog } from '../components/create-server-dialog';
import { RequireAuth } from '../components/require-auth';
import { StatusBadge } from '../components/status-badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Skeleton } from '../components/ui/skeleton';

export const Route = createFileRoute('/dashboard')({
  component: DashboardRoute,
});

function DashboardRoute() {
  const [servers, setServers] = useState<Server[] | null>(null);
  const [error, setError] = useState<string | null>(null);

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
                      <CardTitle>{server.name}</CardTitle>
                      <StatusBadge state={server.state} />
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
                        <dd>{server.host_port}</dd>
                      </div>
                    </dl>
                  </CardContent>
                </Card>
              </Link>
            ))}
          </div>
        )}
      </AppShell>
    </RequireAuth>
  );
}
