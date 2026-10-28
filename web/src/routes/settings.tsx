import { createFileRoute, redirect } from '@tanstack/react-router';
import { Bell, Globe, Network, RefreshCw, Settings2 } from 'lucide-react';
import { AppShell } from '../components/app-shell';
import { RequireAuth } from '../components/require-auth';
import { SettingsTabs } from '../components/settings-tabs';
import { useIsAdmin } from '../lib/auth';
import { DNSSettingsCard } from '../components/dns-settings';
import { PortPoolSettingsCard } from '../components/port-pool-settings';
import { DiscordNotificationSettings } from '../components/discord-notification-settings';
import { UpdateCheckerSettings } from '../components/update-checker-settings';
import { PanelUpdateSettings } from '../components/panel-update-settings';

const tabs = [
  { id: 'updates', label: 'Updates', icon: RefreshCw },
  { id: 'dns', label: 'DNS Providers', icon: Globe },
  { id: 'ports', label: 'Port Pool', icon: Network },
  { id: 'notifications', label: 'Notifications', icon: Bell },
] as const;

type AppSettingsTab = typeof tabs[number]['id'];

export const Route = createFileRoute('/settings')({
  validateSearch: (search: Record<string, unknown>): { tab: AppSettingsTab | 'account' } => ({
    tab: search.tab === 'account' ? 'account'
      : tabs.find((tab) => tab.id === search.tab)?.id ?? 'updates',
  }),
  beforeLoad: ({ search }) => {
    if (search.tab === 'account') {
      throw redirect({ to: '/user-settings', search: { ...search, tab: 'account' }, replace: true });
    }
  },
  component: AppSettingsRoute,
});

function AppSettingsRoute() {
  const isAdmin = useIsAdmin();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const activeTab = search.tab === 'account' ? 'updates' : search.tab;

  return (
    <RequireAuth>
      <AppShell>
        {!isAdmin ? (
          <div className="empty-state rounded-lg border bg-card">
            <Settings2 />
            <h1 className="section-heading">App Settings</h1>
            <p>Only administrators can change app settings.</p>
          </div>
        ) : (<>
        <div className="page-heading">
          <div>
            <h1 className="page-title">App Settings</h1>
            <p className="page-description">Manage shared settings for this installation. Changes apply to all users.</p>
          </div>
        </div>
        <SettingsTabs
          id="app-settings"
          label="App settings sections"
          tabs={tabs}
          activeTab={activeTab}
          onTabChange={(tab) => void navigate({ search: (previous) => ({ ...previous, tab }) })}
        />
        <div id="app-settings-panel" role="tabpanel" aria-labelledby={`app-settings-tab-${activeTab}`} tabIndex={0}>
          {activeTab === 'dns' ? (
            <div className="max-w-3xl"><DNSSettingsCard /></div>
          ) : activeTab === 'updates' ? (
            <div className="max-w-3xl space-y-6"><PanelUpdateSettings /><UpdateCheckerSettings /></div>
          ) : activeTab === 'notifications' ? (
            <div className="max-w-3xl"><DiscordNotificationSettings /></div>
          ) : (
            <div className="max-w-3xl"><PortPoolSettingsCard /></div>
          )}
        </div>
        </>)}
      </AppShell>
    </RequireAuth>
  );
}
