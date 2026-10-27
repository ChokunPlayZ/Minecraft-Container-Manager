import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { ArrowRightLeft, Check, ChevronLeft, ChevronRight, Download, Globe2, Grid2X2, Loader2, Plus, RefreshCw, Trash2, Upload, X } from 'lucide-react';
import { ApiError } from '../api/client';
import { worldApi, type World, type WorldInput, type WorldList, type WorldChunk, type WorldRegion, type WorldRegions } from '../api/worlds';
import type { ServerState } from '../api/types';
import { formatBytes } from '../lib/formatters';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';
import { Switch } from './ui/switch';
import { Skeleton } from './ui/skeleton';
import { useModal } from './ui/modal';

const blank: WorldInput = { name: '', seed: '', level_type: 'minecraft:normal', generate_structures: true };
const message = (error: unknown, fallback: string) => error instanceof ApiError ? error.detail : fallback;
const generatorName = (value: string) => ({ 'minecraft:normal': 'Default', 'minecraft:flat': 'Superflat', 'minecraft:large_biomes': 'Large biomes', 'minecraft:amplified': 'Amplified' }[value] ?? value);

export function WorldsPanel({ serverId, state, serverType }: { serverId: string; state: ServerState; serverType: string }) {
  const unsupported = ['velocity', 'waterfall', 'bungeecord', 'geysermc', 'limbo', 'nanolimbo'].includes(serverType);
  const [data, setData] = useState<WorldList | null>(null);
  const [selected, setSelected] = useState('');
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState<'create' | 'import' | 'edit' | null>(null);
  const [input, setInput] = useState<WorldInput>(blank);
  const [file, setFile] = useState<File | null>(null);
  const [chunksOpen, setChunksOpen] = useState(false);
  const [revision, setRevision] = useState(0);
  const { confirm, dialog } = useModal();
  const editable = !!data?.editable && (state === 'stopped' || state === 'error') && !loadError;
  const world = data?.worlds.find((item) => item.name === selected);

  useEffect(() => {
    if (unsupported) { setLoading(false); return; }
    let active = true;
    void worldApi.list(serverId).then((result) => {
      if (!active) return;
      setData(result); setSelected((previous) => result.worlds.some((item) => item.name === previous) ? previous : result.worlds[0]?.name ?? ''); setLoadError(null);
    }).catch((err) => { if (active) setLoadError(message(err, 'Could not load worlds. Try again.')); }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [serverId, state, revision, unsupported]);

  function open(kind: 'create' | 'import' | 'edit') {
    setError(null); setNotice(null); setFile(null); setChunksOpen(false);
    setInput(kind === 'edit' && world ? { name: world.name, seed: world.seed ?? '', level_type: world.level_type, generate_structures: world.generate_structures } : { ...blank });
    setForm(kind);
  }
  async function run(action: () => Promise<void>) {
    setBusy(true); setError(null); setNotice(null);
    try { await action(); setRevision((value) => value + 1); }
    catch (err) { setError(message(err, 'The world operation failed. Try again.')); }
    finally { setBusy(false); }
  }
  async function save(event: FormEvent) {
    event.preventDefault(); if (!form || !editable) return;
    if (form === 'import' && (!file || file.size > 2 * 1024 ** 3)) { setError('Choose a world ZIP file of at most 2 GiB.'); return; }
    await run(async () => {
      const result = form === 'import' ? await worldApi.import(serverId, input.name, file!) : form === 'edit' ? await worldApi.save(serverId, input) : await worldApi.create(serverId, input);
      setSelected(result.name); setForm(null); setNotice(form === 'import' ? `Imported “${result.name}”. Select Make active to use it.` : form === 'edit' ? 'Generation settings saved.' : `Created “${result.name}”. Terrain generates when this world is active and the server starts.`);
    });
  }
  async function activate() {
    if (!world || !editable) return;
    if (!(await confirm(`Use “${world.name}” on the next server start? Players will join this world with its own inventories and progress.`, { title: 'Switch active world', confirmLabel: 'Make active' }))) return;
    await run(async () => { await worldApi.activate(serverId, world.name); setNotice(`“${world.name}” will load on the next server start.`); });
  }
  async function remove() {
    if (!world || !editable || world.active) return;
    if (!(await confirm(`Remove “${world.name}” and its dimensions from the world library? Its files will move to a recovery folder in Files.`, { title: 'Delete world', confirmLabel: 'Delete world', destructive: true }))) return;
    await run(async () => { const result = await worldApi.remove(serverId, world.name); setNotice(`World removed. Recovery files: ${result.recovery_path}`); setChunksOpen(false); });
  }

  if (unsupported) return <section className="rounded-lg border bg-card p-6"><h2 className="text-lg font-semibold">Worlds</h2><p className="mt-2 text-sm text-muted-foreground">This server type does not store Java worlds. Open a Minecraft game server to manage its worlds.</p></section>;
  return (
    <section className="space-y-5" aria-labelledby="worlds-title">
      {dialog}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div><h2 id="worlds-title" className="text-lg font-semibold tracking-tight">Worlds</h2><p className="mt-1 text-sm text-muted-foreground">Import saves, choose the active world, and maintain generated chunks.</p></div>
        <div className="flex flex-wrap gap-2"><Button variant="outline" disabled={!editable || busy} onClick={() => open('import')}><Upload className="h-4 w-4" />Import ZIP</Button><Button disabled={!editable || busy} onClick={() => open('create')}><Plus className="h-4 w-4" />Create world</Button></div>
      </div>
      {!editable && data && !loadError && <p className="rounded-md border bg-muted/40 px-4 py-3 text-sm">Stop the server to import, export, switch worlds, change generation settings, or reset chunks. You can inspect saved worlds while it runs.</p>}
      {loadError && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-4 text-sm"><p className="text-destructive">{loadError}</p><Button variant="outline" size="sm" onClick={() => setRevision((value) => value + 1)}><RefreshCw className="h-4 w-4" />Retry</Button></div>}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {notice && <p role="status" className="break-words rounded-md border border-primary/30 bg-primary/5 p-3 text-sm">{notice}</p>}
      {form && <form aria-label={form === 'import' ? 'Import world' : form === 'edit' ? 'Generation settings' : 'Create world'} onSubmit={(event) => void save(event)} className="space-y-5 rounded-lg border bg-card p-4 sm:p-6">
        <div className="flex items-center justify-between gap-3"><h3 className="font-semibold">{form === 'import' ? 'Import world' : form === 'edit' ? 'Generation settings' : 'Create world'}</h3><Button type="button" size="icon" variant="ghost" aria-label="Close world form" disabled={busy} onClick={() => setForm(null)}><X className="h-4 w-4" /></Button></div>
        <fieldset disabled={busy || !editable} className="grid min-w-0 gap-4 sm:grid-cols-2">
          <div className="space-y-2"><Label htmlFor="world-name">World folder name</Label><Input autoFocus id="world-name" required pattern="[A-Za-z0-9][A-Za-z0-9._\-]{0,63}" maxLength={64} disabled={form === 'edit'} placeholder="survival" value={input.name} onChange={(event) => setInput({ ...input, name: event.target.value })} /><p className="text-xs text-muted-foreground">Letters, numbers, dots, underscores and hyphens. Each world has its own folder.</p></div>
          {form === 'import' ? <div className="space-y-2"><Label htmlFor="world-zip">World ZIP file</Label><Input id="world-zip" type="file" accept=".zip,application/zip" aria-required="true" onChange={(event) => setFile(event.target.files?.[0] ?? null)} /><p className="text-xs text-muted-foreground">Up to 2 GiB compressed and 8 GiB unpacked. Include level.dat and the world’s region files. Matching Nether and End folders are supported.</p></div> : <>
            <div className="space-y-2"><Label htmlFor="world-seed">Seed</Label><Input id="world-seed" maxLength={128} placeholder="Leave blank for a random seed" value={input.seed} onChange={(event) => setInput({ ...input, seed: event.target.value })} /><p className="text-xs text-muted-foreground">Use a number or text. The seed is fixed after terrain is generated.</p></div>
            <div className="space-y-2"><Label htmlFor="world-generator">World generator</Label><Select id="world-generator" value={input.level_type} onValueChange={(value) => setInput({ ...input, level_type: value })}><option value="minecraft:normal">Default</option><option value="minecraft:flat">Superflat</option><option value="minecraft:large_biomes">Large biomes</option><option value="minecraft:amplified">Amplified</option></Select></div>
            <div className="flex items-center justify-between gap-3 sm:pt-6"><div><Label htmlFor="world-structures">Generate structures</Label><p className="mt-1 text-xs text-muted-foreground">Villages, temples and other generated structures.</p></div><Switch id="world-structures" checked={input.generate_structures} onCheckedChange={(checked) => setInput({ ...input, generate_structures: checked })} /></div>
          </>}
        </fieldset>
        <div className="flex flex-wrap gap-2"><Button disabled={busy || !editable} type="submit">{busy && <Loader2 className="h-4 w-4 animate-spin" />}{busy ? form === 'import' ? 'Importing world…' : 'Saving…' : form === 'import' ? 'Import world' : form === 'edit' ? 'Save generation settings' : 'Create world'}</Button><Button type="button" variant="outline" disabled={busy} onClick={() => setForm(null)}>Cancel</Button></div>
      </form>}
      {loading ? <div aria-label="Loading worlds" className="grid gap-4 md:grid-cols-[15rem_1fr]"><Skeleton className="h-48" /><Skeleton className="h-64" /></div> : data && data.worlds.length === 0 ? <div className="rounded-lg border border-dashed px-6 py-12 text-center"><Globe2 className="mx-auto mb-3 h-7 w-7 text-muted-foreground" /><h3 className="font-semibold">No saved worlds yet</h3><p className="mx-auto mt-2 max-w-lg text-sm text-muted-foreground">Create a world with your preferred seed, import an existing save, or start the server to generate “{data.active_world}”.</p></div> : data && <div className="grid min-w-0 gap-5 lg:grid-cols-[16rem_minmax(0,1fr)]">
        <div className="min-w-0"><div className="mb-2 flex items-center justify-between"><h3 className="text-sm font-medium">World library <span className="text-muted-foreground">({data.worlds.length})</span></h3><Button variant="ghost" size="icon" aria-label="Refresh worlds" disabled={busy} onClick={() => setRevision((value) => value + 1)}><RefreshCw className="h-4 w-4" /></Button></div>
          <div className="overflow-hidden rounded-lg border bg-card">{data.worlds.map((item) => <button type="button" key={item.name} aria-pressed={selected === item.name} disabled={busy} onClick={() => { setSelected(item.name); setChunksOpen(false); setForm(null); setError(null); }} className={`flex w-full items-start gap-3 border-b p-4 text-left last:border-b-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring ${selected === item.name ? 'bg-accent text-accent-foreground' : 'hover:bg-muted/50'}`}><Globe2 className="mt-0.5 h-4 w-4 shrink-0" /><span className="min-w-0 flex-1"><span className="flex flex-wrap items-center gap-x-2 gap-y-1"><span className="break-all text-sm font-medium">{item.name}</span>{item.active && <span className="inline-flex items-center gap-1 text-xs text-primary"><Check className="h-3 w-3" />Active</span>}</span><span className="mt-1 block text-xs text-muted-foreground">{item.generated ? `${formatBytes(item.size_bytes)} · ${item.dimensions.length} dimension${item.dimensions.length === 1 ? '' : 's'}` : 'Not generated yet'}</span></span></button>)}</div>
        </div>
        {world && <div className="min-w-0 space-y-5">
          <div className="rounded-lg border bg-card p-4 sm:p-6">
            <div className="flex flex-wrap items-start justify-between gap-3"><div className="min-w-0"><h3 className="break-all text-xl font-semibold tracking-tight">{world.name}</h3><p className="mt-1 text-sm text-muted-foreground">{world.active ? 'Loads when the server starts.' : 'Available to switch to when the server is stopped.'}</p></div>{!world.active && <Button disabled={!editable || busy} onClick={() => void activate()}><ArrowRightLeft className="h-4 w-4" />Make active</Button>}</div>
            <dl className="mt-6 grid gap-x-6 gap-y-4 sm:grid-cols-2"><div className="sm:col-span-2"><dt className="text-xs text-muted-foreground">{world.generated ? 'Saved world seed' : 'Generation seed'}</dt><dd className="mt-1 break-all font-mono text-sm select-all">{world.seed ?? (world.generated ? 'Unavailable in world metadata' : 'Random on first start')}{world.seed === '' && 'Random on first start'}</dd></div><div><dt className="text-xs text-muted-foreground">{world.generated ? 'Last saved version' : 'Generator'}</dt><dd className="mt-1 text-sm">{world.generated ? world.version || 'Unknown' : generatorName(world.level_type)}</dd></div><div><dt className="text-xs text-muted-foreground">Disk usage</dt><dd className="mt-1 text-sm tabular-nums">{formatBytes(world.size_bytes)}</dd></div><div><dt className="text-xs text-muted-foreground">Last modified</dt><dd className="mt-1 text-sm">{new Date(world.modified_at).toLocaleString()}</dd></div><div><dt className="text-xs text-muted-foreground">Dimensions</dt><dd className="mt-1 text-sm">{world.dimensions.map((dimension) => dimension.name).join(', ')}</dd></div></dl>
            {world.metadata_error && <p className="mt-4 text-sm text-destructive">World metadata could not be read. Export a copy and inspect level.dat before using this save.</p>}
            {!world.generated && <p className="mt-4 text-sm text-muted-foreground">Make this world active, then start the server to generate terrain. Seed and generator settings can be changed until then.</p>}
            <div className="mt-6 flex flex-wrap gap-2 border-t pt-4"><Button variant="outline" disabled={!world.generated || busy} onClick={() => setChunksOpen((value) => !value)} aria-expanded={chunksOpen}><Grid2X2 className="h-4 w-4" />{chunksOpen ? 'Hide chunks' : 'Manage chunks'}</Button>{!world.generated && <Button variant="outline" disabled={!editable || busy} onClick={() => open('edit')}>Generation settings</Button>}<Button variant="outline" disabled={!editable || busy || !world.generated} onClick={() => void run(async () => { await worldApi.export(serverId, world.name); setNotice('World ZIP downloaded.'); })}><Download className="h-4 w-4" />Export ZIP</Button><Button variant="ghost" disabled={!editable || busy || world.active} onClick={() => void remove()}><Trash2 className="h-4 w-4" />Delete world</Button></div>
          </div>
          {chunksOpen && <WorldChunks key={world.name} serverId={serverId} world={world} editable={editable && !busy} onChanged={() => setRevision((value) => value + 1)} />}
        </div>}
      </div>}
    </section>
  );
}

function WorldChunks({ serverId, world, editable, onChanged }: { serverId: string; world: World; editable: boolean; onChanged: () => void }) {
  const [dimension, setDimension] = useState(world.dimensions[0]?.id ?? world.name);
  const [offset, setOffset] = useState(0);
  const [regions, setRegions] = useState<WorldRegions | null>(null);
  const [region, setRegion] = useState<WorldRegion | null>(null);
  const [chunks, setChunks] = useState<WorldChunk[]>([]);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [loading, setLoading] = useState(true);
  const [chunksLoading, setChunksLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  const [cursor, setCursor] = useState(0);
  const grid = useRef<HTMLDivElement>(null);
  const { confirm, dialog } = useModal();
  useEffect(() => {
    let active = true; setLoading(true); setError(null); setRegion(null); setChunks([]); setSelected(new Set());
    void worldApi.regions(serverId, world.name, dimension, offset).then((result) => { if (active) { setRegions(result); setRegion(result.regions[0] ?? null); } }).catch((err) => { if (active) { setRegions(null); setError(message(err, 'Could not load region files. Try again.')); } }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [serverId, world.name, dimension, offset, revision]);
  const loadChunks = useCallback(async (current: WorldRegion) => worldApi.chunks(serverId, world.name, dimension, current.x, current.z), [serverId, world.name, dimension]);
  useEffect(() => {
    if (!region) return;
    let active = true; setChunksLoading(true); setChunks([]); setSelected(new Set()); setError(null);
    void loadChunks(region).then((result) => { if (active) { setChunks(result.chunks); const first = result.chunks[0]; setCursor(first ? (first.z - region.z * 32) * 32 + first.x - region.x * 32 : 0); } }).catch((err) => { if (active) setError(message(err, 'Could not load chunks. Try again.')); }).finally(() => { if (active) setChunksLoading(false); });
    return () => { active = false; };
  }, [region, loadChunks]);
  const indices = new Map(chunks.map((chunk) => [(chunk.z - (region?.z ?? 0) * 32) * 32 + chunk.x - (region?.x ?? 0) * 32, chunk]));
  function toggle(index: number) { setSelected((previous) => { const next = new Set(previous); if (next.has(index)) next.delete(index); else next.add(index); return next; }); }
  async function reset() {
    if (!region || !editable || selected.size === 0) return;
    if (!(await confirm(`Reset ${selected.size} chunk${selected.size === 1 ? '' : 's'} in ${dimension}, region ${region.x}, ${region.z}? Selected chunk coordinates: ${Array.from(selected).sort((a, b) => a - b).slice(0, 8).map((i) => `(${region.x * 32 + i % 32}, ${region.z * 32 + Math.floor(i / 32)})`).join(', ')}${selected.size > 8 ? ` and ${selected.size - 8} more` : ''}. Buildings, items, mobs and points of interest in the selection will be removed. Minecraft regenerates terrain when visited. Recovery copies are saved before changes.`, { title: 'Reset selected chunks', confirmLabel: `Reset ${selected.size} ${selected.size === 1 ? 'chunk' : 'chunks'}`, destructive: true }))) return;
    setBusy(true); setError(null); setNotice(null);
    try { const result = await worldApi.reset(serverId, world.name, { dimension, region_x: region.x, region_z: region.z, chunks: Array.from(selected) }); setNotice(`Reset ${result.reset} chunks. Recovery files: ${result.recovery_path}`); setRevision((value) => value + 1); onChanged(); }
    catch (err) { setError(message(err, 'Could not reset chunks. Refresh and try again.')); }
    finally { setBusy(false); }
  }
  return <section className="min-w-0 space-y-4 rounded-lg border bg-card p-4 sm:p-6" aria-labelledby="world-chunks-title">
    {dialog}
    <div className="flex flex-wrap items-start justify-between gap-3"><div><h3 id="world-chunks-title" className="font-semibold">Chunk management</h3><p className="mt-1 max-w-prose text-sm text-muted-foreground">Each region covers 32 × 32 chunks. Select generated chunks to reset terrain, entities and points of interest.</p></div><Button variant="ghost" size="icon" aria-label="Refresh chunks" disabled={busy} onClick={() => setRevision((value) => value + 1)}><RefreshCw className="h-4 w-4" /></Button></div>
    <div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="chunk-dimension">Dimension</Label><Select id="chunk-dimension" value={dimension} disabled={busy} onValueChange={(value) => { setDimension(value); setOffset(0); setNotice(null); }}>{world.dimensions.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</Select></div><div className="space-y-2"><Label htmlFor="chunk-region">Region</Label><Select id="chunk-region" value={region ? `${region.x},${region.z}` : ''} disabled={busy || loading || !regions?.regions.length} onValueChange={(value) => { setRegion(regions?.regions.find((item) => `${item.x},${item.z}` === value) ?? null); setNotice(null); }}>{regions?.regions.map((item) => <option key={`${item.x},${item.z}`} value={`${item.x},${item.z}`}>{item.x}, {item.z} · {item.chunks} chunks · {formatBytes(item.size_bytes)}</option>)}</Select></div></div>
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    {notice && <p role="status" className="break-words rounded-md bg-primary/5 p-3 text-sm">{notice}</p>}
    {loading || chunksLoading ? <Skeleton className="h-64" aria-label="Loading chunks" /> : !region ? !error && <p className="py-6 text-sm text-muted-foreground">No generated region files in this dimension yet.</p> : !error && <>
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground"><span className="tabular-nums">Chunk X {region.x * 32}–{region.x * 32 + 31} · Z {region.z * 32}–{region.z * 32 + 31}</span><span>{chunks.length.toLocaleString()} generated · {selected.size.toLocaleString()} selected</span></div>
      <div className="flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground"><span className="inline-flex items-center gap-1.5"><span className="h-3 w-3 rounded-sm bg-muted" />Not generated</span><span className="inline-flex items-center gap-1.5"><span className="h-3 w-3 rounded-sm bg-primary/35" />Generated</span><span className="inline-flex items-center gap-1.5"><span className="h-3 w-3 rounded-sm bg-destructive" />Selected for reset</span></div>
      <ChunkAreaSelector key={`${dimension}:${region.x},${region.z}`} region={region} chunks={chunks} disabled={busy} onSelect={setSelected} />
      <div className="overflow-x-auto rounded-md border bg-background p-3"><div ref={grid} role="grid" aria-label="Generated chunk map" aria-rowcount={32} aria-colcount={32} className="mx-auto grid min-w-[448px] max-w-[640px] gap-0.5">{Array.from({ length: 32 }, (_, row) => <div key={row} role="row" className="grid grid-cols-[repeat(32,minmax(0,1fr))] gap-0.5">{Array.from({ length: 32 }, (_, col) => { const i = row * 32 + col; const chunk = indices.get(i); return chunk ? <button key={i} type="button" role="gridcell" data-index={i} tabIndex={cursor === i ? 0 : -1} aria-selected={selected.has(i)} aria-label={`Chunk ${chunk.x}, ${chunk.z}`} title={`Chunk ${chunk.x}, ${chunk.z}${chunk.timestamp ? ` · Saved ${new Date(chunk.timestamp * 1000).toLocaleString()}` : ''}`} disabled={busy} onFocus={() => setCursor(i)} onClick={() => toggle(i)} onKeyDown={(event) => { const delta = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowDown' ? 32 : event.key === 'ArrowUp' ? -32 : 0; if (!delta) return; event.preventDefault(); for (let next = i + delta; next >= 0 && next < 1024; next += delta) { if (indices.has(next)) { setCursor(next); grid.current?.querySelector<HTMLButtonElement>(`[data-index="${next}"]`)?.focus(); break; } } }} className={`aspect-square rounded-[2px] transition-colors focus-visible:z-10 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:cursor-default ${selected.has(i) ? 'bg-destructive' : 'bg-primary/35 hover:bg-primary/60'}`} /> : <span key={i} role="gridcell" aria-label={`Chunk ${region.x * 32 + col}, ${region.z * 32 + row}, not generated`} className="aspect-square rounded-[2px] bg-muted" />; })}</div>)}</div></div>
      {selected.size > 0 && <details className="rounded-md border p-3 text-sm" open={selected.size <= 8}><summary className="cursor-pointer font-medium">Selected chunk coordinates ({selected.size})</summary><p className="mt-2 max-h-32 overflow-auto break-words font-mono text-xs leading-relaxed">{Array.from(selected).sort((a, b) => a - b).map((i) => `(${region.x * 32 + i % 32}, ${region.z * 32 + Math.floor(i / 32)})`).join(', ')}</p></details>}
      <p className="text-xs text-muted-foreground">Click cells to select. Use arrow keys and Space with the keyboard. Resetting makes sectors reusable; it does not shrink region files immediately.</p>
      <div className="flex flex-wrap items-center justify-between gap-3"><div className="flex flex-wrap gap-2"><Button variant="outline" size="sm" disabled={busy || chunks.length === 0} onClick={() => setSelected(new Set(indices.keys()))}>Select all generated</Button><Button variant="ghost" size="sm" disabled={busy || selected.size === 0} onClick={() => setSelected(new Set())}>Clear selection</Button></div><Button variant="destructive" disabled={busy || !editable || selected.size === 0} onClick={() => void reset()}>{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}Reset selected{selected.size > 0 ? ` (${selected.size})` : ''}</Button></div>
    </>}
    {!!regions && regions.total > 100 && <div className="flex items-center justify-between gap-2 border-t pt-3 text-xs text-muted-foreground"><span>Regions {offset + 1}–{Math.min(offset + 100, regions.total)} of {regions.total}</span><div className="flex gap-2"><Button variant="outline" size="icon" aria-label="Previous region page" disabled={busy || loading || offset === 0} onClick={() => setOffset((value) => Math.max(0, value - 100))}><ChevronLeft className="h-4 w-4" /></Button><Button variant="outline" size="icon" aria-label="Next region page" disabled={busy || loading || offset + 100 >= regions.total} onClick={() => setOffset((value) => value + 100)}><ChevronRight className="h-4 w-4" /></Button></div></div>}
  </section>;
}

function ChunkAreaSelector({ region, chunks, disabled, onSelect }: { region: WorldRegion; chunks: WorldChunk[]; disabled: boolean; onSelect: (selection: Set<number>) => void }) {
  const startX = region.x * 32; const startZ = region.z * 32;
  const [range, setRange] = useState({ minX: String(startX), maxX: String(startX), minZ: String(startZ), maxZ: String(startZ) });
  const [error, setError] = useState<string | null>(null);
  function select(event: FormEvent) {
    event.preventDefault(); setError(null);
    const [minX, maxX, minZ, maxZ] = [range.minX, range.maxX, range.minZ, range.maxZ].map(Number);
    if (Object.values(range).some((value) => value.trim() === '') || ![minX, maxX, minZ, maxZ].every(Number.isInteger) || minX > maxX || minZ > maxZ || minX < startX || maxX > startX + 31 || minZ < startZ || maxZ > startZ + 31) { setError('Enter an ordered chunk range inside the current region.'); return; }
    const matches = chunks.filter((chunk) => chunk.x >= minX && chunk.x <= maxX && chunk.z >= minZ && chunk.z <= maxZ);
    if (matches.length === 0) { setError('No generated chunks in that area. Choose another range.'); return; }
    onSelect(new Set(matches.map((chunk) => (chunk.z - startZ) * 32 + chunk.x - startX)));
  }
  return <details className="rounded-md border p-3">
    <summary className="cursor-pointer text-sm font-medium">Select an area by chunk coordinates</summary>
    <form aria-label="Select chunk area" onSubmit={select} className="mt-4 space-y-3">
      <fieldset disabled={disabled} className="grid grid-cols-2 gap-3">
        {([{ key: 'minX', label: 'From chunk X', min: startX, max: startX + 31 }, { key: 'maxX', label: 'To chunk X', min: startX, max: startX + 31 }, { key: 'minZ', label: 'From chunk Z', min: startZ, max: startZ + 31 }, { key: 'maxZ', label: 'To chunk Z', min: startZ, max: startZ + 31 }] as const).map((field) => <div key={field.key} className="min-w-0 space-y-1.5"><Label htmlFor={`area-${field.key}`}>{field.label}</Label><Input id={`area-${field.key}`} type="number" inputMode="numeric" required min={field.min} max={field.max} step={1} value={range[field.key]} onChange={(event) => setRange({ ...range, [field.key]: event.target.value })} /></div>)}
      </fieldset>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <p className="text-xs text-muted-foreground">Selects generated chunks inside the range in this region. Replaces the current selection.</p>
      <Button type="submit" variant="outline" size="sm" disabled={disabled}>Select area</Button>
    </form>
  </details>;
}
