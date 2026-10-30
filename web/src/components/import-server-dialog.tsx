import { useId, useState, type FormEvent } from 'react';
import { Loader2, Upload, X } from 'lucide-react';
import { api, ApiError, type ImportServerInput } from '../api/client';
import type { Server, ServerType } from '../api/types';
import { Button } from './ui/button';
import { DialogBackdrop } from './ui/dialog-backdrop';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { ProgressBar } from './ui/progress';
import { Select } from './ui/select';
import { ServerZipUpload } from './server-zip-upload';
import { ImportServerReleaseFields } from './import-server-release-fields';

const serverTypes: Array<[ServerType, string]> = [
  ['paper', 'Paper'], ['purpur', 'Purpur'], ['folia', 'Folia'], ['pufferfish', 'Pufferfish'],
  ['leaf', 'Leaf'], ['spigot', 'Spigot'], ['vanilla', 'Vanilla'], ['fabric', 'Fabric'],
  ['quilt', 'Quilt'], ['forge', 'Forge'], ['neoforge', 'NeoForge'], ['sponge', 'Sponge'],
  ['youer', 'Youer'], ['mohist', 'Mohist'], ['ketting', 'Ketting'], ['crucible', 'Crucible'],
  ['velocity', 'Velocity'], ['waterfall', 'Waterfall'], ['bungeecord', 'BungeeCord'],
  ['limbo', 'Limbo'], ['nanolimbo', 'NanoLimbo'], ['geysermc', 'GeyserMC'], ['custom', 'Custom JAR'],
];

export function ImportServerDialog({ onImported }: { onImported: (server: Server) => void }) {
  const id = useId();
  const [open, setOpen] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [name, setName] = useState('');
  const [serverType, setServerType] = useState('exported');
  const [version, setVersion] = useState('');
  const [build, setBuild] = useState('');
  const [ram, setRam] = useState('');
  const [java, setJava] = useState('');
  const [privateServer, setPrivateServer] = useState(false);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);

  function startImport() {
    setFile(null); setName(''); setServerType('exported'); setVersion(''); setBuild(''); setRam(''); setJava('');
    setPrivateServer(false); setError(null); setProgress(0); setOpen(true);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!file || busy) return;
    const settings: ImportServerInput = {
      name: name.trim() || undefined,
      server_type: serverType === 'exported' ? undefined : serverType as ServerType,
      version: version.trim() || undefined,
      build: build.trim() || undefined,
      ram_mb: ram ? Number(ram) : undefined,
      java_version: java ? Number(java) : undefined,
      no_host_port: privateServer,
    };
    setBusy(true); setError(null); setProgress(0);
    try {
      const server = await api.importServer(file, settings, (loaded, total) => {
        if (total > 0) setProgress(Math.min(100, Math.round(loaded / total * 100)));
      });
      setOpen(false);
      onImported(server);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Could not import the server. Check your connection and available storage, then try again.');
    } finally {
      setBusy(false);
    }
  }

  return <>
    <Button variant="outline" onClick={startImport}><Upload className="h-4 w-4" />Import server</Button>
    {open && <DialogBackdrop className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onDismiss={() => { if (!busy) setOpen(false); }}
      onMouseDown={(event) => { if (event.target === event.currentTarget && !busy) setOpen(false); }}>
      <div className="max-h-[90dvh] w-full max-w-lg overflow-y-auto rounded-2xl border bg-card p-6 shadow-2xl" aria-describedby={`${id}-description`}>
        <div className="flex items-start justify-between gap-4 border-b pb-4">
          <div><h2 className="text-lg font-semibold">Import server</h2><p id={`${id}-description`} className="mt-1 text-sm text-muted-foreground">Restore a server from an MCM export ZIP.</p></div>
          <Button type="button" variant="ghost" size="icon" aria-label="Close import dialog" disabled={busy} onClick={() => setOpen(false)}><X className="h-4 w-4" /></Button>
        </div>
        <form onSubmit={(event) => void submit(event)} className="mt-5 space-y-5">
          <fieldset disabled={busy} className="min-w-0 space-y-5">
            <ServerZipUpload file={file} disabled={busy} onChange={(selected) => { setFile(selected); setError(null); }} />
            <div className="space-y-2"><Label htmlFor={`${id}-name`}>Server name (optional)</Label><Input id={`${id}-name`} value={name} maxLength={255} onChange={(event) => setName(event.target.value)} placeholder="Use the exported server name" /></div>
            <details className="border-y py-4">
              <summary className="cursor-pointer text-sm font-medium">Server settings</summary>
              <p className="mt-3 text-xs text-muted-foreground">New exports include these settings. For older ZIPs, enter a server name, choose the server type and enter its version. RAM defaults to 2048 MB and Java is selected automatically.</p>
              <div className="mt-4 grid gap-4 sm:grid-cols-2">
                <div className="space-y-2 sm:col-span-2"><Label htmlFor={`${id}-type`}>Server type</Label><Select id={`${id}-type`} value={serverType} onValueChange={(value) => { setServerType(value); setVersion(''); setBuild(''); }} disabled={busy}><option value="exported">Use exported setting</option>{serverTypes.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</Select></div>
                <ImportServerReleaseFields key={serverType} serverType={serverType === 'exported' ? undefined : serverType as ServerType} version={version} build={build} disabled={busy}
                  onVersionChange={(value) => { setVersion(value); setBuild(''); }} onBuildChange={setBuild} />
                <div className="space-y-2"><Label htmlFor={`${id}-ram`}>RAM (MB)</Label><Input id={`${id}-ram`} type="number" min={1} step={1} value={ram} onChange={(event) => setRam(event.target.value)} placeholder="Use exported setting" /></div>
                <div className="space-y-2"><Label htmlFor={`${id}-java`}>Java version</Label><Input id={`${id}-java`} type="number" min={8} max={99} step={1} value={java} onChange={(event) => setJava(event.target.value)} placeholder="Use exported setting" /></div>
              </div>
            </details>
            <label className="flex items-start gap-3 text-sm"><input type="checkbox" className="mt-1 h-4 w-4 accent-primary" checked={privateServer} onChange={(event) => setPrivateServer(event.target.checked)} /><span>Keep server private<span className="mt-1 block text-xs text-muted-foreground">Use the internal container network without publishing a game port.</span></span></label>
          </fieldset>
          <p className="text-xs text-muted-foreground">The server stays stopped after import. A free game port is assigned unless private. Configure additional ports in server settings.</p>
          {busy && <div role="status" aria-live="polite"><ProgressBar value={progress === 100 ? null : progress} label={progress === 100 ? 'Importing server files…' : 'Uploading server ZIP…'} /><p className="mt-2 text-xs text-muted-foreground">Keep this window open until the import finishes.</p></div>}
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
          <div className="flex flex-wrap justify-end gap-2 border-t pt-4"><Button type="button" variant="outline" disabled={busy} onClick={() => setOpen(false)}>Cancel</Button><Button type="submit" disabled={!file || busy}>{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}{busy ? 'Importing…' : 'Import server'}</Button></div>
        </form>
      </div>
    </DialogBackdrop>}
  </>;
}
