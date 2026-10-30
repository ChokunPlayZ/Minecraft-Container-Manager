import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { renderWithAdmin, renderWithUser } from '../src/test/auth';
import { api } from '../src/api/client';
import type { BackupRecord, BackupRetentionPreview, Server } from '../src/api/types';
import { BackupRetentionSettings } from '../src/components/backup-retention-settings';
import { BackupsPanel } from '../src/components/backups-panel';

const server = { id: 'server' } as Server;
const backup: BackupRecord = { id: 'old', server_id: server.id, name: 'before-upgrade', status: 'completed', location: 'local:backup.tar.gz', size_bytes: 1024, created_at: '2026-10-01T00:00:00Z' };
const preview: BackupRetentionPreview = { policy: { keep_count: 10, max_age_days: 0, trash_retention_days: 30 }, to_trash: [{ ...backup, reason: 'count' }], to_purge: [], trash_bytes: 1024, purge_bytes: 0 };
afterEach(() => vi.restoreAllMocks());

describe('backup retention settings', () => {
  beforeEach(() => { vi.spyOn(api, 'getBackupRetention').mockResolvedValue(preview); });

  it('saves per-server limits without running cleanup and requires saved changes for the preview', async () => {
    const save = vi.spyOn(api, 'saveBackupRetention').mockResolvedValue({ ...preview, policy: { ...preview.policy, keep_count: 3 } });
    const cleanup = vi.spyOn(api, 'cleanupBackups');
    renderWithAdmin(<BackupRetentionSettings serverId={server.id} canManage disabled={false} onCleanup={vi.fn()} />);
    const count = await screen.findByLabelText('Backups to keep');
    await userEvent.clear(count); await userEvent.type(count, '3');
    expect(screen.getByRole('button', { name: 'Run cleanup now' })).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'Save policy' }));
    await waitFor(() => expect(save).toHaveBeenCalledWith(server.id, { keep_count: 3, max_age_days: 0, trash_retention_days: 30 }));
    expect(cleanup).not.toHaveBeenCalled();
    expect(await screen.findByText(/Policy saved/)).toBeInTheDocument();
  });

  it('confirms cleanup including expired trash and refreshes the result', async () => {
    vi.mocked(api.getBackupRetention).mockResolvedValue({ ...preview, to_purge: [{ ...backup, id: 'expired', name: 'expired snapshot' }] });
    const cleanup = vi.spyOn(api, 'cleanupBackups').mockResolvedValue({ trashed: 1, purged: 1 });
    const refresh = vi.fn().mockResolvedValue(undefined);
    renderWithAdmin(<BackupRetentionSettings serverId={server.id} canManage disabled={false} onCleanup={refresh} />);
    await screen.findByText('expired snapshot');
    await userEvent.click(screen.getByRole('button', { name: 'Run cleanup now' }));
    const dialog = await screen.findByRole('dialog', { name: 'Run backup cleanup' });
    expect(within(dialog).getByText(/Move 1 backups to trash and permanently delete 1 expired backups/)).toBeInTheDocument();
    expect(cleanup).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Run cleanup' }));
    expect(await screen.findByText('Cleanup complete: 1 moved to trash, 1 permanently deleted.')).toBeInTheDocument();
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('rejects empty, fractional and out-of-range limits, while accepting zero', async () => {
    renderWithAdmin(<BackupRetentionSettings serverId={server.id} canManage disabled={false} onCleanup={vi.fn()} />);
    const count = await screen.findByLabelText('Backups to keep');
    for (const value of ['', '-1', '1.5', '100001']) {
      await userEvent.clear(count); if (value) await userEvent.type(count, value);
      expect(screen.getByRole('button', { name: 'Save policy' })).toBeDisabled();
    }
    await userEvent.clear(count); await userEvent.type(count, '0');
    expect(screen.getByRole('button', { name: 'Save policy' })).toBeEnabled();
  });

  it('shows a retry after loading fails and keeps controls read-only without delete permission', async () => {
    vi.mocked(api.getBackupRetention).mockRejectedValueOnce(new Error('offline'));
    renderWithAdmin(<BackupRetentionSettings serverId={server.id} canManage={false} disabled={false} onCleanup={vi.fn()} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load retention settings');
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByLabelText('Backups to keep')).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Run cleanup now' })).toBeDisabled();
  });
});

