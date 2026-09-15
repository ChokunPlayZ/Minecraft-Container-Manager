import { useEffect, useState } from 'react';

/**
 * Formats a duration in seconds into a clean, human-readable uptime string.
 * Examples:
 *   45 -> "45s"
 *   75 -> "1m 15s"
 *   3665 -> "1h 1m 5s"
 *   90061 -> "1d 1h 1m"
 */
export function formatUptime(seconds: number): string {
  if (!seconds || seconds <= 0) return '0s';
  const total = Math.floor(seconds);

  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;

  if (days > 0) {
    return `${days}d ${hours}h ${minutes}m`;
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m ${secs}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${secs}s`;
  }
  return `${secs}s`;
}

/**
 * Computes elapsed uptime seconds from an ISO/RFC3339 startedAt timestamp string.
 */
export function getUptimeSeconds(
  startedAt: string | null | undefined,
  fallbackSeconds?: number,
): number | null {
  if (startedAt) {
    const startMs = new Date(startedAt).getTime();
    if (!isNaN(startMs)) {
      return Math.max(0, Math.floor((Date.now() - startMs) / 1000));
    }
  }
  if (typeof fallbackSeconds === 'number' && fallbackSeconds >= 0) {
    return fallbackSeconds;
  }
  return null;
}

/**
 * Formats uptime from a startedAt timestamp string.
 */
export function formatUptimeFromStartedAt(
  startedAt: string | null | undefined,
  fallbackSeconds?: number,
): string | null {
  const secs = getUptimeSeconds(startedAt, fallbackSeconds);
  if (secs === null) return null;
  return formatUptime(secs);
}

/**
 * React hook that returns a live, ticking formatted uptime string that updates
 * every second while the server is running.
 */
export function useLiveUptime(
  startedAt: string | null | undefined,
  fallbackSeconds?: number,
  isRunning: boolean = true,
): string | null {
  const [, setTick] = useState(0);

  useEffect(() => {
    if (!isRunning || (!startedAt && fallbackSeconds == null)) {
      return;
    }

    const timer = setInterval(() => {
      setTick((prev) => prev + 1);
    }, 1000);

    return () => clearInterval(timer);
  }, [isRunning, startedAt, fallbackSeconds]);

  if (!isRunning) return null;
  return formatUptimeFromStartedAt(startedAt, fallbackSeconds);
}
