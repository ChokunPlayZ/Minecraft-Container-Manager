import { useState, useEffect, type FormEvent } from 'react';
import { Network, CheckCircle2, AlertCircle, RefreshCw } from 'lucide-react';
import { api, ApiError } from '../api/client';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';

export function parsePortPoolPreview(input: string): { count: number; error?: string } {
  const parts = input.split(',').map((s) => s.trim()).filter(Boolean);
  if (parts.length === 0) {
    return { count: 0, error: 'Port pool cannot be empty' };
  }
  const seen = new Set<number>();
  for (const part of parts) {
    if (part.includes('-')) {
      const sides = part.split('-');
      if (sides.length !== 2) {
        return { count: 0, error: `Invalid range syntax: "${part}"` };
      }
      const start = parseInt(sides[0]?.trim() ?? '', 10);
      const end = parseInt(sides[1]?.trim() ?? '', 10);
      if (isNaN(start) || isNaN(end) || start < 1 || end > 65535 || start > end) {
        return { count: 0, error: `Invalid port range "${part}". Must be 1-65535 and start <= end.` };
      }
      if (end - start > 10000) {
        return { count: 0, error: `Port range "${part}" is too large (max 10,000 per range).` };
      }
      for (let p = start; p <= end; p++) {
        seen.add(p);
      }
    } else {
      const p = parseInt(part, 10);
      if (isNaN(p) || p < 1 || p > 65535) {
        return { count: 0, error: `Invalid port number: "${part}". Must be 1-65535.` };
      }
      seen.add(p);
    }
  }
  return { count: seen.size };
}

export function PortPoolSettingsCard() {
  const [poolInput, setPoolInput] = useState('');
  const [savedPool, setSavedPool] = useState('');
  const [availablePorts, setAvailablePorts] = useState<number[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedMsg, setSavedMsg] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    setError(null);
    try {
      const [settingsRes, portsRes] = await Promise.all([
        api.getSettings(),
        api.availablePorts(),
      ]);
      const current = settingsRes.settings?.port_pool || (Array.isArray(portsRes.pool) ? portsRes.pool.join(', ') : '') || '25565-25665';
      setPoolInput(current);
      setSavedPool(current);
      setAvailablePorts(portsRes.available || []);
    } catch (e) {
      if (e instanceof ApiError) setError(e.detail || e.message);
      else if (e instanceof Error) setError(e.message);
      else setError('Failed to load port settings');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const preview = parsePortPoolPreview(poolInput);
  const isModified = poolInput.trim() !== savedPool.trim();

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (preview.error) {
      setError(preview.error);
      return;
    }
    setSaving(true);
    setError(null);
    setSavedMsg(null);
    try {
      await api.putSettings({ port_pool: poolInput.trim() });
      setSavedPool(poolInput.trim());
      setSavedMsg('Port pool configuration saved successfully.');
      const portsRes = await api.availablePorts();
      setAvailablePorts(portsRes.available || []);
    } catch (e) {
      if (e instanceof ApiError) setError(e.detail || e.message);
      else if (e instanceof Error) setError(e.message);
      else setError('Failed to save port settings');
    } finally {
      setSaving(false);
    }
  }

  const inUseCount = Math.max(0, preview.count - availablePorts.length);

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <CardTitle className="text-base flex items-center gap-2">
              <Network className="h-4 w-4 text-primary" />
              Usable Port Pool
            </CardTitle>
            <CardDescription>
              Configure the pre-forwarded host port pool for your Minecraft servers. MCM will prefill new servers with available ports from this pool and block creating servers on already used ports.
            </CardDescription>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => void load()}
            disabled={loading || saving}
            title="Refresh port status"
          >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-5">
        {error && (
          <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}
        {savedMsg && (
          <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
            <CheckCircle2 className="h-4 w-4 shrink-0" />
            <span>{savedMsg}</span>
          </div>
        )}

        {/* Stats Grid */}
        <div className="grid grid-cols-3 gap-3">
          <div className="rounded-lg border bg-muted/40 p-3">
            <p className="text-xs font-medium text-muted-foreground">Total In Pool</p>
            <p className="mt-1 text-2xl font-bold">{preview.error ? '—' : preview.count}</p>
          </div>
          <div className="rounded-lg border bg-muted/40 p-3">
            <p className="text-xs font-medium text-muted-foreground">Available (Free)</p>
            <p className="mt-1 text-2xl font-bold text-emerald-600 dark:text-emerald-400">
              {loading ? '…' : availablePorts.length}
            </p>
          </div>
          <div className="rounded-lg border bg-muted/40 p-3">
            <p className="text-xs font-medium text-muted-foreground">In Use / Assigned</p>
            <p className="mt-1 text-2xl font-bold text-muted-foreground">
              {loading ? '…' : inUseCount}
            </p>
          </div>
        </div>

        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="port-pool-input">Port Pool Specification</Label>
              <div className="flex gap-1.5">
                <button
                  type="button"
                  onClick={() => setPoolInput('25565-25665')}
                  className="text-xs text-muted-foreground hover:text-primary underline cursor-pointer"
                >
                  Standard (25565-25665)
                </button>
                <span className="text-xs text-muted-foreground">•</span>
                <button
                  type="button"
                  onClick={() => setPoolInput('25565-25575')}
                  className="text-xs text-muted-foreground hover:text-primary underline cursor-pointer"
                >
                  Small (25565-25575)
                </button>
              </div>
            </div>
            <Input
              id="port-pool-input"
              value={poolInput}
              onChange={(e) => {
                setPoolInput(e.target.value);
                setSavedMsg(null);
                setError(null);
              }}
              placeholder="e.g. 25565-25665 or 25565, 25567, 25570-25580"
              className="font-mono text-sm"
              disabled={loading || saving}
            />
            {preview.error ? (
              <p className="text-xs text-destructive">{preview.error}</p>
            ) : (
              <p className="text-xs text-muted-foreground">
                Enter comma-separated port numbers or ranges (e.g., <code className="font-mono bg-muted px-1 py-0.5 rounded">25565-25665</code> or <code className="font-mono bg-muted px-1 py-0.5 rounded">25565, 25570-25580</code>).
              </p>
            )}
          </div>

          <div className="flex items-center gap-3">
            <Button type="submit" disabled={loading || saving || !isModified || !!preview.error}>
              {saving ? 'Saving…' : 'Save Changes'}
            </Button>
            {isModified && (
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setPoolInput(savedPool);
                  setError(null);
                  setSavedMsg(null);
                }}
                disabled={loading || saving}
              >
                Reset
              </Button>
            )}
          </div>
        </form>

        {/* Free Ports Sample */}
        {availablePorts.length > 0 && (
          <div className="space-y-2 border-t pt-4">
            <p className="text-xs font-medium text-muted-foreground">
              Next Available Ports (first {Math.min(availablePorts.length, 12)} of {availablePorts.length}):
            </p>
            <div className="flex flex-wrap gap-1.5">
              {availablePorts.slice(0, 12).map((port) => (
                <span
                  key={port}
                  className="rounded bg-muted px-2 py-0.5 font-mono text-xs font-medium text-foreground"
                >
                  {port}
                </span>
              ))}
              {availablePorts.length > 12 && (
                <span className="self-center text-xs text-muted-foreground">
                  +{availablePorts.length - 12} more
                </span>
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
