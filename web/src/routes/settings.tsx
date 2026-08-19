import { createFileRoute } from '@tanstack/react-router';
import { useEffect, useState, type FormEvent } from 'react';
import { api, ApiError } from '../api/client';
import { AppShell } from '../components/app-shell';
import { RequireAuth } from '../components/require-auth';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';

export const Route = createFileRoute('/settings')({
  component: SettingsRoute,
});

function SettingsRoute() {
  const [gatewayEnabled, setGatewayEnabled] = useState(false);
  const [defaultMessage, setDefaultMessage] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api
      .getSettings()
      .then((res) => {
        if (cancelled) return;
        const s = (res as { settings?: Record<string, string> }).settings ?? {};
        setGatewayEnabled(s.gateway_enabled === 'true');
        setDefaultMessage(
          s.wake_message_default ?? 'Server is waking up, please wait...',
        );
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.detail : 'Failed to load settings');
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      await api.putSettings({
        gateway_enabled: gatewayEnabled ? 'true' : 'false',
        wake_message_default: defaultMessage,
      });
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to save settings');
    } finally {
      setBusy(false);
    }
  }

  return (
    <RequireAuth>
      <AppShell>
        <div className="mb-6">
          <h1 className="text-2xl font-semibold">Settings</h1>
          <p className="text-sm text-muted-foreground">Gateway and wake-on-rejoin defaults.</p>
        </div>

        <Card className="max-w-xl">
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Gateway</CardTitle>
            <CardDescription>
              When enabled, MCM owns each server's public port and wakes sleeping servers on join.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={onSubmit} className="space-y-4">
              {loading ? (
                <p className="text-sm text-muted-foreground">Loading...</p>
              ) : (
                <>
                  <label className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={gatewayEnabled}
                      onChange={(e) => setGatewayEnabled(e.target.checked)}
                    />
                    Enable gateway (wake on connect)
                  </label>
                  <div className="space-y-1.5">
                    <Label htmlFor="default-wake-message">Default wait message</Label>
                    <Input
                      id="default-wake-message"
                      value={defaultMessage}
                      onChange={(e) => setDefaultMessage(e.target.value)}
                    />
                    <p className="text-xs text-muted-foreground">
                      Shown to players while a server boots. Per-server messages override this.
                    </p>
                  </div>
                </>
              )}
              {error && <p className="text-sm text-destructive">{error}</p>}
              {saved && <p className="text-sm text-emerald-600">Settings saved.</p>}
              <Button type="submit" disabled={busy || loading}>
                {busy ? 'Saving...' : 'Save'}
              </Button>
            </form>
          </CardContent>
        </Card>
      </AppShell>
    </RequireAuth>
  );
}
