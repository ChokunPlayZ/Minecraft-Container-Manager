import type { ScheduleStep } from '../api/types';

export const stepActions: Record<ScheduleStep['action'], string> = {
  command: 'Console command', backup: 'Create backup', restart: 'Restart server',
  start: 'Start server', stop: 'Stop server', delay: 'Wait',
};
export const backupTargets = { '': 'Panel default', local: 'Local disk', s3: 'Remote (S3)' };
export const maxChainSteps = 50;

export function stepLabel(step: ScheduleStep): string {
  if (step.action === 'command') return step.command ?? '';
  if (step.action === 'backup') return `Create backup · ${backupTargets[step.backup_target ?? '']}`;
  if (step.action === 'delay') return `Wait ${step.delay_seconds} ${step.delay_seconds === 1 ? 'second' : 'seconds'}`;
  return stepActions[step.action];
}
