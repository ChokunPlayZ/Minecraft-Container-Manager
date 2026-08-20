import { useCallback, useEffect, useState } from 'react';
import { Power, Trash2 } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Mod, Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';

export function ModsPanel({ server }: { server: Server }) {
  const [items, setItems] = useState<Mod[]>([]);
  const [type, setType] = useState<'mods' | 'plugins'>('mods');
  const [error, setError] = useState<string | null>(null);

  const unsupported = server.server_type === 'vanilla';

  const load = useCallback(async () => {
    if (unsupported) {
      setItems([]);
      setError(null);
      return;
    }
    try {
      const res = await api.mods(server.id);
      setItems(res.items ?? []);
      setType(res.type);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load mods');
    }
  }, [server.id, unsupported]);

  useEffect(() => {
    void load();
  }, [load]);

  async function upload(file: File) {
    setError(null);
    try {
      await api.uploadMod(server.id, file);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Upload failed');
    }
  }

  async function toggle(mod: Mod) {
    setError(null);
    try {
      await api.setModEnabled(server.id, mod.name, !mod.enabled);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Update failed');
    }
  }

  async function remove(mod: Mod) {
    if (!window.confirm(`Delete ${mod.file}? This cannot be undone.`)) return;
    setError(null);
    try {
      await api.deleteMod(server.id, mod.name);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Delete failed');
    }
  }

  const label = type === 'mods' ? 'Mods' : 'Plugins';

  if (unsupported) {
    return (
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Mods &amp; plugins</CardTitle>
          <CardDescription>Manage installed artifacts.</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            This server type doesn&apos;t support mods or plugins.
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">{label}</CardTitle>
        <CardDescription>
          Upload, enable, and remove {label.toLowerCase()}.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <input
          type="file"
          accept=".jar"
          className="block w-full text-sm text-muted-foreground file:mr-3 file:rounded-md file:border file:border-border file:bg-secondary file:px-3 file:py-1.5 file:text-sm file:font-medium"
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) void upload(f);
          }}
        />

        {error && <p className="text-sm text-destructive">{error}</p>}

        <div className="space-y-2">
          {items.length === 0 && (
            <p className="text-sm text-muted-foreground">No {label.toLowerCase()} installed.</p>
          )}
          {items.map((m) => (
            <div
              key={m.name}
              className="flex items-center justify-between gap-2 rounded-md border p-2.5 text-sm"
            >
              <div className="min-w-0">
                <p className="truncate font-medium">{m.name}</p>
                <p className="truncate text-xs text-muted-foreground">
                  {m.file} · <span className={m.enabled ? 'text-emerald-600' : ''}>{m.enabled ? 'enabled' : 'disabled'}</span>
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button
                  variant="outline"
                  size="icon"
                  aria-label={m.enabled ? 'Disable' : 'Enable'}
                  title={m.enabled ? 'Disable' : 'Enable'}
                  onClick={() => void toggle(m)}
                >
                  <Power className={`h-4 w-4 ${m.enabled ? 'text-emerald-600' : 'text-muted-foreground'}`} />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Delete"
                  title="Delete"
                  onClick={() => void remove(m)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
