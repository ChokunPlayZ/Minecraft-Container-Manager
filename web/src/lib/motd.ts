export interface MinecraftColor {
  code: string;
  name: string;
  hex: string;
}

export const MINECRAFT_COLORS: MinecraftColor[] = [
  { code: '0', name: 'Black', hex: '#000000' },
  { code: '1', name: 'Dark Blue', hex: '#0000AA' },
  { code: '2', name: 'Dark Green', hex: '#00AA00' },
  { code: '3', name: 'Dark Aqua', hex: '#00AAAA' },
  { code: '4', name: 'Dark Red', hex: '#AA0000' },
  { code: '5', name: 'Dark Purple', hex: '#AA00AA' },
  { code: '6', name: 'Gold', hex: '#FFAA00' },
  { code: '7', name: 'Gray', hex: '#AAAAAA' },
  { code: '8', name: 'Dark Gray', hex: '#555555' },
  { code: '9', name: 'Blue', hex: '#5555FF' },
  { code: 'a', name: 'Green', hex: '#55FF55' },
  { code: 'b', name: 'Aqua', hex: '#55FFFF' },
  { code: 'c', name: 'Red', hex: '#FF5555' },
  { code: 'd', name: 'Light Purple', hex: '#FF55FF' },
  { code: 'e', name: 'Yellow', hex: '#FFFF55' },
  { code: 'f', name: 'White', hex: '#FFFFFF' },
];

export const MINECRAFT_STYLES = [
  { code: 'l', name: 'Bold', tag: 'B' },
  { code: 'o', name: 'Italic', tag: 'I' },
  { code: 'n', name: 'Underline', tag: 'U' },
  { code: 'm', name: 'Strikethrough', tag: 'S' },
  { code: 'k', name: 'Obfuscated / Magic', tag: '✦' },
  { code: 'r', name: 'Reset', tag: '↺' },
];

export const COLOR_CODE_MAP = new Map<string, string>(
  MINECRAFT_COLORS.map((c) => [c.code.toLowerCase(), c.hex]),
);

export interface MotdSpan {
  text: string;
  color?: string;
  bold?: boolean;
  italic?: boolean;
  underline?: boolean;
  strikethrough?: boolean;
  obfuscated?: boolean;
}

export interface MotdLine {
  spans: MotdSpan[];
}

/**
 * Normalizes unicode escape sequences (\u00A7) and ensures section symbols are clean.
 */
export function normalizeMotdInput(raw: string): string {
  if (!raw) return '';
  return raw
    .replace(/\\u00A7/gi, '§')
    .replace(/\\n/g, '\n');
}

/**
 * Strips all Minecraft formatting and color codes (§, &, hex).
 */
