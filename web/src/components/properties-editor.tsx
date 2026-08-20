import { useCallback, useEffect, useState } from 'react';
import { RefreshCw, Save } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Textarea } from './ui/textarea';

export function PropertiesEditor({ server }: { server: Server }) {
  const [content, setContent] = useState('');
  const [exists, setExists] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const load = useCallback(async () => {
    setBusy(true);
    setError(null);
    try {
      const props = await api.getProperties(server.id);
      setContent(props.content);
      setExists(props.exists);
      setLoaded(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load server.properties');
    } finally {
      setBusy(false);
    }
  }, [server.id]);

  useEffect(() => {
    void load();
  }, [load]);

  async function save() {
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      const props = await api.saveProperties(server.id, content);
      setContent(props.content);
      setExists(true);
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to save server.properties');
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between gap-2">
          <div>
            <CardTitle className="text-base">Server Properties</CardTitle>
            <CardDescription>Edit the full raw server.properties file.</CardDescription>
          </div>
          <Button variant="ghost" size="icon" onClick={() => void load()} aria-label="Reload properties">
            <RefreshCw className="h-4 w-4" />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {exists === false && !error && (
          <p className="text-sm text-muted-foreground">
            No server.properties yet — it will be generated when the server first starts. Save to
            create one.
          </p>
        )}
        {error && <p className="text-sm text-destructive">{error}</p>}
        {loaded && (
          <Textarea
            className="min-h-[260px] font-mono text-xs leading-relaxed whitespace-pre"
            value={content}
            spellCheck={false}
            onChange={(e) => setContent(e.target.value)}
            placeholder="# Minecraft server properties"
          />
        )}
        {saved && <p className="text-sm text-emerald-600">Saved.</p>}
        <div className="flex justify-end">
          <Button onClick={() => void save()} disabled={busy || !loaded}>
            <Save className="h-4 w-4" /> {busy ? 'Saving...' : 'Save'}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
