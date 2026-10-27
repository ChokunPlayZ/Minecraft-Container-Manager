import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '../src/api/client';
import type { ScheduledTask } from '../src/api/types';
import { SchedulesPanel } from '../src/components/schedules-panel';

function task(overrides: Partial<ScheduledTask> = {}): ScheduledTask {
  return { id: 'task-1', server_id: 'one', name: 'Nightly restart', action: 'restart', command: '', cron: 'midnight', timezone: 'Asia/Bangkok', enabled: true, next_run_at: '2026-10-28T17:00:00Z', last_run_at: '', last_status: '', last_error: '', created_at: '', updated_at: '', ...overrides };
}

afterEach(() => vi.restoreAllMocks());

describe('SchedulesPanel', () => {
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
    await waitFor(() => expect(create).toHaveBeenCalledWith('one', { name: 'Nightly restart', action: 'restart', command: '', cron: 'midnight', timezone: 'Asia/Bangkok', enabled: true }));
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
