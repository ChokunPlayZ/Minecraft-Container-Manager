/**
 * Formats a byte value into a clean, human-readable string (e.g. 1.45 GB, 250 MB, 12 KB, 0 B).
 */
export function formatBytes(bytes: number, decimals = 1): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  const clampedIndex = Math.min(i, sizes.length - 1);
  return `${parseFloat((bytes / Math.pow(k, clampedIndex)).toFixed(dm))} ${sizes[clampedIndex]}`;
}

/**
 * Formats a percentage number (e.g. 15.2 -> "15.2%").
 */
export function formatPercent(value: number, decimals = 1): string {
  if (!Number.isFinite(value) || value < 0) return '0%';
  return `${value.toFixed(decimals)}%`;
}
