/**
 * Rate-limited and cached fetch utility for external mod APIs
 * (Modrinth, Hangar, SpigotMC / Spiget, CurseForge).
 *
 * Prevents HTTP 429 Too Many Requests by:
 * 1. Enforcing per-domain concurrency limits and minimum intervals between dispatches.
 * 2. Deduplicating identical in-flight requests.
 * 3. In-memory response caching with configurable TTLs.
 * 4. Circuit breaker & backoff when HTTP 429 (Too Many Requests) is returned.
 */

interface CacheEntry<T> {
  data: T;
  timestamp: number;
  ttl: number;
}

interface DomainQueueConfig {
  maxConcurrent: number;
  minIntervalMs: number;
}

const DOMAIN_CONFIGS: Record<string, DomainQueueConfig> = {
  'api.spiget.org': { maxConcurrent: 1, minIntervalMs: 300 }, // Spiget has very aggressive rate limits
  'api.modrinth.com': { maxConcurrent: 2, minIntervalMs: 120 },
  'hangar.papermc.io': { maxConcurrent: 2, minIntervalMs: 120 },
  'api.curseforge.com': { maxConcurrent: 2, minIntervalMs: 150 },
};

const DEFAULT_CONFIG: DomainQueueConfig = {
  maxConcurrent: 2,
  minIntervalMs: 150,
};

interface DomainState {
  activeCount: number;
  lastDispatchedAt: number;
  blockedUntil: number; // Backoff timestamp if rate limited (429)
  rateLimitRemaining: number | null;
  queue: Array<() => void>;
}

const domainStates = new Map<string, DomainState>();
const responseCache = new Map<string, CacheEntry<unknown>>();
const inFlightRequests = new Map<string, Promise<unknown>>();

function getDomainState(hostname: string): DomainState {
  let state = domainStates.get(hostname);
  if (!state) {
    state = {
      activeCount: 0,
      lastDispatchedAt: 0,
      blockedUntil: 0,
      rateLimitRemaining: null,
      queue: [],
    };
    domainStates.set(hostname, state);
  }
  return state;
}

const isTestEnv =
  (typeof process !== 'undefined' && process.env?.NODE_ENV === 'test') ||
  (typeof window !== 'undefined' && Boolean((window as unknown as { __VITEST__?: boolean }).__VITEST__));

export function resolveFetchUrl(targetUrl: string, skipCache = false): string {
  if (targetUrl.startsWith('/') || targetUrl.startsWith('./')) {
    return targetUrl;
  }
  if (isTestEnv) {
    return targetUrl;
  }
  if (
    typeof window !== 'undefined' &&
    (window.location?.search?.includes('mock=true') ||
      window.localStorage?.getItem('mcm-mock') === 'true')
  ) {
    return targetUrl;
  }

  try {
    const u = new URL(targetUrl);
    const host = u.hostname.toLowerCase();
    if (
      host === 'api.modrinth.com' ||
      host === 'hangar.papermc.io' ||
      host === 'api.spiget.org' ||
      host === 'api.curseforge.com'
    ) {
      let p = `/api/proxy?url=${encodeURIComponent(targetUrl)}`;
      if (skipCache) {
        p += '&skip_cache=true';
      }
      return p;
    }
  } catch {
    // fallback
  }
  return targetUrl;
}

function getDomainConfig(hostname: string): DomainQueueConfig {
  if (isTestEnv) {
    return { maxConcurrent: 10, minIntervalMs: 0 };
  }
  return DOMAIN_CONFIGS[hostname] || DEFAULT_CONFIG;
}

/**
 * Checks whether a domain is currently in a 429 rate-limit backoff period.
 */
export function isDomainRateLimited(hostname: string): boolean {
  const state = domainStates.get(hostname);
  if (!state) return false;
  return Date.now() < state.blockedUntil;
}

/**
 * Gets remaining rate-limit backoff time in milliseconds for a domain, or 0 if not limited.
 */
export function getDomainBackoffRemainingMs(hostname: string): number {
  const state = domainStates.get(hostname);
  if (!state) return 0;
  return Math.max(0, state.blockedUntil - Date.now());
}

/**
 * Clears all cached responses and resets domain queues (useful in tests).
 */
export function clearRateLimitCache(): void {
  responseCache.clear();
  inFlightRequests.clear();
  domainStates.clear();
}

export interface RateLimitedFetchOptions {
  cacheTtlMs?: number;
  skipCache?: boolean;
  signal?: AbortSignal;
}

/**
 * Generates a stable cache key for a request.
 */
function getCacheKey(url: string, init?: RequestInit): string {
  const method = (init?.method || 'GET').toUpperCase();
  const body = init?.body ? String(init.body) : '';
  return `${method}:${url}:${body}`;
}

/**
 * Schedules a request execution respecting per-domain rate limits and concurrency limits.
 */
