import { cn } from '../lib/utils';
import type { ServerState } from '../api/types';
import { Badge } from './ui/badge';

const stateStyle: Record<ServerState, string> = {
  stopped: 'bg-muted text-muted-foreground',
  starting: 'bg-amber-100 text-amber-800 border-amber-300',
  running: 'bg-emerald-100 text-emerald-800 border-emerald-300',
  stopping: 'bg-amber-100 text-amber-800 border-amber-300',
  error: 'bg-red-100 text-red-800 border-red-300',
};

export function StatusBadge({ state, className }: { state: ServerState; className?: string }) {
  return <Badge className={cn(stateStyle[state] ?? stateStyle.stopped, className)}>{state}</Badge>;
}
