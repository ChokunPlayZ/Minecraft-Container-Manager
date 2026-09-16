import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  AlertCircle,
  Check,
  Code2,
  Copy,
  ExternalLink,
  Globe,
  Info,
  Key,
  Lock,
  Plus,
  RefreshCw,
  Save,
  Server as ServerIcon,
  Shield,
  Sparkles,
  Trash2,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import {
  DEFAULT_BUNGEE_YAML,
  DEFAULT_VELOCITY_TOML,
  extractContainerId,
  generateForwardingSecret,
  parseBungeeYaml,
  parseVelocityToml,
  PlayerForwardingMode,
  ProxyBackendServer,
  ProxyConfig,
  serializeBungeeYaml,
  serializeVelocityToml,
} from '../lib/proxy-config';
import { MotdDesigner } from './motd-designer';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Select } from './ui/select';
import { Switch } from './ui/switch';
import { Textarea } from './ui/textarea';

interface ProxyConfigEditorProps {
  server: Server;
}

export function ProxyConfigEditor({ server }: { server: Server }) {
  const isVelocity = server.server_type === 'velocity';
  const defaultTemplate = isVelocity ? DEFAULT_VELOCITY_TOML : DEFAULT_BUNGEE_YAML;
  const configFileName = isVelocity ? 'velocity.toml' : 'config.yml';

  const [rawContent, setRawContent] = useState('');
  const [initialContent, setInitialContent] = useState('');
  const [config, setConfig] = useState<ProxyConfig>(() =>
    isVelocity ? parseVelocityToml(defaultTemplate) : parseBungeeYaml(defaultTemplate)
  );
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  // View & Tab
  const [viewMode, setViewMode] = useState<'interactive' | 'raw'>('interactive');
  const [activeTab, setActiveTab] = useState<'servers' | 'general' | 'security'>('servers');

  // Backend MCM Servers available in the panel
  const [panelServers, setPanelServers] = useState<Server[]>([]);
  const [selectedMcmServerId, setSelectedMcmServerId] = useState<string>('');

  // Add Server Form
  const [showAddServer, setShowAddServer] = useState(false);
  const [newServerName, setNewServerName] = useState('');
  const [newServerAddress, setNewServerAddress] = useState('');
  const [newServerMotd, setNewServerMotd] = useState('');
  const [newServerRestricted, setNewServerRestricted] = useState(false);
  const [addServerError, setAddServerError] = useState<string | null>(null);

  // Secret visibility & copied state
  const [showSecret, setShowSecret] = useState(false);
  const [copiedSecret, setCopiedSecret] = useState(false);

  // Load existing servers from panel
  const loadPanelServers = useCallback(async () => {
    try {
      const list = await api.listServers();
      // Exclude self from backend server targets
      setPanelServers(list.filter((s) => s.id !== server.id));
    } catch {
      // Non-fatal
    }
  }, [server.id]);

  const load = useCallback(async () => {
    setBusy(true);
    setError(null);
    try {
      const props = await api.getProperties(server.id);
      const content = props.content || defaultTemplate;
      setRawContent(content);
      setInitialContent(content);
      const parsed = isVelocity ? parseVelocityToml(content) : parseBungeeYaml(content);
      setConfig(parsed);
      setLoaded(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : `Failed to load ${configFileName}`);
    } finally {
      setBusy(false);
    }
  }, [server.id, defaultTemplate, isVelocity, configFileName]);

  useEffect(() => {
    void load();
    void loadPanelServers();
  }, [load, loadPanelServers]);

  const isDirty = useMemo(() => {
    if (viewMode === 'raw') {
      return rawContent !== initialContent;
    }
    const currentSerialized = isVelocity
      ? serializeVelocityToml(config)
      : serializeBungeeYaml(config);
    return currentSerialized !== initialContent;
  }, [viewMode, rawContent, initialContent, isVelocity, config]);

  const handleModeChange = (mode: 'interactive' | 'raw') => {
    if (mode === viewMode) return;
    if (mode === 'raw') {
      const serialized = isVelocity
        ? serializeVelocityToml(config)
        : serializeBungeeYaml(config);
      setRawContent(serialized);
    } else {
      const parsed = isVelocity
        ? parseVelocityToml(rawContent)
        : parseBungeeYaml(rawContent);
      setConfig(parsed);
    }
    setViewMode(mode);
  };

  const handleSave = async () => {
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      const contentToSave =
        viewMode === 'raw'
          ? rawContent
          : isVelocity
            ? serializeVelocityToml(config)
            : serializeBungeeYaml(config);

      const res = await api.saveProperties(server.id, contentToSave);
      setRawContent(res.content);
      setInitialContent(res.content);
      const updatedConfig = isVelocity
        ? parseVelocityToml(res.content)
        : parseBungeeYaml(res.content);
      setConfig(updatedConfig);
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : `Failed to save ${configFileName}`);
    } finally {
      setBusy(false);
    }
  };

  // Add backend server
  const handleAddServer = () => {
    const trimmedName = newServerName.trim().toLowerCase().replace(/[^a-z0-9_-]/g, '');
    const trimmedAddr = newServerAddress.trim();

    if (!trimmedName) {
      setAddServerError('Server name identifier is required (alphanumeric, underscores, hyphens).');
      return;
    }
    if (!trimmedAddr) {
      setAddServerError('Server target address is required.');
      return;
    }
    if (config.servers.some((s) => s.name.toLowerCase() === trimmedName)) {
      setAddServerError(`Server "${trimmedName}" is already registered.`);
      return;
    }

    setSaved(false);
    const newEntry: ProxyBackendServer = {
      name: trimmedName,
      address: trimmedAddr,
      motd: newServerMotd || undefined,
      restricted: newServerRestricted,
    };

    setConfig((prev) => {
      const nextServers = [...prev.servers, newEntry];
      const nextTry = prev.tryServers.length === 0 ? [trimmedName] : prev.tryServers;
      return { ...prev, servers: nextServers, tryServers: nextTry };
    });

    // Reset form
    setShowAddServer(false);
    setNewServerName('');
    setNewServerAddress('');
    setNewServerMotd('');
    setNewServerRestricted(false);
    setSelectedMcmServerId('');
    setAddServerError(null);
  };

  const handleDeleteServer = (name: string) => {
    setSaved(false);
    setConfig((prev) => ({
      ...prev,
      servers: prev.servers.filter((s) => s.name !== name),
      tryServers: prev.tryServers.filter((t) => t !== name),
    }));
  };

  const toggleTryServer = (name: string) => {
    setSaved(false);
    setConfig((prev) => {
      const exists = prev.tryServers.includes(name);
      const nextTry = exists
        ? prev.tryServers.filter((t) => t !== name)
        : [...prev.tryServers, name];
      return { ...prev, tryServers: nextTry };
    });
  };

  // When user picks a server from panel dropdown
  const handleSelectPanelServer = (id: string) => {
    setSelectedMcmServerId(id);
    if (!id) return;
    const target = panelServers.find((s) => s.id === id);
    if (target) {
      // Suggest sanitized name
      const cleanName = target.name
        .toLowerCase()
        .replace(/[^a-z0-9_-]/g, '-')
        .replace(/-+/g, '-')
        .replace(/^-|-$/g, '');
      setNewServerName(cleanName || `server-${target.id.slice(0, 6)}`);

      // Determine container port
      let cPort = 25565;
      if (target.server_type === 'geysermc') cPort = 19132;
      else if (target.server_type === 'velocity' || target.server_type === 'waterfall' || target.server_type === 'bungeecord') cPort = 25577;

      // Reference via container name on Docker network: mcm-<id>:port
      setNewServerAddress(`mcm-${target.id}:${cPort}`);
    }
  };

  const handleGenerateSecret = () => {
    setSaved(false);
    const secret = generateForwardingSecret();
    setConfig((prev) => ({ ...prev, forwardingSecret: secret }));
  };

  const handleCopySecret = () => {
    if (!config.forwardingSecret) return;
    void navigator.clipboard.writeText(config.forwardingSecret);
    setCopiedSecret(true);
    setTimeout(() => setCopiedSecret(false), 2000);
  };

  return (
    <Card className="border-border bg-card shadow-sm">
      <CardHeader className="border-b border-border/40 pb-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <CardTitle className="text-base font-semibold">
                Proxy Configuration ({configFileName})
              </CardTitle>
              <Badge variant="secondary" className="text-[10px] uppercase font-mono tracking-wider">
                {isVelocity ? 'Velocity' : 'BungeeCord'}
              </Badge>
            </div>
            <CardDescription className="text-xs">
              Manage backend Minecraft servers, internal container routes (<code>mcm-:id</code>), player forwarding, and MOTD.
            </CardDescription>
          </div>

          <div className="flex items-center gap-2">
            {/* View Mode Toggle */}
            <div className="inline-flex rounded-lg border border-border bg-muted/40 p-0.5">
              <button
                type="button"
                onClick={() => handleModeChange('interactive')}
                className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                  viewMode === 'interactive'
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                Easy Config
              </button>
              <button
                type="button"
                onClick={() => handleModeChange('raw')}
                className={`flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                  viewMode === 'raw'
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                <Code2 className="h-3 w-3" />
                Raw {isVelocity ? 'TOML' : 'YAML'}
              </button>
            </div>

            {/* Save Button */}
            <Button
              size="sm"
              onClick={() => void handleSave()}
              disabled={busy || !isDirty}
              className="gap-1.5 text-xs h-8"
            >
              {busy ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : saved ? (
                <Check className="h-3.5 w-3.5 text-emerald-400" />
              ) : (
                <Save className="h-3.5 w-3.5" />
              )}
              <span>{saved ? 'Saved' : isDirty ? 'Save Changes' : 'Saved'}</span>
            </Button>
          </div>
        </div>

        {error && (
          <div className="mt-3 flex items-center gap-2 rounded-lg bg-destructive/10 p-2.5 text-xs text-destructive">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}
      </CardHeader>

      <CardContent className="p-6">
        {viewMode === 'raw' ? (
          /* Raw Editor View */
          <div className="space-y-3">
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>Edit raw <code>{configFileName}</code>. Changes sync with Easy Config when saved.</span>
              <span className="font-mono">{rawContent.split('\n').length} lines</span>
            </div>
            <Textarea
              value={rawContent}
              onChange={(e) => {
                setRawContent(e.target.value);
                setSaved(false);
              }}
              rows={22}
              className="font-mono text-xs leading-relaxed bg-muted/20 resize-y"
              placeholder={`# ${configFileName} contents`}
            />
          </div>
        ) : (
          /* Interactive Easy Config View */
          <div className="space-y-6">
            {/* Category Sub-tabs */}
            <div className="flex border-b border-border/60 gap-4">
              <button
                type="button"
                onClick={() => setActiveTab('servers')}
                className={`flex items-center gap-1.5 pb-2.5 text-xs font-medium border-b-2 transition-colors ${
                  activeTab === 'servers'
                    ? 'border-primary text-primary'
                    : 'border-transparent text-muted-foreground hover:text-foreground'
                }`}
              >
                <ServerIcon className="h-3.5 w-3.5" />
                Backend Servers ({config.servers.length})
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('general')}
                className={`flex items-center gap-1.5 pb-2.5 text-xs font-medium border-b-2 transition-colors ${
                  activeTab === 'general'
                    ? 'border-primary text-primary'
                    : 'border-transparent text-muted-foreground hover:text-foreground'
                }`}
              >
                <Globe className="h-3.5 w-3.5" />
                Network &amp; MOTD
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('security')}
                className={`flex items-center gap-1.5 pb-2.5 text-xs font-medium border-b-2 transition-colors ${
                  activeTab === 'security'
                    ? 'border-primary text-primary'
                    : 'border-transparent text-muted-foreground hover:text-foreground'
                }`}
              >
                <Shield className="h-3.5 w-3.5" />
                Security &amp; Forwarding
              </button>
            </div>

            {/* TAB 1: BACKEND SERVERS */}
            {activeTab === 'servers' && (
              <div className="space-y-6">
                <div className="flex items-center justify-between">
                  <div>
                    <h3 className="text-sm font-semibold text-foreground">Backend Minecraft Servers</h3>
                    <p className="text-xs text-muted-foreground">
                      Servers mapped to this proxy. Containers on the panel network can be referenced as <code>mcm-:id:25565</code> without needing exposed host ports.
                    </p>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setShowAddServer(!showAddServer)}
                    className="gap-1.5 text-xs"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    <span>Add Backend Server</span>
                  </Button>
                </div>

                {/* Add Server Inline Form */}
                {showAddServer && (
                  <div className="rounded-xl border border-primary/20 bg-primary/5 p-4 space-y-4 animate-fadeIn">
                    <div className="flex items-center justify-between border-b border-border/40 pb-2">
                      <span className="text-xs font-semibold text-foreground">Register New Backend Server</span>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setShowAddServer(false)}
                        className="h-6 px-2 text-xs text-muted-foreground"
                      >
                        Cancel
                      </Button>
                    </div>

                    {/* Quick Pick from Panel Servers */}
                    {panelServers.length > 0 && (
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium text-foreground">
                          Quick Select from MCM Panel Servers
                        </label>
                        <Select
                          value={selectedMcmServerId}
                          onChange={(e) => handleSelectPanelServer(e.target.value)}
                          className="text-xs"
                        >
                          <option value="">-- Choose an existing panel server --</option>
                          {panelServers.map((ps) => (
                            <option key={ps.id} value={ps.id}>
                              {ps.name} ({ps.server_type} {ps.version}) {ps.host_port > 0 ? `· Port ${ps.host_port}` : '· Behind Proxy'}
                            </option>
                          ))}
                        </Select>
                        <p className="text-[11px] text-muted-foreground">
                          Selecting a server automatically configures its internal Docker container address (<code>mcm-:id</code>).
                        </p>
                      </div>
                    )}

                    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                      <div className="space-y-1.5">
                        <label className="text-xs font-medium text-foreground">Server Identifier (Name)</label>
                        <Input
                          placeholder="e.g. lobby, survival, hub"
                          value={newServerName}
                          onChange={(e) => setNewServerName(e.target.value)}
                          className="text-xs font-mono"
                        />
                        <span className="text-[10px] text-muted-foreground">
                          Used in commands (e.g. <code>/server lobby</code>).
                        </span>
                      </div>

                      <div className="space-y-1.5">
                        <label className="text-xs font-medium text-foreground">Target Address</label>
                        <Input
                          placeholder="mcm-<id>:25565 or 127.0.0.1:25565"
                          value={newServerAddress}
                          onChange={(e) => setNewServerAddress(e.target.value)}
                          className="text-xs font-mono"
                        />
                        <span className="text-[10px] text-muted-foreground">
                          Internal Docker name or hostname:port.
                        </span>
                      </div>
                    </div>

                    {!isVelocity && (
                      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                        <div className="space-y-1.5">
                          <label className="text-xs font-medium text-foreground">MOTD (Optional)</label>
                          <Input
                            placeholder="Server description"
                            value={newServerMotd}
                            onChange={(e) => setNewServerMotd(e.target.value)}
                            className="text-xs"
                          />
                        </div>
                        <div className="flex items-center gap-2 pt-5">
                          <Switch
                            checked={newServerRestricted}
                            onCheckedChange={setNewServerRestricted}
                          />
                          <span className="text-xs font-medium">Restricted access</span>
                        </div>
                      </div>
                    )}

                    {addServerError && (
                      <p className="text-xs text-destructive font-medium">{addServerError}</p>
                    )}

                    <div className="flex justify-end pt-1">
                      <Button size="sm" onClick={handleAddServer} className="text-xs">
                        Confirm &amp; Add Server
                      </Button>
                    </div>
                  </div>
                )}

                {/* Server Cards List */}
                {config.servers.length === 0 ? (
                  <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border p-8 text-center">
                    <ServerIcon className="h-8 w-8 text-muted-foreground/50 mb-2" />
                    <p className="text-sm font-medium text-foreground">No backend servers configured</p>
                    <p className="text-xs text-muted-foreground mt-1 max-w-sm">
                      Click &quot;Add Backend Server&quot; above to connect your Minecraft servers (Paper, Purpur, Fabric, etc.) to this proxy.
                    </p>
                  </div>
                ) : (
                  <div className="space-y-3">
                    {config.servers.map((srv) => {
                      const containerId = extractContainerId(srv.address);
                      const matchedServer = containerId
                        ? panelServers.find((s) => s.id === containerId)
                        : null;
                      const isTryServer = config.tryServers.includes(srv.name);

                      return (
                        <div
                          key={srv.name}
                          className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 sm:flex-row sm:items-center sm:justify-between shadow-xs hover:border-border/80 transition-colors"
                        >
                          <div className="space-y-1.5">
                            <div className="flex items-center gap-2">
                              <span className="font-semibold text-sm text-foreground font-mono">{srv.name}</span>
                              {isTryServer && (
                                <Badge variant="outline" className="text-[10px] bg-primary/10 text-primary border-primary/20">
                                  Default / Fallback
                                </Badge>
                              )}
                              {matchedServer && (
                                <Badge variant="secondary" className="text-[10px] flex items-center gap-1">
                                  <span>Panel: {matchedServer.name}</span>
                                  <span className="text-muted-foreground capitalize">({matchedServer.server_type})</span>
                                </Badge>
                              )}
                            </div>
                            <div className="flex items-center gap-2 text-xs font-mono text-muted-foreground">
                              <span>{srv.address}</span>
                              {containerId && (
                                <span className="text-[10px] text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-1.5 py-0.5 rounded">
                                  Internal Network Route
                                </span>
                              )}
                            </div>
                          </div>

                          <div className="flex items-center gap-2">
                            <Button
                              size="sm"
                              variant={isTryServer ? 'secondary' : 'outline'}
                              onClick={() => toggleTryServer(srv.name)}
                              className="text-xs h-8"
                            >
                              {isTryServer ? 'Remove from Fallbacks' : 'Set as Fallback'}
                            </Button>
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => handleDeleteServer(srv.name)}
                              className="h-8 w-8 p-0 text-destructive hover:text-destructive hover:bg-destructive/10"
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}

                {/* Priority / Fallback Order Information */}
                {config.servers.length > 0 && (
                  <div className="rounded-xl border border-border/60 bg-muted/20 p-4 space-y-2">
                    <div className="flex items-center gap-2 text-xs font-semibold text-foreground">
                      <Sparkles className="h-4 w-4 text-primary" />
                      <span>Connection Priority &amp; Fallbacks ({isVelocity ? 'try' : 'priorities'})</span>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      When players connect to the proxy, they will be sent to the first available server in this order:
                    </p>
                    <div className="flex flex-wrap gap-1.5 pt-1">
                      {config.tryServers.length === 0 ? (
                        <span className="text-xs text-amber-500 font-medium">
                          No fallback servers configured! Players will not know where to connect.
                        </span>
                      ) : (
                        config.tryServers.map((name, idx) => (
                          <span
                            key={name}
                            className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md bg-secondary text-foreground text-xs font-mono"
                          >
                            <span className="text-[10px] text-muted-foreground font-semibold">#{idx + 1}</span>
                            <span>{name}</span>
                          </span>
                        ))
                      )}
                    </div>
                  </div>
                )}
              </div>
            )}

            {/* TAB 2: NETWORK & MOTD */}
            {activeTab === 'general' && (
              <div className="space-y-6">
                <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
                  <div className="space-y-1.5">
                    <label className="text-xs font-medium text-foreground">Bind Address &amp; Port</label>
                    <Input
                      value={config.bind}
                      onChange={(e) => {
                        setSaved(false);
                        setConfig((prev) => ({ ...prev, bind: e.target.value }));
                      }}
                      className="font-mono text-xs"
                      placeholder="0.0.0.0:25577"
                    />
                    <span className="text-[11px] text-muted-foreground">
                      The IP address and port this proxy listens on for incoming players (default: <code>0.0.0.0:25577</code>).
                    </span>
                  </div>

                  <div className="space-y-1.5">
                    <label className="text-xs font-medium text-foreground">Maximum Player Slots</label>
                    <Input
                      type="number"
                      min={1}
                      max={100000}
                      value={config.showMaxPlayers}
                      onChange={(e) => {
                        setSaved(false);
                        setConfig((prev) => ({ ...prev, showMaxPlayers: parseInt(e.target.value, 10) || 500 }));
                      }}
                      className="text-xs"
                    />
                    <span className="text-[11px] text-muted-foreground">
                      Maximum players shown in the multiplayer server browser.
                    </span>
                  </div>
                </div>

                {/* MOTD Designer */}
                <div className="space-y-2 pt-2">
                  <label className="text-xs font-medium text-foreground flex items-center gap-1.5">
                    <Sparkles className="h-3.5 w-3.5 text-primary" />
                    <span>Proxy Message of the Day (MOTD)</span>
                  </label>
                  <MotdDesigner
                    value={config.motd}
                    onChange={(val) => {
                      setSaved(false);
                      setConfig((prev) => ({ ...prev, motd: val }));
                    }}
                  />
                </div>
              </div>
            )}

            {/* TAB 3: SECURITY & FORWARDING */}
            {activeTab === 'security' && (
              <div className="space-y-6">
                {/* Online Mode */}
                <div className="flex items-center justify-between rounded-xl border border-border p-4 bg-card">
                  <div className="space-y-0.5">
                    <span className="text-sm font-semibold text-foreground">Online Mode</span>
                    <p className="text-xs text-muted-foreground">
                      Verifies player accounts with Mojang session servers. Disable only for offline/testing networks.
                    </p>
                  </div>
                  <Switch
                    checked={config.onlineMode}
                    onCheckedChange={(val) => {
                      setSaved(false);
                      setConfig((prev) => ({ ...prev, onlineMode: val }));
                    }}
                  />
                </div>

                {/* Forwarding Mode */}
                <div className="space-y-2">
                  <label className="text-xs font-medium text-foreground flex items-center gap-1.5">
                    <Key className="h-3.5 w-3.5 text-primary" />
                    <span>Player Info Forwarding Mode</span>
                  </label>
                  <Select
                    value={config.playerInfoForwardingMode}
                    onChange={(e) => {
                      setSaved(false);
                      setConfig((prev) => ({
                        ...prev,
                        playerInfoForwardingMode: e.target.value as PlayerForwardingMode,
                      }));
                    }}
                    className="text-xs"
                  >
                    {isVelocity ? (
                      <>
                        <option value="modern">Modern (Recommended, HMAC Secret Token)</option>
                        <option value="bungeeguard">BungeeGuard (Header token)</option>
                        <option value="legacy">Legacy (BungeeCord style)</option>
                        <option value="none">None (Disabled)</option>
                      </>
                    ) : (
                      <>
                        <option value="legacy">BungeeCord IP Forwarding (Enabled)</option>
                        <option value="none">None (Disabled)</option>
                      </>
                    )}
                  </Select>
                  <p className="text-[11px] text-muted-foreground">
                    Securely forwards players&apos; real IP addresses and UUIDs to backend servers so bans, skins, and permissions work properly.
                  </p>
                </div>

                {/* Modern Forwarding Secret (Velocity) */}
                {isVelocity && config.playerInfoForwardingMode === 'modern' && (
                  <div className="rounded-xl border border-primary/20 bg-primary/5 p-4 space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-semibold text-foreground flex items-center gap-1.5">
                        <Lock className="h-3.5 w-3.5 text-primary" />
                        <span>Velocity Forwarding Secret</span>
                      </span>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={handleGenerateSecret}
                        className="text-xs h-7"
                      >
                        Generate New Secret
                      </Button>
                    </div>

                    <div className="flex items-center gap-2">
                      <Input
                        type={showSecret ? 'text' : 'password'}
                        value={config.forwardingSecret}
                        onChange={(e) => {
                          setSaved(false);
                          setConfig((prev) => ({ ...prev, forwardingSecret: e.target.value }));
                        }}
                        placeholder="Click Generate to create a secret"
                        className="font-mono text-xs"
                      />
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setShowSecret(!showSecret)}
                        className="text-xs h-9 px-2.5"
                      >
                        {showSecret ? 'Hide' : 'Show'}
                      </Button>
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={handleCopySecret}
                        disabled={!config.forwardingSecret}
                        className="text-xs h-9 gap-1"
                      >
                        {copiedSecret ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                        <span>{copiedSecret ? 'Copied' : 'Copy'}</span>
                      </Button>
                    </div>

                    <div className="rounded-lg bg-background/60 p-3 text-[11px] text-muted-foreground space-y-1 border border-border/40">
                      <span className="font-semibold text-foreground">How to configure your backend Paper servers:</span>
                      <p>
                        In your backend Paper server&apos;s <code>config/paper-global.yml</code>:
                      </p>
                      <pre className="font-mono text-[10px] bg-muted/60 p-2 rounded mt-1 overflow-x-auto">
{`proxies:
  velocity:
    enabled: true
    online-mode: true
    secret: "${config.forwardingSecret || '<YOUR_SECRET_HERE>'}"`}
                      </pre>
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