export function stripFormatting(text: string): string {
  if (!text) return '';
  return text
    // Replace hex patterns like &#RRGGBB or {#RRGGBB}
    .replace(/(&|§)#[0-9a-fA-F]{6}/g, '')
    .replace(/\{#[0-9a-fA-F]{6}\}/g, '')
    // Replace §x§r§r§g§g§b§b hex pattern
    .replace(/§x(§[0-9a-fA-F]){6}/gi, '')
    // Replace standard § / & color and format codes
    .replace(/[§&][0-9a-fk-orA-FK-OR]/g, '');
}

/**
 * Parses an MOTD string into structured lines and spans for rendering.
 */
export function parseMotdToLines(raw: string): MotdLine[] {
  const normalized = normalizeMotdInput(raw);
  const rawLines = normalized.split('\n');

  return rawLines.slice(0, 2).map((line) => {
    const spans: MotdSpan[] = [];
    let currentColor: string | undefined = undefined;
    let bold = false;
    let italic = false;
    let underline = false;
    let strikethrough = false;
    let obfuscated = false;

    let buffer = '';

    const pushBuffer = () => {
      if (buffer.length > 0) {
        spans.push({
          text: buffer,
          color: currentColor,
          bold,
          italic,
          underline,
          strikethrough,
          obfuscated,
        });
        buffer = '';
      }
    };

    let i = 0;
    while (i < line.length) {
      // Check for hex color: &#RRGGBB or §#RRGGBB
      if (
        (line[i] === '§' || line[i] === '&') &&
        line[i + 1] === '#' &&
        i + 7 < line.length &&
        /^[0-9a-fA-F]{6}$/.test(line.slice(i + 2, i + 8))
      ) {
        pushBuffer();
        currentColor = '#' + line.slice(i + 2, i + 8);
        i += 8;
        continue;
      }

      // Check for Spigot hex: §x§r§r§g§g§b§b
      if (
        line[i] === '§' &&
        (line[i + 1] === 'x' || line[i + 1] === 'X') &&
        i + 13 < line.length
      ) {
        const hexSub = line.slice(i, i + 14);
        const match = /^§x§([0-9a-fA-F])§([0-9a-fA-F])§([0-9a-fA-F])§([0-9a-fA-F])§([0-9a-fA-F])§([0-9a-fA-F])$/i.exec(hexSub);
        if (match) {
          pushBuffer();
          currentColor = `#${match[1]}${match[2]}${match[3]}${match[4]}${match[5]}${match[6]}`;
          i += 14;
          continue;
        }
      }

      // Check for standard color/format code: §[0-9a-fk-or] or &[0-9a-fk-or]
      if ((line[i] === '§' || line[i] === '&') && i + 1 < line.length) {
        const code = line[i + 1].toLowerCase();
        if (COLOR_CODE_MAP.has(code)) {
          pushBuffer();
          currentColor = COLOR_CODE_MAP.get(code);
          // Setting a color in Minecraft resets formatting styles
          bold = false;
          italic = false;
          underline = false;
          strikethrough = false;
          obfuscated = false;
          i += 2;
          continue;
        }

        switch (code) {
          case 'l':
            pushBuffer();
            bold = true;
            i += 2;
            continue;
          case 'o':
            pushBuffer();
            italic = true;
            i += 2;
            continue;
          case 'n':
            pushBuffer();
            underline = true;
            i += 2;
            continue;
          case 'm':
            pushBuffer();
            strikethrough = true;
            i += 2;
            continue;
          case 'k':
            pushBuffer();
            obfuscated = true;
            i += 2;
            continue;
          case 'r':
            pushBuffer();
            currentColor = undefined;
            bold = false;
            italic = false;
            underline = false;
            strikethrough = false;
            obfuscated = false;
            i += 2;
            continue;
        }
      }

      buffer += line[i];
      i += 1;
    }

    pushBuffer();

    return { spans: spans.length > 0 ? spans : [{ text: '' }] };
  });
}

/**
 * Decodes MOTD from server.properties format into two editable lines.
 */
export function decodeMotdFromProperties(rawMotd: string): { line1: string; line2: string } {
  const normalized = normalizeMotdInput(rawMotd);
  const parts = normalized.split('\n');
  return {
    line1: parts[0] ?? '',
    line2: parts[1] ?? '',
  };
}

/**
 * Encodes two lines into a single properties-safe string.
 */
export function encodeMotdForProperties(line1: string, line2: string): string {
  const l1 = line1.replace(/\r?\n/g, '');
  const l2 = line2.replace(/\r?\n/g, '');
  if (!l2) return l1;
  return `${l1}\\n${l2}`;
}

/**
 * Centers an MOTD line by prepending space padding based on visible character count.
 */
export function centerMotdLine(text: string, targetLength = 46): string {
  const visible = stripFormatting(text);
  if (visible.length >= targetLength) return text;
  const padding = Math.floor((targetLength - visible.length) / 2);
  return ' '.repeat(Math.max(0, padding)) + text;
}

// Helpers for color interpolation in gradients
function hexToRgb(hex: string): [number, number, number] {
  const clean = hex.replace('#', '');
  const r = parseInt(clean.slice(0, 2), 16) || 0;
  const g = parseInt(clean.slice(2, 4), 16) || 0;
  const b = parseInt(clean.slice(4, 6), 16) || 0;
  return [r, g, b];
}

function rgbToHex(r: number, g: number, b: number): string {
  return `#${Math.round(r).toString(16).padStart(2, '0')}${Math.round(g).toString(16).padStart(2, '0')}${Math.round(b).toString(16).padStart(2, '0')}`;
}

/**
 * Generates a gradient string across text from startHex to endHex.
 */
export function generateGradient(text: string, startHex: string, endHex: string): string {
  const clean = stripFormatting(text);
  if (clean.length === 0) return '';
  if (clean.length === 1) return `&#${startHex.replace('#', '')}${clean}`;

  const [r1, g1, b1] = hexToRgb(startHex);
  const [r2, g2, b2] = hexToRgb(endHex);

  let result = '';
  for (let i = 0; i < clean.length; i++) {
    const char = clean[i];
    if (char === ' ') {
      result += ' ';
      continue;
    }
    const factor = i / (clean.length - 1);
    const r = r1 + factor * (r2 - r1);
    const g = g1 + factor * (g2 - g1);
    const b = b1 + factor * (b2 - b1);
    const hex = rgbToHex(r, g, b).replace('#', '');
    result += `&#${hex}${char}`;
  }

  return result;
}
