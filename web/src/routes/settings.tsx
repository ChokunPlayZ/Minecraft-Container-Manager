import { createFileRoute } from '@tanstack/react-router';
import { AppShell } from '../components/app-shell';
import { RequireAuth } from '../components/require-auth';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';

export const Route = createFileRoute('/settings')({
  component: SettingsRoute,
});

function SettingsRoute() {
  return (
    <RequireAuth>
      <AppShell>
        <div className="mb-6">
          <h1 className="text-2xl font-semibold">Settings</h1>
          <p className="text-sm text-muted-foreground">Panel preferences.</p>
        </div>

        <Card className="max-w-xl">
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Gateway</CardTitle>
            <CardDescription>
              The gateway and wake-on-rejoin feature have been removed. Each server
              container publishes and owns its own game port directly.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Additional settings will appear here in a future update.
            </p>
          </CardContent>
        </Card>
      </AppShell>
    </RequireAuth>
  );
}
