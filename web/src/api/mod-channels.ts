export const RELEASE_CHANNEL = 'release';
export const UNCLASSIFIED_CHANNEL = 'unclassified';

const stability: Record<string, number> = { alpha: 1, beta: 2, release: 3 };

export function normalizeModChannel(channel?: string): string {
  const normalized = channel?.trim().toLowerCase();
  return normalized === 'stable' ? RELEASE_CHANNEL : normalized || UNCLASSIFIED_CHANNEL;
}

export function modChannelLabel(channel?: string): string {
  const value = normalizeModChannel(channel);
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export function curseForgeChannel(releaseType: number): string {
  return ['', RELEASE_CHANNEL, 'beta', 'alpha'][releaseType] || UNCLASSIFIED_CHANNEL;
}

export function spigetChannel(versionName?: string): string {
  const named = versionName?.match(/(?:^|[^a-z])(alpha|beta|release|stable)(?:$|[^a-z])/i);
  return normalizeModChannel(named?.[1]);
}

export function canSwitchModChannel(current: string, next: string): boolean {
  return stability[current] > 0 && stability[next] > stability[current];
}

export function matchesInstalledArtifact(mod: { file: string; sha1?: string }, filename: string, sha1?: string): boolean {
  if (mod.sha1 && sha1) return mod.sha1.toLowerCase() === sha1.toLowerCase();
  return mod.file.replace(/\.disabled$/, '').toLowerCase() === filename.toLowerCase();
}
