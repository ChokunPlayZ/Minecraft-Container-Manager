import { useEffect, useState, type FormEvent } from 'react';
import { api, ApiError } from '../api/client';
import type { ExtraPort, JavaRelease, Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';
import { Plus, Trash2, Globe, AlertTriangle, RotateCcw } from 'lucide-react';
import { ServerDNSCard } from './server-dns-card';

type SettingsTab = 'general' | 'advanced' | 'dns';

function genId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  // Fallback for non-secure contexts (e.g. plain HTTP on a LAN address)
  // where crypto.randomUUID is not available.
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

export function ServerSettings({
  server,
  onSaved,
  onRebuild,
  onDnsChanged,
}: {
  server: Server;
  onSaved: (s: Server) => void;
  onRebuild?: () => void;
  onDnsChanged?: (joinAddress: string) => void;
}) {
  const [name, setName] = useState(server.name);
  const [ramMb, setRamMb] = useState(server.ram_mb);
  const [hostPort, setHostPort] = useState(server.host_port);
  const [javaVersion, setJavaVersion] = useState(server.java_version ?? 21);
  const [javaReleases, setJavaReleases] = useState<JavaRelease[]>([
    { version: 25, is_lts: true, name: 'Java 25 (LTS)' },
    { version: 24, is_lts: false, name: 'Java 24' },
    { version: 21, is_lts: true, name: 'Java 21 (LTS - Recommended)' },
    { version: 17, is_lts: true, name: 'Java 17 (LTS)' },
    { version: 11, is_lts: true, name: 'Java 11 (LTS)' },
    { version: 8, is_lts: true, name: 'Java 8 (LTS)' },
  ]);
  const [cpuLimit, setCpuLimit] = useState(server.cpu_limit ?? 0);
  const [memoryLimitMb, setMemoryLimitMb] = useState(server.memory_limit_mb ?? 0);
  const [backupEnabled, setBackupEnabled] = useState(server.backup_enabled ?? true);
  const [backupInterval, setBackupInterval] = useState(server.backup_interval_minutes ?? 720);
  const [extraPorts, setExtraPorts] = useState<ExtraPort[]>(server.extra_ports ?? []);
  const [tab, setTab] = useState<SettingsTab>('general');
  const [error, setError] = useState<string | null>(null);
  const [savedNotice, setSavedNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api.javaVersions()
      .then((releases) => {
        if (!cancelled && releases && releases.length > 0) {
          setJavaReleases(releases);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  function newPort(): ExtraPort {
    return {
      id: genId(),
      description: '',
      host_port: 0,
      container_port: 0,
      protocol: 'tcp',
    };
  }

  function updatePort(id: string, patch: Partial<ExtraPort>) {
    setExtraPorts((cur) => cur.map((p) => (p.id === id ? { ...p, ...patch } : p)));
  }

  function removePort(id: string) {
    setExtraPorts((cur) => cur.filter((p) => p.id !== id));
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setSavedNotice(null);
    try {
      const updated = await api.updateServer(server.id, {
        name: name.trim(),
        ram_mb: ramMb,
        host_port: hostPort,
        java_version: javaVersion,
        cpu_limit: cpuLimit,
        memory_limit_mb: memoryLimitMb,
        backup_enabled: backupEnabled,
        backup_interval_minutes: backupInterval,
        extra_ports: extraPorts,
      });
      if (updated.needs_rebuild) {
        setSavedNotice('Settings saved. Container rebuild required for memory and container settings to take effect.');
      } else {
        setSavedNotice('Settings saved successfully.');
      }
      onSaved(updated);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to save settings');
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Settings</CardTitle>
        <CardDescription>
          {tab === 'dns'
            ? 'Configure Cloudflare SRV records and domain routing.'
            : 'Update the server name, resources, backups, and published ports.'}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <div className="flex items-center gap-1 border-b">
            <button
              type="button"
              onClick={() => setTab('general')}
              className={
                tab === 'general'
                  ? 'inline-flex items-center border-b-2 border-primary px-3 py-2 text-sm font-medium text-foreground'
                  : 'inline-flex items-center border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground hover:text-foreground'
              }
            >
              General
            </button>
            <button
              type="button"
              onClick={() => setTab('advanced')}
              className={
                tab === 'advanced'
                  ? 'inline-flex items-center border-b-2 border-primary px-3 py-2 text-sm font-medium text-foreground'
                  : 'inline-flex items-center border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground hover:text-foreground'
              }
            >
              Advanced
            </button>
            <button
              type="button"
              onClick={() => setTab('dns')}
              className={
                tab === 'dns'
                  ? 'inline-flex items-center border-b-2 border-primary px-3 py-2 text-sm font-medium text-foreground'
                  : 'inline-flex items-center border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground hover:text-foreground'
              }
            >
              <Globe className="h-3.5 w-3.5 mr-1.5" />
              DNS / Routing
            </button>
          </div>

          {tab === 'dns' ? (
            <ServerDNSCard server={server} onDnsChanged={onDnsChanged} bare={true} />
          ) : (
            <form onSubmit={onSubmit} className="space-y-4">
              {server.needs_rebuild && (
                <div
                  data-testid="settings-rebuild-alert"
                  className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-xs text-amber-900 dark:border-amber-500/30 dark:bg-amber-950/30 dark:text-amber-200 shadow-2xs"
                >
                  <div className="flex items-start gap-2.5">
                    <AlertTriangle className="h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400 mt-0.5" />
                    <div className="space-y-0.5">
                      <p className="font-semibold">Container Rebuild Required</p>
                      <p>
                        Memory or container settings have changed. The container must be rebuilt to apply these settings.
                      </p>
                      {server.rebuild_reasons && server.rebuild_reasons.length > 0 && (
                        <ul className="list-disc pl-4 space-y-0.5 mt-1 text-amber-800/80 dark:text-amber-300/80">
                          {server.rebuild_reasons.map((r, i) => (
                            <li key={i}>{r}</li>
                          ))}
                        </ul>
                      )}
                    </div>
                  </div>
                  {onRebuild && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={onRebuild}
                      className="shrink-0 gap-1.5 border-amber-500/40 bg-amber-500/20 text-amber-900 hover:bg-amber-500/30 dark:border-amber-400/40 dark:bg-amber-500/20 dark:text-amber-100"
                    >
                      <RotateCcw className="h-3 w-3" />
                      <span>Rebuild container</span>
                    </Button>
                  )}
                </div>
              )}

          {tab === 'general' ? (
            <div className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="edit-name">Name</Label>
                <Input id="edit-name" required value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="edit-ram">RAM (MB)</Label>
                <Input
                  id="edit-ram"
                  type="number"
                  min={512}
                  step={256}
                  value={ramMb}
                  onChange={(e) => setRamMb(Number(e.target.value))}
                />
                <p className="text-xs text-muted-foreground">
                  Changes take effect after rebuilding the container.
                </p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="edit-java">Java Runtime Version</Label>
                <Select
                  id="edit-java"
                  value={String(javaVersion)}
                  onChange={(e) => setJavaVersion(parseInt(e.target.value, 10))}
                >
                  {javaReleases.map((jr) => (
                    <option key={jr.version} value={jr.version}>
                      {jr.name}
                    </option>
                  ))}
                </Select>
                <p className="text-xs text-muted-foreground">
                  Changes take effect after rebuilding the container.
                </p>
              </div>
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <Label htmlFor="edit-port">Game port (host)</Label>
                  <label className="flex items-center gap-1.5 cursor-pointer text-xs text-muted-foreground hover:text-foreground">
                    <input
                      type="checkbox"
                      checked={hostPort === 0}
                      onChange={(e) => setHostPort(e.target.checked ? 0 : 25565)}
                      className="rounded border-border text-primary focus:ring-primary h-3.5 w-3.5"
                    />
                    <span>Behind Proxy (Port 0)</span>
                  </label>
                </div>
                {hostPort === 0 ? (
                  <div className="flex h-9 items-center justify-between px-3 rounded-lg border border-border bg-muted/40 text-xs font-mono text-muted-foreground">
                    <span>Internal Network: mcm-{server.id}</span>
                    <span className="text-[10px] text-primary font-sans font-medium">No host port</span>
                  </div>
                ) : (
                  <Input
                    id="edit-port"
                    type="number"
                    min={0}
                    max={65535}
                    value={hostPort}
                    onChange={(e) => setHostPort(Number(e.target.value))}
                  />
                )}
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span>
                    {hostPort === 0
                      ? 'Accessible to proxies on the panel network via "mcm-:id".'
                      : 'Changes take effect after rebuilding the container.'}
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      const cPort =
                        server.server_type === 'geysermc'
                          ? 19132
                          : server.server_type === 'velocity' ||
                              server.server_type === 'waterfall' ||
                              server.server_type === 'bungeecord'
                            ? 25577
                            : 25565;
                      void navigator.clipboard.writeText(`mcm-${server.id}:${cPort}`);
                    }}
                    className="text-primary hover:underline text-[11px] font-mono cursor-pointer"
                  >
                    Copy Docker addr
                  </button>
                </div>
              </div>
              <fieldset className="space-y-3">
                <legend className="text-sm font-medium">Additional ports</legend>
                <p className="text-xs text-muted-foreground">
                  Publish extra container ports to the host (e.g. a WebUI over TCP or
                  a Bedrock/Geyser adapter over UDP).
                </p>
                {extraPorts.length === 0 && (
                  <p className="text-sm text-muted-foreground">No additional ports configured.</p>
                )}
                {extraPorts.map((p) => (
                  <div key={p.id} className="space-y-3 rounded-md border p-3">
                    <div className="flex items-start gap-3">
                      <div className="flex-1 space-y-1">
                        <Label className="text-xs" htmlFor={`ep-desc-${p.id}`}>Description</Label>
                        <Input
                          id={`ep-desc-${p.id}`}
                          placeholder="e.g. WebUI"
                          value={p.description}
                          onChange={(e) => updatePort(p.id, { description: e.target.value })}
                        />
                      </div>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="mt-6 shrink-0"
                        aria-label={`Remove ${p.description || 'extra port'}`}
                        onClick={() => removePort(p.id)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                      <div className="space-y-1">
                        <Label className="text-xs" htmlFor={`ep-host-${p.id}`}>Host port</Label>
                        <Input
                          id={`ep-host-${p.id}`}
                          type="number"
                          min={1}
                          max={65535}
                          value={p.host_port || ''}
                          onChange={(e) => updatePort(p.id, { host_port: Number(e.target.value) })}
                        />
                      </div>
                      <div className="space-y-1">
                        <Label className="text-xs" htmlFor={`ep-cont-${p.id}`}>Container port</Label>
                        <Input
                          id={`ep-cont-${p.id}`}
                          type="number"
                          min={1}
                          max={65535}
                          value={p.container_port || ''}
                          onChange={(e) => updatePort(p.id, { container_port: Number(e.target.value) })}
                        />
                      </div>
                      <div className="space-y-1">
                        <Label className="text-xs" htmlFor={`ep-proto-${p.id}`}>Protocol</Label>
                        <Select
                          id={`ep-proto-${p.id}`}
                          value={p.protocol}
                          onChange={(e) => updatePort(p.id, { protocol: e.target.value as 'tcp' | 'udp' })}
                        >
                          <option value="tcp">TCP</option>
                          <option value="udp">UDP</option>
                        </Select>
                      </div>
                    </div>
                  </div>
                ))}
                <Button type="button" variant="outline" onClick={() => setExtraPorts((cur) => [...cur, newPort()])}>
                  <Plus className="h-4 w-4" />
                  Add port
                </Button>
              </fieldset>
            </div>
          ) : (
            <div className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="edit-cpu-limit">CPU limit (cores)</Label>
                <Input
                  id="edit-cpu-limit"
                  type="number"
                  min={0}
                  step={0.5}
                  value={cpuLimit}
                  onChange={(e) => setCpuLimit(Number(e.target.value))}
                />
                <p className="text-xs text-muted-foreground">0 means no CPU quota.</p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="edit-memory-limit">Memory limit (MB)</Label>
                <Input
                  id="edit-memory-limit"
                  type="number"
                  min={0}
                  step={64}
                  value={memoryLimitMb}
                  onChange={(e) => setMemoryLimitMb(Number(e.target.value))}
                />
                <p className="text-xs text-muted-foreground">0 allows the configured server RAM plus 2 GiB of overhead.</p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="edit-backup-interval">Automatic backup interval (minutes)</Label>
                <Input
                  id="edit-backup-interval"
                  type="number"
                  min={5}
                  step={5}
                  value={backupInterval}
                  disabled={!backupEnabled}
                  onChange={(e) => setBackupInterval(Number(e.target.value))}
                />
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={backupEnabled}
                  onChange={(e) => setBackupEnabled(e.target.checked)}
                />
                Enable automatic backups
              </label>
            </div>
          )}
          {error && <p className="text-sm text-destructive">{error}</p>}
          {savedNotice && (
            <p className="text-xs text-amber-600 dark:text-amber-400 font-medium">
              {savedNotice}
            </p>
          )}
          <Button type="submit" disabled={busy}>
            {busy ? 'Saving...' : 'Save'}
          </Button>
        </form>
      )}
        </div>
      </CardContent>
    </Card>
  );
}
