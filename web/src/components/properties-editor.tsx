import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  AlertCircle,
  Check,
  Code2,
  Cpu,
  Eye,
  EyeOff,
  Gamepad2,
  Globe,
  Plus,
  RefreshCw,
  Save,
  Search,
  Shield,
  Sliders,
  SlidersHorizontal,
  Sparkles,
  Terminal,
  Trash2,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server } from '../api/types';
import {
  deletePropertyFromLines,
  getCustomProperties,
  getPropertiesMap,
  ParsedPropertyLine,
  parseProperties,
  PROPERTY_CATEGORIES,
  PROPERTY_SCHEMAS,
  PropertyCategory,
  serializeProperties,
  updatePropertyInLines,
} from '../lib/server-properties';
import { MotdDesigner } from './motd-designer';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Select } from './ui/select';
import { Switch } from './ui/switch';
import { Textarea } from './ui/textarea';

const DEFAULT_PROPERTIES_TEMPLATE = `# Minecraft server properties
server-port=25565
motd=A Minecraft Server
max-players=20
gamemode=survival
difficulty=easy
pvp=true
online-mode=true
enable-rcon=false
rcon.port=25575
view-distance=10
simulation-distance=10
white-list=false
level-name=world
`;

const CATEGORY_ICONS: Record<PropertyCategory, typeof Sparkles> = {
  motd: Sparkles,
  general: Globe,
  gameplay: Gamepad2,
  security: Shield,
  performance: Cpu,
  rcon: Terminal,
  custom: Sliders,
};

