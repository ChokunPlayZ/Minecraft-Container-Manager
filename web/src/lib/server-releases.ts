import type { VersionInfo, VersionMeta } from '../api/types';

function isPrerelease(version: string): boolean {
  return /^\d{2}w\d{2}[a-z]$/i.test(version) ||
    /(?:^|[\d._\s-])(?:alpha|beta|pre(?:view|release)?|rc|snapshot|experimental|dev)(?:$|[\d._\s-])/i.test(version);
}

// Keep every release selectable, with the newest stable release first.
function compareReleases(a: string, b: string): number {
  const channelOrder = Number(isPrerelease(a)) - Number(isPrerelease(b));
  if (channelOrder) return channelOrder;
  if (a === 'latest' || b === 'latest') return Number(b === 'latest') - Number(a === 'latest');
  return b.localeCompare(a, undefined, { numeric: true, sensitivity: 'base' });
}

export function sortServerVersions(versions: VersionMeta[]): VersionMeta[] {
  return [...versions].sort((a, b) => compareReleases(a.name, b.name));
}

export function sortServerBuilds(builds: VersionInfo[]): VersionInfo[] {
  return [...builds].sort((a, b) => compareReleases(a.build, b.build));
}
