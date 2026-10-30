import { useRef, useState } from 'react';
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react';
import type { ScheduleStep } from '../api/types';
import { backupTargets, maxChainSteps, stepActions } from '../lib/schedules';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';

export function ScheduleChainEditor({ steps, onChange, disabled }: {
  steps: ScheduleStep[];
  onChange: (steps: ScheduleStep[]) => void;
  disabled: boolean;
}) {
  // Keep each step's controls mounted when it moves so keyboard focus follows it.
  const [keys, setKeys] = useState(() => steps.map((_, index) => index));
  const nextKey = useRef(steps.length);

  function update(index: number, step: ScheduleStep) {
    onChange(steps.map((current, position) => position === index ? step : current));
  }

  function move(index: number, direction: number) {
    const next = [...steps];
    const nextKeys = [...keys];
    [next[index], next[index + direction]] = [next[index + direction], next[index]];
    [nextKeys[index], nextKeys[index + direction]] = [nextKeys[index + direction], nextKeys[index]];
    setKeys(nextKeys);
    onChange(next);
  }

  function add() {
    setKeys([...keys, nextKey.current++]);
    onChange([...steps, { action: 'command', command: '' }]);
  }

  function remove(index: number) {
    setKeys(keys.filter((_, position) => position !== index));
    onChange(steps.filter((_, position) => position !== index));
  }

  return (
    <div className="min-w-0 space-y-3 sm:col-span-2">
      <div>
        <h4 className="text-sm font-medium">Actions in order</h4>
        <p id="schedule-chain-help" className="mt-1 max-w-prose text-xs leading-relaxed text-muted-foreground">Steps run from top to bottom. Backups finish before the next step starts. If a step fails, the chain stops and reports that step.</p>
      </div>
      <ol className="divide-y border-y" aria-label="Chain steps" aria-describedby="schedule-chain-help">
        {steps.map((step, index) => (
          <li key={keys[index]} className="min-w-0 space-y-3 py-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-sm font-medium">Step {index + 1}</span>
              <div className="flex items-center gap-1">
                <Button type="button" variant="ghost" size="icon" disabled={disabled || index === 0} aria-label={`Move step ${index + 1} up`} onClick={() => move(index, -1)}><ArrowUp className="h-4 w-4" /></Button>
                <Button type="button" variant="ghost" size="icon" disabled={disabled || index === steps.length - 1} aria-label={`Move step ${index + 1} down`} onClick={() => move(index, 1)}><ArrowDown className="h-4 w-4" /></Button>
                <Button type="button" variant="ghost" size="icon" disabled={disabled || steps.length === 1} aria-label={`Remove step ${index + 1}`} onClick={() => remove(index)}><Trash2 className="h-4 w-4" /></Button>
              </div>
            </div>
            <div className="grid min-w-0 gap-3 sm:grid-cols-2">
              <div className="min-w-0 space-y-1.5">
                <Label htmlFor={`schedule-step-${index}-action`}>Action for step {index + 1}</Label>
                <Select id={`schedule-step-${index}-action`} disabled={disabled} value={step.action} onValueChange={(action) => update(index, { action: action as ScheduleStep['action'], ...(action === 'command' ? { command: '' } : action === 'delay' ? { delay_seconds: 30 } : action === 'backup' ? { backup_target: '' } : {}) })}>
                  {Object.entries(stepActions).map(([action, label]) => <option key={action} value={action}>{label}</option>)}
                </Select>
              </div>
              {step.action === 'command' && <div className="min-w-0 space-y-1.5"><Label htmlFor={`schedule-step-${index}-command`}>Command for step {index + 1}</Label><Input id={`schedule-step-${index}-command`} disabled={disabled} required maxLength={4096} placeholder="say Backup Starting" className="font-mono" value={step.command ?? ''} onChange={(event) => update(index, { ...step, command: event.target.value })} /></div>}
              {step.action === 'backup' && <div className="min-w-0 space-y-1.5"><Label htmlFor={`schedule-step-${index}-target`}>Backup target for step {index + 1}</Label><Select id={`schedule-step-${index}-target`} disabled={disabled} value={step.backup_target || 'default'} onValueChange={(target) => update(index, { ...step, backup_target: target === 'default' ? '' : target as 'local' | 's3' })}>{Object.entries(backupTargets).map(([value, label]) => <option key={value} value={value || 'default'}>{label}</option>)}</Select></div>}
              {step.action === 'delay' && <div className="min-w-0 space-y-1.5"><Label htmlFor={`schedule-step-${index}-delay`}>Wait for step {index + 1} (seconds)</Label><Input id={`schedule-step-${index}-delay`} disabled={disabled} type="number" required min={1} max={3600} step={1} value={step.delay_seconds || ''} onChange={(event) => update(index, { ...step, delay_seconds: Number(event.target.value) })} /></div>}
            </div>
          </li>
        ))}
      </ol>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button type="button" variant="outline" size="sm" disabled={disabled || steps.length >= maxChainSteps} onClick={add}><Plus className="h-4 w-4" />Add step</Button>
        <span className="text-xs tabular-nums text-muted-foreground">{steps.length} / {maxChainSteps} steps</span>
      </div>
      <p className="max-w-prose text-xs leading-relaxed text-muted-foreground">Console commands require a running server. Backups stop a running server, then start it again. Add a wait step after a backup, start, or restart to allow Minecraft to load before sending commands.</p>
    </div>
  );
}