export function PropertiesEditor({ server }: { server: Server }) {
  const [rawContent, setRawContent] = useState('');
  const [initialContent, setInitialContent] = useState('');
  const [parsedLines, setParsedLines] = useState<ParsedPropertyLine[]>([]);
  const [exists, setExists] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  // View & Filter States
  const [viewMode, setViewMode] = useState<'interactive' | 'raw'>('interactive');
  const [activeCategory, setActiveCategory] = useState<PropertyCategory | 'all'>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [showRconPassword, setShowRconPassword] = useState(false);

  // New Custom Property inputs
  const [newKey, setNewKey] = useState('');
  const [newVal, setNewVal] = useState('');
  const [customError, setCustomError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setBusy(true);
    setError(null);
    try {
      const props = await api.getProperties(server.id);
      const content = props.content || (props.exists ? '' : DEFAULT_PROPERTIES_TEMPLATE);
      setRawContent(content);
      setInitialContent(content);
      setParsedLines(parseProperties(content));
      setExists(props.exists);
      setLoaded(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load server.properties');
    } finally {
      setBusy(false);
    }
  }, [server.id]);

  useEffect(() => {
    void load();
  }, [load]);

  // Derived properties map
  const propertiesMap = useMemo(() => getPropertiesMap(parsedLines), [parsedLines]);
  const customProperties = useMemo(() => getCustomProperties(parsedLines), [parsedLines]);

  // Check if content is dirty compared to saved state
  const isDirty = useMemo(() => {
    if (viewMode === 'raw') {
      return rawContent !== initialContent;
    }
    return serializeProperties(parsedLines) !== initialContent;
  }, [viewMode, rawContent, initialContent, parsedLines]);

  const handleModeChange = (newMode: 'interactive' | 'raw') => {
    if (newMode === viewMode) return;
    if (newMode === 'raw') {
      // Sync interactive lines to raw string
      const updated = serializeProperties(parsedLines);
      setRawContent(updated);
    } else {
      // Sync raw string to interactive lines
      const parsed = parseProperties(rawContent);
      setParsedLines(parsed);
    }
    setViewMode(newMode);
  };

  const handlePropertyChange = (key: string, value: string) => {
    setSaved(false);
    setParsedLines((prev) => {
      const next = updatePropertyInLines(prev, key, value);
      setRawContent(serializeProperties(next));
      return next;
    });
  };

  const handleDeleteProperty = (key: string) => {
    setSaved(false);
    setParsedLines((prev) => {
      const next = deletePropertyFromLines(prev, key);
      setRawContent(serializeProperties(next));
      return next;
    });
  };

  const handleAddCustomProperty = () => {
    const trimmedKey = newKey.trim();
    if (!trimmedKey) {
      setCustomError('Property key cannot be empty');
      return;
    }
    if (trimmedKey.includes('=') || trimmedKey.includes(' ')) {
      setCustomError('Property key cannot contain spaces or "="');
      return;
    }
    setCustomError(null);
    handlePropertyChange(trimmedKey, newVal.trim());
    setNewKey('');
    setNewVal('');
  };

  async function save() {
    setBusy(true);
    setError(null);
    setSaved(false);
    try {
      const contentToSave =
        viewMode === 'raw' ? rawContent : serializeProperties(parsedLines);
      const props = await api.saveProperties(server.id, contentToSave);
      setRawContent(props.content);
      setInitialContent(props.content);
      setParsedLines(parseProperties(props.content));
      setExists(true);
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to save server.properties');
    } finally {
      setBusy(false);
    }
  }

  // Filter schemas based on activeCategory and searchQuery
  const filteredSchemas = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    return PROPERTY_SCHEMAS.filter((schema) => {
      if (activeCategory !== 'all' && schema.category !== activeCategory) {
        return false;
      }
      if (!query) return true;
      return (
        schema.key.toLowerCase().includes(query) ||
        schema.label.toLowerCase().includes(query) ||
        schema.description.toLowerCase().includes(query)
      );
    });
  }, [activeCategory, searchQuery]);

  // Filter custom properties based on searchQuery
  const filteredCustom = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (activeCategory !== 'all' && activeCategory !== 'custom') {
      return [];
    }
    if (!query) return customProperties;
    return customProperties.filter(
      (c) => c.key.toLowerCase().includes(query) || c.value.toLowerCase().includes(query),
    );
  }, [activeCategory, searchQuery, customProperties]);

  const showMotdSection =
    (activeCategory === 'all' || activeCategory === 'motd') &&
    (!searchQuery ||
      'motd'.includes(searchQuery.toLowerCase()) ||
      'message of the day'.includes(searchQuery.toLowerCase()));

  return (
    <Card className="shadow-xs">
      <CardHeader className="pb-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2">
              <CardTitle className="text-base font-semibold">Server Properties</CardTitle>
              {isDirty && (
                <Badge variant="secondary" className="text-[10px] bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/30">
                  Unsaved Changes
                </Badge>
              )}
            </div>
            <CardDescription className="text-xs">
              Configure server parameters, gameplay rules, and design the server MOTD.
            </CardDescription>
          </div>

          <div className="flex items-center gap-2 self-end sm:self-auto">
            {/* View Mode Toggle */}
            <div className="inline-flex rounded-lg border border-input bg-muted/40 p-0.5 text-xs">
              <button
                type="button"
                onClick={() => handleModeChange('interactive')}
                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md font-medium transition-colors ${
                  viewMode === 'interactive'
                    ? 'bg-background text-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                <SlidersHorizontal className="h-3.5 w-3.5" />
                Interactive
              </button>
              <button
                type="button"
                onClick={() => handleModeChange('raw')}
                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md font-medium transition-colors ${
                  viewMode === 'raw'
                    ? 'bg-background text-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                <Code2 className="h-3.5 w-3.5" />
                Raw Config
              </button>
            </div>

            <Button
              variant="ghost"
              size="icon"
              onClick={() => void load()}
              disabled={busy}
              aria-label="Reload properties"
              className="h-8 w-8"
              title="Reload from disk"
            >
              <RefreshCw className={`h-4 w-4 ${busy ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {exists === false && !error && (
          <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-2.5 text-xs text-amber-700 dark:text-amber-400 flex items-center gap-2">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>
              No server.properties file exists yet. A default template is preloaded; click Save to
              create it.
            </span>
          </div>
        )}

        {error && (
          <div className="rounded-md border border-destructive/30 bg-destructive/10 p-2.5 text-xs text-destructive flex items-center gap-2">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {loaded && viewMode === 'interactive' && (
          <div className="space-y-4">
            {/* Search and Category Filter Bar */}
            <div className="space-y-2.5">
              <div className="relative">
                <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
                <Input
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  placeholder="Filter properties (e.g. port, pvp, flight, distance, rcon)..."
                  className="pl-8 h-8 text-xs"
                />
                {searchQuery && (
                  <button
                    type="button"
                    onClick={() => setSearchQuery('')}
                    className="absolute right-2.5 top-1/2 -translate-y-1/2 text-xs text-muted-foreground hover:text-foreground"
                  >
                    ✕
                  </button>
                )}
              </div>

              {/* Category Pills */}
              <div className="flex flex-wrap items-center gap-1.5 text-xs">
                <button
                  type="button"
                  onClick={() => setActiveCategory('all')}
                  className={`px-2.5 py-1 rounded-md text-xs font-medium border transition-colors ${
                    activeCategory === 'all'
                      ? 'bg-primary text-primary-foreground border-primary'
                      : 'border-border bg-muted/40 text-muted-foreground hover:text-foreground'
                  }`}
                >
                  All Settings
                </button>
                {PROPERTY_CATEGORIES.map((cat) => {
                  const Icon = CATEGORY_ICONS[cat.id];
                  const isActive = activeCategory === cat.id;
                  return (
                    <button
                      key={cat.id}
                      type="button"
                      onClick={() => setActiveCategory(cat.id)}
                      className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium border transition-colors ${
                        isActive
                          ? 'bg-primary text-primary-foreground border-primary'
                          : 'border-border bg-muted/40 text-muted-foreground hover:text-foreground'
                      }`}
                    >
                      <Icon className="h-3 w-3" />
                      {cat.label}
                    </button>
                  );
                })}
              </div>
            </div>

            {/* 1. Built-in MOTD Designer */}
            {showMotdSection && (
              <div className="rounded-lg border border-border bg-muted/20 p-4 space-y-3">
                <div className="flex items-center justify-between border-b border-border pb-2">
                  <div className="flex items-center gap-2">
                    <Sparkles className="h-4 w-4 text-amber-500" />
                    <h3 className="text-sm font-semibold">Message of the Day (MOTD)</h3>
                    <code className="text-[10px] font-mono bg-muted px-1.5 py-0.5 rounded text-muted-foreground">
                      motd
                    </code>
                  </div>
                </div>

                <MotdDesigner
                  value={propertiesMap['motd'] ?? 'A Minecraft Server'}
                  onChange={(newMotd) => handlePropertyChange('motd', newMotd)}
                  serverName={server.name}
                  serverVersion={server.version}
                  maxPlayers={propertiesMap['max-players'] || server.ram_mb || 20}
                />
              </div>
            )}

            {/* 2. Structured Mapped Properties Grid */}
            {filteredSchemas.filter((s) => s.type !== 'motd').length > 0 && (
              <div className="space-y-3">
                <div className="grid gap-3 sm:grid-cols-2">
                  {filteredSchemas
                    .filter((s) => s.type !== 'motd')
                    .map((schema) => {
                      const currentValue = propertiesMap[schema.key] ?? schema.defaultValue ?? '';
                      return (
                        <div
                          key={schema.key}
                          className="flex flex-col justify-between rounded-lg border border-border bg-card p-3 shadow-xs space-y-2.5 hover:border-border/80 transition-colors"
                        >
                          <div>
                            <div className="flex items-center justify-between gap-2">
                              <label
                                htmlFor={`prop-${schema.key}`}
                                className="text-xs font-semibold text-foreground tracking-tight"
                              >
                                {schema.label}
                              </label>
                              <code className="text-[10px] font-mono bg-muted/70 px-1 py-0.5 rounded text-muted-foreground">
                                {schema.key}
                              </code>
                            </div>
                            <p className="text-[11px] text-muted-foreground mt-0.5 line-clamp-2">
                              {schema.description}
                            </p>
                          </div>

                          <div className="pt-1">
                            {schema.type === 'boolean' && (
                              <div className="flex items-center justify-between">
                                <span className="text-xs font-mono text-muted-foreground">
                                  {currentValue === 'true' ? 'Enabled' : 'Disabled'}
                                </span>
                                <Switch
                                  id={`prop-${schema.key}`}
                                  aria-label={schema.label}
                                  checked={currentValue === 'true'}
                                  onCheckedChange={(checked) =>
                                    handlePropertyChange(schema.key, checked ? 'true' : 'false')
                                  }
                                />
                              </div>
                            )}

                            {schema.type === 'number' && (
                              <Input
                                id={`prop-${schema.key}`}
                                type="number"
                                min={schema.min}
                                max={schema.max}
                                step={schema.step}
                                value={currentValue}
                                onChange={(e) => handlePropertyChange(schema.key, e.target.value)}
                                className="h-8 text-xs font-mono"
                              />
                            )}

                            {schema.type === 'select' && (
                              <Select
                                id={`prop-${schema.key}`}
                                value={currentValue}
                                onChange={(e) => handlePropertyChange(schema.key, e.target.value)}
                                className="h-8 text-xs font-mono"
                              >
                                {schema.options?.map((opt) => (
                                  <option key={opt.value} value={opt.value}>
                                    {opt.label}
                                  </option>
                                ))}
                              </Select>
                            )}

                            {schema.type === 'text' && schema.key === 'rcon.password' && (
                              <div className="relative">
                                <Input
                                  id={`prop-${schema.key}`}
                                  type={showRconPassword ? 'text' : 'password'}
                                  value={currentValue}
                                  placeholder={schema.placeholder}
                                  onChange={(e) => handlePropertyChange(schema.key, e.target.value)}
                                  className="h-8 text-xs font-mono pr-8"
                                />
                                <button
                                  type="button"
                                  onClick={() => setShowRconPassword((v) => !v)}
                                  className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                                  title={showRconPassword ? 'Hide password' : 'Show password'}
                                >
                                  {showRconPassword ? (
                                    <EyeOff className="h-3.5 w-3.5" />
                                  ) : (
                                    <Eye className="h-3.5 w-3.5" />
                                  )}
                                </button>
                              </div>
                            )}

                            {schema.type === 'text' && schema.key !== 'rcon.password' && (
                              <Input
                                id={`prop-${schema.key}`}
                                type="text"
                                value={currentValue}
                                placeholder={schema.placeholder}
                                onChange={(e) => handlePropertyChange(schema.key, e.target.value)}
                                className="h-8 text-xs font-mono"
                              />
                            )}
                          </div>
                        </div>
                      );
                    })}
                </div>
              </div>
            )}

            {/* 3. Custom / Unmapped Properties */}
            {(activeCategory === 'all' || activeCategory === 'custom') && (
              <div className="rounded-lg border border-border bg-card p-4 space-y-3">
                <div className="flex items-center justify-between border-b border-border pb-2">
                  <div className="flex items-center gap-2">
                    <Sliders className="h-4 w-4 text-primary" />
                    <h3 className="text-sm font-semibold">Custom & Mod Properties</h3>
                    <Badge variant="outline" className="text-[10px]">
                      {filteredCustom.length}
                    </Badge>
                  </div>
                  <span className="text-[11px] text-muted-foreground">
                    Forge, Fabric, or custom plugin entries
                  </span>
                </div>

                {filteredCustom.length > 0 ? (
                  <div className="space-y-2">
                    {filteredCustom.map((item) => (
                      <div
                        key={item.key}
                        className="flex items-center gap-2 rounded-md border border-input/60 bg-muted/20 p-2 text-xs"
                      >
                        <span className="w-1/3 min-w-[120px] font-mono font-medium truncate text-foreground">
                          {item.key}
                        </span>
                        <Input
                          value={item.value}
                          onChange={(e) => handlePropertyChange(item.key, e.target.value)}
                          className="h-7 text-xs font-mono flex-1"
                        />
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          onClick={() => handleDeleteProperty(item.key)}
                          className="h-7 w-7 text-destructive hover:text-destructive hover:bg-destructive/10"
                          title="Delete property"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground py-1">
                    No custom or mod-added properties detected.
                  </p>
                )}

                {/* Add new custom property form */}
                <div className="pt-2 border-t border-border/60">
                  <span className="text-xs font-medium text-foreground block mb-1.5">
                    Add New Property
                  </span>
                  <div className="flex flex-col sm:flex-row items-center gap-2">
                    <Input
                      placeholder="Property key (e.g. view-distance-no-tick)"
                      value={newKey}
                      onChange={(e) => setNewKey(e.target.value)}
                      className="h-8 text-xs font-mono"
                    />
                    <Input
                      placeholder="Value"
                      value={newVal}
                      onChange={(e) => setNewVal(e.target.value)}
                      className="h-8 text-xs font-mono"
                    />
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={handleAddCustomProperty}
                      className="h-8 text-xs shrink-0 w-full sm:w-auto gap-1"
                    >
                      <Plus className="h-3.5 w-3.5" />
                      Add
                    </Button>
                  </div>
                  {customError && (
                    <p className="text-xs text-destructive mt-1">{customError}</p>
                  )}
                </div>
              </div>
            )}
          </div>
        )}

        {/* Raw Config View */}
        {loaded && viewMode === 'raw' && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>Direct text representation of server.properties</span>
              <span>Lines: {rawContent.split('\n').length}</span>
            </div>
            <Textarea
              className="min-h-[380px] font-mono text-xs leading-relaxed whitespace-pre bg-card/60"
              value={rawContent}
              spellCheck={false}
              onChange={(e) => {
                setSaved(false);
                setRawContent(e.target.value);
              }}
              placeholder="# Minecraft server properties"
            />
          </div>
        )}

        {/* Footer actions & save */}
        <div className="flex items-center justify-between pt-2 border-t border-border">
          <div>
            {saved && (
              <span className="inline-flex items-center gap-1.5 text-xs text-emerald-600 dark:text-emerald-400 font-medium">
                <Check className="h-3.5 w-3.5" />
                Properties saved successfully.
              </span>
            )}
            {isDirty && !saved && (
              <span className="text-xs text-muted-foreground">
                Don't forget to save your modifications.
              </span>
            )}
          </div>

          <Button onClick={() => void save()} disabled={busy || !loaded} className="gap-1.5">
            <Save className="h-4 w-4" />
            {busy ? 'Saving...' : 'Save Properties'}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
