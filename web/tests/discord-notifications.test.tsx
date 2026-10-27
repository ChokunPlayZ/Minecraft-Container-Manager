import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { DiscordNotificationSettings } from '../src/components/discord-notification-settings';
import type { DiscordSettings } from '../src/api/types';

const saved: DiscordSettings = { enabled: true, configured: true, events: ['server_started', 'server_stopped', 'server_crashed'] };

describe('Discord notification settings', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(api, 'getDiscordSettings').mockResolvedValue(saved);
  });

  it('loads switches without revealing the saved webhook', async () => {
    render(<DiscordNotificationSettings />);
    expect(await screen.findByRole('switch', { name: 'Enable Discord notifications' })).toBeChecked();
    expect(screen.getByRole('switch', { name: 'Server crashes' })).toBeChecked();
    expect(screen.getByRole('switch', { name: 'Backup failed' })).not.toBeChecked();
    expect(screen.getByLabelText('Discord webhook URL')).toHaveValue('');
    expect(screen.getByRole('button', { name: 'Send test message' })).toBeEnabled();
  });

  it('saves event choices and preserves a saved webhook when blank', async () => {
    const user = userEvent.setup();
    const save = vi.spyOn(api, 'saveDiscordSettings').mockResolvedValue(saved);
    render(<DiscordNotificationSettings />);
    await user.click(await screen.findByRole('switch', { name: 'Server starts' }));
    await user.click(screen.getByRole('switch', { name: 'Backup failed' }));
    await user.click(screen.getByRole('button', { name: 'Save notification settings' }));
    expect(save).toHaveBeenCalledWith({ enabled: true, events: ['server_stopped', 'server_crashed', 'backup_failed'] });
    expect(await screen.findByRole('status')).toHaveTextContent('settings saved');
  });

  it('tests an unsaved URL while automatic notifications are disabled', async () => {
    const user = userEvent.setup();
    vi.spyOn(api, 'getDiscordSettings').mockResolvedValue({ ...saved, enabled: false, configured: false });
    const test = vi.spyOn(api, 'testDiscordWebhook').mockResolvedValue({ ok: true, message: 'Test message sent to Discord.' });
    render(<DiscordNotificationSettings />);
    const field = await screen.findByLabelText('Discord webhook URL');
    expect(screen.getByRole('button', { name: 'Send test message' })).toBeDisabled();
    await user.type(field, 'https://discord.com/api/webhooks/123/token');
    await user.click(screen.getByRole('button', { name: 'Send test message' }));
    expect(test).toHaveBeenCalledWith('https://discord.com/api/webhooks/123/token');
    expect(await screen.findByRole('status')).toHaveTextContent('Test message sent');
  });

  it('sends a test with the saved webhook and displays delivery errors', async () => {
    const user = userEvent.setup();
    const test = vi.spyOn(api, 'testDiscordWebhook').mockRejectedValue(new Error('Discord webhook was rejected; replace the webhook URL and try again'));
    render(<DiscordNotificationSettings />);
    await user.click(await screen.findByRole('button', { name: 'Send test message' }));
    expect(test).toHaveBeenCalledWith(undefined);
    expect(await screen.findByRole('alert')).toHaveTextContent('replace the webhook URL');
    expect(screen.getByRole('button', { name: 'Send test message' })).toBeEnabled();
  });

  it('removes the saved webhook only on save and disables notifications', async () => {
    const user = userEvent.setup();
    const save = vi.spyOn(api, 'saveDiscordSettings').mockResolvedValue({ ...saved, enabled: false, configured: false });
    render(<DiscordNotificationSettings />);
    await user.click(await screen.findByRole('button', { name: 'Remove saved webhook' }));
    expect(save).not.toHaveBeenCalled();
    expect(screen.getByRole('switch', { name: 'Enable Discord notifications' })).not.toBeChecked();
    expect(screen.getByRole('button', { name: 'Send test message' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Save notification settings' }));
    expect(save).toHaveBeenCalledWith({ enabled: false, webhook_url: '', events: saved.events });
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Remove saved webhook' })).not.toBeInTheDocument());
  });

  it('offers retry after loading fails', async () => {
    const user = userEvent.setup();
    vi.spyOn(api, 'getDiscordSettings').mockRejectedValueOnce(new Error('Could not load settings')).mockResolvedValue(saved);
    render(<DiscordNotificationSettings />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load settings');
    await user.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByRole('switch', { name: 'Server crashes' })).toBeChecked();
  });
});
