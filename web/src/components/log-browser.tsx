import { useEffect, useMemo, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight, FileText, RefreshCw, Search, X } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { ServerLogContent, ServerLogFile } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';

const LINES_PER_PAGE = 250;

function errorMessage(err: unknown) {
  return err instanceof ApiError ? err.detail : 'Could not load the saved logs. Try refreshing.';
}

export function LogBrowser({ serverId }: { serverId: string }) {
  const [files, setFiles] = useState<ServerLogFile[]>([]);
  const [selectedPath, setSelectedPath] = useState('');
  const [members, setMembers] = useState<ServerLogFile[]>([]);
  const [selectedEntry, setSelectedEntry] = useState('');
  const [preview, setPreview] = useState<ServerLogContent | null>(null);
  const [loadingFiles, setLoadingFiles] = useState(true);
  const [loadingMembers, setLoadingMembers] = useState(false);
  const [loadingContent, setLoadingContent] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);
  const [contentError, setContentError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  const [filter, setFilter] = useState('');
  const [wrap, setWrap] = useState(true);
  const [page, setPage] = useState(0);
  const outputRef = useRef<HTMLDivElement>(null);
  const selectedFile = files.find((file) => file.path === selectedPath);
  const isArchive = selectedFile?.archive ?? false;

  useEffect(() => {
    let active = true;
    setLoadingFiles(true);
    setFileError(null);
    api.listServerLogs(serverId).then(({ files: next }) => {
      if (!active) return;
      setFiles(next);
      setSelectedPath((current) => next.some((file) => file.path === current) ? current : next[0]?.path ?? '');
    }).catch((err: unknown) => {
      if (active) setFileError(errorMessage(err));
    }).finally(() => {
      if (active) setLoadingFiles(false);
    });
    return () => { active = false; };
  }, [serverId, revision]);

  useEffect(() => {
    let active = true;
    setMembers([]);
    setSelectedEntry('');
    setContentError(null);
    setLoadingMembers(isArchive);
    if (isArchive) {
      api.listServerLogs(serverId, selectedPath).then(({ files: next }) => {
        if (!active) return;
        setMembers(next);
        setSelectedEntry(next[0]?.path ?? '');
      }).catch((err: unknown) => {
        if (active) setContentError(errorMessage(err));
      }).finally(() => {
        if (active) setLoadingMembers(false);
      });
    }
    return () => { active = false; };
  }, [serverId, selectedPath, isArchive, revision]);

  useEffect(() => {
    let active = true;
    setPreview(null);
    setPage(0);
    const ready = !!selectedPath && (!isArchive || !!selectedEntry);
    setLoadingContent(ready);
    if (ready) {
      setContentError(null);
      api.readServerLog(serverId, selectedPath, isArchive ? selectedEntry : undefined).then((next) => {
        if (active) setPreview(next);
      }).catch((err: unknown) => {
        if (active) setContentError(errorMessage(err));
      }).finally(() => {
        if (active) setLoadingContent(false);
      });
    }
    return () => { active = false; };
  }, [serverId, selectedPath, selectedEntry, isArchive, revision]);

  const lines = useMemo(() => {
    if (!preview?.content) return [];
    const text = preview.content.replace(/\r\n/g, '\n');
    const split = text.split('\n');
    if (split[split.length - 1] === '') split.pop();
    return split.map((text, index) => ({ number: index + 1, text }));
  }, [preview]);
  const filtered = useMemo(() => {
    const query = filter.trim().toLowerCase();
    return query ? lines.filter((line) => line.text.toLowerCase().includes(query)) : lines;
  }, [lines, filter]);
  const pageCount = Math.max(1, Math.ceil(filtered.length / LINES_PER_PAGE));
  const currentPage = Math.min(page, pageCount - 1);
  const visibleLines = filtered.slice(currentPage * LINES_PER_PAGE, (currentPage + 1) * LINES_PER_PAGE);
  const loading = loadingFiles || loadingMembers || loadingContent;

  useEffect(() => {
    if (outputRef.current) outputRef.current.scrollTop = 0;
  }, [currentPage, preview, filter]);

  return (
    <Card className="min-w-0 overflow-hidden">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-5">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <FileText className="h-4 w-4 text-primary" aria-hidden="true" />
            <CardTitle className="text-base">Saved server logs</CardTitle>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">Read current and archived logs without extracting files.</p>
        </div>
        <Button variant="outline" size="sm" disabled={loading} onClick={() => setRevision((value) => value + 1)}>
          <RefreshCw className={loading ? 'animate-spin' : ''} aria-hidden="true" />
          Refresh logs
        </Button>
      </CardHeader>
      <CardContent className="space-y-4 p-4 sm:p-5">
        {fileError && <p role="alert" className="text-sm text-destructive">{fileError}</p>}
        {!loadingFiles && !fileError && files.length === 0 ? (
          <div className="py-12 text-center">
            <h3 className="font-medium">No saved logs yet</h3>
            <p className="mx-auto mt-2 max-w-md text-sm text-muted-foreground">Logs will appear here when the server writes files to logs/ or crash-reports/.</p>
          </div>
        ) : (
          <>
            <div className={`grid min-w-0 gap-4 ${isArchive ? 'sm:grid-cols-2' : ''}`}>
              <div className="min-w-0 space-y-2">
                <Label htmlFor="server-log-file">Log file</Label>
                <Select id="server-log-file" value={selectedPath} disabled={loadingFiles || !files.length} onValueChange={(selectedValue) => {
                  setSelectedPath(selectedValue);
                  setSelectedEntry('');
                }}>
                  {!files.length && <option value="">{loadingFiles ? 'Loading logs…' : 'No logs available'}</option>}
                  {files.map((file) => <option key={file.path} value={file.path}>{file.path}</option>)}
                </Select>
                {selectedFile && <p className="break-words text-xs text-muted-foreground">{(selectedFile.size_bytes / 1024).toLocaleString(undefined, { maximumFractionDigits: 1 })} KB · Modified {new Date(selectedFile.modified_at).toLocaleString()}</p>}
              </div>
              {isArchive && (
                <div className="min-w-0 space-y-2">
                  <Label htmlFor="server-log-entry">Log inside archive</Label>
                  <Select id="server-log-entry" value={selectedEntry} disabled={loadingMembers || !members.length} onValueChange={(selectedValue) => setSelectedEntry(selectedValue)}>
                    {!members.length && <option value="">{loadingMembers ? 'Reading archive…' : 'No log files in this archive'}</option>}
                    {members.map((file) => <option key={file.path} value={file.path}>{file.path}</option>)}
                  </Select>
                </div>
              )}
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <div className="relative min-w-0 flex-1 basis-48">
                <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" aria-hidden="true" />
                <Input aria-label="Search saved log" placeholder="Search this log…" value={filter} onChange={(event) => {
                  setFilter(event.target.value);
                  setPage(0);
                }} className="pl-9 pr-10" />
                {filter && <Button variant="ghost" size="icon" className="absolute right-0 top-0" aria-label="Clear log search" onClick={() => { setFilter(''); setPage(0); }}><X /></Button>}
              </div>
              <label className="flex shrink-0 items-center gap-2 text-sm">
                <input type="checkbox" checked={wrap} onChange={(event) => setWrap(event.target.checked)} className="h-4 w-4 accent-primary" />
                Wrap lines
              </label>
            </div>
            {contentError && <p role="alert" className="text-sm text-destructive">{contentError}</p>}
            {preview?.truncated && <p role="status" className="text-sm text-muted-foreground">Showing the first {preview.limit_bytes / (1024 * 1024)} MB. Search covers this preview. Download the full file from Files to read the rest.</p>}
            <div ref={outputRef} role="region" aria-label="Saved log content" aria-busy={loading} tabIndex={0} className="console-output h-[28rem] overflow-auto rounded-md p-3 font-mono text-xs leading-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:p-4">
              {loading ? <p role="status">Loading saved log…</p> : contentError || fileError ? <p>The log could not be loaded. Refresh logs to try again.</p> : !preview ? <p>{isArchive ? 'This archive contains no readable .log or .txt files.' : 'Select a log file to read it.'}</p> : lines.length === 0 ? <p>This log is empty.</p> : filtered.length === 0 ? <p>No lines match your search.</p> : visibleLines.map((line) => (
                <div key={line.number} className="flex min-w-full items-start gap-4">
                  <span aria-hidden="true" className="w-12 shrink-0 select-none text-right tabular-nums opacity-70">{line.number}</span>
                  <span className={wrap ? 'min-w-0 whitespace-pre-wrap break-words [overflow-wrap:anywhere]' : 'whitespace-pre'}>{line.text || '\u00a0'}</span>
                </div>
              ))}
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
              <p role="status">{preview ? `${filtered.length.toLocaleString()}${filter.trim() ? ` matching / ${lines.length.toLocaleString()}` : ''} lines${filtered.length ? ` · Showing ${(currentPage * LINES_PER_PAGE + 1).toLocaleString()}–${Math.min((currentPage + 1) * LINES_PER_PAGE, filtered.length).toLocaleString()}` : ''}` : 'Saved logs: .log, .txt, .gz, .tar, .tar.gz, .tgz'}</p>
              <div className="flex items-center gap-2">
                <Button variant="outline" size="sm" aria-label="Previous log page" disabled={loading || currentPage === 0} onClick={() => setPage(currentPage - 1)}><ChevronLeft /></Button>
                <span className="tabular-nums">Page {currentPage + 1} of {pageCount}</span>
                <Button variant="outline" size="sm" aria-label="Next log page" disabled={loading || currentPage >= pageCount - 1} onClick={() => setPage(currentPage + 1)}><ChevronRight /></Button>
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
