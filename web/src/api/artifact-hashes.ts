// Keep bounded, short-lived checksums declared by catalog metadata. Download
// requests carry them to the server for validation before publication.
const artifacts = new Map<string, { hashes: Record<string, string>; expires: number }>();
const digestLengths: Record<string, number> = { md5: 32, sha1: 40, sha256: 64, sha512: 128 };

export function rememberArtifactHashes(
  url: string | undefined,
  hashes: Record<string, string | undefined> | undefined,
): void {
  if (!url || url.length > 8192 || !hashes) return;
  const verified: Record<string, string> = {};
  for (const [algorithm, value] of Object.entries(hashes)) {
    if (value && digestLengths[algorithm] === value.length && /^[a-f0-9]+$/i.test(value)) {
      verified[algorithm] = value;
    }
  }
  if (Object.keys(verified).length === 0) return;
  prune();
  artifacts.delete(url);
  artifacts.set(url, { hashes: verified, expires: Date.now() + 300000 });
  while (artifacts.size > 200) {
    const oldest = artifacts.keys().next().value;
    if (oldest !== undefined) artifacts.delete(oldest);
  }
}

function prune(): void {
  for (const [url, entry] of artifacts) {
    if (entry.expires <= Date.now()) artifacts.delete(url);
  }
}

export function getArtifactHashes(url: string): Record<string, string> | undefined {
  prune();
  const hashes = artifacts.get(url)?.hashes;
  return hashes ? { ...hashes } : undefined;
}

export function clearArtifactHashes(): void {
  artifacts.clear();
}