describe('backup trash and protection', () => {
  beforeEach(() => {
    vi.spyOn(api, 'getBackupProgress').mockResolvedValue({ active: false });
    vi.spyOn(api, 'subscribeBackupEvents').mockReturnValue(vi.fn());
  });

  it('moves a backup to recoverable trash with confirmation', async () => {
    vi.spyOn(api, 'listBackups').mockResolvedValue({ backups: [backup] });
    const remove = vi.spyOn(api, 'deleteBackup').mockResolvedValue({ ok: true });
    renderWithAdmin(<BackupsPanel server={server} />);
    await screen.findByText(backup.name);
    await userEvent.click(screen.getByRole('button', { name: 'Move to trash' }));
    const dialog = await screen.findByRole('dialog', { name: 'Move backup to trash' });
    expect(remove).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Move to trash' }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith(backup.id));
  });

  it('separates trash from active backups and recovers without overwriting the world', async () => {
    vi.spyOn(api, 'listBackups').mockResolvedValue({ backups: [], trash: [{ ...backup, deleted_at: '2026-10-30T00:00:00Z' }], policy: preview.policy });
    const recover = vi.spyOn(api, 'recoverBackup').mockResolvedValue({ ok: true });
    const restore = vi.spyOn(api, 'restoreBackup');
    renderWithAdmin(<BackupsPanel server={server} />);
    await screen.findByText('No backups yet');
    await userEvent.click(screen.getByRole('button', { name: 'Trash (1)' }));
    expect(screen.getByText(backup.name)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Download backup' })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Recover' }));
    await waitFor(() => expect(recover).toHaveBeenCalledWith(backup.id));
    expect(restore).not.toHaveBeenCalled();
  });

  it('prevents deleting protected snapshots and allows removing protection', async () => {
    vi.spyOn(api, 'listBackups').mockResolvedValue({ backups: [{ ...backup, protected: true }] });
    const protect = vi.spyOn(api, 'protectBackup').mockResolvedValue({ ok: true });
    renderWithAdmin(<BackupsPanel server={server} />);
    await screen.findByText('Protected');
    expect(screen.getByRole('button', { name: 'Move to trash' })).toBeDisabled();
    expect(screen.getByLabelText(`Select ${backup.name}`)).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'Unprotect backup' }));
    await waitFor(() => expect(protect).toHaveBeenCalledWith(backup.id, false));
  });

  it('bulk selection skips protected snapshots and refreshes after a partial failure', async () => {
    const list = vi.spyOn(api, 'listBackups').mockResolvedValue({ backups: [backup, { ...backup, id: 'second', name: 'second snapshot' }, { ...backup, id: 'protected', name: 'protected snapshot', protected: true }] });
    const remove = vi.spyOn(api, 'deleteBackup').mockResolvedValueOnce({ ok: true }).mockRejectedValueOnce(new Error('offline'));
    renderWithAdmin(<BackupsPanel server={server} />);
    await screen.findByText('protected snapshot');
    await userEvent.click(screen.getByLabelText('Select all eligible backups'));
    expect(screen.getByText('2 selected')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Move selected to trash' }));
    const dialog = await screen.findByRole('dialog', { name: 'Move backups to trash' });
    await userEvent.click(within(dialog).getByRole('button', { name: 'Move to trash' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('1 of 2 backups updated.');
    expect(remove.mock.calls.map(([id]) => id)).toEqual(['old', 'second']);
    await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
  });

  it('confirms permanently emptying trash and does not offer management to viewers', async () => {
    vi.spyOn(api, 'listBackups').mockResolvedValue({ backups: [], trash: [{ ...backup, deleted_at: '2026-10-30T00:00:00Z' }] });
    const empty = vi.spyOn(api, 'emptyBackupTrash').mockResolvedValue({ trashed: 0, purged: 1 });
    const view = renderWithAdmin(<BackupsPanel server={server} />);
    await screen.findByText('No backups yet');
    await userEvent.click(screen.getByRole('button', { name: 'Trash (1)' }));
    await userEvent.click(screen.getByRole('button', { name: 'Empty trash' }));
    const dialog = await screen.findByRole('dialog', { name: 'Permanently delete backups' });
    expect(empty).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Delete permanently' }));
    await waitFor(() => expect(empty).toHaveBeenCalledWith(server.id));
    view.unmount();
    renderWithUser(<BackupsPanel server={server} />, { id: 'viewer', email: 'v@example.test', role: 'viewer', all_servers: true, permissions: [], server_ids: [], disabled: false });
    await screen.findByText('No backups yet');
    await userEvent.click(screen.getByRole('button', { name: 'Trash (1)' }));
    expect(screen.getByRole('button', { name: 'Empty trash' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Recover' })).toBeDisabled();
  });
});
