import React, { useState, useMemo, useEffect, useRef } from 'react';
import {
  Package,
  X,
  Search,
  Check,
  Copy,
  Terminal,
  Sparkles,
  AlertCircle,
  Code,
} from 'lucide-react';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Textarea } from './ui/textarea';
import { Switch } from './ui/switch';
import { Badge } from './ui/badge';
import { PlayerAvatar } from './player-avatar';
import {
  searchMinecraftItems,
  ITEM_CATEGORIES,
  ItemCategory,
  MinecraftItem,
} from '../lib/minecraft-items';
import {
  isModernVersion,
  NbtFormat,
  getPresetsForItem,
  buildGiveCommand,
  validateNbt,
  NbtPreset,
} from '../lib/minecraft-nbt';

export interface GiveItemModalProps {
  open: boolean;
  onClose: () => void;
  player: string;
  serverVersion?: string;
  onSubmit: (args: { item: string; amount: number; nbt?: string }) => Promise<void>;
  busy?: boolean;
}

export function GiveItemModal({
  open,
  onClose,
  player,
  serverVersion,
  onSubmit,
  busy = false,
}: GiveItemModalProps) {
  const defaultModern = useMemo(() => isModernVersion(serverVersion), [serverVersion]);

  // Form state
  const [itemQuery, setItemQuery] = useState('minecraft:diamond_sword');
  const [selectedCategory, setSelectedCategory] = useState<ItemCategory | 'all'>('all');
  const [amount, setAmount] = useState<number>(1);
  const [enableNbt, setEnableNbt] = useState<boolean>(false);
  const [nbtFormat, setNbtFormat] = useState<NbtFormat>(defaultModern ? 'modern' : 'legacy');
  const [rawNbt, setRawNbt] = useState<string>('');
  const [dropdownOpen, setDropdownOpen] = useState<boolean>(false);
  const [highlightedIndex, setHighlightedIndex] = useState<number>(0);
  const [copied, setCopied] = useState<boolean>(false);
  const [submitting, setSubmitting] = useState<boolean>(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const inputRef = useRef<HTMLInputElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Sync format with server version initially or when server changes
  useEffect(() => {
    setNbtFormat(defaultModern ? 'modern' : 'legacy');
  }, [defaultModern]);

  // Filter items for autocomplete
  const searchResults = useMemo(() => {
    return searchMinecraftItems(itemQuery, selectedCategory, 15);
  }, [itemQuery, selectedCategory]);

  // Check if current item query matches an exact item or is custom
  const exactMatch = useMemo(() => {
    const clean = itemQuery.trim().toLowerCase();
    return searchResults.find(
      (i) => i.id.toLowerCase() === clean || i.id.replace('minecraft:', '').toLowerCase() === clean,
    );
  }, [itemQuery, searchResults]);

  // NBT Presets for currently entered/selected item
  const itemPresets = useMemo(() => {
    return getPresetsForItem(itemQuery);
  }, [itemQuery]);

  // Validation
  const nbtValidation = useMemo(() => {
    if (!enableNbt || !rawNbt.trim()) return { valid: true };
    return validateNbt(rawNbt);
  }, [enableNbt, rawNbt]);

  // Command preview
  const commandPreview = useMemo(() => {
    const activeNbt = enableNbt && rawNbt.trim() ? rawNbt.trim() : undefined;
    return buildGiveCommand(player, itemQuery, activeNbt, amount);
  }, [player, itemQuery, enableNbt, rawNbt, amount]);

  // Close dropdown on click outside
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node) &&
        inputRef.current &&
        !inputRef.current.contains(e.target as Node)
      ) {
        setDropdownOpen(false);
      }
    }
    if (dropdownOpen) {
      document.addEventListener('mousedown', handleClickOutside);
      return () => document.removeEventListener('mousedown', handleClickOutside);
    }
  }, [dropdownOpen]);

  // Reset highlight index when results change
  useEffect(() => {
    setHighlightedIndex(0);
  }, [searchResults]);

  if (!open) return null;

  function handleSelectItem(item: MinecraftItem) {
    setItemQuery(item.id);
    setDropdownOpen(false);
  }

  function handleApplyPreset(preset: NbtPreset) {
    setEnableNbt(true);
    const snippet = nbtFormat === 'modern' ? preset.modern : preset.legacy;
    setRawNbt(snippet);
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (!dropdownOpen) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        setDropdownOpen(true);
        e.preventDefault();
      }
      return;
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setHighlightedIndex((prev) => (prev + 1) % Math.max(1, searchResults.length));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setHighlightedIndex((prev) => (prev - 1 + searchResults.length) % Math.max(1, searchResults.length));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (searchResults[highlightedIndex]) {
        handleSelectItem(searchResults[highlightedIndex]);
      } else {
        setDropdownOpen(false);
      }
    } else if (e.key === 'Escape') {
      setDropdownOpen(false);
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!itemQuery.trim()) {
      setSubmitError('Item identifier cannot be empty');
      return;
    }
    if (enableNbt && !nbtValidation.valid) {
      setSubmitError(nbtValidation.error || 'Invalid NBT/Component syntax');
      return;
    }

    setSubmitting(true);
    setSubmitError(null);
    try {
      const activeNbt = enableNbt && rawNbt.trim() ? rawNbt.trim() : undefined;
      await onSubmit({
        item: itemQuery.trim(),
        amount: Math.max(1, amount),
        nbt: activeNbt,
      });
      onClose();
    } catch (err: unknown) {
      setSubmitError(err instanceof Error ? err.message : 'Failed to give item');
    } finally {
      setSubmitting(false);
    }
  }

  function copyCommand() {
    navigator.clipboard.writeText('/' + commandPreview);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="give-item-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 p-4 backdrop-blur-xs overflow-y-auto animate-in fade-in duration-150"
    >
      <div className="relative w-full max-w-xl rounded-xl border bg-card p-6 shadow-2xl text-card-foreground my-8 max-h-[90vh] flex flex-col">
        {/* Header */}
        <div className="flex items-start justify-between pb-4 border-b">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Package className="h-5 w-5" />
            </div>
            <div>
              <h2 id="give-item-title" className="text-lg font-semibold tracking-tight">
                Give Items
              </h2>
              <div className="flex items-center gap-2 mt-0.5 text-xs text-muted-foreground">
                <span>Target:</span>
                <span className="inline-flex items-center gap-1 font-medium text-foreground bg-muted px-1.5 py-0.5 rounded">
                  <PlayerAvatar name={player} size={14} />
                  {player}
                </span>
                {serverVersion && (
                  <span className="text-[11px] text-muted-foreground/80">
                    • Minecraft {serverVersion}
                  </span>
                )}
              </div>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={submitting || busy}
            className="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Scrollable Form Body */}
        <form onSubmit={handleSubmit} className="space-y-4 pt-4 overflow-y-auto pr-1 flex-1">
          {submitError && (
            <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-xs text-destructive border border-destructive/20">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{submitError}</span>
            </div>
          )}

          {/* Item Selector with Autocomplete */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label htmlFor="item-input" className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                Item Identifier
              </label>
              <span className="text-[11px] text-muted-foreground">
                {exactMatch ? (
                  <span className="text-emerald-500 font-medium">✓ {exactMatch.name}</span>
                ) : (
                  'Custom or vanilla ID'
                )}
              </span>
            </div>

            {/* Category Filter Pills */}
            <div className="flex items-center gap-1 overflow-x-auto pb-1 scrollbar-none text-[11px]">
              <button
                type="button"
                onClick={() => setSelectedCategory('all')}
                className={`rounded-full px-2 py-0.5 whitespace-nowrap transition-colors ${
                  selectedCategory === 'all'
                    ? 'bg-primary text-primary-foreground font-medium'
                    : 'bg-muted text-muted-foreground hover:bg-secondary hover:text-foreground'
                }`}
              >
                All Items
              </button>
              {ITEM_CATEGORIES.map((cat) => (
                <button
                  key={cat.id}
                  type="button"
                  onClick={() => setSelectedCategory(cat.id)}
                  className={`rounded-full px-2 py-0.5 whitespace-nowrap transition-colors ${
                    selectedCategory === cat.id
                      ? 'bg-primary text-primary-foreground font-medium'
                      : 'bg-muted text-muted-foreground hover:bg-secondary hover:text-foreground'
                  }`}
                >
                  {cat.label}
                </button>
              ))}
            </div>

            {/* Input with live dropdown */}
            <div className="relative">
              <div className="relative flex items-center">
                <Search className="absolute left-3 h-4 w-4 text-muted-foreground pointer-events-none" />
                <Input
                  ref={inputRef}
                  id="item-input"
                  value={itemQuery}
                  onChange={(e) => {
                    setItemQuery(e.target.value);
                    setDropdownOpen(true);
                  }}
                  onFocus={() => setDropdownOpen(true)}
                  onKeyDown={handleKeyDown}
                  placeholder="e.g. diamond_sword, bow, elytra..."
                  className="pl-9 pr-8 font-mono text-sm"
                  autoComplete="off"
                  autoFocus
                />
                {itemQuery && (
                  <button
                    type="button"
                    onClick={() => {
                      setItemQuery('');
                      inputRef.current?.focus();
                    }}
                    className="absolute right-2.5 text-muted-foreground hover:text-foreground"
                    aria-label="Clear item input"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>

              {/* Autocomplete Dropdown */}
              {dropdownOpen && (
                <div
                  ref={dropdownRef}
                  className="absolute z-20 mt-1 max-h-60 w-full overflow-y-auto rounded-lg border bg-popover text-popover-foreground shadow-xl text-xs py-1"
                >
                  {searchResults.length > 0 ? (
                    searchResults.map((item, idx) => {
                      const isHighlighted = idx === highlightedIndex;
                      const isCurrent = item.id.toLowerCase() === itemQuery.trim().toLowerCase();
                      return (
                        <button
                          key={item.id}
                          type="button"
                          onClick={() => handleSelectItem(item)}
                          onMouseEnter={() => setHighlightedIndex(idx)}
                          className={`flex w-full items-center justify-between px-3 py-2 text-left transition-colors ${
                            isHighlighted ? 'bg-accent text-accent-foreground' : ''
                          }`}
                        >
                          <div className="flex items-center gap-2 min-w-0">
                            <span className="font-medium text-foreground truncate">{item.name}</span>
                            <span className="font-mono text-[11px] text-muted-foreground truncate">
                              {item.id}
                            </span>
                          </div>
                          <div className="flex items-center gap-1.5 shrink-0 ml-2">
                            <Badge variant="outline" className="text-[10px] uppercase tracking-wider py-0 px-1.5 h-4">
                              {item.category}
                            </Badge>
                            {isCurrent && <Check className="h-3.5 w-3.5 text-primary" />}
                          </div>
                        </button>
                      );
                    })
                  ) : (
                    <div className="px-3 py-3 text-muted-foreground text-center">
                      No matching item found. Press Enter to use &quot;<span className="font-mono text-foreground">{itemQuery}</span>&quot;.
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>

          {/* Amount Stepper and Quick Chips */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <label htmlFor="amount-input" className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                Amount
              </label>
              <div className="flex items-center gap-1">
                {[1, 16, 32, 64].map((count) => (
                  <button
                    key={count}
                    type="button"
                    onClick={() => setAmount(count)}
                    className={`rounded px-1.5 py-0.5 text-[11px] transition-colors ${
                      amount === count
                        ? 'bg-primary text-primary-foreground font-semibold'
                        : 'bg-muted text-muted-foreground hover:bg-secondary hover:text-foreground'
                    }`}
                  >
                    {count}x
                  </button>
                ))}
              </div>
            </div>
            <Input
              id="amount-input"
              type="number"
              min={1}
              max={64}
              value={amount}
              onChange={(e) => setAmount(parseInt(e.target.value, 10) || 1)}
              className="font-mono text-sm w-full"
            />
          </div>

          {/* Raw NBT / Components Toggle & Section */}
          <div className="rounded-lg border bg-muted/20 p-3 space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Switch
                  id="enable-nbt"
                  checked={enableNbt}
                  onCheckedChange={setEnableNbt}
                />
                <label
                  htmlFor="enable-nbt"
                  className="text-xs font-medium cursor-pointer select-none flex items-center gap-1.5"
                >
                  <Sparkles className="h-3.5 w-3.5 text-amber-500" />
                  <span>Modify NBT / Item Components</span>
                </label>
              </div>

              {enableNbt && (
                <div className="flex items-center gap-1 bg-muted/60 p-0.5 rounded text-[11px]">
                  <button
                    type="button"
                    onClick={() => setNbtFormat('modern')}
                    className={`px-2 py-0.5 rounded transition-colors ${
                      nbtFormat === 'modern'
                        ? 'bg-background shadow-xs text-foreground font-medium'
                        : 'text-muted-foreground hover:text-foreground'
                    }`}
                    title="1.20.5+ Item Components syntax: [key=value]"
                  >
                    1.20.5+ Components
                  </button>
                  <button
                    type="button"
                    onClick={() => setNbtFormat('legacy')}
                    className={`px-2 py-0.5 rounded transition-colors ${
                      nbtFormat === 'legacy'
                        ? 'bg-background shadow-xs text-foreground font-medium'
                        : 'text-muted-foreground hover:text-foreground'
                    }`}
                    title="Legacy SNBT syntax: {tag:value}"
                  >
                    Legacy SNBT
                  </button>
                </div>
              )}
            </div>

            {enableNbt && (
              <div className="space-y-2 pt-1 border-t border-border/50 animate-in fade-in duration-150">
                {/* Preset Chips */}
                <div className="space-y-1.5">
                  <div className="flex items-center justify-between text-[11px] text-muted-foreground">
                    <span>Quick Presets:</span>
                    {rawNbt && (
                      <button
                        type="button"
                        onClick={() => setRawNbt('')}
                        className="text-primary hover:underline"
                      >
                        Clear NBT
                      </button>
                    )}
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {itemPresets.map((preset) => (
                      <button
                        key={preset.id}
                        type="button"
                        onClick={() => handleApplyPreset(preset)}
                        title={preset.description}
                        className="rounded-md border bg-card px-2 py-1 text-[11px] text-foreground hover:bg-accent hover:border-primary/50 transition-all text-left shadow-xs flex items-center gap-1"
                      >
                        {preset.label}
                      </button>
                    ))}
                  </div>
                </div>

                {/* Monospace Raw NBT Textarea */}
                <div className="space-y-1">
                  <div className="flex items-center justify-between text-[11px]">
                    <span className="text-muted-foreground flex items-center gap-1">
                      <Code className="h-3 w-3" />
                      <span>
                        {nbtFormat === 'modern' ? 'Components String [ ... ]' : 'SNBT String { ... }'}
                      </span>
                    </span>
                    {!nbtValidation.valid && (
                      <span className="text-destructive font-medium">
                        {nbtValidation.error}
                      </span>
                    )}
                  </div>
                  <Textarea
                    value={rawNbt}
                    onChange={(e) => setRawNbt(e.target.value)}
                    placeholder={
                      nbtFormat === 'modern'
                        ? "[enchantments={levels:{'minecraft:sharpness':5}},unbreakable={}]"
                        : '{Enchantments:[{id:"minecraft:sharpness",lvl:5s}],Unbreakable:1b}'
                    }
                    className={`font-mono text-xs min-h-[75px] ${
                      !nbtValidation.valid ? 'border-destructive focus-visible:ring-destructive' : ''
                    }`}
                  />
                  <p className="text-[10px] text-muted-foreground">
                    Tip: You can edit or paste any arbitrary Minecraft NBT / components here (e.g. custom enchants, potion effects, display names).
                  </p>
                </div>
              </div>
            )}
          </div>

          {/* Command Preview */}
          <div className="space-y-1.5 rounded-lg border bg-muted/40 p-3">
            <div className="flex items-center justify-between text-[11px] text-muted-foreground">
              <span className="flex items-center gap-1 font-medium">
                <Terminal className="h-3 w-3" />
                <span>Command Preview</span>
              </span>
              <button
                type="button"
                onClick={copyCommand}
                className="flex items-center gap-1 text-primary hover:underline"
              >
                {copied ? (
                  <>
                    <Check className="h-3 w-3 text-emerald-500" />
                    <span className="text-emerald-500">Copied</span>
                  </>
                ) : (
                  <>
                    <Copy className="h-3 w-3" />
                    <span>Copy</span>
                  </>
                )}
              </button>
            </div>
            <div className="rounded bg-black/40 p-2 font-mono text-xs text-emerald-400 break-all select-all">
              /{commandPreview}
            </div>
          </div>

          {/* Footer actions */}
          <div className="flex items-center justify-end gap-2 pt-2 border-t">
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={submitting || busy}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={submitting || busy || (enableNbt && !nbtValidation.valid)}
              className="gap-1.5"
            >
              <Package className="h-4 w-4" />
              <span>{submitting ? 'Giving...' : `Give to ${player}`}</span>
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
