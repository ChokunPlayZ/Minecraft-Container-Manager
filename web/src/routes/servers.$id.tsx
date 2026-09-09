import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useCallback, useEffect, useState } from 'react';
import {
  Activity,
  Archive,
  ArrowLeft,
  Check,
  ChevronDown,
  Copy,
  Cpu,
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
import { AppShell } from '../components/app-shell';
import { BackupsPanel } from '../components/backups-panel';
import { ConsoleViewer } from '../components/console-viewer';
import { FileManager } from '../components/file-manager';
import { InstallPanel } from '../components/install-panel';
import { ModsPanel } from '../components/mods-panel';
import { PlayersPanel } from '../components/players-panel';
import { PropertiesEditor } from '../components/properties-editor';
import { RequireAuth } from '../components/require-auth';
import { ServerSettings } from '../components/server-settings';
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
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<boolean>(false);
  const [copiedAddress, setCopiedAddress] = useState(false);
  const { confirm, dialog } = useModal();

  const [dnsAddress, setDnsAddress] = useState<string>('');
  const [copiedDns, setCopiedDns] = useState(false);

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
    const timer = setInterval(() => void pollStatus(), 3000);
    return () => clearInterval(timer);
  }, [server, pollStatus]);

  const run = useCallback(
    async (fn: () => Promise<Server>) => {
      if (!server) return;
      setBusy(true);
      setError(null);
      try {
        const updated = await fn();
        setServer(updated);
        setStatusState(updated.state);
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
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to kill server');
    } finally {
      setBusy(false);
    }
  }

  function copyDnsAddress() {
    if (!dnsAddress) return;
    void navigator.clipboard.writeText(dnsAddress);
    setCopiedDns(true);
    setTimeout(() => setCopiedDns(false), 2000);
  }

  function copyAddress() {
    if (!server) return;
    const host = typeof window !== 'undefined' && window.location.hostname ? window.location.hostname : 'localhost';
    const address = `${host}:${server.host_port}`;
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

  const state = statusState ?? server.state;
  const isRunning = state === 'running';
  const effectiveTab = tab === 'overview' ? 'console' : tab;

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
                    title="Click to copy host & port"
                    className="inline-flex items-center gap-1.5 rounded-md border bg-muted/30 px-2 py-0.5 text-xs hover:bg-muted hover:text-foreground transition-colors"
                  >
                    <span className="font-mono">Port {server.host_port}</span>
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
                </div>
              </div>

              {/* Right Power Actions */}
              <div className="flex flex-wrap items-center gap-2 lg:justify-end">
                {!isRunning ? (
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
                      disabled={busy}
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
                  <DropdownMenuContent align="end" className="w-48">
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

          {error && (
            <Card className="mb-5 border-destructive/50 bg-destructive/10">
              <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
            </Card>
          )}

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
                  key={`${server.name}-${server.ram_mb}-${server.host_port}`}
                  server={server}
                  onSaved={(s) => setServer(s)}
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
        </AppShell>
      </RequireAuth>
    </>
  );
}

