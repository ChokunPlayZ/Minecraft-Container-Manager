export const safeMethods = new Set(['GET', 'HEAD', 'OPTIONS']);

let csrfToken: string | null = null;
let csrfPromise: Promise<string> | null = null;

function getCsrfFromCookie(): string | null {
  if (typeof document === 'undefined') return null;
  const match = document.cookie.match(/(?:^|;\s*)mcm_csrf=([^;]+)/);
  return match ? decodeURIComponent(match[1]) : null;
}

export async function ensureCsrf(forceRefresh = false): Promise<string> {
  if (!forceRefresh) {
    const cookieToken = getCsrfFromCookie();
    if (cookieToken) {
      csrfToken = cookieToken;
      return cookieToken;
    }
    if (csrfToken) return csrfToken;
  }
  if (!csrfPromise || forceRefresh) {
    csrfPromise = fetch('/api/auth/csrf', { credentials: 'include' })
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error('csrf unavailable'))))
      .then((body) => {
        csrfToken = String(body?.csrf_token ?? getCsrfFromCookie() ?? '');
        return csrfToken;
      })
      .finally(() => {
        csrfPromise = null;
      });
  }
  return csrfPromise;
}


export async function authenticatedFetch(url: string, init: RequestInit = {}): Promise<Response> {
  const method = (init.method ?? 'GET').toUpperCase();
  const headers = new Headers(init.headers);
  if (!safeMethods.has(method)) headers.set('X-CSRF-Token', await ensureCsrf());
  let response = await fetch(url, { ...init, credentials: 'include', headers });
  if (response.status === 403 && !safeMethods.has(method)) {
    const body = await response.clone().json().catch(() => null);
    if (
      String(body?.error?.code ?? '').startsWith('csrf') ||
      String(body?.error?.message ?? '').toLowerCase().includes('csrf')
    ) {
      headers.set('X-CSRF-Token', await ensureCsrf(true));
      response = await fetch(url, { ...init, credentials: 'include', headers });
    }
  }
  return response;
}
