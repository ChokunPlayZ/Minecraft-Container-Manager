export type ThemeMode = 'light' | 'dark' | 'system';

const THEME_COOKIE = 'mcm_theme';
const STORAGE_KEY = 'mcm-theme';

export function getSystemTheme(): 'light' | 'dark' {
  if (typeof window === 'undefined') return 'light';
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function normalizeColorClass(v: string | null | undefined): 'light' | 'dark' {
  return v === 'dark' ? 'dark' : 'light';
}

/** Reads the effective color class from cookie, storage, or system preference. */
export function resolveTheme(): 'light' | 'dark' {
  if (typeof document === 'undefined') return 'light';
  const cookie = document.cookie
    .split(';')
    .map((c) => c.trim())
    .find((c) => c.startsWith(`${THEME_COOKIE}=`));
  if (cookie) return normalizeColorClass(cookie.split('=')[1]);

  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored) return normalizeColorClass(stored);
  } catch {
    /* ignore storage errors */
  }
  return getSystemTheme();
}

export function setClass(theme: ThemeMode): void {
  if (typeof document === 'undefined') return;
  const resolved = theme === 'system' ? getSystemTheme() : theme;
  document.documentElement.classList.toggle('dark', resolved === 'dark');
}

export function setCookie(theme: ThemeMode): void {
  if (typeof document === 'undefined') return;
  // 10 year expiry, not httpOnly so JS can read it back for the inline script.
  document.cookie = `${THEME_COOKIE}=${theme}; path=/; max-age=${60 * 60 * 24 * 365 * 10}; SameSite=Lax`;
}

export function getStoredMode(): ThemeMode | null {
  if (typeof window === 'undefined') return null;
  try {
    const s = window.localStorage.getItem(STORAGE_KEY);
    return s === 'light' || s === 'dark' || s === 'system' ? s : null;
  } catch {
    return null;
  }
}

export function persistMode(theme: ThemeMode): void {
  setCookie(theme);
  try {
    window.localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    /* ignore storage errors */
  }
}
