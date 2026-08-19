import { useCallback, useEffect, useRef, useState } from 'react';
import { api, ApiError } from '../api/client';
import type { ConsoleLine } from '../api/types';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';

export function ConsoleViewer({ serverId }: { serverId: string }) {
  const [lines, setLines] = useState<ConsoleLine[]>([]);
  const [error, setError] = useState<string | null>(null);
  const lastTimestamp = useRef<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const poll = useCallback(async () => {
    try {
      const since = lastTimestamp.current ?? undefined;
      const newLines = await api.consoleTail(serverId, since ? { since } : undefined);
      if (newLines.length === 0) return;
      setLines((prev) => {
        const key = (l: ConsoleLine) => `${l.timestamp ?? ''}|${l.message}`;
        const seen = new Set(prev.map(key));
        const merged = [...prev];
        for (const line of newLines) {
          const k = key(line);
          if (!seen.has(k)) {
            merged.push(line);
            seen.add(k);
          }
        }
        return merged.slice(-500);
      });
      lastTimestamp.current = newLines[newLines.length - 1].timestamp;
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Console unavailable');
    }
  }, [serverId]);

  useEffect(() => {
    void poll();
    const id = setInterval(() => void poll(), 2000);
    return () => clearInterval(id);
  }, [poll]);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [lines]);

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Console</CardTitle>
      </CardHeader>
      <CardContent>
        <div
          ref={scrollRef}
          className="h-72 overflow-y-auto rounded-md border bg-black/90 p-3 font-mono text-xs leading-5 text-emerald-100"
        >
          {lines.length === 0 && !error ? (
            <p className="text-muted-foreground/70">Waiting for log output...</p>
          ) : (
            lines.map((line, i) => (
              <div key={`${line.timestamp}-${i}`} className="whitespace-pre-wrap">
                {line.timestamp && <span className="mr-2 text-emerald-400/60">{line.timestamp}</span>}
                {line.message}
              </div>
            ))
          )}
          {error && <p className="text-red-400">{error}</p>}
        </div>
      </CardContent>
    </Card>
  );
}
