import { usePermissions } from '../lib/auth';
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Archive,
  ArrowUp,
  Download,
  Database,
  File,
  FileArchive,
  FilePlus2,
  FileText,
  Folder,
  FolderPlus,
  Globe,
  Layers,
  Loader2,
  Pencil,
  RefreshCw,
  Save,
  Trash2,
  TextCursorInput,
  Upload,
  X,
} from 'lucide-react';
import { ProgressBar } from './ui/progress';
import { api, ApiError, FileConflictError } from '../api/client';
import type { FileContent, FileEntry, Server } from '../api/types';
import { FileConflictDialog } from './file-conflict-dialog';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Textarea } from './ui/textarea';
import { useModal } from './ui/modal';
import { NBTEditor } from './nbt-editor';
import { isNBTFile } from '../lib/nbt';
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuLabel, ContextMenuSeparator, ContextMenuTrigger } from './ui/context-menu';

function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(value >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
}

function joinPath(dir: string, name: string): string {
  if (!dir) return name;
  return `${dir}/${name}`;
}

// A small set of extensions we consider editable text. Everything else opens
// for download instead of loading into the editor.
const TEXT_EXTENSIONS = new Set([
  // Config / data
  'txt', 'json', 'yml', 'yaml', 'toml', 'ini', 'cfg', 'conf', 'properties', 'env',
  'xml', 'html', 'htm', 'css', 'js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx', 'md', 'sh',
  'bat', 'ps1', 'cmd', 'csv', 'log', 'tsv', 'sql', 'mcmeta', 'mcfunction',
  'lang', 'snbt', 'npy', 'pem', 'key', 'crt', 'ignore', 'gitignore',
]);

const EDITABLE_NAMES = new Set([
  'server.properties',
  'eula.txt',
  'banned-ips.json',
  'banned-players.json',
  'ops.json',
  'whitelist.json',
  'usercache.json',
  'paper-global.yml',
  'paper-world-defaults.yml',
  'bukkit.yml',
  'spigot.yml',
  'config.yml',
]);

function isEditable(entry: FileEntry): boolean {
  if (entry.is_directory) return false;
  if (isNBTFile(entry.name)) return false;
  const lower = entry.name.toLowerCase();
  const idx = lower.lastIndexOf('.');
  const ext = idx >= 0 ? lower.slice(idx + 1) : '';
  return TEXT_EXTENSIONS.has(ext) || EDITABLE_NAMES.has(lower);
}

