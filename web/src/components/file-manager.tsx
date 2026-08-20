import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Archive,
  ArrowUp,
  Download,
  File,
  FileArchive,
  FilePlus2,
  FileText,
  Folder,
  FolderPlus,
  Globe,
  Layers,
  Pencil,
  RefreshCw,
  Save,
  Trash2,
  Upload,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { FileEntry, Server } from '../api/types';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Textarea } from './ui/textarea';

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
  'bat', 'ps1', 'cmd', 'csv', 'log', 'tsv', 'sql', 'nbt', 'dat', 'mcmeta', 'mcfunction',
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
  const lower = entry.name.toLowerCase();
  const idx = lower.lastIndexOf('.');
  const ext = idx >= 0 ? lower.slice(idx + 1) : '';
  return TEXT_EXTENSIONS.has(ext) || EDITABLE_NAMES.has(lower);
}

export function FileManager({ server }: { server: Server }) {
  const [cwd, setCwd] = useState('');
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [showUrlInput, setShowUrlInput] = useState(false);
  const [url, setUrl] = useState('');
  const [urlName, setUrlName] = useState('');

  // Editor state.
  const [editorPath, setEditorPath] = useState<string | null>(null);
  const [editorContent, setEditorContent] = useState('');
  const [editorDirty, setEditorDirty] = useState(false);
  const [editorError, setEditorError] = useState<string | null>(null);
  const [editorBusy, setEditorBusy] = useState(false);

  const load = useCallback(async () => {
    setError(null);
    try {
      const res = await api.listFiles(server.id, cwd);
      setEntries(res.entries ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load files');
    }
  }, [server.id, cwd]);

  useEffect(() => {
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

  function closeEditor() {
    if (editorDirty && !window.confirm('Discard unsaved changes?')) return;
    setEditorPath(null);
    setEditorContent('');
    setEditorDirty(false);
    setEditorError(null);
  }

  async function openEditor(path: string) {
    if (editorDirty && editorPath !== path && !window.confirm('Discard unsaved changes?')) return;
    setEditorBusy(true);
    setEditorError(null);
    setEditorPath(path);
    try {
      const res = await api.readFileContent(server.id, path);
      setEditorContent(res.content);
      setEditorDirty(false);
    } catch (err) {
      setEditorContent('');
      setEditorDirty(false);
      setEditorError(err instanceof ApiError ? err.detail : 'Failed to load file');
    } finally {
      setEditorBusy(false);
    }
  }

  function newFile() {
    const name = window.prompt('New file name (e.g. ops.json):', 'new-file.txt');
    if (!name || !name.trim()) return;
    const base = cwd ? `${cwd}/${name.trim()}` : name.trim();
    if (editorDirty && !window.confirm('Discard unsaved changes?')) return;
    setEditorPath(base);
    setEditorContent('');
    setEditorDirty(true);
    setEditorError(null);
  }

  async function saveEditor() {
    if (!editorPath) return;
    setEditorBusy(true);
    setEditorError(null);
    try {
      await api.writeFileContent(server.id, editorPath, editorContent);
      setEditorDirty(false);
      await load();
    } catch (err) {
      setEditorError(err instanceof ApiError ? err.detail : 'Failed to save file');
    } finally {
      setEditorBusy(false);
    }
  }

  async function handleUpload(files: File[]) {
    if (files.length === 0) return;
    setBusy(true);
    setError(null);
    setUploadProgress(0);
    try {
      await api.uploadFiles(server.id, files, cwd, (loaded, total) => {
        setUploadProgress(total > 0 ? Math.round((loaded / total) * 100) : 0);
      });
      setUploadProgress(null);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Upload failed');
      setUploadProgress(null);
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

  async function handleArchive(entry: FileEntry) {
    await run(() => api.archiveFile(server.id, joinPath(cwd, entry.name)));
  }

  async function handleUnzip(entry: FileEntry) {
    await run(() => api.unzipFile(server.id, joinPath(cwd, entry.name), cwd));
  }

  async function handleRename(entry: FileEntry) {
    const name = window.prompt(`Rename "${entry.name}" to:`, entry.name);
    if (!name || name.trim() === entry.name) return;
    await run(() => api.renameFile(server.id, joinPath(cwd, entry.name), name.trim()));
  }

  async function handleDelete(entry: FileEntry) {
    const what = entry.is_directory ? 'folder' : 'file';
    if (!window.confirm(`Delete ${what} "${entry.name}"? This cannot be undone.`)) return;
    const path = joinPath(cwd, entry.name);
    await run(() => api.deleteFile(server.id, path));
    if (path === editorPath) closeEditor();
  }

  async function handleMkdir() {
    const name = window.prompt('New folder name:');
    if (!name || !name.trim()) return;
    await run(() => api.mkdir(server.id, joinPath(cwd, name.trim())));
  }

  async function handleDownloadFromUrl() {
    const trimmed = url.trim();
    if (!trimmed) return;
    await run(() => api.downloadFromUrl(server.id, trimmed, cwd, urlName.trim() || undefined));
    setUrl('');
    setUrlName('');
    setShowUrlInput(false);
  }

  function handleEntryClick(entry: FileEntry) {
    if (entry.is_directory) {
      setCwd(joinPath(cwd, entry.name));
      return;
    }
    if (isEditable(entry)) {
      void openEditor(joinPath(cwd, entry.name));
      return;
    }
    void handleDownload(entry);
  }

  return (
    <div className="space-y-4">
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
        <Button variant="outline" size="sm" onClick={() => fileInputRef.current?.click()} disabled={busy}>
          <Upload className="h-4 w-4" /> Upload
        </Button>
        <Button variant="outline" size="sm" onClick={() => void handleMkdir()} disabled={busy}>
          <FolderPlus className="h-4 w-4" /> New folder
        </Button>
        <Button variant="outline" size="sm" onClick={() => newFile()} disabled={busy}>
          <FilePlus2 className="h-4 w-4" /> New file
        </Button>
        <Button variant="outline" size="sm" onClick={() => setShowUrlInput((v) => !v)} disabled={busy}>
          <Globe className="h-4 w-4" /> From URL
        </Button>
        <Button variant="ghost" size="icon" onClick={() => void load()} title="Refresh" aria-label="Refresh">
          <RefreshCw className="h-4 w-4" />
        </Button>
      </div>

      {showUrlInput && (
        <div className="space-y-2 rounded-md border p-3">
          <Input
            placeholder="https://example.com/file.jar"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
          <div className="flex items-center gap-2">
            <Input
              placeholder="Optional filename"
              value={urlName}
              onChange={(e) => setUrlName(e.target.value)}
            />
            <Button size="sm" onClick={() => void handleDownloadFromUrl()} disabled={busy || !url.trim()}>
              Download
            </Button>
          </div>
        </div>
      )}

      {error && <p className="text-sm text-destructive">{error}</p>}

      {uploadProgress !== null && (
        <div className="flex items-center gap-3 rounded-md border p-3 text-sm">
          <span className="shrink-0 text-muted-foreground">Uploading...</span>
          <div className="h-2 flex-1 overflow-hidden rounded-full bg-secondary">
            <div
              className="h-full bg-primary transition-all"
              style={{ width: `${uploadProgress}%` }}
            />
          </div>
          <span className="shrink-0 tabular-nums text-muted-foreground">{uploadProgress}%</span>
        </div>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        {/* Left: browser */}
        <div className="rounded-lg border">
          {/* Breadcrumb */}
          <div className="flex items-center gap-1 border-b px-3 py-2 text-sm">
            <button
              type="button"
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
                  onClick={() => navigate(i + 1)}
                  className="max-w-[9rem] truncate rounded px-1.5 py-0.5 font-medium text-muted-foreground hover:bg-secondary hover:text-foreground"
                >
                  {seg}
                </button>
              </span>
            ))}
            {(cwd === '' || cwd.includes('/')) && (
              <Button variant="ghost" size="icon" title="Go up" onClick={goUp} className="ml-1 h-6 w-6">
                <ArrowUp className="h-3.5 w-3.5" />
              </Button>
            )}
          </div>

          {/* Entry list */}
          <div className="max-h-[60vh] divide-y overflow-y-auto">
            {entries.length === 0 && (
              <p className="p-4 text-sm text-muted-foreground">This folder is empty.</p>
            )}
            {entries.map((e) => {
              const editable = isEditable(e);
              return (
                <div
                  key={e.name}
                  className="flex items-center justify-between gap-2 p-2 text-sm hover:bg-accent/40"
                >
                  <button
                    type="button"
                    className="flex min-w-0 items-center gap-2.5 text-left"
                    onClick={() => handleEntryClick(e)}
                  >
                    {e.is_directory ? (
                      <Folder className="h-4 w-4 shrink-0 text-amber-600" />
                    ) : editable ? (
                      <FileText className="h-4 w-4 shrink-0 text-sky-600" />
                    ) : e.name.toLowerCase().endsWith('.zip') ? (
                      <FileArchive className="h-4 w-4 shrink-0 text-sky-600" />
                    ) : (
                      <File className="h-4 w-4 shrink-0 text-muted-foreground" />
                    )}
                    <span className="truncate font-medium">{e.name}</span>
                  </button>
                  <div className="flex shrink-0 items-center gap-1">
                    <span className="mr-1 text-xs text-muted-foreground">
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
                    {editable && (
                      <Button
                        variant="ghost"
                        size="icon"
                        title="Edit"
                        aria-label="Edit"
                        onClick={() => void openEditor(joinPath(cwd, e.name))}
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                    )}
                    {e.is_directory || e.name.toLowerCase().endsWith('.zip') ? (
                      <Button
                        variant="ghost"
                        size="icon"
                        title={e.is_directory ? 'Archive as zip' : 'Unzip'}
                        aria-label={e.is_directory ? 'Archive' : 'Unzip'}
                        onClick={() => void (e.is_directory ? handleArchive(e) : handleUnzip(e))}
                      >
                        {e.is_directory ? (
                          <Archive className="h-4 w-4" />
                        ) : (
                          <Layers className="h-4 w-4" />
                        )}
                      </Button>
                    ) : (
                      <Button
                        variant="ghost"
                        size="icon"
                        title="Archive as zip"
                        aria-label="Archive"
                        onClick={() => void handleArchive(e)}
                      >
                        <Archive className="h-4 w-4" />
                      </Button>
                    )}
                    <Button variant="ghost" size="icon" title="Rename" aria-label="Rename" onClick={() => void handleRename(e)}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      title="Delete"
                      aria-label="Delete"
                      onClick={() => void handleDelete(e)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {/* Right: editor */}
        <div className="rounded-lg border">
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
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    size="sm"
                    onClick={() => void saveEditor()}
                    disabled={editorBusy || !editorPath}
                  >
                    <Save className="h-4 w-4" /> {editorBusy ? 'Saving...' : 'Save'}
                  </Button>
                  <Button variant="ghost" size="icon" onClick={closeEditor} title="Close" aria-label="Close editor">
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              </div>
              {editorError && <p className="border-b px-3 py-2 text-sm text-destructive">{editorError}</p>}
              <Textarea
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
        </div>
      </div>
    </div>
  );
}
