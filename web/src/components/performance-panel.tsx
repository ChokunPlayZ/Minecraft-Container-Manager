import { useEffect, useRef, useState, type KeyboardEvent } from 'react';
import { ExternalLink, Loader2, RefreshCw, Zap } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { PerformanceHistory, PerformanceRange, PerformanceSample, SparkReport } from '../api/types';
import { formatBytes } from '../lib/formatters';
import { Button } from './ui/button';
import { Select } from './ui/select';
import { Skeleton } from './ui/skeleton';

type Metric = 'cpu_percent' | 'memory_bytes' | 'players' | 'tps' | 'tick_ms';
type ChartProps = {
  history: PerformanceHistory;
  metric: Metric;
  title: string;
  description: string;
  format: (value: number) => string;
  minimumScale: number;
  selected: number | null;
  onSelect: (index: number | null) => void;
  reference?: number;
  className?: string;
};

function timeLabel(value: string | number, includeDate = false) {
  return new Date(value).toLocaleString(undefined, { ...(includeDate ? { month: 'short', day: 'numeric' } as const : {}), hour: '2-digit', minute: '2-digit' });
}

function validValue(sample: PerformanceSample | undefined, metric: Metric): number | null {
  const value = sample?.[metric];
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null;
}

function MetricChart({ history, metric, title, description, format, minimumScale, selected, onSelect, reference, className = '' }: ChartProps) {
  const { samples } = history;
  const start = new Date(history.start).getTime();
  const end = new Date(history.end).getTime();
  const values = samples.map((sample) => validValue(sample, metric));
  const maximum = Math.max(minimumScale, ...values.filter((value): value is number => value !== null)) * 1.08;
  const x = (sample: PerformanceSample) => Math.max(0, Math.min(600, (new Date(sample.timestamp).getTime() - start) / Math.max(1, end - start) * 600));
  const y = (value: number) => 155 - value / maximum * 150;
  let path = '';
  let connected = false;
  samples.forEach((sample, index) => {
    const value = values[index];
    if (value === null) { connected = false; return; }
    const gap = index > 0 && new Date(sample.timestamp).getTime() - new Date(samples[index - 1].timestamp).getTime() > history.resolution_seconds * 2500;
    path += `${connected && !gap ? 'L' : 'M'}${x(sample).toFixed(2)},${y(value).toFixed(2)} `;
    connected = true;
  });
  const current = selected === null ? samples.length - 1 : selected;
  const value = values[current] ?? null;
  const hasData = values.some((value) => value !== null);
  function inspect(event: KeyboardEvent<SVGSVGElement>) {
    const next = event.key === 'ArrowLeft' ? Math.max(0, current - 1)
      : event.key === 'ArrowRight' ? Math.min(samples.length - 1, current + 1)
      : event.key === 'Home' ? 0 : event.key === 'End' ? samples.length - 1 : null;
    if (event.key === 'Escape') { onSelect(null); return; }
    if (next === null || samples.length === 0) return;
    event.preventDefault(); onSelect(next);
  }
  return (
    <figure className={`min-w-0 rounded-xl border bg-card p-5 ${className}`}>
      <figcaption className="mb-4 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <span className="text-sm font-semibold">{title}</span>
        <span className="text-lg font-semibold tabular-nums">{value === null ? '—' : format(value)}</span>
      </figcaption>
      <div className="relative flex h-40 gap-2">
        <div aria-hidden="true" className="flex w-12 shrink-0 flex-col justify-between pb-1 text-right text-xs tabular-nums text-muted-foreground">
          <span>{format(maximum)}</span><span>{format(maximum / 2)}</span><span>{format(0)}</span>
        </div>
        <svg viewBox="0 0 600 160" preserveAspectRatio="none" className="h-40 min-w-0 flex-1 overflow-visible rounded-sm text-primary outline-none focus-visible:ring-2 focus-visible:ring-ring"
          role="img" aria-label={`${title} history. ${hasData ? 'Use left and right arrow keys to inspect readings.' : 'No readings available.'}${selected !== null && samples[current] ? ` ${timeLabel(samples[current].timestamp, true)}: ${value === null ? 'Unavailable' : format(value)}.` : ''}`}
          tabIndex={hasData ? 0 : undefined} onKeyDown={inspect} onBlur={() => onSelect(null)} onPointerLeave={() => onSelect(null)}
          onPointerMove={(event) => {
            if (samples.length === 0) return;
            const box = event.currentTarget.getBoundingClientRect();
            const target = start + (event.clientX - box.left) / Math.max(1, box.width) * (end - start);
            let nearest = 0;
            samples.forEach((sample, index) => { if (Math.abs(new Date(sample.timestamp).getTime() - target) < Math.abs(new Date(samples[nearest].timestamp).getTime() - target)) nearest = index; });
            onSelect(nearest);
          }}>
          {[5, 80, 155].map((position) => <line key={position} x1="0" x2="600" y1={position} y2={position} className="stroke-border" vectorEffect="non-scaling-stroke" />)}
          {reference !== undefined && <line x1="0" x2="600" y1={y(reference)} y2={y(reference)} className="stroke-muted-foreground" strokeDasharray="4 5" vectorEffect="non-scaling-stroke" />}
          <path d={path} fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
          {samples.length === 1 && values[0] !== null && <circle cx={x(samples[0])} cy={y(values[0])} r="3" fill="currentColor" />}
          {selected !== null && samples[current] && <>
            <line x1={x(samples[current])} x2={x(samples[current])} y1="0" y2="160" className="stroke-muted-foreground" strokeDasharray="3 4" vectorEffect="non-scaling-stroke" />
            {value !== null && <circle cx={x(samples[current])} cy={y(value)} r="3" fill="currentColor" />}
          </>}
        </svg>
        {!hasData && <p className="pointer-events-none absolute inset-0 flex items-center justify-center pl-14 text-sm text-muted-foreground">No readings available</p>}
      </div>
      <div aria-hidden="true" className="mt-2 flex justify-between gap-2 pl-14 text-xs tabular-nums text-muted-foreground"><span>{timeLabel(start, end - start > 86400000)}</span><span>{timeLabel(end, end - start > 86400000)}</span></div>
      <p className="mt-3 text-xs leading-5 text-muted-foreground">{description}</p>
    </figure>
  );
}

