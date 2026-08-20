import { useState, type FormEvent } from 'react';
import { api, ApiError } from '../api/client';
import type { ExtraPort, Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';
import { Plus, Trash2 } from 'lucide-react';

export function ServerSettings({ server, onSaved }: { server: Server; onSaved: (s: Server) => void }) {
  const [name, setName] = useState(server.name);
  const [ramMb, setRamMb] = useState(server.ram_mb);
  const [cpuLimit, setCpuLimit] = useState(server.cpu_limit ?? 0);
  const [memoryLimitMb, setMemoryLimitMb] = useState(server.memory_limit_mb ?? 0);
  const [backupEnabled, setBackupEnabled] = useState(server.backup_enabled ?? true);
  const [backupInterval, setBackupInterval] = useState(server.backup_interval_minutes ?? 720);
  const [extraPorts, setExtraPorts] = useState<ExtraPort[]>(server.extra_ports ?? []);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function newPort(): ExtraPort {
    return {
      id: crypto.randomUUID(),
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
    try {
      const updated = await api.updateServer(server.id, {
        name: name.trim(),
        ram_mb: ramMb,
        cpu_limit: cpuLimit,
        memory_limit_mb: memoryLimitMb,
        backup_enabled: backupEnabled,
        backup_interval_minutes: backupInterval,
        extra_ports: extraPorts,
      });
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
        <CardDescription>Update the server name, resources, backups, and published ports.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} className="space-y-4">
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
          </div>
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
            <p className="text-xs text-muted-foreground">0 falls back to the RAM-derived default.</p>
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
          <fieldset className="space-y-2">
            <legend className="text-sm font-medium">Additional ports</legend>
            <p className="text-xs text-muted-foreground">
              Publish extra container ports to the host (e.g. a WebUI over TCP or
              a Bedrock/Geyser adapter over UDP).
            </p>
            {extraPorts.length === 0 && (
              <p className="text-sm text-muted-foreground">No additional ports configured.</p>
            )}
            {extraPorts.map((p) => (
              <div key={p.id} className="grid grid-cols-2 gap-2 rounded-md border p-2 sm:grid-cols-[1fr_auto_auto_auto_auto] sm:items-center">
                <div className="col-span-2 space-y-1 sm:col-span-1">
                  <Label className="text-xs" htmlFor={`ep-desc-${p.id}`}>Description</Label>
                  <Input
                    id={`ep-desc-${p.id}`}
                    placeholder="e.g. WebUI"
                    value={p.description}
                    onChange={(e) => updatePort(p.id, { description: e.target.value })}
                  />
                </div>
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
                <div className="col-span-2 flex items-end justify-end sm:col-span-1">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={`Remove ${p.description || 'extra port'}`}
                    onClick={() => removePort(p.id)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))}
            <Button type="button" variant="outline" onClick={() => setExtraPorts((cur) => [...cur, newPort()])}>
              <Plus className="h-4 w-4" />
              Add port
            </Button>
          </fieldset>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button type="submit" disabled={busy}>
            {busy ? 'Saving...' : 'Save'}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
