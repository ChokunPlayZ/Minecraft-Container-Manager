import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Archive,
  ArrowUp,
  Download,
  File,
  FileArchive,
  Folder,
  FolderPlus,
  Globe,
  Layers,
  Pencil,
  Trash2,
  Upload,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { FileEntry, Server } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';

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

export function FilesPanel({ server }: { server: Server }) {
  const [cwd, setCwd] = useState('');
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [showUrlInput, setShowUrlInput] = useState(false);
  const [url, setUrl] = useState('');
  const [urlName, setUrlName] = useState('');

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

  async function handleUpload(file: File) {
    await run(() => api.uploadFile(server.id, file, cwd));
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
    await run(() => api.deleteFile(server.id, joinPath(cwd, entry.name)));
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

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <Layers className="h-4 w-4" /> Files
        </CardTitle>
        <CardDescription>Browse and manage the server&apos;s data directory.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Breadcrumb */}
        <div className="flex flex-wrap items-center gap-1 text-sm">
          <button
            type="button"
            onClick={() => setCwd('')}
            className="inline-flex items-center rounded px-1.5 py-0.5 font-medium text-muted-foreground hover:bg-secondary hover:text-foreground"
          >
            data
          </button>
          {segments().map((seg, i) => (
            <span key={`${i}-${seg}`} className="inline-flex items-center gap-1">
              <span className="text-muted-foreground/50">/</span>
              <button
                type="button"
                onClick={() => navigate(i + 1)}
                className="rounded px-1.5 py-0.5 font-medium text-muted-foreground hover:bg-secondary hover:text-foreground"
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

        {/* Toolbar */}
        <div className="flex flex-wrap items-center gap-2">
          <input
            ref={fileInputRef}
            type="file"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) void handleUpload(f);
              e.target.value = '';
            }}
          />
          <Button variant="outline" size="sm" onClick={() => fileInputRef.current?.click()} disabled={busy}>
            <Upload className="h-4 w-4" /> Upload
          </Button>
          <Button variant="outline" size="sm" onClick={() => void handleMkdir()} disabled={busy}>
            <FolderPlus className="h-4 w-4" /> New folder
          </Button>
          <Button variant="outline" size="sm" onClick={() => setShowUrlInput((v) => !v)} disabled={busy}>
            <Globe className="h-4 w-4" /> From URL
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

        {/* Entry list */}
        <div className="space-y-2">
          {entries.length === 0 && (
            <p className="text-sm text-muted-foreground">This folder is empty.</p>
          )}
          {entries.map((e) => (
            <div
              key={e.name}
              className="flex items-center justify-between gap-2 rounded-md border p-2 text-sm"
            >
              <button
                type="button"
                className="flex min-w-0 items-center gap-2.5 text-left"
                disabled={!e.is_directory}
                onClick={() => e.is_directory && setCwd(joinPath(cwd, e.name))}
              >
                {e.is_directory ? (
                  <Folder className="h-4 w-4 shrink-0 text-amber-600" />
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
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

