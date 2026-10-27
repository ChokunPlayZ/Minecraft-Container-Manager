import { useMemo, useState } from 'react';
import { AlertTriangle, ChevronLeft, ChevronRight, Loader2 } from 'lucide-react';
import type { FileContent } from '../api/types';
import { compareFileLines } from '../lib/file-diff';
import { Button } from './ui/button';
import { DialogBackdrop } from './ui/dialog-backdrop';

const PAGE_SIZE = 200;

export function FileConflictDialog({ current, content, busy, error, onCancel, onReload, onOverwrite }: {
  current: FileContent;
  content: string;
  busy: boolean;
  error: string | null;
  onCancel: () => void;
  onReload: () => void;
  onOverwrite: () => void;
}) {
  const rows = useMemo(() => compareFileLines(current.content, content), [current.content, content]);
  const [page, setPage] = useState(0);
  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
  const visible = rows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  return (
    <DialogBackdrop onDismiss={() => { if (!busy) onCancel(); }} className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="flex max-h-full w-full max-w-6xl flex-col overflow-hidden rounded-xl border bg-card text-card-foreground" aria-describedby="file-conflict-description" aria-busy={busy}>
        <div className="shrink-0 border-b p-4 sm:p-6">
          <h2 className="flex items-center gap-2 text-lg font-semibold"><AlertTriangle className="h-5 w-5 shrink-0 text-amber-700 dark:text-amber-400" aria-hidden="true" />File changed on server</h2>
          <p className="mt-2 break-all font-mono text-sm">{current.path}</p>
          <p id="file-conflict-description" className="mt-2 text-sm text-muted-foreground">
            {current.exists
              ? 'This file changed since you opened it. Compare the server version with your edits before replacing it.'
              : 'This file was deleted since you opened it. Your edits are still available. You can keep editing or recreate the file.'}
          </p>
        </div>
        <div className="min-h-0 flex-1 overflow-auto">
          {error && <p role="alert" className="border-b px-4 py-3 text-sm text-destructive sm:px-6">{error}</p>}
          <div className="grid md:grid-cols-2">
            {(['server', 'yours'] as const).map((side) => (
              <section key={side} className="min-w-0 border-b md:border-b-0 md:first:border-r" aria-label={side === 'server' ? 'Server version comparison' : 'Your edits comparison'}>
                <h3 className="flex items-center justify-between gap-2 border-b bg-muted/50 px-4 py-3 text-sm font-medium">
                  {side === 'server' ? 'Server version' : 'Your edits'}
                  <span className="text-xs font-normal text-muted-foreground">{side === 'server' ? '− replaced lines' : '+ your lines'}</span>
                </h3>
                <div key={page} tabIndex={0} role="region" aria-label={side === 'server' ? 'Server file content' : 'Your file content'} className="max-h-[42vh] overflow-auto font-mono text-xs leading-6 focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring">
                  {visible.map((row, index) => {
                    const line = row[side];
                    return <div key={page * PAGE_SIZE + index} className={`flex min-h-6 min-w-max ${row.changed && line ? side === 'server' ? 'bg-destructive/10' : 'bg-primary/10' : ''}`}>
                      <span aria-hidden="true" className="sticky left-0 w-12 shrink-0 select-none bg-card px-2 text-right tabular-nums text-muted-foreground">{line?.number ?? ''}</span>
                      <span className="w-5 shrink-0 select-none text-center text-muted-foreground">{row.changed && line ? side === 'server' ? '−' : '+' : ''}</span>
                      <pre className="m-0 px-2 font-mono">{line?.text || ' '}</pre>
                    </div>;
                  })}
                  {rows.length === 0 && <p className="px-4 py-6 text-muted-foreground">Empty file</p>}
                  {side === 'server' && !current.exists && <p className="px-4 py-3 font-sans text-sm text-muted-foreground">File no longer exists on the server.</p>}
                </div>
              </section>
            ))}
          </div>
        </div>
        {pages > 1 && <div className="flex shrink-0 items-center justify-between gap-2 border-t px-4 py-2 text-xs text-muted-foreground">
          <span aria-live="polite">Comparison page {page + 1} of {pages}</span>
          <div className="flex gap-1">
            <Button variant="ghost" size="icon" aria-label="Previous comparison page" disabled={page === 0} onClick={() => setPage(page - 1)}><ChevronLeft /></Button>
            <Button variant="ghost" size="icon" aria-label="Next comparison page" disabled={page + 1 === pages} onClick={() => setPage(page + 1)}><ChevronRight /></Button>
          </div>
        </div>}
        <div className="flex shrink-0 flex-col gap-2 border-t p-4 sm:flex-row sm:flex-wrap sm:justify-end sm:p-6">
          <Button variant="outline" disabled={busy} onClick={onCancel}>Keep editing</Button>
          {current.exists && <Button variant="outline" disabled={busy} onClick={onReload}>Use server version</Button>}
          <Button variant="destructive" disabled={busy} onClick={onOverwrite}>
            {busy && <Loader2 className="animate-spin" aria-hidden="true" />}
            {busy ? 'Saving…' : current.exists ? 'Overwrite server file' : 'Recreate file'}
          </Button>
        </div>
      </div>
    </DialogBackdrop>
  );
}
