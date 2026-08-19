import { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { ConsoleLine } from '../api/types';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';

export function ConsoleViewer({ serverId }: { serverId: string }) {
  const [lines, setLines] = useState<ConsoleLine[]>([]);
  const [error, setError] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const seen = new Set<string>();
    const close = api.openConsoleStream(serverId, (line) => {
      const key = `${line.timestamp ?? ''}|${line.message}`;
      if (seen.size > 5000) seen.clear();
      if (seen.has(key)) return;
      seen.add(key);
      setLines((prev) => {
        const next = [...prev, line];
        return next.slice(-500);
      });
      setError(null);
    });
    return close;
  }, [serverId]);

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
