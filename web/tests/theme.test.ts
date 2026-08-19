import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  getSystemTheme,
  normalizeColorClass,
  persistMode,
  resolveTheme,
  setClass,
  setCookie,
} from '../src/store/theme';

describe('theme store', () => {
  const originalCookie = document.cookie;

  beforeEach(() => {
    document.documentElement.classList.remove('dark');
    window.localStorage.clear();
  });

  afterEach(() => {
    document.cookie = originalCookie;
    document.documentElement.className = '';
  });

  it('resolves system preference to a concrete color', () => {
    const mq = {
      matches: true,
      media: '(prefers-color-scheme: dark)',
    };
    vi.spyOn(window, 'matchMedia').mockReturnValue(mq as MediaQueryList);
    expect(getSystemTheme()).toBe('dark');
  });

  it('normalizeColorClass maps dark and anything else', () => {
    expect(normalizeColorClass('dark')).toBe('dark');
    expect(normalizeColorClass('system')).toBe('light');
    expect(normalizeColorClass(null)).toBe('light');
  });

  it('setClass toggles the dark class on the document element', () => {
    setClass('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    setClass('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('setCookie persists the theme as a readable cookie', () => {
    setCookie('dark');
    expect(document.cookie).toContain('mcm_theme=dark');
  });

  it('persistMode writes both cookie and localStorage', () => {
    persistMode('system');
    expect(document.cookie).toContain('mcm_theme=system');
    expect(window.localStorage.getItem('mcm-theme')).toBe('system');
  });

  it('resolveTheme reads the cookie first when present', () => {
    window.localStorage.setItem('mcm-theme', 'light');
    setCookie('dark');
    expect(resolveTheme()).toBe('dark');
  });
});
