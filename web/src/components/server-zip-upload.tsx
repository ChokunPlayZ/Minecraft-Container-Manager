import { useId, useRef, useState, type DragEvent } from 'react';
import { FileArchive, Upload, X } from 'lucide-react';
import { formatBytes } from '../lib/formatters';
import { cn } from '../lib/utils';
import { Button } from './ui/button';

export function ServerZipUpload({ file, onChange, disabled = false }: {
  file: File | null;
  onChange: (file: File | null) => void;
  disabled?: boolean;
}) {
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  const dragDepth = useRef(0);
  const [dragging, setDragging] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function selectFiles(files: File[]) {
    if (disabled || files.length === 0) return;
    if (files.length !== 1) {
      setError('Choose one server ZIP at a time.');
      return;
    }
    const selected = files[0];
    if (!selected.name.toLowerCase().endsWith('.zip')) {
      setError('Choose a .zip file exported from MCM.');
      return;
    }
    if (selected.size === 0 || selected.size > 2 * 1024 ** 3) {
      setError('Choose a non-empty ZIP file of at most 2 GiB.');
      return;
    }
    setError(null);
    onChange(selected);
  }

  function hasFiles(event: DragEvent) {
    return Array.from(event.dataTransfer.types).includes('Files');
  }

  return <div className="space-y-2">
    <p id={`${id}-label`} className="text-sm font-medium">Server ZIP file</p>
    <div role="group" aria-label="ZIP upload area" aria-describedby={`${id}-help`} aria-disabled={disabled}
      className={cn('flex min-h-40 flex-col justify-center rounded-lg border border-dashed p-4 transition-colors duration-150',
        dragging && !disabled ? 'border-primary bg-primary/10' : error ? 'border-destructive/60 bg-destructive/5' : 'border-input bg-muted/20',
        disabled && 'opacity-60')}
      onDragEnter={(event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        if (disabled) return;
        dragDepth.current += 1;
        setDragging(true);
      }}
      onDragOver={(event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = disabled ? 'none' : 'copy';
      }}
      onDragLeave={(event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        dragDepth.current = Math.max(0, dragDepth.current - 1);
        if (dragDepth.current === 0) setDragging(false);
      }}
      onDrop={(event) => {
        event.preventDefault();
        dragDepth.current = 0;
        setDragging(false);
        selectFiles(Array.from(event.dataTransfer.files));
      }}>
      <input ref={input} type="file" accept=".zip,application/zip" className="sr-only" tabIndex={-1}
        aria-label="Server ZIP file" aria-required="true" disabled={disabled} aria-invalid={!!error}
        aria-describedby={`${id}-help${error ? ` ${id}-error` : ''}`}
        onChange={(event) => {
          selectFiles(Array.from(event.target.files ?? []));
          event.target.value = '';
        }} />
      {file ? <>
        <div className="flex items-start gap-3">
          <FileArchive className="mt-1 h-7 w-7 shrink-0 text-primary" aria-hidden="true" />
          <div role="status" className="min-w-0 flex-1">
            <p className="break-words text-sm font-medium [overflow-wrap:anywhere]">{file.name}</p>
            <p className="mt-1 text-xs text-muted-foreground">{formatBytes(file.size)} · ZIP archive</p>
          </div>
          <Button type="button" variant="ghost" size="icon" className="shrink-0" aria-label="Remove selected ZIP" disabled={disabled} onClick={() => {
            setError(null);
            onChange(null);
            if (input.current) input.current.value = '';
          }}><X className="h-4 w-4" /></Button>
        </div>
        <div className="mt-4 flex flex-wrap items-center justify-between gap-2">
          <p className="text-xs text-muted-foreground">{dragging ? 'Drop to replace this ZIP' : 'Drop another ZIP to replace it'}</p>
          <Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => input.current?.click()}>Change ZIP</Button>
        </div>
      </> : <div className="flex flex-col items-center gap-3 text-center">
        <Upload className="h-7 w-7 text-primary" aria-hidden="true" />
        <div><p className="text-sm font-medium">{dragging ? 'Drop your ZIP to select it' : 'Drop your server ZIP here'}</p><p className="mt-1 text-xs text-muted-foreground">or choose a file from your device</p></div>
        <Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => input.current?.click()}>Choose ZIP</Button>
      </div>}
    </div>
    <p id={`${id}-help`} className="text-xs text-muted-foreground">MCM export ZIP · Up to 2 GiB compressed, 8 GiB extracted.</p>
    {error && <p id={`${id}-error`} role="alert" className="text-sm text-destructive">{error}{file && ' Your selected ZIP has been kept.'}</p>}
  </div>;
}
