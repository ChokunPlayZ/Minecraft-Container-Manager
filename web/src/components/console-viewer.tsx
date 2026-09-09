import { useEffect, useLayoutEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from 'react';
import {
  ArrowDownToLine,
  Eraser,
  Search,
  Send,
  Terminal,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { ConsoleLine } from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';

const AUTO_SCROLL_THRESHOLD_PX = 24;

const QUICK_COMMANDS = ['list', 'tps', 'status', 'help', 'save-all'];

export function ConsoleViewer({ serverId, running }: { serverId: string; running?: boolean }) {
  const [lines, setLines] = useState<ConsoleLine[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [command, setCommand] = useState('');
  const [sending, setSending] = useState(false);
  const [filter, setFilter] = useState('');
  const [autoScrollLocked, setAutoScrollLocked] = useState(false);
  const [history, setHistory] = useState<string[]>([]);
  const [historyIdx, setHistoryIdx] = useState<number>(-1);

  const scrollRef = useRef<HTMLDivElement>(null);
  const shouldAutoScrollRef = useRef(true);

  useEffect(() => {
    const seen = new Set<string>();
    const close = api.openConsoleStream(serverId, (line) => {
      const key = `${line.timestamp ?? ''}|${line.message}`;
      if (seen.size > 5000) seen.clear();
      if (seen.has(key)) return;
      seen.add(key);
      setLines((prev) => {
        const next = [...prev, line];
        return next.slice(-1000);
      });
      setError(null);
    });
    return close;
  }, [serverId]);

  useLayoutEffect(() => {
    const consoleElement = scrollRef.current;
    if (consoleElement && shouldAutoScrollRef.current && !autoScrollLocked) {
      consoleElement.scrollTop = consoleElement.scrollHeight;
    }
  }, [lines, autoScrollLocked]);

  function handleConsoleScroll() {
    const consoleElement = scrollRef.current;
    if (!consoleElement) return;

    const distanceFromBottom =
      consoleElement.scrollHeight - consoleElement.clientHeight - consoleElement.scrollTop;
    shouldAutoScrollRef.current = distanceFromBottom <= AUTO_SCROLL_THRESHOLD_PX;
  }

  const filteredLines = useMemo(() => {
    if (!filter.trim()) return lines;
    const query = filter.toLowerCase();
    return lines.filter(
      (l) =>
        (l.message && l.message.toLowerCase().includes(query)) ||
        (l.timestamp && l.timestamp.toLowerCase().includes(query)),
    );
  }, [lines, filter]);

  async function sendCommand(cmdToSend?: string, e?: FormEvent) {
    e?.preventDefault();
    const text = (cmdToSend ?? command).trim();
    if (!text || sending) return;
    setSending(true);
    try {
      await api.consoleCommand(serverId, text);
      setHistory((prev) => [text, ...prev.filter((c) => c !== text)].slice(0, 50));
      setHistoryIdx(-1);
      setCommand('');
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Could not send the command');
    } finally {
      setSending(false);
    }
  }

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'ArrowUp') {
      if (history.length === 0) return;
      e.preventDefault();
      const nextIdx = Math.min(historyIdx + 1, history.length - 1);
      setHistoryIdx(nextIdx);
      setCommand(history[nextIdx] ?? '');
    } else if (e.key === 'ArrowDown') {
      if (history.length === 0) return;
      e.preventDefault();
      const nextIdx = historyIdx - 1;
      if (nextIdx < 0) {
        setHistoryIdx(-1);
        setCommand('');
      } else {
        setHistoryIdx(nextIdx);
        setCommand(history[nextIdx] ?? '');
      }
    }
  }

  return (
    <Card className="flex flex-col border-border/70 shadow-sm">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3 border-b bg-muted/20 px-4 py-3 sm:px-5">
        <div className="flex items-center gap-2">
          <Terminal className="h-4 w-4 text-emerald-500" />
          <CardTitle className="text-base font-semibold">Live Server Console</CardTitle>
          <Badge variant="outline" className="text-xs font-normal">
            {lines.length} lines
          </Badge>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* Filter logs input */}
          <div className="relative w-44 sm:w-56">
            <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
            <input
              type="text"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Search logs..."
              className="h-8 w-full rounded-md border border-input bg-background pl-8 pr-7 text-xs shadow-xs placeholder:text-muted-foreground focus:outline-hidden focus:ring-1 focus:ring-ring"
            />
            {filter && (
              <button
                type="button"
                onClick={() => setFilter('')}
                className="absolute right-2 top-2 text-muted-foreground hover:text-foreground"
                aria-label="Clear filter"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>

          {/* Auto-scroll toggle */}
          <Button
            variant={autoScrollLocked ? 'default' : 'outline'}
            size="sm"
            className="h-8 gap-1 text-xs"
            onClick={() => setAutoScrollLocked((prev) => !prev)}
            title={autoScrollLocked ? 'Auto-scroll is paused' : 'Auto-scroll is enabled'}
          >
            <ArrowDownToLine className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">{autoScrollLocked ? 'Scroll Paused' : 'Auto-scroll'}</span>
          </Button>

          {/* Clear display */}
          <Button
            variant="outline"
            size="sm"
            className="h-8 gap-1 text-xs"
            onClick={() => setLines([])}
            title="Clear buffer in browser"
          >
            <Eraser className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">Clear</span>
          </Button>
        </div>
      </CardHeader>

      <CardContent className="p-4 sm:p-5">
        <div
          ref={scrollRef}
          onScroll={handleConsoleScroll}
          className="h-96 min-h-[360px] overflow-y-auto rounded-lg border border-neutral-800 bg-neutral-950 p-3.5 font-mono text-xs leading-5 text-emerald-100 shadow-inner sm:h-[460px]"
        >
          {filteredLines.length === 0 && !error ? (
            <div className="flex h-full items-center justify-center text-neutral-500">
              {filter ? `No log lines matching "${filter}"` : 'Waiting for server log output...'}
            </div>
          ) : (
            filteredLines.map((line, i) => (
              <div key={`${line.timestamp}-${i}`} className="whitespace-pre-wrap hover:bg-neutral-900/60">
                {line.timestamp && <span className="mr-2 select-none text-emerald-500/60">{line.timestamp}</span>}
                <span className={line.level === 'WARN' ? 'text-amber-300' : line.level === 'ERROR' ? 'text-rose-400' : 'text-neutral-200'}>
                  {line.message}
                </span>
              </div>
            ))
          )}
          {error && <p className="mt-2 text-rose-400 font-semibold">{error}</p>}
        </div>

        {/* Quick command buttons */}
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-xs text-muted-foreground mr-1">Quick:</span>
            {QUICK_COMMANDS.map((qc) => (
              <Button
                key={qc}
                type="button"
                variant="outline"
                size="sm"
                className="h-7 px-2 font-mono text-xs text-muted-foreground hover:text-foreground"
                disabled={!running || sending}
                onClick={() => void sendCommand(qc)}
              >
                /{qc}
              </Button>
            ))}
          </div>
          {filter && (
            <span className="text-xs text-muted-foreground">
              Showing {filteredLines.length} of {lines.length} lines
            </span>
          )}
        </div>

        {/* Command form */}
        <form onSubmit={(e) => void sendCommand(undefined, e)} className="mt-2.5 flex gap-2">
          <Input
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={
              running
                ? 'Enter command (e.g. say Hello, gamemode survival @a) — Press ↑/↓ for history'
                : 'Start the server to send commands'
            }
            disabled={!running || sending}
            aria-label="Console command"
            className="font-mono text-xs sm:text-sm"
          />
          <Button
            type="submit"
            size="default"
            className="shrink-0 gap-1.5"
            disabled={!running || sending || command.trim() === ''}
            aria-label="Send command"
          >
            <Send className="h-4 w-4" />
            <span className="hidden sm:inline">Send</span>
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