const errorMessage = (err: unknown, fallback: string) => err instanceof ApiError ? err.detail : fallback;

export function PerformancePanel({ serverId, isRunning }: { serverId: string; isRunning: boolean }) {
  const [range, setRange] = useState<PerformanceRange>('1h');
  const [history, setHistory] = useState<PerformanceHistory | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [captureError, setCaptureError] = useState<string | null>(null);
  const [report, setReport] = useState<SparkReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [reload, setReload] = useState(0);
  const [selected, setSelected] = useState<number | null>(null);
  const reportRevision = useRef(0);
  const capturing = report?.status === 'capturing';

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    // Sequential polling avoids stale responses when a server responds slowly.
    async function load() {
      const revision = reportRevision.current;
      try {
        const result = await api.performanceHistory(serverId, range);
        if (!active) return;
        setHistory(result); setLoadError(null);
        if (revision === reportRevision.current) setReport(result.report);
      } catch (err) {
        if (active) setLoadError(errorMessage(err, 'Could not load performance history. Try again.'));
      } finally {
        if (active) { setLoading(false); timer = setTimeout(() => void load(), capturing ? 3000 : 30000); }
      }
    }
    void load();
    return () => { active = false; clearTimeout(timer); };
  }, [serverId, range, reload, capturing]);

  function changeRange(value: string) {
    setRange(value as PerformanceRange); setSelected(null); setLoading(true); setHistory(null);
  }

  async function capture() {
    setBusy(true); setCaptureError(null); reportRevision.current++;
    try {
      const result = await api.captureSparkReport(serverId);
      reportRevision.current++;
      setReport(result); setReload((value) => value + 1);
    } catch (err) { setCaptureError(errorMessage(err, 'Could not start a spark report. Try again.')); }
    finally { setBusy(false); }
  }

  const support = history?.spark;
  const shared = history ? { history, selected, onSelect: setSelected } : null;
  const selectedTime = history && selected !== null ? history.samples[selected]?.timestamp : undefined;
  const freshReportURL = report?.url?.match(/^https:\/\/spark\.lucko\.me\/[A-Za-z0-9]+(?:\?[A-Za-z0-9_=&%-]+)?$/) ? report.url : undefined;

  return (
    <section className="space-y-5" aria-labelledby="performance-title">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div><h2 id="performance-title" className="text-lg font-semibold tracking-tight">Performance history</h2><p className="mt-1 text-sm text-muted-foreground">Sampled every 30 seconds. History is kept for 7 days.</p></div>
        <div className="flex items-center gap-2">
          <Select aria-label="History time range" value={range} onValueChange={changeRange} className="w-36">
            <option value="1h">Last hour</option><option value="6h">Last 6 hours</option><option value="24h">Last 24 hours</option><option value="7d">Last 7 days</option>
          </Select>
          <Button variant="outline" size="icon" aria-label="Refresh performance history" disabled={loading} onClick={() => setReload((value) => value + 1)}><RefreshCw className="h-4 w-4" /></Button>
        </div>
      </div>

      {loadError && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive"><span>{loadError} {history ? 'Showing the last available readings.' : ''}</span><Button variant="outline" size="sm" onClick={() => setReload((value) => value + 1)}>Retry</Button></div>}
      {!isRunning && <p className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">Server is offline. Previous history is still available; new tick readings resume when the server starts.</p>}

      {loading ? <div aria-label="Loading performance history" className="grid gap-4 md:grid-cols-2">{Array.from({ length: 5 }, (_, index) => <Skeleton key={index} className="h-64 rounded-xl" />)}</div> : shared && <>
        {shared.history.samples.length === 0 && <div className="rounded-lg border border-dashed p-6 text-center"><h3 className="text-sm font-semibold">No performance history yet</h3><p className="mt-1 text-sm text-muted-foreground">The first reading appears within 30 seconds. Try a wider time range to see older history.</p></div>}
        <div className="flex flex-wrap justify-between gap-2 text-xs text-muted-foreground">
          <span>{selectedTime ? `Inspecting ${timeLabel(selectedTime, true)}` : 'Point at a chart or use its arrow keys to inspect readings.'}</span>
          <span>{range === '1h' ? '30-second readings' : `${shared.history.resolution_seconds / 60}-minute averages · peak player count`}</span>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <MetricChart {...shared} metric="cpu_percent" title="CPU usage" format={(value) => `${value.toFixed(1)}%`} minimumScale={100} description="Container CPU. 100% equals one fully used core." />
          <MetricChart {...shared} metric="memory_bytes" title="Memory" format={formatBytes} minimumScale={1024 ** 2} description="Total memory used by the container." />
          <MetricChart {...shared} metric="players" title="Player count" format={(value) => String(Math.round(value))} minimumScale={1} description="Connected players; longer ranges show the peak in each interval." className="md:col-span-2" />
          <MetricChart {...shared} metric="tps" title="TPS" format={(value) => value.toFixed(1)} minimumScale={20} reference={20} description="Spark's 5-second TPS. The dashed line marks 20 TPS." />
          <MetricChart {...shared} metric="tick_ms" title="Tick time" format={(value) => `${value.toFixed(1)} ms`} minimumScale={50} reference={50} description="Median tick duration over 10 seconds. Dashed line: 50 ms, the budget for 20 TPS." />
        </div>
        <p className="text-xs leading-5 text-muted-foreground">Gaps mean readings were unavailable. TPS and tick time require a running server with spark.</p>
      </>}

      <div className="rounded-xl border bg-card p-5 sm:p-6" aria-labelledby="spark-title">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="max-w-prose"><h3 id="spark-title" className="font-semibold">Spark report</h3><p className="mt-1 text-sm leading-6 text-muted-foreground">Capture a 60-second profile and upload it to spark. If a profiler is already running, open its current report.</p></div>
          <Button variant="outline" disabled={!isRunning || !support?.available || busy || capturing} onClick={() => void capture()} className="gap-2">
            {busy || capturing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Zap className="h-4 w-4" />}{busy || capturing ? 'Capturing report…' : 'Capture spark report'}
          </Button>
        </div>
        <p className="mt-3 text-xs text-muted-foreground">{!support ? 'Checking spark availability…' : support.source === 'bundled' ? 'Spark is bundled with this Paper server.' : support.available ? `Spark detected in the ${support.source === 'mod' ? 'mod' : 'plugin'} list. Restart after installation to load it.` : 'Spark was not detected. Install and enable the spark plugin or mod, then restart. Paper 1.21.1 and newer include spark.'}{!isRunning ? ' Start the server to capture a report.' : ''}</p>
        {captureError && <p role="alert" className="mt-3 text-sm text-destructive">{captureError}</p>}
        <div aria-live="polite" className="mt-3 text-sm">
          {capturing && <p className="text-muted-foreground">Profiling and uploading can take up to 90 seconds. You can leave this tab while the capture runs.</p>}
          {report?.status === 'failed' && <p role="alert" className="text-destructive">{report.error}</p>}
          {report?.status === 'completed' && freshReportURL && <div className="flex flex-wrap items-center gap-x-4 gap-y-2"><a href={freshReportURL} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1.5 font-medium text-primary underline underline-offset-4">{report.existing ? 'Open active spark report' : 'Open spark report'}<ExternalLink className="h-3.5 w-3.5" /></a><span className="text-xs text-muted-foreground">{timeLabel(report.started_at, true)}</span></div>}
        </div>
      </div>
    </section>
  );
}
