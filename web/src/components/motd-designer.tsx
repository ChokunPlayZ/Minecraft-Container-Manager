import { useEffect, useMemo, useRef, useState } from 'react';
import {
  AlignHorizontalJustifyCenter,
  Eraser,
  Palette,
  Sparkles,
  Wand2,
} from 'lucide-react';
import {
  centerMotdLine,
  decodeMotdFromProperties,
  encodeMotdForProperties,
  generateGradient,
  MINECRAFT_COLORS,
  MINECRAFT_STYLES,
  parseMotdToLines,
  stripFormatting,
} from '../lib/motd';
import { Button } from './ui/button';
import { Input } from './ui/input';

interface MotdDesignerProps {
  value: string;
  onChange: (newValue: string) => void;
  serverName?: string;
  serverVersion?: string;
  maxPlayers?: number | string;
}

const SYMBOL_PRESETS = [
  '★', '☆', '⚡', '➤', '➔', '»', '«', '✔', '✖', '●', '✦', '✧', '⚔', '⛏', '❤', '❘', '[', ']', '•', '◆',
];

const GRADIENT_PRESETS = [
  { name: 'Fire Flame', from: '#ff1b6b', to: '#ff930f' },
  { name: 'Ocean Aqua', from: '#00c6ff', to: '#0072ff' },
  { name: 'Emerald', from: '#11998e', to: '#38ef7d' },
  { name: 'Cyberpunk', from: '#f857a6', to: '#ff5858' },
  { name: 'Golden Sun', from: '#f7971e', to: '#ffd200' },
  { name: 'Royal Violet', from: '#8a2387', to: '#e94057' },
];

const RANDOM_CHARS = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+-=';

