import { describe, expect, it } from 'vitest';
import {
  centerMotdLine,
  decodeMotdFromProperties,
  encodeMotdForProperties,
  generateGradient,
  parseMotdToLines,
  stripFormatting,
} from '../src/lib/motd';

describe('motd utilities', () => {
  it('strips standard section codes and ampersand codes', () => {
    const formatted = '§aHello §lWorld §c! &6Minecraft';
    expect(stripFormatting(formatted)).toBe('Hello World ! Minecraft');
  });

  it('strips hex codes', () => {
    const hexMotd = '&#ff00ffGradient &#00ff00Text';
    expect(stripFormatting(hexMotd)).toBe('Gradient Text');
  });

  it('parses formatted spans with colors and styles', () => {
    const raw = '§aGreen §lBold §rReset';
    const lines = parseMotdToLines(raw);

    expect(lines.length).toBe(1);
    const spans = lines[0].spans;
    expect(spans.length).toBeGreaterThan(0);

    // First span has green color (#55FF55)
    expect(spans[0].color).toBe('#55FF55');
    expect(spans[0].text).toBe('Green ');

    // Second span has bold
    expect(spans[1].bold).toBe(true);
    expect(spans[1].text).toBe('Bold ');

    // Third span reset
    expect(spans[2].bold).toBe(false);
    expect(spans[2].color).toBeUndefined();
    expect(spans[2].text).toBe('Reset');
  });

  it('parses multi-line MOTD with max 2 lines', () => {
    const multi = 'Line 1\\nLine 2\\nLine 3 Extra';
    const lines = parseMotdToLines(multi);

    expect(lines.length).toBe(2);
    expect(lines[0].spans[0].text).toBe('Line 1');
    expect(lines[1].spans[0].text).toBe('Line 2');
  });

  it('handles unicode escape sequences \\u00A7', () => {
    const escaped = '\\u00A76Gold \\u00A7bAqua';
    const lines = parseMotdToLines(escaped);

    expect(lines[0].spans[0].color).toBe('#FFAA00');
    expect(lines[0].spans[1].color).toBe('#55FFFF');
  });

  it('encodes and decodes multi-line properties strings', () => {
    const encoded = encodeMotdForProperties('Top Line', 'Bottom Line');
    expect(encoded).toBe('Top Line\\nBottom Line');

    const decoded = decodeMotdFromProperties(encoded);
    expect(decoded.line1).toBe('Top Line');
    expect(decoded.line2).toBe('Bottom Line');
  });

  it('centers MOTD lines based on visible characters', () => {
    const text = '§aShort Text';
    const centered = centerMotdLine(text, 20);

    expect(centered.startsWith('     ')).toBe(true);
    expect(stripFormatting(centered).trim()).toBe('Short Text');
  });

  it('generates gradient with interpolated hex codes', () => {
    const grad = generateGradient('TEST', '#ff0000', '#0000ff');
    expect(grad).toContain('&#');
    expect(stripFormatting(grad)).toBe('TEST');
  });
});