async function scheduleDomainRequest<T>(
  hostname: string,
  execute: () => Promise<T>,
  signal?: AbortSignal,
): Promise<T> {
  const state = getDomainState(hostname);
  const config = getDomainConfig(hostname);

  return new Promise<T>((resolve, reject) => {
    const runTask = async () => {
      if (signal?.aborted) {
        reject(new DOMException('The operation was aborted', 'AbortError'));
        return;
      }

      // Check if domain is currently blocked due to a 429 response
      const now = Date.now();
      if (state.blockedUntil > now) {
        const waitMs = state.blockedUntil - now;
        await new Promise((r) => setTimeout(r, Math.min(waitMs, 10000)));
      }

      // Ensure minimum interval between requests has passed
      const elapsed = Date.now() - state.lastDispatchedAt;
      if (elapsed < config.minIntervalMs) {
        await new Promise((r) => setTimeout(r, config.minIntervalMs - elapsed));
      }

      if (signal?.aborted) {
        reject(new DOMException('The operation was aborted', 'AbortError'));
        return;
      }

      state.activeCount++;
      state.lastDispatchedAt = Date.now();

      try {
        const result = await execute();
        resolve(result);
      } catch (err) {
        reject(err);
      } finally {
        state.activeCount--;
        // Process next item in domain queue
        const next = state.queue.shift();
        if (next) {
          next();
        }
      }
    };

    if (state.activeCount < config.maxConcurrent) {
      void runTask();
    } else {
      state.queue.push(() => void runTask());
    }
  });
}

/**
 * Executes a rate-limited, cached JSON HTTP request with 429 backoff handling.
 */
export async function rateLimitedFetchJson<T>(
  url: string,
  init?: RequestInit,
  options?: RateLimitedFetchOptions,
): Promise<T> {
  const cacheKey = getCacheKey(url, init);
  const cacheTtlMs = options?.cacheTtlMs ?? 180000; // default 3 minutes
  const skipCache = options?.skipCache ?? false;

  // 1. Check in-memory cache
  if (!skipCache) {
    const cached = responseCache.get(cacheKey) as CacheEntry<T> | undefined;
    if (cached && Date.now() - cached.timestamp < cached.ttl) {
      return cached.data;
    }
  }

  // 2. Check in-flight request deduplication
  if (inFlightRequests.has(cacheKey)) {
    return inFlightRequests.get(cacheKey) as Promise<T>;
  }

  // Parse hostname for domain queue
  let hostname = 'default';
  try {
    hostname = new URL(url).hostname;
  } catch {
    // Relative or invalid URL, fallback to default
  }

  const requestPromise = scheduleDomainRequest(
    hostname,
    async () => {
      const effectiveUrl = resolveFetchUrl(url, skipCache);
      let resp: Response;
      try {
        resp = await fetch(effectiveUrl, {
          ...init,
          credentials: 'include',
          signal: options?.signal,
        });
      } catch (fetchErr) {
        if (effectiveUrl !== url) {
          resp = await fetch(url, {
            ...init,
            signal: options?.signal,
          });
        } else {
          throw fetchErr;
        }
      }

      // Track rate limit headers
      const remainingHeader =
        resp.headers?.get('x-ratelimit-remaining') ||
        resp.headers?.get('x-ratelimit-remaining-minute');
      if (remainingHeader) {
        const remaining = parseInt(remainingHeader, 10);
        if (!isNaN(remaining)) {
          const state = getDomainState(hostname);
          state.rateLimitRemaining = remaining;
          // If 1 or fewer requests remaining, apply a short throttle pause
          if (remaining <= 1) {
            state.blockedUntil = Date.now() + 2000;
          }
        }
      }

      // Handle HTTP 429 Too Many Requests
      if (resp.status === 429) {
        const state = getDomainState(hostname);
        const retryAfterHeader = resp.headers?.get('retry-after');
        let backoffMs = 8000; // default 8s backoff
        if (retryAfterHeader) {
          const retrySec = parseInt(retryAfterHeader, 10);
          if (!isNaN(retrySec) && retrySec > 0) {
            backoffMs = Math.min(retrySec * 1000, 60000);
          }
        }
        state.blockedUntil = Date.now() + backoffMs;

        // If we have any cached data (even stale), return it on 429 to avoid crashing
        const stale = responseCache.get(cacheKey) as CacheEntry<T> | undefined;
        if (stale) {
          return stale.data;
        }

        throw new Error(
          `Rate limit exceeded for ${hostname} (HTTP 429). Please wait a moment before trying again.`,
        );
      }

      if (!resp.ok) {
        throw new Error(`Request failed with status ${resp.status}`);
      }

      const data = (await resp.json()) as T;

      // Store in cache if successful and TTL > 0
      if (cacheTtlMs > 0) {
        responseCache.set(cacheKey, {
          data,
          timestamp: Date.now(),
          ttl: cacheTtlMs,
        });
      }

      return data;
    },
    options?.signal,
  );

  inFlightRequests.set(cacheKey, requestPromise);

  try {
    return await requestPromise;
  } finally {
    inFlightRequests.delete(cacheKey);
  }
}