export function MotdDesigner({
  value,
  onChange,
  serverName = 'Minecraft Server',
  serverVersion = '1.21.4',
  maxPlayers = 20,
}: MotdDesignerProps) {
  const decoded = useMemo(() => decodeMotdFromProperties(value), [value]);
  const [line1, setLine1] = useState(decoded.line1);
  const [line2, setLine2] = useState(decoded.line2);
  const [activeLine, setActiveLine] = useState<1 | 2>(1);
  const [customHex, setCustomHex] = useState('#55FFFF');
  const [showGradientModal, setShowGradientModal] = useState(false);
  const [gradientText, setGradientText] = useState('SPECIAL EVENT!');
  const [gradientFrom, setGradientFrom] = useState('#f7971e');
  const [gradientTo, setGradientTo] = useState('#ffd200');

  // Animation tick for obfuscated (§k) text
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const timer = setInterval(() => {
      setTick((t) => (t + 1) % 1000);
    }, 80);
    return () => clearInterval(timer);
  }, []);

  // Synchronize internal lines when incoming value prop changes externally
  useEffect(() => {
    const next = decodeMotdFromProperties(value);
    setLine1(next.line1);
    setLine2(next.line2);
  }, [value]);

  const line1Ref = useRef<HTMLInputElement>(null);
  const line2Ref = useRef<HTMLInputElement>(null);

  const notifyChange = (newLine1: string, newLine2: string) => {
    setLine1(newLine1);
    setLine2(newLine2);
    const encoded = encodeMotdForProperties(newLine1, newLine2);
    onChange(encoded);
  };

  const getActiveInput = () => (activeLine === 1 ? line1Ref.current : line2Ref.current);

  const insertTextAtActive = (textToInsert: string) => {
    const input = getActiveInput();
    const currentVal = activeLine === 1 ? line1 : line2;

    if (!input) {
      if (activeLine === 1) notifyChange(line1 + textToInsert, line2);
      else notifyChange(line1, line2 + textToInsert);
      return;
    }

    const start = input.selectionStart ?? currentVal.length;
    const end = input.selectionEnd ?? currentVal.length;

    let updated: string;
    if (start !== end) {
      // Selected range: wrap or prefix
      const selected = currentVal.substring(start, end);
      updated = currentVal.slice(0, start) + textToInsert + selected + currentVal.slice(end);
    } else {
      updated = currentVal.slice(0, start) + textToInsert + currentVal.slice(end);
    }

    if (activeLine === 1) notifyChange(updated, line2);
    else notifyChange(line1, updated);

    setTimeout(() => {
      input.focus();
      const newPos = start + textToInsert.length;
      input.setSelectionRange(newPos, newPos);
    }, 10);
  };

  const applyColor = (code: string) => {
    insertTextAtActive(`§${code}`);
  };

  const applyHex = (hex: string) => {
    insertTextAtActive(`&#${hex.replace('#', '')}`);
  };

  const applyStyle = (code: string) => {
    insertTextAtActive(`§${code}`);
  };

  const handleCenter = (lineNum: 1 | 2) => {
    if (lineNum === 1) {
      notifyChange(centerMotdLine(line1), line2);
    } else {
      notifyChange(line1, centerMotdLine(line2));
    }
  };

  const handleStrip = (lineNum: 1 | 2) => {
    if (lineNum === 1) {
      notifyChange(stripFormatting(line1), line2);
    } else {
      notifyChange(line1, stripFormatting(line2));
    }
  };

  const handleInsertGradient = () => {
    if (!gradientText.trim()) return;
    const grad = generateGradient(gradientText, gradientFrom, gradientTo);
    insertTextAtActive(grad);
    setShowGradientModal(false);
  };

  // Parse lines for live preview
  const previewLines = useMemo(() => {
    return parseMotdToLines(`${line1}\n${line2}`);
  }, [line1, line2]);

  const visibleCount1 = stripFormatting(line1).length;
  const visibleCount2 = stripFormatting(line2).length;

  return (
    <div className="space-y-4">
      {/* 1. Live Minecraft Server List Card Preview */}
      <div>
        <div className="flex items-center justify-between pb-1.5">
          <label className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
            <Sparkles className="h-3.5 w-3.5 text-amber-500" />
            Live Minecraft Server Browser Preview
          </label>
          <span className="text-[11px] text-muted-foreground">Faithful multiplayer client view</span>
        </div>

        <div className="rounded-lg border border-border bg-[#101114] p-3 text-zinc-100 shadow-md">
          <div className="flex items-start gap-3">
            {/* Minecraft Grass Block SVG Icon */}
            <div className="h-12 w-12 shrink-0 rounded bg-[#1e2025] p-1 border border-zinc-700/60 shadow-inner flex items-center justify-center">
              <svg
                viewBox="0 0 16 16"
                className="h-10 w-10 image-pixelated"
                shapeRendering="crispEdges"
                aria-label="Minecraft Server Icon"
              >
                {/* Grass top */}
                <rect x="0" y="0" width="16" height="5" fill="#588c2b" />
                <rect x="2" y="1" width="3" height="2" fill="#71b337" />
                <rect x="8" y="2" width="4" height="2" fill="#71b337" />
                <rect x="0" y="4" width="2" height="3" fill="#588c2b" />
                <rect x="4" y="4" width="3" height="3" fill="#588c2b" />
                <rect x="10" y="4" width="2" height="3" fill="#588c2b" />
                <rect x="14" y="4" width="2" height="2" fill="#588c2b" />
                {/* Dirt body */}
                <rect x="0" y="7" width="16" height="9" fill="#866043" />
                <rect x="2" y="5" width="2" height="2" fill="#866043" />
                <rect x="7" y="5" width="3" height="2" fill="#866043" />
                <rect x="12" y="5" width="2" height="2" fill="#866043" />
                <rect x="3" y="8" width="2" height="2" fill="#62442d" />
                <rect x="11" y="9" width="3" height="2" fill="#62442d" />
                <rect x="6" y="12" width="2" height="2" fill="#62442d" />
                <rect x="1" y="13" width="3" height="2" fill="#503520" />
                <rect x="13" y="13" width="2" height="2" fill="#503520" />
              </svg>
            </div>

            {/* Content Area */}
            <div className="flex-1 min-w-0">
              {/* Header row: Name, ping, player count */}
              <div className="flex items-center justify-between gap-2 border-b border-zinc-800/80 pb-1 mb-1.5 font-mono text-xs">
                <span className="font-bold text-white tracking-wide truncate drop-shadow-[1px_1px_0px_#000]">
                  {serverName || 'Minecraft Server'}
                </span>

                <div className="flex items-center gap-3 shrink-0">
                  <span className="text-[10px] text-zinc-400 bg-zinc-800/70 px-1.5 py-0.5 rounded">
                    {serverVersion}
                  </span>
                  <div className="flex items-baseline gap-0.5 text-zinc-400 drop-shadow-[1px_1px_0px_#000]">
                    <span className="text-zinc-200">0</span>
                    <span className="text-zinc-500">/</span>
                    <span className="text-zinc-300">{maxPlayers}</span>
                  </div>
                  {/* Ping bars */}
                  <div className="flex items-end gap-0.5 h-3.5 w-4" title="Ping: 12ms">
                    <span className="w-0.5 h-1 bg-emerald-400 rounded-xs" />
                    <span className="w-0.5 h-1.5 bg-emerald-400 rounded-xs" />
                    <span className="w-0.5 h-2 bg-emerald-400 rounded-xs" />
                    <span className="w-0.5 h-2.5 bg-emerald-400 rounded-xs" />
                    <span className="w-0.5 h-3 bg-emerald-400 rounded-xs" />
                  </div>
                </div>
              </div>

              {/* 2-line MOTD live rendering */}
              <div className="font-mono text-xs leading-5 min-h-[42px] select-text">
                {previewLines.map((lineObj, lIdx) => (
                  <div
                    key={lIdx}
                    className="truncate drop-shadow-[1.5px_1.5px_0px_#000000] whitespace-pre"
                  >
                    {lineObj.spans.map((span, sIdx) => {
                      let text = span.text;
                      if (span.obfuscated && text.length > 0) {
                        text = text
                          .split('')
                          .map((ch, charIdx) =>
                            ch === ' '
                              ? ' '
                              : RANDOM_CHARS[(tick + charIdx * 7 + lIdx * 13) % RANDOM_CHARS.length],
                          )
                          .join('');
                      }

                      return (
                        <span
                          key={sIdx}
                          style={{
                            color: span.color || '#AAAAAA',
                            fontWeight: span.bold ? '700' : 'normal',
                            fontStyle: span.italic ? 'italic' : 'normal',
                            textDecoration: [
                              span.underline && 'underline',
                              span.strikethrough && 'line-through',
                            ]
                              .filter(Boolean)
                              .join(' '),
                          }}
                        >
                          {text || (lIdx === 0 && lineObj.spans.length === 1 ? ' ' : '')}
                        </span>
                      );
                    })}
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* 2. Formatting Toolbar */}
      <div className="rounded-lg border border-border bg-card p-3 space-y-3 shadow-xs">
        <div className="flex flex-wrap items-center justify-between gap-2">
          {/* Target line selector pills */}
          <div className="inline-flex items-center rounded-md border border-input bg-muted/30 p-0.5 text-xs">
            <button
              type="button"
              onClick={() => setActiveLine(1)}
              className={`px-3 py-1 rounded-sm font-medium transition-colors ${
                activeLine === 1
                  ? 'bg-background text-foreground shadow-xs'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              Line 1 {visibleCount1 > 0 && `(${visibleCount1}c)`}
            </button>
            <button
              type="button"
              onClick={() => setActiveLine(2)}
              className={`px-3 py-1 rounded-sm font-medium transition-colors ${
                activeLine === 2
                  ? 'bg-background text-foreground shadow-xs'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              Line 2 {visibleCount2 > 0 && `(${visibleCount2}c)`}
            </button>
          </div>

          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-7 text-xs gap-1"
              onClick={() => setShowGradientModal((v) => !v)}
            >
              <Wand2 className="h-3 w-3 text-primary" />
              Gradient Maker
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 text-xs gap-1"
              onClick={() => handleCenter(activeLine)}
              title="Center text in line"
            >
              <AlignHorizontalJustifyCenter className="h-3 w-3" />
              Center
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 text-xs gap-1 text-destructive hover:text-destructive"
              onClick={() => handleStrip(activeLine)}
              title="Remove color and format codes"
            >
              <Eraser className="h-3 w-3" />
              Strip
            </Button>
          </div>
        </div>

        {/* Gradient Generator Panel (Collapsible) */}
        {showGradientModal && (
          <div className="rounded-md border border-primary/30 bg-primary/5 p-3 space-y-2.5 animate-in fade-in">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-foreground flex items-center gap-1.5">
                <Wand2 className="h-3.5 w-3.5 text-primary" />
                Text Gradient Generator
              </span>
              <button
                type="button"
                onClick={() => setShowGradientModal(false)}
                className="text-xs text-muted-foreground hover:text-foreground"
              >
                ✕
              </button>
            </div>

            <div className="grid gap-2 sm:grid-cols-3">
              <div className="sm:col-span-3">
                <Input
                  value={gradientText}
                  onChange={(e) => setGradientText(e.target.value)}
                  placeholder="Text to apply gradient to..."
                  className="h-8 text-xs"
                />
              </div>
              <div className="flex items-center gap-1.5">
                <label className="text-[11px] text-muted-foreground">From:</label>
                <input
                  type="color"
                  value={gradientFrom}
                  onChange={(e) => setGradientFrom(e.target.value)}
                  className="h-7 w-8 cursor-pointer rounded border border-input bg-transparent p-0.5"
                />
                <span className="font-mono text-xs">{gradientFrom}</span>
              </div>
              <div className="flex items-center gap-1.5">
                <label className="text-[11px] text-muted-foreground">To:</label>
                <input
                  type="color"
                  value={gradientTo}
                  onChange={(e) => setGradientTo(e.target.value)}
                  className="h-7 w-8 cursor-pointer rounded border border-input bg-transparent p-0.5"
                />
                <span className="font-mono text-xs">{gradientTo}</span>
              </div>
              <div className="flex justify-end">
                <Button
                  type="button"
                  size="sm"
                  className="h-7 text-xs"
                  onClick={handleInsertGradient}
                >
                  Insert Gradient
                </Button>
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-1 pt-1 border-t border-border/50">
              <span className="text-[10px] text-muted-foreground mr-1">Presets:</span>
              {GRADIENT_PRESETS.map((preset) => (
                <button
                  key={preset.name}
                  type="button"
                  onClick={() => {
                    setGradientFrom(preset.from);
                    setGradientTo(preset.to);
                  }}
                  className="px-2 py-0.5 rounded text-[10px] font-medium border border-border/70 hover:border-primary/50 transition-colors"
                  style={{
                    background: `linear-gradient(90deg, ${preset.from}22, ${preset.to}22)`,
                  }}
                >
                  {preset.name}
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Palette: 16 Minecraft Colors */}
        <div className="flex flex-wrap items-center gap-1">
          <span className="text-[11px] font-medium text-muted-foreground mr-1 flex items-center gap-1">
            <Palette className="h-3 w-3" />
            Colors:
          </span>
          {MINECRAFT_COLORS.map((col) => (
            <button
              key={col.code}
              type="button"
              onClick={() => applyColor(col.code)}
              title={`${col.name} (§${col.code})`}
              className="group relative h-6 w-6 rounded border border-zinc-700/80 shadow-xs transition-transform hover:scale-110 focus:outline-none focus:ring-2 focus:ring-ring"
              style={{ backgroundColor: col.hex }}
            >
              <span className="sr-only">{col.name}</span>
            </button>
          ))}

          {/* Custom Hex Color Picker */}
          <div className="flex items-center gap-1 pl-1 ml-1 border-l border-border">
            <input
              type="color"
              value={customHex}
              onChange={(e) => setCustomHex(e.target.value)}
              className="h-6 w-6 cursor-pointer rounded border border-input p-0"
              title="Custom Hex Color"
            />
            <button
              type="button"
              onClick={() => applyHex(customHex)}
              className="px-1.5 py-0.5 rounded border border-input bg-muted text-[10px] font-mono hover:bg-accent"
              title="Insert custom hex color"
            >
              Hex
            </button>
          </div>
        </div>

        {/* Formatting Styles: Bold, Italic, Underline, Strikethrough, Magic, Reset */}
        <div className="flex flex-wrap items-center gap-1 border-t border-border pt-2">
          <span className="text-[11px] font-medium text-muted-foreground mr-1">Styles:</span>
          {MINECRAFT_STYLES.map((st) => (
            <button
              key={st.code}
              type="button"
              onClick={() => applyStyle(st.code)}
              title={`${st.name} (§${st.code})`}
              className="h-6 min-w-6 px-1.5 rounded border border-input bg-muted/60 text-xs font-mono font-semibold hover:bg-accent hover:text-accent-foreground transition-colors"
            >
              {st.tag}
            </button>
          ))}

          {/* Symbol Presets */}
          <div className="flex flex-wrap items-center gap-1 pl-2 ml-1 border-l border-border">
            <span className="text-[11px] font-medium text-muted-foreground mr-0.5">Icons:</span>
            {SYMBOL_PRESETS.map((sym) => (
              <button
                key={sym}
                type="button"
                onClick={() => insertTextAtActive(sym)}
                className="h-6 w-6 rounded border border-input/60 bg-muted/30 text-xs hover:bg-primary/20 hover:border-primary/40 transition-colors"
                title={`Insert ${sym}`}
              >
                {sym}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* 3. Text inputs for Line 1 & Line 2 */}
      <div className="space-y-3">
        <div className="space-y-1">
          <div className="flex items-center justify-between text-xs">
            <label
              htmlFor="motd-line-1"
              className={`font-medium ${activeLine === 1 ? 'text-primary font-semibold' : 'text-muted-foreground'}`}
            >
              Line 1
            </label>
            <span
              className={`text-[11px] ${
                visibleCount1 > 50 ? 'text-amber-500 font-medium' : 'text-muted-foreground'
              }`}
            >
              {visibleCount1}/50 chars {visibleCount1 > 50 && '(may wrap on some clients)'}
            </span>
          </div>
          <Input
            id="motd-line-1"
            ref={line1Ref}
            value={line1}
            onFocus={() => setActiveLine(1)}
            onChange={(e) => notifyChange(e.target.value, line2)}
            placeholder="e.g. §aMega SMP Server §7| §ePvP & Survival"
            className="font-mono text-xs"
          />
        </div>

        <div className="space-y-1">
          <div className="flex items-center justify-between text-xs">
            <label
              htmlFor="motd-line-2"
              className={`font-medium ${activeLine === 2 ? 'text-primary font-semibold' : 'text-muted-foreground'}`}
            >
              Line 2
            </label>
            <span
              className={`text-[11px] ${
                visibleCount2 > 50 ? 'text-amber-500 font-medium' : 'text-muted-foreground'
              }`}
            >
              {visibleCount2}/50 chars {visibleCount2 > 50 && '(may wrap on some clients)'}
            </span>
          </div>
          <Input
            id="motd-line-2"
            ref={line2Ref}
            value={line2}
            onFocus={() => setActiveLine(2)}
            onChange={(e) => notifyChange(line1, e.target.value)}
            placeholder="e.g. §bJoin now: §fplay.example.com"
            className="font-mono text-xs"
          />
        </div>
      </div>
    </div>
  );
}
