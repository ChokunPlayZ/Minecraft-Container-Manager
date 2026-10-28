import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { routeTree } from '../src/routeTree.gen';

vi.mock('../src/styles/index.css', () => ({}));

function renderSettings(path: string) {
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [path] }),
    defaultPendingMinMs: 0,
  });
  render(<RouterProvider router={router} />);
  return router;
}

describe('App and user settings', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    window.localStorage.setItem('mcm-mock', 'true');
    window.localStorage.setItem('mcm-theme', 'light');
    document.cookie = 'mcm_theme=light; path=/';
    document.documentElement.classList.remove('dark');
    vi.spyOn(api, 'totpStatus').mockResolvedValue({ totp_enabled: false });
    vi.spyOn(api, 'passkeyList').mockResolvedValue([]);
    vi.spyOn(api, 'getPanelUpdate');
    vi.spyOn(api, 'getSettings');
    vi.spyOn(api, 'getDiscordSettings');
  });

  afterEach(() => {
    window.localStorage.clear();
    document.cookie = 'mcm_theme=; path=/; max-age=0';
    document.documentElement.classList.remove('dark');
  });

  it('opens shared updates by default and keeps account controls on the user page', async () => {
    const user = userEvent.setup();
    const router = renderSettings('/settings');
    expect(await screen.findByRole('heading', { name: 'App Settings' })).toBeVisible();
    expect(screen.getByRole('tab', { name: 'Updates' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('tab', { name: 'Account Security' })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Email')).not.toBeInTheDocument();
    expect(api.totpStatus).not.toHaveBeenCalled();
    expect(api.getPanelUpdate).toHaveBeenCalled();

    await user.click(within(screen.getByRole('complementary')).getByRole('link', { name: 'User settings' }));
    expect(await screen.findByRole('heading', { name: 'User Settings' })).toBeVisible();
    expect(screen.getByLabelText('Email')).toHaveValue('admin@mcm.panel');
    expect(await screen.findByText('No passkeys registered.')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Set up 2FA' })).toBeEnabled();
    expect(screen.queryByRole('tab', { name: 'Updates' })).not.toBeInTheDocument();
    expect(router.state.location.pathname).toBe('/user-settings');
    expect(within(screen.getByRole('navigation', { name: 'Main navigation' })).getByRole('link', { name: 'App Settings' })).not.toHaveAttribute('aria-current');
    expect(within(screen.getByRole('complementary')).getByRole('link', { name: 'User settings' })).toHaveAttribute('aria-current', 'page');
  });

  it('redirects bookmarked account settings without loading shared settings', async () => {
    const router = renderSettings('/settings?tab=account');
    expect(await screen.findByRole('heading', { name: 'User Settings' })).toBeVisible();
    expect(router.state.location.pathname).toBe('/user-settings');
    expect(screen.getByRole('tab', { name: 'Account Security' })).toHaveAttribute('aria-selected', 'true');
    expect(api.getPanelUpdate).not.toHaveBeenCalled();
    expect(api.getSettings).not.toHaveBeenCalled();
    expect(api.getDiscordSettings).not.toHaveBeenCalled();
  });

  it.each([
    ['updates', 'Updates'], ['dns', 'DNS Providers'],
    ['ports', 'Port Pool'], ['notifications', 'Notifications'],
  ])('preserves the shared %s settings bookmark', async (tab, name) => {
    const router = renderSettings(`/settings?tab=${tab}`);
    expect(await screen.findByRole('heading', { name: 'App Settings' })).toBeVisible();
    expect(screen.getByRole('tab', { name })).toHaveAttribute('aria-selected', 'true');
    expect(router.state.location.pathname).toBe('/settings');
    expect(screen.queryByLabelText('Email')).not.toBeInTheDocument();
    expect(api.totpStatus).not.toHaveBeenCalled();
  });

  it('keeps profile saves tied to the signed-in user', async () => {
    const user = userEvent.setup();
    const save = vi.spyOn(api, 'updateUser').mockResolvedValue({
      id: 'admin-1', email: 'new@example.com', totp_enabled: false, created_at: '', updated_at: '',
    });
    renderSettings('/user-settings');
    const email = await screen.findByLabelText('Email');
    await user.clear(email);
    await user.type(email, 'new@example.com');
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByText('Saved.')).toBeVisible();
    expect(save).toHaveBeenCalledWith('admin-1', { email: 'new@example.com' });
    expect(api.getSettings).not.toHaveBeenCalled();
  });

  it('saves appearance in the browser and stays in sync with the header theme menu', async () => {
    const user = userEvent.setup();
    const save = vi.spyOn(api, 'putSettings');
    renderSettings('/user-settings?tab=appearance');
    const theme = await screen.findByRole('combobox', { name: 'Theme' });
    await user.click(theme);
    await user.click(screen.getByRole('option', { name: 'Dark' }));
    expect(document.documentElement).toHaveClass('dark');
    expect(window.localStorage.getItem('mcm-theme')).toBe('dark');
    expect(save).not.toHaveBeenCalled();
    expect(api.getSettings).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }));
    await user.click(screen.getByRole('menuitem', { name: 'Light' }));
    expect(theme).toHaveTextContent('Light');
    expect(document.documentElement).not.toHaveClass('dark');
  });

  it('supports keyboard navigation between app sections and keeps the URL in sync', async () => {
    const user = userEvent.setup();
    const router = renderSettings('/settings?tab=updates');
    const updates = await screen.findByRole('tab', { name: 'Updates' });
    updates.focus();
    await user.keyboard('{End}');
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Notifications' })).toHaveAttribute('aria-selected', 'true'));
    expect(screen.getByRole('tab', { name: 'Notifications' })).toHaveFocus();
    expect(router.state.location.search.tab).toBe('notifications');
    await user.keyboard('{ArrowRight}');
    await waitFor(() => expect(updates).toHaveAttribute('aria-selected', 'true'));
    expect(updates).toHaveFocus();
  });
});
