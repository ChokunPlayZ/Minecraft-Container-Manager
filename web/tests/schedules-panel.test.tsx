import { renderWithAdmin as render } from '../src/test/auth';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '../src/api/client';
import type { ScheduledTask } from '../src/api/types';
import { SchedulesPanel } from '../src/components/schedules-panel';

function task(overrides: Partial<ScheduledTask> = {}): ScheduledTask {
  return { id: 'task-1', server_id: 'one', name: 'Nightly restart', action: 'restart', command: '', backup_target: '', cron: 'midnight', timezone: 'Asia/Bangkok', enabled: true, next_run_at: '2026-10-28T17:00:00Z', last_run_at: '', last_status: '', last_error: '', created_at: '', updated_at: '', ...overrides };
}

afterEach(() => vi.restoreAllMocks());

describe('SchedulesPanel', () => {
  it('creates an ordered backup chain, then edits, reorders, and removes steps', async () => {
    vi.spyOn(api, 'listSchedules').mockResolvedValue({ schedules: [], default_timezone: 'Asia/Bangkok' });
    const create = vi.spyOn(api, 'createSchedule').mockImplementation(async (_id, input) => task(input));
    const update = vi.spyOn(api, 'updateSchedule').mockImplementation(async (_id, _taskId, input) => task(input));
    const user = userEvent.setup();
    render(<SchedulesPanel serverId="one" />);
    await user.click(await screen.findByRole('button', { name: 'Add schedule' }));
    await user.type(screen.getByLabelText('Task name'), 'Announced backup');
    await user.type(screen.getByLabelText('Console command'), 'broadcast "Backup Starting"');
    await user.click(screen.getByLabelText('Action'));
    await user.click(screen.getByRole('option', { name: 'Action chain' }));
    expect(screen.getByLabelText('Command for step 1')).toHaveValue('broadcast "Backup Starting"');
    expect(screen.getByRole('button', { name: 'Move step 1 up' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Remove step 1' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Add step' }));
    await user.type(screen.getByLabelText('Command for step 2'), 'save-all');
    await user.click(screen.getByRole('button', { name: 'Add step' }));
    await user.click(screen.getByLabelText('Action for step 3'));
    await user.click(screen.getByRole('option', { name: 'Create backup' }));
    await user.click(screen.getByLabelText('Backup target for step 3'));
    await user.click(screen.getByRole('option', { name: 'Remote (S3)' }));
    await user.click(screen.getByRole('button', { name: 'Add step' }));
    await user.click(screen.getByLabelText('Action for step 4'));
    await user.click(screen.getByRole('option', { name: 'Wait' }));
    await user.clear(screen.getByLabelText('Wait for step 4 (seconds)'));
    await user.type(screen.getByLabelText('Wait for step 4 (seconds)'), '45');
    await user.click(screen.getByRole('button', { name: 'Add step' }));
    await user.type(screen.getByLabelText('Command for step 5'), 'broadcast "Backup Complete"');
    await user.click(screen.getByRole('button', { name: 'Create schedule' }));
    const steps = [
      { action: 'command', command: 'broadcast "Backup Starting"' },
      { action: 'command', command: 'save-all' },
      { action: 'backup', backup_target: 's3' },
      { action: 'delay', delay_seconds: 45 },
      { action: 'command', command: 'broadcast "Backup Complete"' },
    ];
    await waitFor(() => expect(create).toHaveBeenCalledWith('one', { name: 'Announced backup', action: 'chain', command: '', backup_target: '', steps, cron: 'midnight', timezone: 'Asia/Bangkok', enabled: true }));
    const list = await screen.findByRole('list', { name: 'Steps for Announced backup' });
    expect(within(list).getAllByRole('listitem').map((item) => item.textContent)).toEqual(['broadcast "Backup Starting"', 'save-all', 'Create backup · Remote (S3)', 'Wait 45 seconds', 'broadcast "Backup Complete"']);
    await user.click(screen.getByRole('button', { name: 'Edit Announced backup' }));
    expect(screen.getByLabelText('Backup target for step 3')).toHaveTextContent('Remote (S3)');
    expect(screen.getByLabelText('Wait for step 4 (seconds)')).toHaveValue(45);
    await user.click(screen.getByRole('button', { name: 'Move step 3 up' }));
    expect(screen.getByLabelText('Backup target for step 2')).toHaveTextContent('Remote (S3)');
    expect(screen.getByLabelText('Command for step 3')).toHaveValue('save-all');
    await user.click(screen.getByRole('button', { name: 'Move step 2 down' }));
    expect(screen.getByLabelText('Backup target for step 3')).toHaveTextContent('Remote (S3)');
    await user.click(screen.getByRole('button', { name: 'Remove step 2' }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith('one', 'task-1', expect.objectContaining({ steps: [steps[0], steps[2], steps[3], steps[4]] })));
  });

  it('retains a chain after a save failure and clears steps when switching to a single action', async () => {
    const original = task({ name: 'Maintenance', action: 'chain', steps: [{ action: 'backup', backup_target: 'local' }, { action: 'delay', delay_seconds: 30 }] });
    vi.spyOn(api, 'listSchedules').mockResolvedValue({ schedules: [original], default_timezone: 'UTC' });
    const update = vi.spyOn(api, 'updateSchedule').mockRejectedValueOnce(new ApiError(400, 'Invalid schedule')).mockImplementation(async (_id, _taskId, input) => task(input));
    const user = userEvent.setup();
    render(<SchedulesPanel serverId="one" />);
    await user.click(await screen.findByRole('button', { name: 'Edit Maintenance' }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid schedule');
    expect(screen.getByLabelText('Backup target for step 1')).toHaveTextContent('Local disk');
    expect(screen.getByLabelText('Wait for step 2 (seconds)')).toHaveValue(30);
    await user.click(screen.getByLabelText('Action'));
    await user.click(screen.getByRole('option', { name: 'Restart server' }));
    expect(screen.queryByRole('list', { name: 'Chain steps' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(update).toHaveBeenLastCalledWith('one', 'task-1', expect.objectContaining({ action: 'restart', command: '', backup_target: '' })));
    expect(update.mock.calls.at(-1)?.[2]).not.toHaveProperty('steps');
  });

  it('creates a remote backup and preserves its target when editing', async () => {
    vi.spyOn(api, 'listSchedules').mockResolvedValue({ schedules: [], default_timezone: 'UTC' });
    const create = vi.spyOn(api, 'createSchedule').mockImplementation(async (_id, input) => task(input));
    const update = vi.spyOn(api, 'updateSchedule').mockImplementation(async (_id, _taskId, input) => task(input));
    const user = userEvent.setup();
    render(<SchedulesPanel serverId="one" />);
    await user.click(await screen.findByRole('button', { name: 'Add schedule' }));
    await user.type(screen.getByLabelText('Task name'), 'Offsite snapshot');
    await user.click(screen.getByLabelText('Action'));
    await user.click(screen.getByRole('option', { name: 'Create backup' }));
    await user.click(screen.getByLabelText('Backup target'));
    await user.click(screen.getByRole('option', { name: 'Remote (S3)' }));
    await user.click(screen.getByRole('button', { name: 'Create schedule' }));
    await waitFor(() => expect(create).toHaveBeenCalledWith('one', expect.objectContaining({ action: 'backup', backup_target: 's3' })));
    expect(await screen.findByText('Create backup · Every day at midnight · Remote (S3)')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Edit Offsite snapshot' }));
    expect(screen.getByLabelText('Backup target')).toHaveTextContent('Remote (S3)');
    await user.click(screen.getByLabelText('Backup target'));
    await user.click(screen.getByRole('option', { name: 'Local disk' }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith('one', 'task-1', expect.objectContaining({ backup_target: 'local' })));
    expect(await screen.findByText('Create backup · Every day at midnight · Local disk')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Edit Offsite snapshot' }));
    await user.click(screen.getByLabelText('Backup target'));
    await user.click(screen.getByRole('option', { name: 'Panel default' }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(update).toHaveBeenLastCalledWith('one', 'task-1', expect.objectContaining({ action: 'backup', backup_target: '' })));
    await user.click(screen.getByRole('button', { name: 'Edit Offsite snapshot' }));
    await user.click(screen.getByLabelText('Backup target'));
    await user.click(screen.getByRole('option', { name: 'Remote (S3)' }));
    await user.click(screen.getByLabelText('Action'));
    await user.click(screen.getByRole('option', { name: 'Restart server' }));
    expect(screen.queryByLabelText('Backup target')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(update).toHaveBeenLastCalledWith('one', 'task-1', expect.objectContaining({ action: 'restart', backup_target: '' })));
  });

  it('creates a backup at a fixed interval without a console command', async () => {
    vi.spyOn(api, 'listSchedules').mockResolvedValue({ schedules: [], default_timezone: 'Asia/Bangkok' });
    const create = vi.spyOn(api, 'createSchedule').mockImplementation(async (_id, input) => task(input));
    const user = userEvent.setup();
    render(<SchedulesPanel serverId="one" />);
    await user.click(await screen.findByRole('button', { name: 'Add schedule' }));
    await user.type(screen.getByLabelText('Task name'), 'World snapshot');
    await user.type(screen.getByLabelText('Console command'), 'save-all');
    await user.click(screen.getByLabelText('Action'));
    await user.click(screen.getByRole('option', { name: 'Create backup' }));
    expect(screen.queryByLabelText('Console command')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Backup target')).toHaveTextContent('Panel default');
    await user.click(screen.getByLabelText('Frequency'));
    await user.click(screen.getByRole('option', { name: 'Fixed interval' }));
    await user.clear(screen.getByLabelText('Interval (minutes)'));
    await user.type(screen.getByLabelText('Interval (minutes)'), '95');
    await user.click(screen.getByRole('button', { name: 'Create schedule' }));
    await waitFor(() => expect(create).toHaveBeenCalledWith('one', { name: 'World snapshot', action: 'backup', command: '', backup_target: '', cron: '@every 95m', timezone: 'Asia/Bangkok', enabled: true }));
    expect(await screen.findByText('Create backup · Every 95 minutes · Panel default')).toBeInTheDocument();
  });

  it('edits and resumes a migrated paused backup through schedules', async () => {
    const original = task({ name: 'Automatic backup', action: 'backup', cron: '@every 720m', enabled: false, next_run_at: '' });
    vi.spyOn(api, 'listSchedules').mockResolvedValue({ schedules: [original], default_timezone: 'UTC' });
    const update = vi.spyOn(api, 'updateSchedule').mockImplementation(async (_id, _taskId, input) => task(input));
    const toggle = vi.spyOn(api, 'setScheduleEnabled').mockResolvedValue({ ...original, cron: '@every 37m', enabled: true });
    const user = userEvent.setup();
    render(<SchedulesPanel serverId="one" />);
    expect(await screen.findByText('Create backup · Every 12 hours · Panel default')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Edit Automatic backup' }));
    expect(screen.getByLabelText('Frequency')).toHaveTextContent('Fixed interval');
    expect(screen.getByLabelText('Interval (minutes)')).toHaveValue(720);
    await user.clear(screen.getByLabelText('Interval (minutes)'));
    await user.type(screen.getByLabelText('Interval (minutes)'), '37');
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith('one', 'task-1', expect.objectContaining({ action: 'backup', cron: '@every 37m', enabled: false })));
    await user.click(await screen.findByRole('switch', { name: 'Enable Automatic backup' }));
    expect(toggle).toHaveBeenCalledWith('one', 'task-1', true);
  });

  it('creates a midnight restart in the chosen timezone', async () => {
    vi.spyOn(api, 'listSchedules').mockResolvedValue({ schedules: [], default_timezone: 'Asia/Bangkok' });
    const create = vi.spyOn(api, 'createSchedule').mockImplementation(async (_id, input) => task(input));
    const user = userEvent.setup();
    render(<SchedulesPanel serverId="one" />);
    expect(await screen.findByText('No scheduled tasks')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Add schedule' }));
    expect(screen.getByLabelText('Timezone')).toHaveValue('Asia/Bangkok');
    await user.type(screen.getByLabelText('Task name'), 'Nightly restart');
    await user.click(screen.getByLabelText('Action'));
    await user.click(screen.getByRole('option', { name: 'Restart server' }));
    await user.clear(screen.getByLabelText('Timezone'));
    await user.type(screen.getByLabelText('Timezone'), 'Asia/Bangkok');
    await user.click(screen.getByRole('button', { name: 'Create schedule' }));
    await waitFor(() => expect(create).toHaveBeenCalledWith('one', { name: 'Nightly restart', action: 'restart', command: '', backup_target: '', cron: 'midnight', timezone: 'Asia/Bangkok', enabled: true }));
    expect(await screen.findByText('Nightly restart')).toBeInTheDocument();
    expect(screen.getByText('Not run yet')).toBeInTheDocument();
    expect(screen.queryByRole('form')).not.toBeInTheDocument();
  });

  it('preserves custom command input when the server rejects cron, then saves', async () => {
    vi.spyOn(api, 'listSchedules').mockResolvedValue({ schedules: [], default_timezone: 'UTC' });
    const create = vi.spyOn(api, 'createSchedule').mockRejectedValueOnce(new ApiError(400, 'Invalid cron minute')).mockImplementation(async (_id, input) => task(input));
    const user = userEvent.setup();
    render(<SchedulesPanel serverId="one" />);
    await user.click(await screen.findByRole('button', { name: 'Add schedule' }));
    await user.type(screen.getByLabelText('Task name'), 'World save');
    await user.type(screen.getByLabelText('Console command'), 'save-all');
    await user.click(screen.getByLabelText('Frequency'));
    await user.click(screen.getByRole('option', { name: 'Custom cron expression' }));
    await user.type(screen.getByLabelText('Cron expression'), '99 * * * *');
    await user.click(screen.getByRole('button', { name: 'Create schedule' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid cron minute');
    expect(screen.getByLabelText('Console command')).toHaveValue('save-all');
    await user.clear(screen.getByLabelText('Cron expression'));
    await user.type(screen.getByLabelText('Cron expression'), '*/15 * * * *');
    await user.click(screen.getByRole('button', { name: 'Create schedule' }));
    expect(await screen.findByText('World save')).toBeInTheDocument();
    expect(create).toHaveBeenLastCalledWith('one', expect.objectContaining({ action: 'command', command: 'save-all', cron: '*/15 * * * *' }));
  });

  it('pauses, edits, and deletes a schedule after confirmation', async () => {
    const original = task();
    vi.spyOn(api, 'listSchedules').mockResolvedValue({ schedules: [original], default_timezone: 'UTC' });
    const toggle = vi.spyOn(api, 'setScheduleEnabled').mockResolvedValue(task({ enabled: false, next_run_at: '' }));
    const update = vi.spyOn(api, 'updateSchedule').mockImplementation(async (_id, _taskId, input) => task(input));
    const remove = vi.spyOn(api, 'deleteSchedule').mockResolvedValue({ ok: true });
    const user = userEvent.setup();
    render(<SchedulesPanel serverId="one" />);
    await user.click(await screen.findByRole('switch', { name: 'Enable Nightly restart' }));
    expect(toggle).toHaveBeenCalledWith('one', 'task-1', false);
    expect(await screen.findByRole('switch', { name: 'Enable Nightly restart' })).toHaveAttribute('aria-checked', 'false');
    await user.click(screen.getByRole('button', { name: 'Edit Nightly restart' }));
    expect(screen.getByLabelText('Frequency')).toHaveTextContent('Every day at midnight');
    await user.clear(screen.getByLabelText('Task name'));
    await user.type(screen.getByLabelText('Task name'), 'Weekly restart');
    await user.click(screen.getByLabelText('Frequency'));
    await user.click(screen.getByRole('option', { name: 'Every Sunday at midnight' }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith('one', 'task-1', expect.objectContaining({ name: 'Weekly restart', cron: '0 0 * * 0', enabled: false })));
    await user.click(await screen.findByRole('button', { name: 'Delete Weekly restart' }));
    expect(remove).not.toHaveBeenCalled();
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Delete schedule' }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith('one', 'task-1'));
    expect(await screen.findByText('No scheduled tasks')).toBeInTheDocument();
  });

  it('shows execution failures and recovers a failed list request', async () => {
    vi.spyOn(api, 'listSchedules').mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ schedules: [task({ last_status: 'failed', last_error: 'server is not running' })], default_timezone: 'UTC' });
    const user = userEvent.setup();
    render(<SchedulesPanel serverId="one" />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load schedules');
    await user.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByText('server is not running')).toBeInTheDocument();
    expect(screen.getByText('Failed')).toBeInTheDocument();
  });
});
