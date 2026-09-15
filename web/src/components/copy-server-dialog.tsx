import { useEffect, useState } from 'react';
import {
  AlertCircle,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  FileText,
  Globe,
  HardDrive,
  Loader2,
  Package,
  Puzzle,
  Settings,
  Sliders,
  Users,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { CopyServerInput, Server } from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Switch } from './ui/switch';

interface CopyServerDialogProps {
  server: Server;
  open: boolean;
  onClose: () => void;
  onCopied?: (newServer: Server) => void;
}

export function CopyServerDialog({ server, open, onClose, onCopied }: CopyServerDialogProps) {
  const [name, setName] = useState(`${server.name} (Copy)`);
  const [portMode, setPortMode] = useState<'auto' | 'custom'>('auto');
  const [customPort, setCustomPort] = useState('');
  const [ramMb, setRamMb] = useState(server.ram_mb || 2048);

  // Component toggles
  const [includeWorld, setIncludeWorld] = useState(true);
  const [includeConfig, setIncludeConfig] = useState(true);
  const [includePlugins, setIncludePlugins] = useState(true);
  const [includeMods, setIncludeMods] = useState(true);
  const [includePlayerData, setIncludePlayerData] = useState(true);
  const [includeLogs, setIncludeLogs] = useState(false);

  // Advanced options
  const [customExcludes, setCustomExcludes] = useState('');
  const [showAdvanced, setShowAdvanced] = useState(false);

  // State
  const [availablePorts, setAvailablePorts] = useState<number[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset or initialize on open
  useEffect(() => {
    if (open) {
      setName(`${server.name} (Copy)`);
      setPortMode('auto');
      setCustomPort('');
      setRamMb(server.ram_mb || 2048);
      setIncludeWorld(true);
      setIncludeConfig(true);
      setIncludePlugins(true);
      setIncludeMods(true);
      setIncludePlayerData(true);
      setIncludeLogs(false);
      setCustomExcludes('');
      setShowAdvanced(false);
      setError(null);
      setBusy(false);

      // Fetch available ports for suggestions
      api.availablePorts()
        .then((res) => {
          if (res?.available && res.available.length > 0) {
            setAvailablePorts(res.available);
          }
        })
        .catch(() => {
          // ignore port pool query errors
        });
    }
  }, [open, server]);

  if (!open) return null;

  const nextFreePort = availablePorts.length > 0 ? availablePorts[0] : undefined;

  // Preset handlers
  function applyPreset(preset: 'full' | 'clean-world' | 'template') {
    if (preset === 'full') {
      setIncludeWorld(true);
      setIncludeConfig(true);
      setIncludePlugins(true);
      setIncludeMods(true);
      setIncludePlayerData(true);
      setIncludeLogs(false);
    } else if (preset === 'clean-world') {
      setIncludeWorld(false);
      setIncludePlayerData(false);
      setIncludeConfig(true);
      setIncludePlugins(true);
      setIncludeMods(true);
      setIncludeLogs(false);
    } else if (preset === 'template') {
      setIncludeWorld(false);
      setIncludePlayerData(false);
      setIncludeConfig(true);
      setIncludePlugins(true);
      setIncludeMods(true);
      setIncludeLogs(false);
    }
  }

  async function handleCopy(e: React.FormEvent) {
    e.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName) {
      setError('Please provide a name for the new server.');
      return;
    }

    let parsedPort: number | undefined;
    if (portMode === 'custom') {
      const p = parseInt(customPort, 10);
      if (isNaN(p) || p < 1 || p > 65535) {
        setError('Please enter a valid port between 1 and 65535.');
        return;
      }
      parsedPort = p;
    }

    const excludesList = customExcludes
      .split(',')
      .map((s) => s.trim())
      .filter((s) => s.length > 0);

    const payload: CopyServerInput = {
      name: trimmedName,
      host_port: parsedPort,
      ram_mb: ramMb > 0 ? ramMb : server.ram_mb,
      include_world: includeWorld,
      include_config: includeConfig,
      include_plugins: includePlugins,
      include_mods: includeMods,
      include_player_data: includePlayerData,
      include_logs: includeLogs,
      custom_excludes: excludesList.length > 0 ? excludesList : undefined,
    };

    setBusy(true);
    setError(null);

    try {
      const newServer = await api.copyServer(server.id, payload);
      onClose();
      onCopied?.(newServer);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.detail || err.message);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('An unexpected error occurred while copying the server.');
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm animate-in fade-in-0 duration-200"
      role="presentation"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !busy) onClose();
      }}
    >
      <div
        className="w-full max-w-xl max-h-[90vh] overflow-y-auto rounded-xl border border-border bg-card text-card-foreground shadow-2xl animate-in zoom-in-95 duration-200"
        role="dialog"
        aria-modal="true"
        aria-labelledby="copy-dialog-title"
      >
        {/* Header */}
        <div className="sticky top-0 z-10 flex items-center justify-between border-b bg-card/95 px-6 py-4 backdrop-blur-md">
          <div className="flex items-center gap-2.5">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Copy className="h-5 w-5" />
            </div>
            <div>
              <h2 id="copy-dialog-title" className="text-lg font-semibold leading-tight">
                Copy Server
              </h2>
              <p className="text-xs text-muted-foreground">
                Clone <span className="font-medium text-foreground">{server.name}</span> to a new instance
              </p>
            </div>
          </div>
          <button
            type="button"
            disabled={busy}
            onClick={onClose}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors disabled:opacity-50"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <form onSubmit={handleCopy} className="p-6 space-y-6">
          {/* Source Server Info Pill */}
          <div className="flex items-center justify-between rounded-lg border bg-muted/30 px-3.5 py-2.5 text-xs text-muted-foreground">
            <div className="flex items-center gap-2">
              <HardDrive className="h-3.5 w-3.5 text-primary" />
              <span>Source: <strong className="text-foreground">{server.name}</strong></span>
            </div>
            <div className="flex items-center gap-1.5">
              <Badge variant="secondary" className="text-[11px] font-mono capitalize">
                {server.server_type} {server.version}
              </Badge>
              <Badge variant="outline" className="text-[11px] font-mono">
                Port {server.host_port}
              </Badge>
            </div>
          </div>

          {/* Running State Notice */}
          {server.state === 'running' && (
            <div className="flex items-start gap-2.5 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-300">
              <AlertCircle className="h-4 w-4 shrink-0 mt-0.5 text-amber-500" />
              <div>
                <strong>Server is currently running:</strong> MCM can copy active server files safely
                (locks are excluded), but stopping the server first ensures the cleanest world snapshot.
              </div>
            </div>
          )}

          {/* Target Name */}
          <div className="space-y-1.5">
            <Label htmlFor="copy-name" className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              New Server Name <span className="text-destructive">*</span>
            </Label>
            <Input
              id="copy-name"
              type="text"
              required
              disabled={busy}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Survival SMP (Copy)"
              className="h-9"
            />
          </div>

          {/* Port Configuration */}
          <div className="space-y-2">
            <Label className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Game Port Allocation
            </Label>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                disabled={busy}
                onClick={() => setPortMode('auto')}
                className={`flex items-center justify-between rounded-lg border p-3 text-left transition-all ${
                  portMode === 'auto'
                    ? 'border-primary bg-primary/5 text-foreground ring-1 ring-primary'
                    : 'border-border bg-card hover:bg-muted/40 text-muted-foreground'
                }`}
              >
                <div>
                  <div className="text-xs font-medium text-foreground">Auto-allocate</div>
                  <div className="text-[11px] text-muted-foreground">
                    {nextFreePort ? `Suggested: ${nextFreePort}` : 'From pool'}
                  </div>
                </div>
                {portMode === 'auto' && <Check className="h-4 w-4 text-primary shrink-0" />}
              </button>

              <button
                type="button"
                disabled={busy}
                onClick={() => setPortMode('custom')}
                className={`flex items-center justify-between rounded-lg border p-3 text-left transition-all ${
                  portMode === 'custom'
                    ? 'border-primary bg-primary/5 text-foreground ring-1 ring-primary'
                    : 'border-border bg-card hover:bg-muted/40 text-muted-foreground'
                }`}
              >
                <div>
                  <div className="text-xs font-medium text-foreground">Custom Port</div>
                  <div className="text-[11px] text-muted-foreground">Specify manual port</div>
                </div>
                {portMode === 'custom' && <Check className="h-4 w-4 text-primary shrink-0" />}
              </button>
            </div>

            {portMode === 'custom' && (
              <div className="pt-1.5">
                <Input
                  type="number"
                  min="1"
                  max="65535"
                  disabled={busy}
                  placeholder="e.g. 25566"
                  value={customPort}
                  onChange={(e) => setCustomPort(e.target.value)}
                  className="h-9"
                />
              </div>
            )}
          </div>

          {/* Component Selection */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <Label className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Include / Exclude Components
                </Label>
                <p className="text-[11px] text-muted-foreground">
                  Choose which data and directories to duplicate.
                </p>
              </div>

              {/* Quick Preset Buttons */}
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-6 px-2 text-[11px]"
                  disabled={busy}
                  onClick={() => applyPreset('full')}
                >
                  Full Clone
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-6 px-2 text-[11px]"
                  disabled={busy}
                  onClick={() => applyPreset('clean-world')}
                >
                  Reset World
                </Button>
              </div>
            </div>

            <div className="divide-y divide-border/60 rounded-lg border bg-card/60">
              {/* World */}
              <div className="flex items-center justify-between p-3 transition-colors hover:bg-muted/30">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-500">
                    <Globe className="h-4 w-4" />
                  </div>
                  <div>
                    <div className="text-xs font-medium">World Data</div>
                    <div className="text-[11px] text-muted-foreground">
                      Level files, region chunks, Overworld, Nether, and End saves
                    </div>
                  </div>
                </div>
                <Switch
                  checked={includeWorld}
                  onCheckedChange={setIncludeWorld}
                  disabled={busy}
                  aria-label="Include World Data"
                />
              </div>

              {/* Config */}
              <div className="flex items-center justify-between p-3 transition-colors hover:bg-muted/30">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-md bg-sky-500/10 text-sky-500">
                    <Settings className="h-4 w-4" />
                  </div>
                  <div>
                    <div className="text-xs font-medium">Server Configurations</div>
                    <div className="text-[11px] text-muted-foreground">
                      server.properties, config/ directory, and root .yml/.toml files
                    </div>
                  </div>
                </div>
                <Switch
                  checked={includeConfig}
                  onCheckedChange={setIncludeConfig}
                  disabled={busy}
                  aria-label="Include Server Configurations"
                />
              </div>

              {/* Plugins */}
              <div className="flex items-center justify-between p-3 transition-colors hover:bg-muted/30">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-md bg-indigo-500/10 text-indigo-500">
                    <Puzzle className="h-4 w-4" />
                  </div>
                  <div>
                    <div className="text-xs font-medium">Plugins</div>
                    <div className="text-[11px] text-muted-foreground">
                      All installed plugin JARs and data in plugins/
                    </div>
                  </div>
                </div>
                <Switch
                  checked={includePlugins}
                  onCheckedChange={setIncludePlugins}
                  disabled={busy}
                  aria-label="Include Plugins"
                />
              </div>

              {/* Mods */}
              <div className="flex items-center justify-between p-3 transition-colors hover:bg-muted/30">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-500/10 text-amber-500">
                    <Package className="h-4 w-4" />
                  </div>
                  <div>
                    <div className="text-xs font-medium">Mods</div>
                    <div className="text-[11px] text-muted-foreground">
                      All loader mods and dependencies in mods/
                    </div>
                  </div>
                </div>
                <Switch
                  checked={includeMods}
                  onCheckedChange={setIncludeMods}
                  disabled={busy}
                  aria-label="Include Mods"
                />
              </div>

              {/* Player Data */}
              <div className="flex items-center justify-between p-3 transition-colors hover:bg-muted/30">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-md bg-purple-500/10 text-purple-500">
                    <Users className="h-4 w-4" />
                  </div>
                  <div>
                    <div className="text-xs font-medium">Player Data & Lists</div>
                    <div className="text-[11px] text-muted-foreground">
                      Player inventories, stats, advancements, ops.json, whitelist
                    </div>
                  </div>
                </div>
                <Switch
                  checked={includePlayerData}
                  onCheckedChange={setIncludePlayerData}
                  disabled={busy}
                  aria-label="Include Player Data"
                />
              </div>

              {/* Logs */}
              <div className="flex items-center justify-between p-3 transition-colors hover:bg-muted/30">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-md bg-muted text-muted-foreground">
                    <FileText className="h-4 w-4" />
                  </div>
                  <div>
                    <div className="text-xs font-medium">Logs & Crash Reports</div>
                    <div className="text-[11px] text-muted-foreground">
                      Past execution logs in logs/ and crash-reports/ (default excluded)
                    </div>
                  </div>
                </div>
                <Switch
                  checked={includeLogs}
                  onCheckedChange={setIncludeLogs}
                  disabled={busy}
                  aria-label="Include Logs and Crash Reports"
                />
              </div>
            </div>
          </div>

          {/* Advanced / Resource Options Collapsible */}
          <div className="rounded-lg border bg-card/40">
            <button
              type="button"
              disabled={busy}
              onClick={() => setShowAdvanced(!showAdvanced)}
              className="flex w-full items-center justify-between px-3.5 py-2.5 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors"
            >
              <span className="flex items-center gap-2">
                <Sliders className="h-3.5 w-3.5" />
                Advanced Settings & Excludes
              </span>
              {showAdvanced ? (
                <ChevronDown className="h-3.5 w-3.5" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5" />
              )}
            </button>

            {showAdvanced && (
              <div className="border-t p-3.5 space-y-3.5">
                {/* Custom Excludes */}
                <div className="space-y-1">
                  <Label htmlFor="custom-excludes" className="text-xs">
                    Custom Exclude Patterns
                  </Label>
                  <Input
                    id="custom-excludes"
                    type="text"
                    disabled={busy}
                    placeholder="e.g. backups/*, dynmap/*, *.tmp"
                    value={customExcludes}
                    onChange={(e) => setCustomExcludes(e.target.value)}
                    className="h-8 text-xs font-mono"
                  />
                  <p className="text-[11px] text-muted-foreground">
                    Comma-separated relative paths or glob patterns to skip.
                  </p>
                </div>

                {/* RAM Limit */}
                <div className="space-y-1">
                  <Label htmlFor="copy-ram" className="text-xs">
                    RAM Allocation (MB)
                  </Label>
                  <Input
                    id="copy-ram"
                    type="number"
                    min="512"
                    step="512"
                    disabled={busy}
                    value={ramMb}
                    onChange={(e) => setRamMb(parseInt(e.target.value, 10) || 0)}
                    className="h-8 text-xs"
                  />
                </div>
              </div>
            )}
          </div>

          {/* Error Notice */}
          {error && (
            <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive">
              {error}
            </div>
          )}

          {/* Actions */}
          <div className="flex items-center justify-end gap-2.5 pt-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={onClose}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              size="sm"
              disabled={busy || !name.trim()}
              className="gap-2"
            >
              {busy ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  <span>Copying server...</span>
                </>
              ) : (
                <>
                  <Copy className="h-3.5 w-3.5" />
                  <span>Copy Server</span>
                </>
              )}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
