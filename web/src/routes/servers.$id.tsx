import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useCallback, useEffect, useState } from 'react';
import {
  Activity,
  AlertTriangle,
  Archive,
  ArrowLeft,
  Check,
  ChevronDown,
  Clock,
  Copy,
  Cpu,
  Download,
  FolderOpen,
  Globe,
  HardDrive,
  Package,
  Play,
  RefreshCw,
  RotateCcw,
  Settings2,
  Square,
  Terminal,
  Trash2,
  Users,
  Zap,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server, ServerState } from '../api/types';
import { useLiveUptime } from '../lib/uptime';
import { AppShell } from '../components/app-shell';
import { RequireAuth } from '../components/require-auth';
import { BackupsPanel } from '../components/backups-panel';
import { ConsoleViewer } from '../components/console-viewer';
import { CopyServerDialog } from '../components/copy-server-dialog';
import { FileManager } from '../components/file-manager';
import { InstallPanel } from '../components/install-panel';
import { ModsPanel } from '../components/mods-panel';
import { PlayersPanel } from '../components/players-panel';
import { PropertiesEditor } from '../components/properties-editor';
import { ServerInstallProgress } from '../components/server-install-progress';
import { ServerSettings } from '../components/server-settings';
import { ServerStatsGrid } from '../components/server-stats-grid';
import { StatusBadge } from '../components/status-badge';
import { Button } from '../components/ui/button';
import { Card, CardContent } from '../components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../components/ui/dropdown-menu';
import { useModal } from '../components/ui/modal';

export const Route = createFileRoute('/servers/$id')({
  component: ServerDetailRoute,
});

type ServerTab = 'console' | 'players' | 'files' | 'mods' | 'backups' | 'settings' | 'overview';

