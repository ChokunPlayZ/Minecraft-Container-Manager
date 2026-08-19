import { useEffect, useState, type FormEvent } from 'react';
import { Loader2 } from 'lucide-react';
import { api, ApiError, type CreateServerInput } from '../api/client';
import type { ServerType, VersionInfo, VersionMeta } from '../api/types';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';

export function CreateServerDialog({ onCreated }: { onCreated: () => void }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [serverType, setServerType] = useState<ServerType>('paper');
  const [version, setVersion] = useState('');
  const [build, setBuild] = useState('');
  const [ramMb, setRamMb] = useState(2048);
  const [versions, setVersions] = useState<VersionMeta[]>([]);
  const [builds, setBuilds] = useState<VersionInfo[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [loadingBuilds, setLoadingBuilds] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoadingVersions(true);
    setError(null);
    api
      .jarVersions(serverType)
      .then((v) => {
        if (cancelled) return;
        setVersions(v);
        setVersion(v[0]?.name ?? '');
      })
      .catch((err) => !cancelled && setError(err instanceof ApiError ? err.detail : 'Failed to load versions'))
      .finally(() => !cancelled && setLoadingVersions(false));
    return () => {
      cancelled = true;
    };
  }, [open, serverType]);

  useEffect(() => {
    if (!open || !version) return;
    let cancelled = false;
    setLoadingBuilds(true);
    api
      .jarBuilds(serverType, version)
      .then((b) => {
        if (cancelled) return;
        setBuilds(b);
        setBuild(b[0]?.build ?? '');
      })
      .catch((err) => !cancelled && setError(err instanceof ApiError ? err.detail : 'Failed to load builds'))
      .finally(() => !cancelled && setLoadingBuilds(false));
    return () => {
      cancelled = true;
    };
  }, [open, serverType, version]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const input: CreateServerInput = {
      name: name.trim(),
      server_type: serverType,
      version,
      build,
      ram_mb: ramMb,
    };
    try {
      await api.createServer(input);
      setOpen(false);
      setName('');
      onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to create server');
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Button onClick={() => setOpen(true)}>Create server</Button>
      {open && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={() => setOpen(false)}>
          <div
            className="w-full max-w-md rounded-lg border bg-card p-6 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="text-lg font-semibold">Create server</h2>
            <form onSubmit={onSubmit} className="mt-4 space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="server-name">Name</Label>
                <Input
                  id="server-name"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="My survival world"
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <Label htmlFor="server-type">Type</Label>
                  <Select
                    id="server-type"
                    value={serverType}
                    onChange={(e) => setServerType(e.target.value as ServerType)}
                  >
                    <option value="paper">Paper</option>
                    <option value="fabric">Fabric</option>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="server-ram">RAM (MB)</Label>
                  <Input
                    id="server-ram"
                    type="number"
                    min={512}
                    step={256}
                    value={ramMb}
                    onChange={(e) => setRamMb(Number(e.target.value))}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <Label htmlFor="server-version">Version</Label>
                  {loadingVersions ? (
                    <div className="flex h-9 items-center text-muted-foreground">
                      <Loader2 className="h-4 w-4 animate-spin" />
                    </div>
                  ) : (
                    <Select
                      id="server-version"
                      value={version}
                      onChange={(e) => setVersion(e.target.value)}
                      disabled={versions.length === 0}
                    >
                      {versions.map((v) => (
                        <option key={v.name} value={v.name}>
                          {v.latest ?? v.name}
                        </option>
                      ))}
                    </Select>
                  )}
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="server-build">Build</Label>
                  {loadingBuilds ? (
                    <div className="flex h-9 items-center text-muted-foreground">
                      <Loader2 className="h-4 w-4 animate-spin" />
                    </div>
                  ) : (
                    <Select
                      id="server-build"
                      value={build}
                      onChange={(e) => setBuild(e.target.value)}
                      disabled={builds.length === 0}
                    >
                      {builds.map((b) => (
                        <option key={b.build} value={b.build}>
                          {b.build}
                        </option>
                      ))}
                    </Select>
                  )}
                </div>
              </div>
              {error && <p className="text-sm text-destructive">{error}</p>}
              <div className="flex justify-end gap-2">
                <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={busy || loadingVersions || loadingBuilds}>
                  {busy ? 'Creating...' : 'Create'}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  );
}
