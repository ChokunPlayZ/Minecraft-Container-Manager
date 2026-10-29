import { usePermissions } from '../lib/auth';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ArrowLeft,
  ChevronDown,
  ChevronRight,
  Database,
  Loader2,
  Plus,
  RefreshCw,
  Save,
  Search,
  Trash2,
} from 'lucide-react';
import { ApiError } from '../api/client';
import { nbtApi, type NBTFile, type NBTTag } from '../api/nbt';
import {
  documentError,
  isArray,
  isContainer,
  newTag,
  playerRootPath,
  TAG_TYPES,
  tagAt,
  tagSummary,
  updateTag,
  valueError,
} from '../lib/nbt';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';
import { Textarea } from './ui/textarea';
import { useModal } from './ui/modal';

type Props = {
  serverId: string;
  path: string;
  state: string;
  onClose: () => void;
  onDirtyChange?: (dirty: boolean) => void;
};
const keyOf = (path: number[]) => path.join('.');
const errorText = (err: unknown, fallback: string) => (err instanceof ApiError ? err.detail : fallback);

export function NBTEditor({ serverId, path, state, onClose, onDirtyChange }: Props) {
  const [file, setFile] = useState<NBTFile | null>(null);
  const [savedRoot, setSavedRoot] = useState<NBTTag | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [conflict, setConflict] = useState(false);
  const [selected, setSelected] = useState<number[]>([]);
  const [expanded, setExpanded] = useState(new Set(['']));
  const [query, setQuery] = useState('');
  const [searchLimit, setSearchLimit] = useState(100);
  const [epoch, setEpoch] = useState(0);
  const selectedPanel = useRef<HTMLDivElement>(null);
  const { confirm, dialog } = useModal();
  const dirty = !!file && file.root !== savedRoot;
  const can = usePermissions();
  const editable = can('files.write', serverId) && !!file?.editable && ['stopped', 'error'].includes(state);
  const validation = useMemo(() => (file ? documentError(file.root) : null), [file]);

  useEffect(() => {
    onDirtyChange?.(dirty);
    return () => onDirtyChange?.(false);
  }, [dirty, onDirtyChange]);
  useEffect(() => {
    if (!dirty) return;
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', beforeUnload);
    return () => window.removeEventListener('beforeunload', beforeUnload);
  }, [dirty]);

  const load = useCallback(async () => {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const next = await nbtApi.read(serverId, path);
      setFile(next);
      setSavedRoot(next.root);
      setSelected([]);
      setExpanded(new Set(['']));
      setConflict(false);
      setEpoch((v) => v + 1);
    } catch (err) {
      setError(errorText(err, 'Could not open this NBT file. Try again.'));
    } finally {
      setBusy(false);
    }
  }, [serverId, path]);
  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    if (selected.length && window.innerWidth < 1024)
      selectedPanel.current?.scrollIntoView?.({ block: 'nearest' });
  }, [selected]);

  async function reload() {
    if (
      dirty &&
      !(await confirm('Reload the saved file and discard your unsaved NBT changes?', {
        title: 'Reload NBT file',
        confirmLabel: 'Reload and discard',
        destructive: true,
      }))
    )
      return;
    void load();
  }
  async function close() {
    if (
      busy ||
      (dirty &&
        !(await confirm('Discard your unsaved NBT changes?', {
          title: 'Discard changes',
          confirmLabel: 'Discard',
          destructive: true,
        })))
    )
      return;
    onClose();
  }
  async function save() {
    if (!file || !dirty || !editable || validation || busy || conflict) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const next = await nbtApi.save(serverId, file);
      setFile(next);
      setSavedRoot(next.root);
      setNotice('NBT changes saved.');
      setEpoch((v) => v + 1);
    } catch (err) {
      setError(errorText(err, 'Could not save NBT data. Your edits are still here.'));
      if (err instanceof ApiError && err.code === 'nbt_changed') setConflict(true);
    } finally {
      setBusy(false);
    }
  }
  function change(path: number[], update: (tag: NBTTag) => NBTTag) {
    setFile((prev) => (prev ? { ...prev, root: updateTag(prev.root, path, update) } : prev));
    setNotice(null);
  }
  function select(path: number[]) {
    setSelected(path);
    setExpanded((prev) => new Set([...prev, ...path.map((_, i) => keyOf(path.slice(0, i))), keyOf(path)]));
  }

  const matches = useMemo(() => {
    if (!file || !query.trim()) return [];
    const found: { path: number[]; label: string; tag: NBTTag }[] = [];
    const q = query.trim().toLowerCase();
    function visit(tag: NBTTag, path: number[], label: string) {
      if (
        `${label} ${TAG_TYPES[tag.type]} ${tag.value ?? ''}`.toLowerCase().includes(q) ||
        tag.values?.some((value) => value.toLowerCase().includes(q))
      )
        found.push({ path, label, tag });
      tag.children?.forEach((child, i) =>
        visit(child, [...path, i], `${label}/${tag.type === 9 ? `[${i}]` : child.name || '(unnamed)'}`),
      );
    }
    visit(file.root, [], file.root.name || 'root');
    return found;
  }, [file, query]);
  const current = file ? tagAt(file.root, selected) : null;
  const parent = file && selected.length ? tagAt(file.root, selected.slice(0, -1)) : null;
  const playerPath = file ? playerRootPath(file.root) : null;

  return (
    <section aria-label="NBT editor" className="min-w-0 space-y-4">
      {dialog}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <Button
            variant="outline"
            size="icon"
            aria-label="Back to browser"
            onClick={() => void close()}
            disabled={busy}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="min-w-0">
            <h3 className="section-heading">NBT editor</h3>
            <p className="break-all font-mono text-xs text-muted-foreground">{path}</p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {dirty && <span className="text-xs text-muted-foreground">Unsaved changes</span>}
          <Button size="sm" variant="outline" onClick={() => void reload()} disabled={busy}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Reload
          </Button>
          <Button
            size="sm"
            onClick={() => void save()}
            disabled={busy || !dirty || !editable || !!validation || conflict}
          >
            {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}Save
            NBT
          </Button>
        </div>
      </div>
      <p className="text-sm text-muted-foreground">
        Browse all tag types, including lists, compounds and arrays. Changes keep the original compression;
        every save creates a backup beside the file.
      </p>
      {!editable && file && (
        <p role="status" className="rounded-lg border bg-muted/40 p-3 text-sm">
          {file.revision === 'demo'
            ? 'Demo NBT data is read-only.'
            : 'Read-only. Stop the server and finish active tasks, then reload this file to edit.'}
        </p>
      )}
      {error && (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/40 p-3 text-sm"
        >
          <p>{error}</p>
          {!file && (
            <Button size="sm" variant="outline" onClick={() => void load()} disabled={busy}>
              Try again
            </Button>
          )}
          {conflict && (
            <Button size="sm" variant="outline" onClick={() => void reload()}>
              Reload saved file
            </Button>
          )}
        </div>
      )}
      {notice && (
        <div role="status" className="space-y-1 rounded-lg border bg-accent/40 p-3 text-sm">
          <p>{notice}</p>
          {file?.backup_path && (
            <a
              className="block break-all text-xs text-primary underline underline-offset-4"
              href={`/api/servers/${encodeURIComponent(serverId)}/files/download?${new URLSearchParams({ path: file.backup_path })}`}
            >
              Download previous version: {file.backup_path}
            </a>
          )}
        </div>
      )}
      {!file && busy && (
        <p role="status" className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Reading NBT data…
        </p>
      )}
      {file && current && (
        <>
          <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
            <span>
              {file.compression === 'none' ? 'Uncompressed' : `${file.compression.toUpperCase()} compression`}
            </span>
            <span>Java Edition NBT</span>
          </div>
          {playerPath !== null && (
            <PlayerQuickFields
              root={file.root}
              path={playerPath}
              disabled={!editable || busy}
              change={change}
              select={select}
            />
          )}
          {validation && (
            <p role="alert" className="text-sm text-destructive">
              {validation}
            </p>
          )}
          <div className="grid min-w-0 items-start overflow-hidden rounded-lg border bg-card lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <div className="min-w-0 border-b lg:border-b-0 lg:border-r">
              <div className="space-y-3 border-b p-4">
                <h4 className="font-medium">Tags</h4>
                <div className="relative">
                  <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
                  <Input
                    aria-label="Search NBT tags"
                    placeholder="Search names, paths or values"
                    value={query}
                    onChange={(e) => {
                      setQuery(e.target.value);
                      setSearchLimit(100);
                    }}
                    className="pl-9"
                  />
                </div>
              </div>
              <nav aria-label="NBT tags" className="max-h-[32rem] overflow-auto p-2">
                {query.trim() ? (
                  <>
                    <p className="p-2 text-xs text-muted-foreground" role="status">
                      {matches.length} matching tags
                    </p>
                    {matches.slice(0, searchLimit).map((m) => (
                      <button
                        key={keyOf(m.path)}
                        type="button"
                        onClick={() => select(m.path)}
                        aria-current={keyOf(m.path) === keyOf(selected) ? 'true' : undefined}
                        className={`block w-full rounded-md px-3 py-2 text-left text-sm ${keyOf(m.path) === keyOf(selected) ? 'bg-accent text-accent-foreground' : 'hover:bg-muted'}`}
                      >
                        <span className="block break-all font-mono text-xs">{m.label}</span>
                        <span className="text-xs text-muted-foreground">
                          {TAG_TYPES[m.tag.type]} · {tagSummary(m.tag)}
                        </span>
                      </button>
                    ))}
                    {matches.length > searchLimit && (
                      <Button size="sm" variant="ghost" onClick={() => setSearchLimit((n) => n + 100)}>
                        Show more matches
                      </Button>
                    )}
                  </>
                ) : (
                  <TagBranch
                    tag={file.root}
                    path={[]}
                    selected={selected}
                    expanded={expanded}
                    select={select}
                    toggle={(p) =>
                      setExpanded((prev) => {
                        const next = new Set(prev);
                        const k = keyOf(p);
                        if (next.has(k)) next.delete(k);
                        else next.add(k);
                        return next;
                      })
                    }
                  />
                )}
              </nav>
            </div>
            <div ref={selectedPanel} className="min-w-0 space-y-4 p-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h4 className="break-all font-medium">
                  {current.name || (selected.length ? `[${selected.at(-1)}]` : 'Root tag')}
                </h4>
                <span className="text-xs text-muted-foreground">
                  {TAG_TYPES[current.type]}
                  {current.type === 9 ? ` of ${TAG_TYPES[current.list_type ?? 0] || 'End'}` : ''}
                </span>
              </div>
              <p className="break-all font-mono text-xs text-muted-foreground">
                {[
                  'root',
                  ...selected.map((_, i) => {
                    const p = tagAt(file.root, selected.slice(0, i));
                    const t = tagAt(file.root, selected.slice(0, i + 1));
                    return p.type === 9 ? `[${selected[i]}]` : t.name || '(unnamed)';
                  }),
                ].join(' / ')}
              </p>
              <fieldset
                key={`${keyOf(selected)}-${epoch}`}
                disabled={!editable || busy}
                className="min-w-0 space-y-4"
              >
                {parent?.type !== 9 && (
                  <div className="space-y-2">
                    <Label htmlFor="nbt-tag-name">Tag name</Label>
                    <Input
                      id="nbt-tag-name"
                      value={current.name}
                      onChange={(e) => change(selected, (t) => ({ ...t, name: e.target.value }))}
                    />
                  </div>
                )}
                {!isContainer(current) && (
                  <div className="space-y-2">
                    <Label htmlFor="nbt-tag-value">{isArray(current) ? 'Array values' : 'Value'}</Label>
                    {isArray(current) ? (
                      <>
                        <Textarea
                          id="nbt-tag-value"
                          aria-describedby="nbt-array-help"
                          defaultValue={(current.values ?? []).join(', ')}
                          onChange={(e) =>
                            change(selected, (t) => ({
                              ...t,
                              values: e.target.value.trim()
                                ? e.target.value
                                    .trim()
                                    .split(/[\s,]+/)
                                    .filter(Boolean)
                                : [],
                            }))
                          }
                          rows={8}
                          className="font-mono text-xs"
                        />
                        <p id="nbt-array-help" className="text-xs text-muted-foreground">
                          Signed integers separated by commas or spaces. Long values keep all 64 bits.
                        </p>
                      </>
                    ) : current.type === 8 ? (
                      <Textarea
                        id="nbt-tag-value"
                        value={current.value ?? ''}
                        onChange={(e) => change(selected, (t) => ({ ...t, value: e.target.value }))}
                        rows={5}
                        className="font-mono text-sm"
                      />
                    ) : (
                      <Input
                        id="nbt-tag-value"
                        value={current.value ?? ''}
                        onChange={(e) => change(selected, (t) => ({ ...t, value: e.target.value }))}
                        className="font-mono"
                      />
                    )}
                  </div>
                )}
                {isContainer(current) && (
                  <AddTagForm
                    key={`${keyOf(selected)}-${current.list_type}`}
                    tag={current}
                    add={(child) =>
                      change(selected, (t) => ({
                        ...t,
                        list_type: t.type === 9 ? child.type : undefined,
                        children: [...(t.children ?? []), child],
                      }))
                    }
                  />
                )}
                {!!selected.length && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      const index = selected.at(-1)!;
                      const p = selected.slice(0, -1);
                      change(p, (t) => ({ ...t, children: t.children?.filter((_, i) => i !== index) }));
                      setSelected(p);
                    }}
                  >
                    <Trash2 className="mr-2 h-4 w-4" />
                    Remove tag
                  </Button>
                )}
              </fieldset>
            </div>
          </div>
        </>
      )}
    </section>
  );
}