export function ServerDetailRoute() {
  const { id } = Route.useParams();
  const navigate = useNavigate();
  const [tab, setTab] = useState<ServerTab>('console');
  const [server, setServer] = useState<Server | null>(null);
  const [statusState, setStatusState] = useState<ServerState | null>(null);
  const [statusStartedAt, setStatusStartedAt] = useState<string | null | undefined>(undefined);
  const [statusUptimeSeconds, setStatusUptimeSeconds] = useState<number | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<boolean>(false);
  const [copiedAddress, setCopiedAddress] = useState(false);
  const [copyDialogOpen, setCopyDialogOpen] = useState(false);
  const { confirm, dialog } = useModal();

  const [dnsAddress, setDnsAddress] = useState<string>('');
  const [copiedDns, setCopiedDns] = useState(false);

  const state = statusState ?? server?.state ?? 'stopped';
  const isRunning = state === 'running';
  const isInstalling = state === 'installing' || state === 'building';
  const isActive = isRunning || isInstalling;
  const effectiveStartedAt = statusStartedAt !== undefined ? statusStartedAt : server?.started_at;
  const effectiveUptimeSec = statusUptimeSeconds !== undefined ? statusUptimeSeconds : server?.uptime_seconds;
  const liveUptime = useLiveUptime(effectiveStartedAt, effectiveUptimeSec, isRunning);
  const effectiveTab = tab === 'overview' ? 'console' : tab;

  const loadDns = useCallback(async (sid: string) => {
    try {
      const res = await api.getServerDNS(sid);
      if (res.join_address && res.record) {
        setDnsAddress(res.join_address);
      } else {
        setDnsAddress('');
      }
    } catch {
      setDnsAddress('');
    }
  }, []);

  const loadServer = useCallback(async () => {
    setError(null);
    try {
      const s = await api.getServer(id);
      setServer(s);
      setStatusState(s.state);
      setStatusStartedAt(s.started_at);
      setStatusUptimeSeconds(s.uptime_seconds);
      void loadDns(id);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load server');
    }
  }, [id, loadDns]);

  const pollStatus = useCallback(async () => {
    if (!server) return;
    try {
      const status = await api.serverStatus(id);
      setStatusState(status.state);
      if (status.started_at !== undefined) {
        setStatusStartedAt(status.started_at);
      }
      if (status.uptime_seconds !== undefined) {
        setStatusUptimeSeconds(status.uptime_seconds);
      }
    } catch {
      /* keep last known */
    }
  }, [id, server]);

  useEffect(() => {
    void loadServer();
  }, [loadServer]);

  useEffect(() => {
    if (!server) return;
    void pollStatus();
    const interval = isInstalling ? 2000 : 3000;
    const timer = setInterval(() => void pollStatus(), interval);
    return () => clearInterval(timer);
  }, [server, pollStatus, isInstalling]);

  const run = useCallback(
    async (fn: () => Promise<Server>) => {
      if (!server) return;
      setBusy(true);
      setError(null);
      try {
        const updated = await fn();
        setServer(updated);
        setStatusState(updated.state);
        setStatusStartedAt(updated.started_at);
        setStatusUptimeSeconds(updated.uptime_seconds);
      } catch (err) {
        setError(err instanceof ApiError ? err.detail : 'Action failed');
      } finally {
        setBusy(false);
      }
    },
    [server],
  );

  async function handleDelete() {
    if (!server) return;
    const confirmed = await confirm(`Delete server "${server.name}"? This action cannot be undone.`, {
      title: 'Delete server',
      confirmLabel: 'Delete',
      destructive: true,
    });
    if (!confirmed) return;
    try {
      await api.deleteServer(server.id);
      void navigate({ to: '/dashboard', replace: true });
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to delete server');
    }
  }

  async function handleRecreate() {
    if (!server) return;
    const confirmed = await confirm(
      `Rebuild the container for "${server.name}"? This stops the server now and provisions a fresh container on the next start.`,
      { title: 'Rebuild container', confirmLabel: 'Rebuild', destructive: true },
    );
    if (!confirmed) return;
    try {
      setBusy(true);
      setError(null);
      const updated = await api.recreateServer(server.id);
      setServer(updated);
      setStatusState('stopped');
      setStatusStartedAt(null);
      setStatusUptimeSeconds(0);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to rebuild container');
    } finally {
      setBusy(false);
    }
  }

  async function handleKill() {
    if (!server) return;
    const confirmed = await confirm(
      `Force-kill "${server.name}"? This terminates the container immediately without saving worlds. Use only if unresponsive.`,
      { title: 'Force-kill server', confirmLabel: 'Force-kill', destructive: true },
    );
    if (!confirmed) return;
    try {
      setBusy(true);
      setError(null);
      const updated = await api.killServer(server.id);
      setServer(updated);
      setStatusState(updated.state);
      setStatusStartedAt(null);
      setStatusUptimeSeconds(0);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to kill server');
    } finally {
      setBusy(false);
    }
  }

  function handleExport() {
    if (!server) return;
    api.exportServer(server.id, server.name);
  }

  function copyDnsAddress() {
    if (!dnsAddress) return;
    void navigator.clipboard.writeText(dnsAddress);
    setCopiedDns(true);
    setTimeout(() => setCopiedDns(false), 2000);
  }

  function copyAddress() {
    if (!server) return;
    let address = '';
    if (server.host_port > 0) {
      const host = typeof window !== 'undefined' && window.location.hostname ? window.location.hostname : 'localhost';
      address = `${host}:${server.host_port}`;
    } else {
      const cPort =
        server.server_type === 'geysermc'
          ? 19132
          : server.server_type === 'velocity' ||
              server.server_type === 'waterfall' ||
              server.server_type === 'bungeecord'
            ? 25577
            : 25565;
      address = `mcm-${server.id}:${cPort}`;
    }
    void navigator.clipboard.writeText(address);
    setCopiedAddress(true);
    setTimeout(() => setCopiedAddress(false), 2000);
  }

  if (!server) {
    return (
      <RequireAuth>
        <AppShell>
          <div className="flex h-64 items-center justify-center">
            <p className="text-muted-foreground">{error ?? 'Loading server details...'}</p>
          </div>
        </AppShell>
      </RequireAuth>
    );
  }

  const TABS: { id: ServerTab; label: string; icon: typeof Terminal }[] = [
    { id: 'console', label: 'Console', icon: Terminal },
    { id: 'players', label: 'Players', icon: Users },
    { id: 'files', label: 'Files', icon: FolderOpen },
    { id: 'mods', label: 'Mods & Plugins', icon: Package },
    { id: 'backups', label: 'Backups', icon: Archive },
    { id: 'settings', label: 'Settings', icon: Settings2 },
  ];

  return (
    <>
      {dialog}
      <RequireAuth>
        <AppShell>
          {/* Breadcrumbs */}
          <div className="mb-4 flex items-center gap-2 text-sm text-muted-foreground">
            <Link
              to="/dashboard"
              className="inline-flex items-center gap-1 transition-colors hover:text-foreground"
            >
              <ArrowLeft className="h-4 w-4" /> Servers
            </Link>
            <span>/</span>
            <span className="truncate font-medium text-foreground">{server.name}</span>
          </div>

          {/* Hero Header Card */}
          <div className="mb-6 rounded-xl border bg-card p-5 shadow-xs sm:p-6">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              {/* Left specs & title */}
              <div className="min-w-0 space-y-2">
                <div className="flex flex-wrap items-center gap-3">
                  <h1 className="truncate text-2xl font-bold tracking-tight text-foreground sm:text-3xl">
                    {server.name}
                  </h1>
                  <StatusBadge state={state} />
                  {server.needs_rebuild && (
                    <span
                      data-testid="header-rebuild-badge"
                      className="inline-flex items-center gap-1 rounded-full border border-amber-500/40 bg-amber-500/10 px-2.5 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-400"
                    >
                      <RotateCcw className="h-3 w-3" />
                      Rebuild required
                    </span>
                  )}
                </div>

                <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-muted-foreground sm:text-sm">
                  {/* Server type & version */}
                  <span className="inline-flex items-center gap-1 rounded-md bg-muted/60 px-2 py-0.5 font-medium text-foreground">
                    <Activity className="h-3.5 w-3.5 text-primary" />
                    <span className="capitalize">{server.server_type}</span> {server.version}
                    {server.build ? ` #${server.build}` : ''}
                  </span>

                  {/* Click to copy SRV join address if active */}
                  {dnsAddress && (
                    <button
                      type="button"
                      onClick={copyDnsAddress}
                      title="Minecraft SRV Join Address — click to copy"
                      className="inline-flex items-center gap-1.5 rounded-md border border-primary/40 bg-primary/10 px-2.5 py-0.5 text-xs text-primary hover:bg-primary/20 hover:text-primary transition-colors font-medium shadow-2xs"
                    >
                      <Globe className="h-3.5 w-3.5" />
                      <span className="font-mono font-semibold">{dnsAddress}</span>
                      {copiedDns ? (
                        <Check className="h-3 w-3 text-emerald-500" />
                      ) : (
                        <Copy className="h-3 w-3 opacity-70" />
                      )}
                      {copiedDns && (
                        <span className="text-[10px] text-emerald-600 font-semibold">Copied!</span>
                      )}
                    </button>
                  )}

                  {/* Click to copy host & port address */}
                  <button
                    type="button"
                    onClick={copyAddress}
                    title={server.host_port > 0 ? "Click to copy host & port" : "Click to copy internal Docker network address"}
                    className="inline-flex items-center gap-1.5 rounded-md border bg-muted/30 px-2 py-0.5 text-xs hover:bg-muted hover:text-foreground transition-colors cursor-pointer"
                  >
                    <span className="font-mono">
                      {server.host_port > 0 ? `Port ${server.host_port}` : `Behind Proxy (mcm-${server.id.slice(0, 8)})`}
                    </span>
                    {copiedAddress ? (
                      <Check className="h-3 w-3 text-emerald-500" />
                    ) : (
                      <Copy className="h-3 w-3" />
                    )}
                    {copiedAddress && (
                      <span className="text-[10px] text-emerald-600 font-semibold">Copied!</span>
                    )}
                  </button>

                  {/* RAM limit */}
                  <span className="inline-flex items-center gap-1">
                    <HardDrive className="h-3.5 w-3.5" />
                    {server.ram_mb} MB RAM
                  </span>

                  {/* CPU limit if configured */}
                  {server.cpu_limit > 0 && (
                    <span className="inline-flex items-center gap-1">
                      <Cpu className="h-3.5 w-3.5" />
                      {server.cpu_limit} CPU
                    </span>
                  )}

                  {/* Live Uptime */}
                  {isRunning && liveUptime && (
                    <span
                      data-testid="server-uptime"
                      className="inline-flex items-center gap-1.5 rounded-md border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-0.5 text-xs text-emerald-700 dark:text-emerald-400 font-medium shadow-2xs"
                      title={
                        effectiveStartedAt
                          ? `Started: ${new Date(effectiveStartedAt).toLocaleString()}`
                          : 'Server is running'
                      }
                    >
                      <Clock className="h-3.5 w-3.5 text-emerald-500 animate-pulse" />
                      <span className="font-mono font-medium">Uptime: {liveUptime}</span>
                    </span>
                  )}
                </div>
              </div>

              {/* Right Power Actions */}
              <div className="flex flex-wrap items-center gap-2 lg:justify-end">
                {!isActive ? (
                  <Button
                    variant="default"
                    size="sm"
                    className="gap-1.5 bg-emerald-600 hover:bg-emerald-500 text-white font-medium shadow-xs"
                    disabled={busy || state === 'starting'}
                    onClick={() => void run(() => api.startServer(server.id))}
                  >
                    <Play className={`h-4 w-4 ${busy ? 'animate-spin' : ''}`} />
                    <span>Start Server</span>
                  </Button>
                ) : (
                  <>
                    <Button
                      variant="outline"
                      size="sm"
                      className="gap-1.5"
                      disabled={busy}
                      onClick={() => void run(() => api.stopServer(server.id))}
                    >
                      <Square className="h-3.5 w-3.5 text-amber-500" />
                      <span>Stop</span>
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="gap-1.5"
                      disabled={busy || isInstalling}
                      onClick={() => void run(() => api.restartServer(server.id))}
                    >
                      <RefreshCw className={`h-3.5 w-3.5 ${busy ? 'animate-spin' : ''}`} />
                      <span>Restart</span>
                    </Button>
                  </>
                )}

                {/* More actions dropdown */}
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="outline" size="sm" className="gap-1 px-2.5">
                      <span>Actions</span>
                      <ChevronDown className="h-3.5 w-3.5" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-52">
                    <DropdownMenuItem onSelect={handleExport}>
                      <Download className="h-4 w-4 mr-2" />
                      <span>Export server (.zip)</span>
                    </DropdownMenuItem>
                    <DropdownMenuItem disabled={busy} onSelect={() => setCopyDialogOpen(true)}>
                      <Copy className="h-4 w-4 mr-2" />
                      <span>Copy server</span>
                    </DropdownMenuItem>
                    <DropdownMenuItem disabled={busy} onSelect={() => void handleRecreate()}>
                      <RotateCcw className="h-4 w-4 mr-2" />
                      <span>Rebuild container</span>
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      disabled={busy || state === 'stopped' || state === 'stopping'}
                      onSelect={() => void handleKill()}
                      className="text-destructive focus:text-destructive"
                    >
                      <Zap className="h-4 w-4 mr-2" />
                      <span>Force-kill server</span>
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      disabled={busy}
                      onSelect={() => void handleDelete()}
                      className="text-destructive focus:text-destructive"
                    >
                      <Trash2 className="h-4 w-4 mr-2" />
                      <span>Delete server</span>
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
          </div>

          {/* Installation Progress Banner */}
          {isInstalling && (
            <ServerInstallProgress serverId={server.id} state={state} />
          )}

          {error && (
            <Card className="mb-5 border-destructive/50 bg-destructive/10">
              <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
            </Card>
          )}

          {server.needs_rebuild && (
            <div
              data-testid="rebuild-warning-banner"
              className="mb-5 flex flex-col gap-3 rounded-xl border border-amber-500/40 bg-amber-500/10 p-4 text-amber-950 dark:border-amber-500/30 dark:bg-amber-950/30 dark:text-amber-200 sm:flex-row sm:items-center sm:justify-between shadow-2xs"
            >
              <div className="flex items-start gap-3">
                <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400" />
                <div className="space-y-1 text-sm">
                  <p className="font-semibold text-amber-900 dark:text-amber-200">
                    Container Rebuild Required
                  </p>
                  <p className="text-xs text-amber-800/90 dark:text-amber-300/90">
                    Memory or container settings have changed since this container was provisioned. Rebuild the container to apply your changes.
                  </p>
                  {server.rebuild_reasons && server.rebuild_reasons.length > 0 && (
                    <ul className="mt-1.5 list-disc pl-4 text-xs space-y-0.5 text-amber-800/80 dark:text-amber-300/80">
                      {server.rebuild_reasons.map((r, i) => (
                        <li key={i}>{r}</li>
                      ))}
                    </ul>
                  )}
                </div>
              </div>
              <Button
                variant="outline"
                size="sm"
                disabled={busy}
                onClick={() => void handleRecreate()}
                className="shrink-0 gap-1.5 border-amber-500/40 bg-amber-500/20 text-amber-900 hover:bg-amber-500/30 hover:text-amber-950 dark:border-amber-400/40 dark:bg-amber-500/20 dark:text-amber-100 dark:hover:bg-amber-500/30 font-medium"
              >
                <RotateCcw className="h-3.5 w-3.5" />
                <span>Rebuild container</span>
              </Button>
            </div>
          )}

          {/* Server Stats Grid (CPU, MEM, Disk, Network) */}
          <ServerStatsGrid serverId={server.id} isRunning={isRunning} />

          {/* Tab Navigation */}
          <div
            className="mb-6 flex items-center gap-1 overflow-x-auto rounded-xl border bg-muted/40 p-1.5 shadow-2xs"
            role="tablist"
            aria-label="Server sections"
          >
            {TABS.map((t) => {
              const Icon = t.icon;
              const isSelected = effectiveTab === t.id;
              return (
                <button
                  key={t.id}
                  type="button"
                  onClick={() => setTab(t.id)}
                  role="tab"
                  aria-selected={isSelected}
                  className={`inline-flex shrink-0 items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all ${
                    isSelected
                      ? 'bg-background text-foreground shadow-xs'
                      : 'text-muted-foreground hover:bg-background/60 hover:text-foreground'
                  }`}
                >
                  <Icon className={`h-4 w-4 ${isSelected ? 'text-primary' : ''}`} />
                  <span>{t.label}</span>
                </button>
              );
            })}
          </div>

          {/* Tab Panes */}
          {effectiveTab === 'console' && (
            <ConsoleViewer serverId={server.id} running={isRunning} />
          )}

          {effectiveTab === 'players' && (
            <PlayersPanel server={server} />
          )}

          {effectiveTab === 'files' && (
            <FileManager server={server} />
          )}

          {effectiveTab === 'mods' && (
            <ModsPanel server={server} />
          )}

          {effectiveTab === 'backups' && (
            <BackupsPanel server={server} />
          )}

          {effectiveTab === 'settings' && (
            <div className="space-y-6">
              <div>
                <h2 className="text-lg font-semibold tracking-tight">Server Configuration</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  Manage resources, network ports, server software, and server.properties.
                </p>
              </div>

              <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1.5fr)_minmax(340px,1fr)]">
                <ServerSettings
                  key={`${server.name}-${server.ram_mb}-${server.host_port}-${server.needs_rebuild}`}
                  server={server}
                  onSaved={(s) => setServer(s)}
                  onRebuild={() => void handleRecreate()}
                  onDnsChanged={(addr) => setDnsAddress(addr)}
                />
                <InstallPanel
                  serverId={server.id}
                  serverType={server.server_type}
                  onInstalled={loadServer}
                />
              </div>

              <div className="pt-2">
                <PropertiesEditor server={server} />
              </div>
            </div>
          )}

          {server && (
            <CopyServerDialog
              server={server}
              open={copyDialogOpen}
              onClose={() => setCopyDialogOpen(false)}
              onCopied={(newServer) => {
                void navigate({ to: '/servers/$id', params: { id: newServer.id } });
              }}
            />
          )}
        </AppShell>
      </RequireAuth>
    </>
  );
}

