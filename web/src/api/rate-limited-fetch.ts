import { clearArtifactHashes } from './artifact-hashes';
import { authenticatedFetch } from './csrf';
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

export function resolveFetchUrl(targetUrl: string, skipCache = false): string {
  if (targetUrl.startsWith('/') || targetUrl.startsWith('./')) {
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
 clearArtifactHashes();
  responseCache.clear();
  inFlightRequests.clear();
  for(const [key,state] of domainStates){if(state.activeCount===0&&state.queue.length===0)domainStates.delete(key);}
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
  const headers=Array.from(new Headers(init?.headers).entries()).sort(([a],[b])=>a.localeCompare(b));
  return JSON.stringify([method,url,body,headers]);
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

  return new Promise<T>((resolve,reject)=>{
    const runTask=()=>{
      // Reserve before any pacing, backoff or cancellation await.
      state.activeCount++;
      const dispatchAt=Math.max(Date.now(),state.lastDispatchedAt+config.minIntervalMs,state.blockedUntil);
      state.lastDispatchedAt=dispatchAt;
      void (async()=>{
        try{
          while(true){
            if(signal?.aborted)throw new DOMException('The operation was aborted','AbortError');
            const delay=Math.max(dispatchAt,state.blockedUntil)-Date.now();
            if(delay<=0)break;
            await new Promise<void>((done,fail)=>{
              const abort=()=>{clearTimeout(timer);fail(new DOMException('The operation was aborted','AbortError'));};
              const timer=setTimeout(()=>{signal?.removeEventListener('abort',abort);done();},delay);
              signal?.addEventListener('abort',abort,{once:true});
            });
          }
          resolve(await execute());
        }catch(error){reject(error);}
        finally{state.activeCount--;state.queue.shift()?.();}
      })();
    };
    if(state.activeCount<config.maxConcurrent)runTask();else state.queue.push(runTask);
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
  pruneResponseCache();
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
      const resp = effectiveUrl.startsWith('/api/')
        ? await authenticatedFetch(effectiveUrl,{...init,signal:options?.signal??init?.signal})
        : await fetch(effectiveUrl,{...init,signal:options?.signal??init?.signal});

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
        pruneResponseCache();
        const bytes=JSON.stringify(data).length*2;
        if(bytes<=MAX_CACHE_BYTES) responseCache.set(cacheKey, {
          data,
          timestamp: Date.now(),
          ttl: cacheTtlMs,
        });
      }

      pruneResponseCache();
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

const MAX_CACHE_BYTES=16*1024*1024;
function pruneResponseCache():void{
 let bytes=0;
 for(const [key,entry] of responseCache){
  if(Date.now()-entry.timestamp>=entry.ttl){responseCache.delete(key);continue;}
  bytes+=key.length*2+JSON.stringify(entry.data).length*2;
 }
 while(responseCache.size>200||bytes>MAX_CACHE_BYTES){
  const key=responseCache.keys().next().value;if(key===undefined)break;
  const entry=responseCache.get(key);bytes-=key.length*2+JSON.stringify(entry?.data).length*2;responseCache.delete(key);
 }
}
