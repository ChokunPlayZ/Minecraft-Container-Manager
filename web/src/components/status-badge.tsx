import { Loader2 } from 'lucide-react';
import { cn } from '../lib/utils';
import type { ServerState } from '../api/types';
import { Badge } from './ui/badge';

const stateStyle: Record<ServerState, string> = {
  stopped: 'bg-muted text-muted-foreground',
  starting: 'bg-amber-100 text-amber-800 border-amber-300',
  running: 'bg-emerald-100 text-emerald-800 border-emerald-300',
  stopping: 'bg-amber-100 text-amber-800 border-amber-300',
  error: 'bg-red-100 text-red-800 border-red-300',
  installing: 'bg-blue-100 text-blue-800 border-blue-300',
  building: 'bg-indigo-100 text-indigo-800 border-indigo-300',
};

export function StatusBadge({ state, className }: { state: ServerState; className?: string }) {
  const isLoading = state === 'installing' || state === 'building' || state === 'starting' || state === 'stopping';
  return (
    <Badge className={cn(stateStyle[state] ?? stateStyle.stopped, 'inline-flex items-center', className)}>
      {isLoading && <Loader2 className="h-3 w-3 animate-spin mr-1 shrink-0" data-testid="status-loader" />}
      {state}
    </Badge>
  );
}