export function FileManager({ server, onNBTDirtyChange }: { server: Server; onNBTDirtyChange?: (dirty: boolean) => void }) {
  const can = usePermissions();
  const canWrite = can('files.write', server.id);
  const canDelete = can('files.delete', server.id);
  const [nbtPath, setNBTPath] = useState<string | null>(null);
  const [cwd, setCwd] = useState('');
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [uploadStats, setUploadStats] = useState<{ fileName: string; loaded: number; total: number } | null>(null);
  const [activeTask, setActiveTask] = useState<{
    title: string;
    subtext?: string;
    percent?: number;
    showPercent?: boolean;
  } | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [showUrlInput, setShowUrlInput] = useState(false);
  const [url, setUrl] = useState('');
  const [urlName, setUrlName] = useState('');
  const [selectedNames, setSelectedNames] = useState<Set<string>>(new Set());
  const selectionAnchor = useRef<string | null>(null);

  // Editor state.
  const [editorPath, setEditorPath] = useState<string | null>(null);
  const [editorContent, setEditorContent] = useState('');
  const [editorDirty, setEditorDirty] = useState(false);
  const [editorError, setEditorError] = useState<string | null>(null);
  const [editorBusy, setEditorBusy] = useState(false);
  const [editorRevision, setEditorRevision] = useState<string | null>(null);
  const [editorConflict, setEditorConflict] = useState<FileContent | null>(null);
  const editorRequest = useRef(0);
  const { confirm, prompt, dialog } = useModal();

  useEffect(() => {
    const unsub = api.subscribeTaskEvents(server.id, (p) => {
      if (p.operation === 'archive') {
        if (p.stage === 'completed') {
          setActiveTask({
            title: p.stage_title || 'Compression Complete',
            subtext: p.message,
            percent: 100,
            showPercent: true,
          });
          setTimeout(() => {
            setActiveTask((curr) => (curr?.percent === 100 ? null : curr));
          }, 800);
        } else if (p.stage === 'failed') {
          setActiveTask({
            title: 'Compression Failed',
            subtext: p.error || p.message,
            percent: 100,
            showPercent: false,
          });
        } else {
          let sub = p.message || '';
          if (p.bytes_total && p.bytes_total > 0) {
            sub = `${formatSize(p.bytes_done || 0)} / ${formatSize(p.bytes_total)} • ${p.message || ''}`;
          }
          setActiveTask({
            title: p.stage_title || 'Compressing Files',
            subtext: sub,
            percent: p.percent,
            showPercent: true,
          });
        }
      }
    });
    return unsub;
  }, [server.id]);

  const load = useCallback(async () => {
    setError(null);
    try {
      const res = await api.listFiles(server.id, cwd);
      const nextEntries = res.entries ?? [];
      setEntries(nextEntries);
      setSelectedNames((previous) => new Set(nextEntries.filter((entry) => previous.has(entry.name)).map((entry) => entry.name)));
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load files');
    }
  }, [server.id, cwd]);

  useEffect(() => {
    setSelectedNames(new Set());
    selectionAnchor.current = null;
    void load();
  }, [load]);

  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await action();
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Action failed');
    } finally {
      setBusy(false);
    }
  }

  function segments(): string[] {
    return cwd ? cwd.split('/').filter(Boolean) : [];
  }

  function goUp() {
    const parts = segments();
    parts.pop();
    setCwd(parts.join('/'));
  }

  function navigate(seg: number) {
    const parts = segments();
    setCwd(parts.slice(0, seg).join('/'));
  }

  async function closeEditor() {
    if (editorBusy) return;
    if (editorDirty && !(await confirm('Discard unsaved changes?', { title: 'Discard changes', confirmLabel: 'Discard', destructive: true }))) return;
    clearEditor();
  }

  function clearEditor() {
    setEditorPath(null);
    setEditorContent('');
    setEditorDirty(false);
    setEditorError(null);
    setEditorRevision(null);
    setEditorConflict(null);
    editorRequest.current++;
  }

  async function openEditor(path: string) {
    if (editorBusy) return;
    if (editorDirty && !(await confirm('Discard unsaved changes?', { title: 'Discard changes', confirmLabel: 'Discard', destructive: true }))) return;
    setEditorBusy(true);
    setEditorError(null);
    setEditorPath(path);
    setEditorContent('');
    setEditorDirty(false);
    setEditorRevision(null);
    setEditorConflict(null);
    const request = ++editorRequest.current;
    try {
      const res = await api.readFileContent(server.id, path);
      if (request !== editorRequest.current) return;
      setEditorContent(res.content);
      setEditorRevision(res.revision);
      setEditorDirty(false);
    } catch (err) {
      if (request !== editorRequest.current) return;
      setEditorContent('');
      setEditorDirty(false);
      setEditorError(err instanceof ApiError ? err.detail : 'Failed to load file');
    } finally {
      if (request === editorRequest.current) setEditorBusy(false);
    }
  }

  async function newFile() {
    if (editorBusy) return;
    const name = await prompt('New file name (e.g. ops.json):', 'new-file.txt', { title: 'New file' });
    if (!name || !name.trim()) return;
    const base = cwd ? `${cwd}/${name.trim()}` : name.trim();
    if (editorDirty && !(await confirm('Discard unsaved changes?', { title: 'Discard changes', confirmLabel: 'Discard', destructive: true }))) return;
    setEditorPath(base);
    setEditorContent('');
    setEditorDirty(true);
    setEditorError(null);
    setEditorRevision('');
    setEditorConflict(null);
    editorRequest.current++;
  }

  async function saveEditor(expectedRevision = editorRevision) {
    if (!editorPath || editorBusy || expectedRevision === null) return;
    setEditorBusy(true);
    setEditorError(null);
    try {
      const saved = await api.writeFileContent(server.id, editorPath, editorContent, expectedRevision);
      setEditorRevision(saved.revision);
      setEditorConflict(null);
      setEditorDirty(false);
      await load();
    } catch (err) {
      if (err instanceof FileConflictError) {
        setEditorConflict(err.current);
        setEditorError(editorConflict ? 'The file changed again. Review the updated comparison before saving.' : null);
      } else {
        setEditorError(err instanceof ApiError ? err.detail : 'Failed to save file');
      }
    } finally {
      setEditorBusy(false);
    }
  }

  function useServerVersion() {
    if (!editorConflict || editorBusy) return;
    setEditorContent(editorConflict.content);
    setEditorRevision(editorConflict.revision);
    setEditorDirty(false);
    setEditorError(null);
    setEditorConflict(null);
  }

  async function handleUpload(files: File[]) {
    if (files.length === 0) return;
    setBusy(true);
    setError(null);
    setUploadProgress(0);
    const totalBytes = files.reduce((acc, f) => acc + f.size, 0);
    const summaryName = files.length === 1 ? files[0].name : `${files.length} files (${formatSize(totalBytes)})`;
    setUploadStats({ fileName: summaryName, loaded: 0, total: totalBytes });
    try {
      await api.uploadFiles(server.id, files, cwd, (loaded, total) => {
        setUploadProgress(total > 0 ? Math.round((loaded / total) * 100) : 0);
        setUploadStats({ fileName: summaryName, loaded, total });
      });
      setUploadProgress(null);
      setUploadStats(null);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Upload failed');
      setUploadProgress(null);
      setUploadStats(null);
    } finally {
      setBusy(false);
    }
  }

  async function handleDownload(entry: FileEntry) {
    try {
      const blob = await api.downloadFile(server.id, joinPath(cwd, entry.name));
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = entry.name;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(a.href);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Download failed');
    }
  }

  function selectEntry(name: string, modifiers: { shiftKey: boolean; ctrlKey: boolean; metaKey: boolean }) {
    if (busy) return;
    const anchorIndex = entries.findIndex((entry) => entry.name === selectionAnchor.current);
    const targetIndex = entries.findIndex((entry) => entry.name === name);
    if (modifiers.shiftKey && anchorIndex >= 0 && targetIndex >= 0) {
      const range = entries.slice(Math.min(anchorIndex, targetIndex), Math.max(anchorIndex, targetIndex) + 1);
      setSelectedNames((previous) => new Set([
        ...(modifiers.ctrlKey || modifiers.metaKey ? previous : []),
        ...range.map((entry) => entry.name),
      ]));
      return;
    }
    selectionAnchor.current = name;
    setSelectedNames((prev) => {
      if (!modifiers.ctrlKey && !modifiers.metaKey) return new Set([name]);
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  }

  function selectContextEntry(name: string) {
    if (busy || selectedNames.has(name)) return;
    selectionAnchor.current = name;
    setSelectedNames(new Set([name]));
  }

  function toggleSelectAll() {
    selectionAnchor.current = null;
    if (selectedNames.size === entries.length) {
      setSelectedNames(new Set());
    } else {
      setSelectedNames(new Set(entries.map((e) => e.name)));
    }
  }

  async function handleArchiveSelected() {
    if (selectedNames.size === 0) return;
    const names = Array.from(selectedNames);
    const defaultName = names.length === 1 ? `${names[0]}.zip` : 'archive.zip';
    const zipName = await prompt(
      `Enter name for zip archive (${names.length} items selected):`,
      defaultName,
      { title: 'Create Zip Archive', confirmLabel: 'Create Zip' },
    );
    if (!zipName || !zipName.trim()) return;
    const sources = names.map((n) => joinPath(cwd, n));
    setActiveTask({
      title: `Compressing ${zipName.trim()}...`,
      subtext: `Scanning ${names.length} items...`,
      percent: 0,
      showPercent: true,
    });
    try {
      await run(() => api.archiveFiles(server.id, sources, cwd, zipName.trim()));
      setSelectedNames(new Set());
    } finally {
      setTimeout(() => {
        setActiveTask((curr) => (curr?.percent === 100 ? null : curr));
      }, 600);
    }
  }

  async function handleArchive(entry: FileEntry) {
    const defaultName = `${entry.name}.zip`;
    const zipName = await prompt(
      `Create zip archive for "${entry.name}":`,
      defaultName,
      { title: 'Archive as Zip', confirmLabel: 'Archive' },
    );
    if (!zipName || !zipName.trim()) return;
    setActiveTask({
      title: `Compressing ${zipName.trim()}...`,
      subtext: `Scanning ${entry.name}...`,
      percent: 0,
      showPercent: true,
    });
    try {
      await run(() => api.archiveFile(server.id, joinPath(cwd, entry.name), zipName.trim(), cwd));
    } finally {
      setTimeout(() => {
        setActiveTask((curr) => (curr?.percent === 100 ? null : curr));
      }, 600);
    }
  }

  async function handleUnzip(entry: FileEntry) {
    const baseName = entry.name.replace(/\.zip$/i, '');
    const choice = await prompt(
      `Extract "${entry.name}" into folder (leave empty to extract in current folder):`,
      baseName,
      { title: 'Unzip Archive', confirmLabel: 'Extract' },
    );
    if (choice === null) return;
    const dest = choice.trim() ? joinPath(cwd, choice.trim()) : cwd;
    setActiveTask({ title: `Extracting ${entry.name}...`, subtext: `Unzipping contents into ${dest || 'current directory'}` });
    try {
      await run(() => api.unzipFile(server.id, joinPath(cwd, entry.name), dest));
    } finally {
      setActiveTask(null);
    }
  }

  async function handleRename(entry: FileEntry) {
    const name = await prompt(`Rename "${entry.name}" to:`, entry.name, { title: 'Rename item', confirmLabel: 'Rename' });
    if (!name || name.trim() === entry.name) return;
    await run(() => api.renameFile(server.id, joinPath(cwd, entry.name), name.trim()));
  }

  async function handleDelete(entry: FileEntry) {
    await handleDeleteEntries([entry]);
  }

  async function handleDeleteSelected() {
    await handleDeleteEntries(entries.filter((entry) => selectedNames.has(entry.name)));
  }

  async function handleDeleteEntries(targets: FileEntry[]) {
    if (!canDelete || busy || editorBusy || targets.length === 0) return;
    const paths = targets.map((entry) => joinPath(cwd, entry.name));
    const single = targets.length === 1 ? targets[0] : null;
    const what = single?.is_directory ? 'folder' : 'file';
    const title = single ? `Delete ${what}` : 'Delete selected items';
    const description = single
      ? `Delete ${what} "${single.name}"? This cannot be undone.`
      : `Delete ${targets.length} selected items? Folders and their contents will also be deleted. This cannot be undone.\n\n${targets.map((entry) => entry.name).join('\n')}`;
    const deletesEditor = editorPath !== null && paths.some((path) => editorPath === path || editorPath.startsWith(`${path}/`));
    const message = deletesEditor && editorDirty ? `${description}\n\nUnsaved changes in "${editorPath}" will also be discarded.` : description;
    if (!(await confirm(message, { title, confirmLabel: single ? 'Delete' : `Delete ${targets.length} items`, destructive: true }))) return;

    setBusy(true);
    setError(null);
    const deleted = new Set<string>();
    const failures: string[] = [];
    try {
      for (const [index, entry] of targets.entries()) {
        try {
          await api.deleteFile(server.id, paths[index]);
          deleted.add(entry.name);
          if (editorPath === paths[index] || editorPath?.startsWith(`${paths[index]}/`)) clearEditor();
        } catch (err) {
          failures.push(`${entry.name}: ${err instanceof ApiError ? err.detail : 'Delete failed'}`);
        }
      }
      setSelectedNames((previous) => new Set([...previous].filter((name) => !deleted.has(name))));
      await load();
      if (failures.length > 0) {
        setError(`Could not delete ${failures.length} of ${targets.length} items. ${failures.join('; ')}`);
      }
    } finally {
      setBusy(false);
    }
  }

  async function handleMkdir() {
    const name = await prompt('New folder name:', '', { title: 'New folder' });
    if (!name || !name.trim()) return;
    await run(() => api.mkdir(server.id, joinPath(cwd, name.trim())));
  }

  async function handleDownloadFromUrl() {
    const trimmed = url.trim();
    if (!trimmed) return;
    setActiveTask({ title: 'Downloading file from URL...', subtext: trimmed });
    try {
      await run(() => api.downloadFromUrl(server.id, trimmed, cwd, urlName.trim() || undefined));
      setUrl('');
      setUrlName('');
      setShowUrlInput(false);
    } finally {
      setActiveTask(null);
    }
  }

  function handleEntryClick(entry: FileEntry) {
    if (entry.is_directory) {
      setCwd(joinPath(cwd, entry.name));
      return;
    }
    if (isNBTFile(entry.name)) {
      void openNBT(joinPath(cwd, entry.name));
      return;
    }
    if (isEditable(entry)) {
      void openEditor(joinPath(cwd, entry.name));
      return;
    }
    void handleDownload(entry);
  }

  async function openNBT(path: string) {
    if (editorBusy) return;
    if (editorDirty && !(await confirm('Discard unsaved text changes and open the NBT editor?', { title: 'Discard changes', confirmLabel: 'Discard', destructive: true }))) return;
    setEditorPath(null); setEditorDirty(false); setNBTPath(path);
  }

  if (nbtPath) return <NBTEditor key={nbtPath} serverId={server.id} path={nbtPath} state={server.state} onDirtyChange={onNBTDirtyChange} onClose={() => { setNBTPath(null); void load(); }} />;

  return (
    <>
      {dialog}
      {editorConflict && <FileConflictDialog
        key={editorConflict.revision}
        current={editorConflict}
        content={editorContent}
        busy={editorBusy}
        error={editorError}
        onCancel={() => { setEditorConflict(null); setEditorError(null); }}
        onReload={useServerVersion}
        onOverwrite={() => void saveEditor(editorConflict.revision)}
      />}
      <div className="space-y-4">
      <div><h2 className="section-heading">Files</h2><p className="mt-1 text-sm text-muted-foreground">Browse server files and edit configuration.</p></div>
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="hidden"
          onChange={(e) => {
            const files = e.target.files ? Array.from(e.target.files) : [];
            if (files.length) void handleUpload(files);
            e.target.value = '';
          }}
        />
        <Button size="sm" onClick={() => fileInputRef.current?.click()} disabled={!canWrite || busy}>
          <Upload className="h-4 w-4" /> Upload
        </Button>
        <Button variant="outline" size="sm" onClick={() => void handleMkdir()} disabled={!canWrite || busy}>
          <FolderPlus className="h-4 w-4" /> New folder
        </Button>
        <Button variant="outline" size="sm" onClick={() => void newFile()} disabled={!canWrite || busy || editorBusy}>
          <FilePlus2 className="h-4 w-4" /> New file
        </Button>
        <Button variant="outline" size="sm" onClick={() => setShowUrlInput((v) => !v)} disabled={!canWrite || busy}>
          <Globe className="h-4 w-4" /> From URL
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void handleArchiveSelected()}
          disabled={!canWrite || busy || selectedNames.size === 0}
          title="Zip selected items"
        >
          <Archive className="h-4 w-4 mr-1 text-primary" /> Zip ({selectedNames.size})
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void handleDeleteSelected()}
          disabled={!canDelete || busy || editorBusy || selectedNames.size === 0}
          className="text-destructive hover:text-destructive"
        >
          <Trash2 className="h-4 w-4" /> Delete selected ({selectedNames.size})
        </Button>
        <Button variant="ghost" size="icon" onClick={() => void load()} disabled={busy} title="Refresh" aria-label="Refresh">
          <RefreshCw className="h-4 w-4" />
        </Button>
      </div>

      {showUrlInput && (
        <div className="space-y-2 rounded-md border p-3">
          <Input
            aria-label="File URL"
            placeholder="https://example.com/file.jar"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
          <div className="flex items-center gap-2">
            <Input
              aria-label="Download filename"
              placeholder="Optional filename"
              value={urlName}
              onChange={(e) => setUrlName(e.target.value)}
            />
            <Button size="sm" onClick={() => void handleDownloadFromUrl()} disabled={!canWrite || busy || !url.trim()}>
              Download
            </Button>
          </div>
        </div>
      )}

      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}

      {/* Active Long-Running Task Progress Bar (Unzip, Archive, URL Download) */}
      {activeTask && (
        <div className="space-y-2 rounded-xl border border-primary/20 bg-primary/5 p-3.5 animate-fadeIn">
          <ProgressBar
            value={activeTask.percent}
            label={
              <span className="flex items-center gap-2 font-medium">
                <Loader2 className="h-4 w-4 animate-spin text-primary shrink-0" />
                <span>{activeTask.title}</span>
              </span>
            }
            subtext={activeTask.subtext}
            showPercent={activeTask.showPercent ?? (activeTask.percent !== undefined)}
            variant="default"
            size="md"
          />
        </div>
      )}

      {/* Upload Progress Bar */}
      {uploadProgress !== null && (
        <div className="space-y-2 rounded-xl border border-primary/20 bg-primary/5 p-3.5 animate-fadeIn">
          <ProgressBar
            value={uploadProgress}
            label={
              <span className="flex items-center gap-2 truncate font-medium">
                <Upload className="h-4 w-4 text-primary  shrink-0" />
                <span className="truncate">Uploading {uploadStats?.fileName || 'files'}...</span>
              </span>
            }
            subtext={
              uploadStats && uploadStats.total > 0
                ? `${formatSize(uploadStats.loaded)} / ${formatSize(uploadStats.total)}`
                : undefined
            }
            variant="default"
            size="md"
          />
        </div>
      )}

      <div className={`grid items-start gap-5 ${editorPath ? 'xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]' : ''}`}>
        {/* Left: browser */}
        <div className="min-w-0 rounded-lg border bg-card">
          {/* Breadcrumb */}
          <div className="flex min-w-0 flex-wrap items-center gap-1 border-b px-4 py-3 text-sm">
            <button
              type="button"
              disabled={busy}
              onClick={() => setCwd('')}
              className="inline-flex items-center rounded px-1.5 py-0.5 font-medium text-muted-foreground hover:bg-secondary hover:text-foreground"
            >
              <Folder className="mr-1 h-4 w-4 text-amber-600" /> server root
            </button>
            {segments().map((seg, i) => (
              <span key={`${i}-${seg}`} className="inline-flex items-center gap-1">
                <span className="text-muted-foreground/50">/</span>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => navigate(i + 1)}
                  className="max-w-[9rem] truncate rounded px-1.5 py-0.5 font-medium text-muted-foreground hover:bg-secondary hover:text-foreground"
                >
                  {seg}
                </button>
              </span>
            ))}
            {(cwd === '' || cwd.includes('/')) && (
              <Button aria-label="Go up" variant="ghost" size="icon" title="Go up" disabled={busy} onClick={goUp} className="ml-1 h-6 w-6">
                <ArrowUp className="h-3.5 w-3.5" />
              </Button>
            )}
          </div>

          {/* Selection header */}
          {entries.length > 0 && (
            <div className="flex items-center justify-between border-b px-3 py-1.5 text-xs text-muted-foreground bg-muted/20">
              <label className="flex items-center gap-1.5 cursor-pointer hover:text-foreground select-none">
                <input
                  type="checkbox"
                  disabled={busy}
                  ref={(input) => { if (input) input.indeterminate = selectedNames.size > 0 && selectedNames.size < entries.length; }}
                  checked={selectedNames.size > 0 && selectedNames.size === entries.length}
                  onChange={toggleSelectAll}
                  className="rounded border-border text-primary focus:ring-primary cursor-pointer"
                />
                <span>Select all ({entries.length})</span>
              </label>
              {selectedNames.size > 0 && (
                <span className="font-medium text-foreground">
                  {selectedNames.size} selected
                </span>
              )}
            </div>
          )}

          {/* Entry list */}
          <ContextMenu>
          <ContextMenuTrigger asChild>
          <div tabIndex={0} aria-label="File browser" className="max-h-[65vh] min-h-40 divide-y overflow-y-auto rounded-b-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring">
            {entries.length === 0 && (
              <div className="empty-state"><Folder /><h3 className="section-heading">Empty folder</h3><p>Upload a file or create a folder using the controls above.</p></div>
            )}
            {entries.map((e) => {
              const editable = isEditable(e);
              const isSelected = selectedNames.has(e.name);
              const multipleSelected = isSelected && selectedNames.size > 1;
              return (
                <ContextMenu key={e.name} onOpenChange={(open) => { if (open) selectContextEntry(e.name); }}>
                <ContextMenuTrigger asChild>
                <div
                  tabIndex={0}
                  aria-label={e.name}
                  onClick={(event) => {
                    if ((event.target as HTMLElement).closest('button, input')) return;
                    selectEntry(e.name, event);
                  }}
                  className={`flex select-none flex-col items-stretch gap-3 px-4 py-3 text-sm transition-colors sm:flex-row sm:items-center sm:justify-between ${
                    isSelected ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/40'
                  } focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring`}
                >
                  <div className="flex min-w-0 flex-1 items-center gap-2">
                    <input
                      type="checkbox"
                      aria-label={`Select ${e.name}`}
                      checked={isSelected}
                      disabled={busy}
                      onChange={(event) => selectEntry(e.name, { shiftKey: (event.nativeEvent as MouseEvent).shiftKey === true, ctrlKey: true, metaKey: false })}
                      onClick={(ev) => ev.stopPropagation()}
                      className="rounded border-border text-primary focus:ring-primary cursor-pointer shrink-0"
                    />
                    <button
                      type="button"
                      disabled={busy}
                      className="flex min-w-0 items-center gap-2 text-left truncate"
                      onClick={(event) => {
                        event.stopPropagation();
                        selectEntry(e.name, event);
                        if (!event.ctrlKey && !event.metaKey && !event.shiftKey) handleEntryClick(e);
                      }}
                    >
                      {e.is_directory ? (
                        <Folder className="h-4 w-4 shrink-0 text-amber-600" />
                      ) : isNBTFile(e.name) ? (
                        <Database className="h-4 w-4 shrink-0 text-primary" />
                      ) : editable ? (
                        <FileText className="h-4 w-4 shrink-0 text-sky-600" />
                      ) : e.name.toLowerCase().endsWith('.zip') ? (
                        <FileArchive className="h-4 w-4 shrink-0 text-sky-600" />
                      ) : (
                        <File className="h-4 w-4 shrink-0 text-muted-foreground" />
                      )}
                      <span className="truncate font-medium" title={e.name}>{e.name}</span>
                    </button>
                  </div>
                  <div className="flex shrink-0 flex-wrap items-center justify-end gap-1 border-t pt-2 sm:border-0 sm:pt-0">
                    <span className="mr-auto text-xs text-muted-foreground sm:mr-1">
                      {e.is_directory ? '' : formatSize(e.size_bytes)}
                    </span>
                    {!e.is_directory && (
                      <Button
                        variant="ghost"
                        size="icon"
                        title="Download"
                        aria-label="Download"
                        onClick={() => void handleDownload(e)}
                      >
                        <Download className="h-4 w-4" />
                      </Button>
                    )}
                    {!e.is_directory && isNBTFile(e.name) && (
                      <Button variant="ghost" size="icon" title="Open as NBT" aria-label={`Open ${e.name} as NBT`} disabled={busy} onClick={() => void openNBT(joinPath(cwd, e.name))}>
                        <Database className="h-4 w-4" />
                      </Button>
                    )}
                    {editable && (
                      <Button
                        variant="ghost"
                        size="icon"
                        title="Edit"
                        aria-label="Edit"
                        disabled={busy}
                        onClick={() => void openEditor(joinPath(cwd, e.name))}
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                    )}
                    {e.name.toLowerCase().endsWith('.zip') ? (
                      <Button
                        variant="ghost"
                        size="icon"
                        title="Unzip archive"
                        aria-label="Unzip"
                        disabled={!canWrite || busy}
                        onClick={() => void handleUnzip(e)}
                      >
                        <Layers className="h-4 w-4 text-sky-600" />
                      </Button>
                    ) : (
                      <Button
                        variant="ghost"
                        size="icon"
                        title="Archive as zip"
                        aria-label="Archive"
                        disabled={!canWrite || busy}
                        onClick={() => void handleArchive(e)}
                      >
                        <Archive className="h-4 w-4" />
                      </Button>
                    )}
                    <Button variant="ghost" size="icon" title="Rename" aria-label="Rename" disabled={!canWrite || busy} onClick={() => void handleRename(e)}>
                      <TextCursorInput className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      title="Delete"
                      aria-label="Delete"
                      disabled={!canDelete || busy || editorBusy}
                      onClick={() => void handleDelete(e)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                </ContextMenuTrigger>
                <ContextMenuContent aria-label={multipleSelected ? `Actions for ${selectedNames.size} selected items` : `Actions for ${e.name}`}>
                  <ContextMenuLabel>{multipleSelected ? `${selectedNames.size} items selected` : e.name}</ContextMenuLabel>
                  {multipleSelected ? <>
                    <ContextMenuItem disabled={!canWrite || busy} onSelect={() => void handleArchiveSelected()}><Archive />Zip selected ({selectedNames.size})</ContextMenuItem>
                    <ContextMenuSeparator />
                    <ContextMenuItem className="text-destructive focus:text-destructive" disabled={!canDelete || busy || editorBusy} onSelect={() => void handleDeleteSelected()}><Trash2 />Delete selected ({selectedNames.size})</ContextMenuItem>
                  </> : <>
                  {(e.is_directory || editable || isNBTFile(e.name)) && <ContextMenuItem disabled={busy || editorBusy} onSelect={() => handleEntryClick(e)}>
                    {e.is_directory ? <Folder /> : isNBTFile(e.name) ? <Database /> : <Pencil />}
                    {e.is_directory ? 'Open folder' : isNBTFile(e.name) ? 'Open as NBT' : 'Edit'}
                  </ContextMenuItem>}
                  {!e.is_directory && <ContextMenuItem disabled={busy} onSelect={() => void handleDownload(e)}><Download />Download</ContextMenuItem>}
                  <ContextMenuSeparator />
                  {!e.is_directory && e.name.toLowerCase().endsWith('.zip') ?
                    <ContextMenuItem disabled={!canWrite || busy} onSelect={() => void handleUnzip(e)}><Layers />Unzip archive</ContextMenuItem> :
                    <ContextMenuItem disabled={!canWrite || busy} onSelect={() => void handleArchive(e)}><Archive />Archive as zip</ContextMenuItem>}
                  <ContextMenuItem disabled={!canWrite || busy} onSelect={() => void handleRename(e)}><TextCursorInput />Rename</ContextMenuItem>
                  <ContextMenuSeparator />
                  <ContextMenuItem className="text-destructive focus:text-destructive" disabled={!canDelete || busy || editorBusy} onSelect={() => void handleDelete(e)}><Trash2 />Delete</ContextMenuItem>
                  </>}
                </ContextMenuContent>
                </ContextMenu>
              );
            })}
          </div>
          </ContextMenuTrigger>
          <ContextMenuContent aria-label="Folder actions">
            <ContextMenuLabel>{cwd || 'server root'}</ContextMenuLabel>
            <ContextMenuItem disabled={!canWrite || busy || editorBusy} onSelect={() => void newFile()}><FilePlus2 />New file</ContextMenuItem>
            <ContextMenuItem disabled={!canWrite || busy} onSelect={() => void handleMkdir()}><FolderPlus />New folder</ContextMenuItem>
            <ContextMenuItem disabled={!canWrite || busy} onSelect={() => fileInputRef.current?.click()}><Upload />Upload files</ContextMenuItem>
            <ContextMenuItem disabled={!canWrite || busy} onSelect={() => setShowUrlInput(true)}><Globe />Download from URL</ContextMenuItem>
            {selectedNames.size > 0 && <>
              <ContextMenuSeparator />
              <ContextMenuItem disabled={!canWrite || busy} onSelect={() => void handleArchiveSelected()}><Archive />Zip selected ({selectedNames.size})</ContextMenuItem>
              <ContextMenuItem className="text-destructive focus:text-destructive" disabled={!canDelete || busy || editorBusy} onSelect={() => void handleDeleteSelected()}><Trash2 />Delete selected ({selectedNames.size})</ContextMenuItem>
            </>}
            <ContextMenuSeparator />
            <ContextMenuItem disabled={busy} onSelect={() => void load()}><RefreshCw />Refresh</ContextMenuItem>
          </ContextMenuContent>
          </ContextMenu>
        </div>

        {/* Right: editor */}
        {(editorPath !== null || editorBusy) && <div className="min-w-0 rounded-lg border bg-card">
          {editorPath === null && !editorBusy ? (
            <div className="flex h-full min-h-[20rem] flex-col items-center justify-center gap-2 p-6 text-center text-sm text-muted-foreground">
              <FileText className="h-10 w-10 text-muted-foreground/40" />
              <p>Select a file to view and edit it, or create a new file.</p>
            </div>
          ) : (
            <div className="flex h-full flex-col">
              <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
                <div className="flex min-w-0 items-center gap-2">
                  <FileText className="h-4 w-4 shrink-0 text-sky-600" />
                  <span className="truncate font-mono text-sm">{editorPath ?? ''}</span>
                  {editorDirty && (
                    <span className="shrink-0 rounded bg-amber-500/15 px-1.5 py-0.5 text-xs text-amber-700 dark:text-amber-400">
                      unsaved
                    </span>
                  )}
                </div>
                <div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
                  <Button
                    size="sm"
                    onClick={() => void saveEditor()}
                    disabled={!canWrite || busy || editorBusy || !editorPath || editorRevision === null}
                  >
                    <Save className="h-4 w-4" /> {editorBusy ? 'Saving...' : 'Save'}
                  </Button>
                  <Button variant="ghost" size="icon" disabled={editorBusy} onClick={() => void closeEditor()} title="Close" aria-label="Close editor">
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              </div>
              {editorError && !editorConflict && <p role="alert" className="border-b px-3 py-2 text-sm text-destructive">{editorError}</p>}
              <Textarea
                aria-label="File content"
                disabled={!canWrite || busy || editorBusy || editorRevision === null}
                className="min-h-[26rem] flex-1 resize-none rounded-none border-0 bg-transparent font-mono text-xs leading-relaxed whitespace-pre focus-visible:ring-0"
                value={editorContent}
                spellCheck={false}
                onChange={(e) => {
                  setEditorContent(e.target.value);
                  setEditorDirty(true);
                }}
                placeholder="# Edit file content"
              />
              <div className="border-t px-3 py-2 text-xs text-muted-foreground">
                Editing from server root: <span className="font-mono">{cwd ? `${cwd}` : '/ (data)'}</span>
              </div>
            </div>
          )}
        </div>}
      </div>
      </div>
    </>
  );
}