function TagBranch({
  tag,
  path,
  selected,
  expanded,
  select,
  toggle,
}: {
  tag: NBTTag;
  path: number[];
  selected: number[];
  expanded: Set<string>;
  select: (path: number[]) => void;
  toggle: (path: number[]) => void;
}) {
  const [limit, setLimit] = useState(100);
  const key = keyOf(path),
    open = expanded.has(key),
    container = isContainer(tag);
  return (
    <div>
      <div
        className={`flex min-w-0 items-center rounded-md ${key === keyOf(selected) ? 'bg-accent text-accent-foreground' : 'hover:bg-muted'}`}
        style={{ paddingLeft: Math.min(path.length, 12) * 12 }}
      >
        {container ? (
          <button
            type="button"
            aria-label={`${open ? 'Collapse' : 'Expand'} ${tag.name || 'root'}`}
            aria-expanded={open}
            className="flex h-9 w-8 shrink-0 items-center justify-center rounded-md focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => toggle(path)}
          >
            {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          </button>
        ) : (
          <span className="w-8 shrink-0" />
        )}
        <button
          type="button"
          aria-current={key === keyOf(selected) ? 'true' : undefined}
          aria-label={`${tag.name || (path.length ? `[${path.at(-1)}]` : 'Root tag')} — ${TAG_TYPES[tag.type]}`}
          onClick={() => select(path)}
          className="min-w-0 flex-1 rounded-md py-2 pr-2 text-left focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="flex min-w-0 items-baseline justify-between gap-2">
            <span className="truncate font-mono text-xs">
              {tag.name || (path.length ? `[${path.at(-1)}]` : 'Root tag')}
            </span>
            <span className="shrink-0 text-xs text-muted-foreground">{TAG_TYPES[tag.type]}</span>
          </span>
          <span className="block truncate text-xs text-muted-foreground">{tagSummary(tag)}</span>
        </button>
      </div>
      {container && open && (
        <>
          {tag.children?.slice(0, limit).map((child, i) => (
            <TagBranch
              key={i}
              tag={child}
              path={[...path, i]}
              selected={selected}
              expanded={expanded}
              select={select}
              toggle={toggle}
            />
          ))}
          {(tag.children?.length ?? 0) > limit && (
            <Button variant="ghost" size="sm" onClick={() => setLimit((n) => n + 100)}>
              Show more tags ({tag.children!.length - limit} remaining)
            </Button>
          )}
          {!tag.children?.length && (
            <p className="px-8 py-3 text-xs text-muted-foreground">
              Empty {TAG_TYPES[tag.type].toLowerCase()}
            </p>
          )}
        </>
      )}
    </div>
  );
}

function AddTagForm({ tag, add }: { tag: NBTTag; add: (tag: NBTTag) => void }) {
  const [type, setType] = useState(String(tag.type === 9 && tag.list_type ? tag.list_type : 8));
  const [name, setName] = useState('');
  const duplicate = tag.type === 10 && !!tag.children?.some((t) => t.name === name);
  return (
    <div className="space-y-3 border-t pt-4">
      <p className="text-sm font-medium">{tag.type === 9 ? 'Add list item' : 'Add child tag'}</p>
      {tag.type === 10 && (
        <div className="space-y-2">
          <Label htmlFor="nbt-new-name">New tag name</Label>
          <Input id="nbt-new-name" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
      )}
      <div className="space-y-2">
        <Label htmlFor="nbt-new-type">New tag type</Label>
        <Select
          id="nbt-new-type"
          value={type}
          onValueChange={setType}
          disabled={tag.type === 9 && !!tag.list_type}
        >
          {TAG_TYPES.slice(1).map((label, i) => (
            <option key={label} value={i + 1}>
              {label}
            </option>
          ))}
        </Select>
      </div>
      {duplicate && <p className="text-xs text-destructive">A tag with this name already exists.</p>}
      <Button
        size="sm"
        variant="outline"
        disabled={duplicate}
        onClick={() => {
          add(newTag(Number(type), tag.type === 9 ? '' : name));
          setName('');
        }}
      >
        <Plus className="mr-2 h-4 w-4" />
        {tag.type === 9 ? 'Add item' : 'Add tag'}
      </Button>
      <p className="text-xs text-muted-foreground">Additions and removals are applied when you save.</p>
    </div>
  );
}

function PlayerQuickFields({
  root,
  path,
  disabled,
  change,
  select,
}: {
  root: NBTTag;
  path: number[];
  disabled: boolean;
  change: (path: number[], update: (tag: NBTTag) => NBTTag) => void;
  select: (path: number[]) => void;
}) {
  const player = tagAt(root, path);
  const fields = [
    ['Health', 'Health'],
    ['foodLevel', 'Food level'],
    ['foodSaturationLevel', 'Saturation'],
    ['XpLevel', 'XP level'],
    ['XpTotal', 'Total XP'],
    ['XpP', 'XP progress'],
    ['playerGameType', 'Game mode'],
    ['Dimension', 'Dimension'],
  ];
  const posIndex =
    player.children?.findIndex(
      (t) =>
        t.name === 'Pos' && t.type === 9 && t.children?.length === 3 && t.children.every((c) => c.type === 6),
    ) ?? -1;
  return (
    <div className="space-y-4 rounded-lg border bg-card p-4">
      <h4 className="flex items-center gap-2 font-medium">
        <Database className="h-4 w-4 text-muted-foreground" />
        Player quick edit
      </h4>
      <fieldset disabled={disabled} className="grid min-w-0 grid-cols-2 gap-4 xl:grid-cols-4">
        {fields.map(([name, label]) => {
          const index =
            player.children?.findIndex(
              (t) =>
                t.name === name &&
                (name === 'Dimension' ? [3, 8].includes(t.type) : [1, 2, 3, 4, 5, 6].includes(t.type)),
            ) ?? -1;
          if (index < 0) return null;
          const tag = player.children![index],
            id = `nbt-quick-${name}`;
          return (
            <div key={name} className="min-w-0 space-y-2">
              <Label htmlFor={id}>{label}</Label>
              {name === 'playerGameType' ? (
                <Select
                  id={id}
                  value={tag.value}
                  onValueChange={(value) => change([...path, index], (t) => ({ ...t, value }))}
                >
                  {!['0', '1', '2', '3'].includes(tag.value ?? '') && (
                    <option value={tag.value}>Unknown ({tag.value})</option>
                  )}
                  <option value="0">Survival</option>
                  <option value="1">Creative</option>
                  <option value="2">Adventure</option>
                  <option value="3">Spectator</option>
                </Select>
              ) : (
                <Input
                  id={id}
                  value={tag.value ?? ''}
                  aria-invalid={!!valueError(tag.type, tag.value ?? '')}
                  onChange={(e) => change([...path, index], (t) => ({ ...t, value: e.target.value }))}
                />
              )}
            </div>
          );
        })}
        {posIndex >= 0 &&
          player.children![posIndex].children!.map((t, i) => (
            <div key={i} className="space-y-2">
              <Label htmlFor={`nbt-pos-${i}`}>Position {['X', 'Y', 'Z'][i]}</Label>
              <Input
                id={`nbt-pos-${i}`}
                value={t.value ?? ''}
                onChange={(e) => change([...path, posIndex, i], (tag) => ({ ...tag, value: e.target.value }))}
              />
            </div>
          ))}
      </fieldset>
      <div className="flex flex-wrap gap-2">
        {['Inventory', 'EnderItems', 'abilities', 'Attributes', 'attributes'].map((name) => {
          const index = player.children?.findIndex((t) => t.name === name) ?? -1;
          return index < 0 ? null : (
            <Button key={name} size="sm" variant="outline" onClick={() => select([...path, index])}>
              Browse {name}
            </Button>
          );
        })}
      </div>
    </div>
  );
}
