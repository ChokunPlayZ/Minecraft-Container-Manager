import { Loader2 } from 'lucide-react';
import { cn } from '../lib/utils';
import type { ServerState } from '../api/types';
import { Badge } from './ui/badge';

const stateStyle: Record<ServerState, string> = {
  stopped: 'border-transparent bg-muted text-muted-foreground',
  starting: 'border-transparent bg-amber-500/10 text-amber-800 dark:text-amber-300',
  running: 'border-transparent bg-emerald-500/10 text-emerald-800 dark:text-emerald-300',
  stopping: 'border-transparent bg-amber-500/10 text-amber-800 dark:text-amber-300',
  error: 'border-transparent bg-destructive/10 text-destructive',
  installing: 'border-transparent bg-sky-500/10 text-sky-800 dark:text-sky-300',
  building: 'border-transparent bg-sky-500/10 text-sky-800 dark:text-sky-300',
};

export function StatusBadge({ state, className }: { state: ServerState; className?: string }) {
  const pending = ['installing', 'building', 'starting', 'stopping'].includes(state);
  return (
    <Badge className={cn(stateStyle[state] ?? stateStyle.stopped, 'inline-flex shrink-0 items-center capitalize', className)}>
      {pending ? <Loader2 className="h-3 w-3 animate-spin" data-testid="status-loader" /> : <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />}
      {state}
    </Badge>
  );
}
