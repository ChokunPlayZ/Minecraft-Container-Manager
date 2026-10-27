import { ArrowUpCircle } from 'lucide-react';
import type { ServerUpdateCheck } from '../api/types';
import { Badge } from './ui/badge';
import { hasAvailableUpdate } from '../lib/use-update-checker';

export function UpdateAvailableBadge({ result }: { result?: ServerUpdateCheck }) {
  if (!hasAvailableUpdate(result)) return null;
  const parts = [result?.software?.available ? `Software: ${result.software.latest_version}${result.software.latest_build ? ` build ${result.software.latest_build}` : ''}` : '', Object.keys(result?.mods?.updates ?? {}).length ? `${Object.keys(result?.mods?.updates ?? {}).length} plugin/mod updates` : ''].filter(Boolean);
  return <Badge title={parts.join(' · ')}><ArrowUpCircle className="h-3 w-3" aria-hidden="true" />Update Available</Badge>;
}
